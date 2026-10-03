package mail

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"math/big"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testUsername = "slip@example.com"
	// Deliberately shaped like a Google app password.
	testPassword = "abcd efgh ijkl mnop"
)

// fakeSMTP is a minimal single-connection SMTP server used to drive the real
// net/smtp client through each path of GmailSender.
type fakeSMTP struct {
	t        *testing.T
	listener net.Listener

	offerSTARTTLS bool
	offerAuth     bool
	authCode      int
	serverTLS     *tls.Config
	// Error replies to inject (0 = accept).
	starttlsCode int
	mailCode     int
	rcptCode     int
	dataEndCode  int

	mu          sync.Mutex
	commands    []string
	tlsActive   bool
	authOverTLS bool
	authUser    string
	authPass    string
	mailFrom    string
	rcpts       []string
	data        string
	done        chan struct{}
}

func newFakeSMTP(t *testing.T, implicitTLS bool, serverTLS *tls.Config) *fakeSMTP {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &fakeSMTP{t: t, serverTLS: serverTLS, authCode: 235, done: make(chan struct{})}
	if implicitTLS {
		server.listener = tls.NewListener(listener, serverTLS)
		server.tlsActive = true
	} else {
		server.listener = listener
	}
	t.Cleanup(func() { _ = server.listener.Close() })
	return server
}

func (s *fakeSMTP) addr() string {
	return s.listener.Addr().String()
}

func (s *fakeSMTP) serve() {
	go func() {
		defer close(s.done)
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		s.session(conn)
	}()
}

func (s *fakeSMTP) wait(t *testing.T) {
	t.Helper()
	select {
	case <-s.done:
	case <-time.After(10 * time.Second):
		t.Fatal("fake SMTP session did not finish")
	}
}

func (s *fakeSMTP) record(command string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commands = append(s.commands, command)
}

func (s *fakeSMTP) sawCommand(verb string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, command := range s.commands {
		if command == verb {
			return true
		}
	}
	return false
}

func (s *fakeSMTP) session(conn net.Conn) {
	reader := textproto.NewReader(bufio.NewReader(conn))
	write := func(format string, args ...any) {
		_, _ = fmt.Fprintf(conn, format+"\r\n", args...)
	}
	write("220 fake.smtp ESMTP ready")

	for {
		line, err := reader.ReadLine()
		if err != nil {
			return
		}
		verb := strings.ToUpper(strings.Fields(line + " x")[0])
		s.record(verb)

		switch verb {
		case "EHLO", "HELO":
			lines := []string{"fake.smtp greets you"}
			s.mu.Lock()
			if s.offerSTARTTLS && !s.tlsActive {
				lines = append(lines, "STARTTLS")
			}
			s.mu.Unlock()
			if s.offerAuth {
				lines = append(lines, "AUTH PLAIN LOGIN")
			}
			lines = append(lines, "SIZE 35882577")
			for index, text := range lines {
				separator := "-"
				if index == len(lines)-1 {
					separator = " "
				}
				write("250%s%s", separator, text)
			}
		case "STARTTLS":
			if s.starttlsCode != 0 {
				write("%d 4.7.0 TLS not available due to temporary reason", s.starttlsCode)
				continue
			}
			write("220 2.0.0 Ready to start TLS")
			tlsConn := tls.Server(conn, s.serverTLS)
			if err := tlsConn.Handshake(); err != nil {
				return
			}
			conn = tlsConn
			reader = textproto.NewReader(bufio.NewReader(conn))
			s.mu.Lock()
			s.tlsActive = true
			s.mu.Unlock()
		case "AUTH":
			fields := strings.Fields(line)
			if len(fields) == 3 && strings.EqualFold(fields[1], "PLAIN") {
				decoded, _ := base64.StdEncoding.DecodeString(fields[2])
				parts := strings.Split(string(decoded), "\x00")
				s.mu.Lock()
				s.authOverTLS = s.tlsActive
				if len(parts) == 3 {
					s.authUser, s.authPass = parts[1], parts[2]
				}
				s.mu.Unlock()
			}
			if s.authCode == 235 {
				write("235 2.7.0 Accepted")
			} else {
				write("%d 5.7.8 Username and Password not accepted", s.authCode)
			}
		case "MAIL":
			if s.mailCode != 0 {
				write("%d 5.4.5 Daily user sending limit exceeded", s.mailCode)
				continue
			}
			s.mu.Lock()
			s.mailFrom = extractPath(line)
			s.mu.Unlock()
			write("250 2.1.0 OK")
		case "RCPT":
			if s.rcptCode != 0 {
				write("%d 5.1.1 No such user", s.rcptCode)
				continue
			}
			s.mu.Lock()
			s.rcpts = append(s.rcpts, extractPath(line))
			s.mu.Unlock()
			write("250 2.1.5 OK")
		case "DATA":
			write("354 Go ahead")
			body, err := reader.ReadDotBytes()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.data = string(body)
			s.mu.Unlock()
			if s.dataEndCode != 0 {
				write("%d 5.3.4 Message size exceeds fixed limit", s.dataEndCode)
				continue
			}
			write("250 2.0.0 OK queued")
		case "QUIT":
			write("221 2.0.0 Bye")
			return
		case "RSET", "NOOP":
			write("250 OK")
		default:
			write("502 5.5.1 Unrecognized command")
		}
	}
}

func extractPath(line string) string {
	start := strings.Index(line, "<")
	end := strings.Index(line, ">")
	if start < 0 || end < start {
		return ""
	}
	return line[start+1 : end]
}

// testCertificate returns a self-signed certificate for smtp.gmail.com plus a
// pool that trusts it.
func testCertificate(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: GmailHost},
		DNSNames:              []string{GmailHost},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		MinVersion:   tls.VersionTLS12,
	}, pool
}

func testSender(dialAddr string, roots *x509.CertPool) *GmailSender {
	sender := NewGmailSender("")
	sender.dialAddr = dialAddr
	sender.rootCAs = roots
	sender.dialTimeout = 2 * time.Second
	sender.ioTimeout = 5 * time.Second
	return sender
}

func testMessage() Message {
	return Message{
		To:      "budi@example.com",
		Cc:      []string{"hr@example.com"},
		Subject: goldenSubject,
		Text:    "Halo Budi,\nterlampir slip gaji Anda.",
		HTML:    "<p>Halo Budi,</p><p>terlampir slip gaji Anda.</p>",
		Attachments: []Attachment{{
			Filename:    "slip.pdf",
			ContentType: "application/pdf",
			Data:        []byte("%PDF-1.4 test"),
		}},
	}
}

func testCredentials(port int) Credentials {
	return Credentials{Username: testUsername, Password: testPassword, Port: port, SenderName: "HR Kantor"}
}

func requireSendError(t *testing.T, err error, category ErrorCategory) *SendError {
	t.Helper()
	if err == nil {
		t.Fatalf("expected a %s error, got nil", category)
	}
	sendErr, ok := AsSendError(err)
	if !ok {
		t.Fatalf("expected *SendError, got %T: %v", err, err)
	}
	if sendErr.Category != category {
		t.Fatalf("category = %q (%s), want %q", sendErr.Category, sendErr.Message(), category)
	}
	if strings.Contains(err.Error(), testPassword) || strings.Contains(err.Error(), "abcdefghijklmnop") {
		t.Fatalf("error text leaks the password: %q", err.Error())
	}
	return sendErr
}

func TestGmailSenderSTARTTLSDelivers(t *testing.T) {
	serverTLS, roots := testCertificate(t)
	server := newFakeSMTP(t, false, serverTLS)
	server.offerSTARTTLS = true
	server.offerAuth = true
	server.serve()

	err := testSender(server.addr(), roots).Send(context.Background(), testCredentials(PortSTARTTLS), testMessage())
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	server.wait(t)

	server.mu.Lock()
	defer server.mu.Unlock()
	if !server.authOverTLS {
		t.Fatal("AUTH must only be sent after the STARTTLS upgrade")
	}
	if server.authUser != testUsername || server.authPass != testPassword {
		t.Fatalf("AUTH PLAIN credentials = %q/%q", server.authUser, server.authPass)
	}
	if server.mailFrom != testUsername {
		t.Fatalf("MAIL FROM = %q, want the Gmail username", server.mailFrom)
	}
	if strings.Join(server.rcpts, ",") != "budi@example.com,hr@example.com" {
		t.Fatalf("RCPT TO = %v", server.rcpts)
	}
	if !strings.Contains(server.data, "From: \"HR Kantor\" <slip@example.com>") {
		t.Fatalf("message lacks the display-name From header:\n%s", server.data)
	}
	if !strings.Contains(server.data, "Subject: =?UTF-8?q?Slip_Gaji_=E2=80=94_Oktober_2026_(Revisi_1)?=") {
		t.Fatalf("message lacks the RFC 2047 subject:\n%s", server.data)
	}
	starttlsIndex, authIndex := -1, -1
	for index, command := range server.commands {
		if command == "STARTTLS" && starttlsIndex < 0 {
			starttlsIndex = index
		}
		if command == "AUTH" && authIndex < 0 {
			authIndex = index
		}
	}
	if starttlsIndex < 0 || authIndex < starttlsIndex {
		t.Fatalf("expected STARTTLS before AUTH, commands = %v", server.commands)
	}
}

func TestGmailSenderAbortsWhenSTARTTLSNotOffered(t *testing.T) {
	serverTLS, roots := testCertificate(t)
	server := newFakeSMTP(t, false, serverTLS)
	server.offerSTARTTLS = false
	server.offerAuth = true
	server.serve()

	err := testSender(server.addr(), roots).Send(context.Background(), testCredentials(PortSTARTTLS), testMessage())
	sendErr := requireSendError(t, err, CategoryTLS)
	if sendErr.Message() != "TLS gagal: server tidak menawarkan STARTTLS" {
		t.Fatalf("message = %q", sendErr.Message())
	}
	_ = server.listener.Close()
	server.wait(t)
	if server.sawCommand("AUTH") || server.sawCommand("MAIL") {
		t.Fatalf("credentials or mail must never be sent without TLS, commands = %v", server.commands)
	}
	if !server.sawCommand("QUIT") {
		t.Fatalf("the client must QUIT instead of dropping the connection, commands = %v", server.commands)
	}
}

// RFC 5321 4.1.1.10: after an error reply the client still sends QUIT before
// closing the channel.
func TestGmailSenderQuitsAfterErrorReplies(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*fakeSMTP)
		category  ErrorCategory
		message   string
	}{
		{"STARTTLS refused", func(s *fakeSMTP) { s.starttlsCode = 454 }, CategoryTLS, "TLS gagal saat terhubung ke smtp.gmail.com:587 (454)"},
		{"MAIL rejected", func(s *fakeSMTP) { s.mailCode = 550 }, CategoryRejected, "Server email menolak pengiriman (550)"},
		{"RCPT rejected", func(s *fakeSMTP) { s.rcptCode = 550 }, CategoryRecipient, "Alamat penerima ditolak (550)"},
		{"DATA rejected", func(s *fakeSMTP) { s.dataEndCode = 552 }, CategoryRejected, "Server email menolak pengiriman (552)"},
		{"AUTH temporarily refused", func(s *fakeSMTP) { s.authCode = 454 }, CategoryTemporary, "Server email menolak sementara (454), coba lagi nanti"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serverTLS, roots := testCertificate(t)
			server := newFakeSMTP(t, false, serverTLS)
			server.offerSTARTTLS = true
			server.offerAuth = true
			tc.configure(server)
			server.serve()

			err := testSender(server.addr(), roots).Send(context.Background(), testCredentials(PortSTARTTLS), testMessage())
			sendErr := requireSendError(t, err, tc.category)
			if sendErr.Message() != tc.message {
				t.Fatalf("message = %q, want %q", sendErr.Message(), tc.message)
			}
			server.wait(t)
			if !server.sawCommand("QUIT") {
				t.Fatalf("expected QUIT after the error reply, commands = %v", server.commands)
			}
		})
	}
}

func TestGmailSenderDevPathQuitsAfterRcptReject(t *testing.T) {
	serverTLS, _ := testCertificate(t)
	server := newFakeSMTP(t, false, serverTLS)
	server.rcptCode = 550
	server.serve()

	err := NewGmailSender(server.addr()).Send(context.Background(), Credentials{Username: testUsername}, testMessage())
	requireSendError(t, err, CategoryRecipient)
	server.wait(t)
	if !server.sawCommand("QUIT") {
		t.Fatalf("expected QUIT after the RCPT rejection, commands = %v", server.commands)
	}
}

func TestGmailSenderAuthRejected(t *testing.T) {
	serverTLS, roots := testCertificate(t)
	server := newFakeSMTP(t, false, serverTLS)
	server.offerSTARTTLS = true
	server.offerAuth = true
	server.authCode = 535
	server.serve()

	err := testSender(server.addr(), roots).Send(context.Background(), testCredentials(PortSTARTTLS), testMessage())
	sendErr := requireSendError(t, err, CategoryAuth)
	if sendErr.Message() != "Autentikasi ditolak (535)" {
		t.Fatalf("message = %q, want %q", sendErr.Message(), "Autentikasi ditolak (535)")
	}
	server.wait(t)
	if server.sawCommand("MAIL") {
		t.Fatal("MAIL must not be sent after a failed AUTH")
	}
}

func TestGmailSenderRejectsUntrustedCertificate(t *testing.T) {
	serverTLS, _ := testCertificate(t)
	server := newFakeSMTP(t, false, serverTLS)
	server.offerSTARTTLS = true
	server.offerAuth = true
	server.serve()

	// No custom roots: the self-signed certificate must fail verification.
	err := testSender(server.addr(), nil).Send(context.Background(), testCredentials(PortSTARTTLS), testMessage())
	sendErr := requireSendError(t, err, CategoryTLS)
	if !strings.HasPrefix(sendErr.Message(), "TLS gagal") {
		t.Fatalf("message = %q", sendErr.Message())
	}
	server.wait(t)
	if server.sawCommand("AUTH") {
		t.Fatal("AUTH must not be sent when the TLS handshake fails")
	}
}

func TestGmailSenderImplicitTLS(t *testing.T) {
	serverTLS, roots := testCertificate(t)
	server := newFakeSMTP(t, true, serverTLS)
	server.offerAuth = true
	server.serve()

	err := testSender(server.addr(), roots).Send(context.Background(), testCredentials(PortImplicitTLS), testMessage())
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	server.wait(t)
	if server.sawCommand("STARTTLS") {
		t.Fatal("port 465 must not issue STARTTLS")
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if !server.authOverTLS || server.authPass != testPassword {
		t.Fatal("expected AUTH PLAIN over the implicit TLS connection")
	}
}

func TestGmailSenderDevPlainPath(t *testing.T) {
	serverTLS, _ := testCertificate(t)
	server := newFakeSMTP(t, false, serverTLS)
	// Even if the capture server advertises STARTTLS/AUTH, the dev path must
	// stay plain and never send credentials.
	server.offerSTARTTLS = true
	server.offerAuth = true
	server.serve()

	sender := NewGmailSender(server.addr())
	if sender.DevSMTPAddr() != server.addr() {
		t.Fatalf("DevSMTPAddr = %q", sender.DevSMTPAddr())
	}
	creds := Credentials{Username: testUsername, SenderName: "HR Kantor"}
	if err := sender.Send(context.Background(), creds, testMessage()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	server.wait(t)
	if server.sawCommand("STARTTLS") || server.sawCommand("AUTH") {
		t.Fatalf("dev path must be plain SMTP without AUTH, commands = %v", server.commands)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.mailFrom != testUsername || len(server.rcpts) != 2 {
		t.Fatalf("envelope = %q -> %v", server.mailFrom, server.rcpts)
	}
	if !strings.Contains(server.data, "To: <budi@example.com>") {
		t.Fatalf("unexpected message:\n%s", server.data)
	}
}

func TestGmailSenderDialFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedAddr := listener.Addr().String()
	_ = listener.Close()

	err = testSender(closedAddr, nil).Send(context.Background(), testCredentials(PortSTARTTLS), testMessage())
	sendErr := requireSendError(t, err, CategoryConnect)
	if sendErr.Message() != "Tidak bisa terhubung ke smtp.gmail.com:587" {
		t.Fatalf("message = %q", sendErr.Message())
	}
}

func TestGmailSenderConfigErrors(t *testing.T) {
	sender := testSender("127.0.0.1:1", nil)

	noPassword := testCredentials(PortSTARTTLS)
	noPassword.Password = ""
	requireSendError(t, sender.Send(context.Background(), noPassword, testMessage()), CategoryConfig)

	badPort := testCredentials(25)
	requireSendError(t, sender.Send(context.Background(), badPort, testMessage()), CategoryConfig)

	noUser := testCredentials(PortSTARTTLS)
	noUser.Username = ""
	requireSendError(t, sender.Send(context.Background(), noUser, testMessage()), CategoryConfig)

	badMessage := testMessage()
	badMessage.To = "not-an-address"
	requireSendError(t, sender.Send(context.Background(), testCredentials(PortSTARTTLS), badMessage), CategoryMessage)
}

func TestGmailSenderHonoursContextDeadline(t *testing.T) {
	// A server that accepts but never greets.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(3 * time.Second)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = testSender(listener.Addr().String(), nil).Send(ctx, testCredentials(PortSTARTTLS), testMessage())
	if time.Since(started) > 2*time.Second {
		t.Fatalf("Send ignored the context deadline (took %s)", time.Since(started))
	}
	requireSendError(t, err, CategoryTimeout)
}

const wantDevCaptureMessage = "Mode development: email dokumen diarahkan ke Mailpit di %s, tetapi server itu tidak bisa dihubungi. " +
	"Jalankan Mailpit (mis. docker run -d -p 1025:1025 -p 8025:8025 axllent/mailpit) atau set DOCUMENT_MAIL_DEV_SMTP_ADDR=off untuk mengirim lewat Gmail."

// requireNoSecrets checks a stored/returned error text names neither the
// credentials nor the recipients.
func requireNoSecrets(t *testing.T, text string) {
	t.Helper()
	for _, secret := range []string{testPassword, testUsername, "budi@example.com", "hr@example.com"} {
		if strings.Contains(text, secret) {
			t.Fatalf("error text leaks %q: %q", secret, text)
		}
	}
}

// Development capture server down: the error says what to do, in Indonesian.
func TestGmailSenderDevCaptureUnreachable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedAddr := listener.Addr().String()
	_ = listener.Close()

	sender := NewGmailSender(closedAddr)
	sender.dialTimeout = 2 * time.Second
	err = sender.Send(context.Background(), testCredentials(PortSTARTTLS), testMessage())
	sendErr := requireSendError(t, err, CategoryConnect)
	if !sendErr.DevCapture {
		t.Fatal("dev path failure must be marked DevCapture")
	}
	want := fmt.Sprintf(wantDevCaptureMessage, closedAddr)
	if sendErr.Message() != want || err.Error() != want {
		t.Fatalf("message = %q, want %q", sendErr.Message(), want)
	}
	requireNoSecrets(t, sendErr.Message())
}

// A capture server that accepts but never answers times out with the same
// guidance.
func TestGmailSenderDevCaptureTimeout(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(3 * time.Second)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err = NewGmailSender(listener.Addr().String()).Send(ctx, testCredentials(PortSTARTTLS), testMessage())
	sendErr := requireSendError(t, err, CategoryTimeout)
	if sendErr.Message() != fmt.Sprintf(wantDevCaptureMessage, listener.Addr().String()) {
		t.Fatalf("message = %q", sendErr.Message())
	}
	requireNoSecrets(t, sendErr.Message())
}

// Other dev-path failures keep their own category text: only "cannot reach
// the capture server" gets the Mailpit guidance.
func TestGmailSenderDevCaptureOtherErrorsUnchanged(t *testing.T) {
	sender := NewGmailSender("127.0.0.1:1")
	noUser := testCredentials(PortSTARTTLS)
	noUser.Username = ""
	sendErr := requireSendError(t, sender.Send(context.Background(), noUser, testMessage()), CategoryConfig)
	if strings.Contains(sendErr.Message(), "Mode development") {
		t.Fatalf("config error must not mention the capture server: %q", sendErr.Message())
	}
	rejected := &SendError{Category: CategoryRecipient, Code: 550, Addr: "localhost:1025", DevCapture: true}
	if rejected.Message() != "Alamat penerima ditolak (550)" {
		t.Fatalf("message = %q", rejected.Message())
	}
}

// Without a capture address (every APP_ENV but development) the sender only
// ever targets smtp.gmail.com over TLS, and its failures never carry the
// development hint.
func TestGmailSenderProductionNeverUsesCapture(t *testing.T) {
	sender := NewGmailSender("")
	if sender.DevSMTPAddr() != "" || sender.host != GmailHost {
		t.Fatalf("production sender: dev %q host %q", sender.DevSMTPAddr(), sender.host)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedAddr := listener.Addr().String()
	_ = listener.Close()
	// The seam redirects the dial to a closed local port; nothing reaches
	// Gmail. The reported address is still the Gmail host.
	err = testSender(closedAddr, nil).Send(context.Background(), testCredentials(PortSTARTTLS), testMessage())
	sendErr := requireSendError(t, err, CategoryConnect)
	if sendErr.DevCapture || strings.Contains(sendErr.Message(), "Mailpit") || sendErr.Addr != "smtp.gmail.com:587" {
		t.Fatalf("production error = %+v (%q)", sendErr, sendErr.Message())
	}

	// Plain SMTP is never used: a server without STARTTLS gets no MAIL/DATA.
	serverTLS, _ := testCertificate(t)
	server := newFakeSMTP(t, false, serverTLS)
	server.offerSTARTTLS = false
	server.serve()
	err = testSender(server.addr(), nil).Send(context.Background(), testCredentials(PortSTARTTLS), testMessage())
	requireSendError(t, err, CategoryTLS)
	server.wait(t)
	if server.sawCommand("MAIL") || server.sawCommand("DATA") {
		t.Fatalf("production path delivered without TLS, commands = %v", server.commands)
	}
}
