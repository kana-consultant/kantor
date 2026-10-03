package mail

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	netmail "net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	// GmailHost is the only SMTP host documents are ever sent through. It is
	// a constant on purpose: a free-form host would turn the settings page
	// into a relay-redirect / SSRF / port-scan primitive.
	GmailHost = "smtp.gmail.com"
	// PortSTARTTLS is submission with a mandatory STARTTLS upgrade.
	PortSTARTTLS = 587
	// PortImplicitTLS is SMTPS (TLS from the first byte).
	PortImplicitTLS = 465

	defaultDialTimeout = 15 * time.Second
	// defaultIOTimeout bounds the whole SMTP conversation when the caller's
	// context has no earlier deadline.
	defaultIOTimeout = 60 * time.Second
	helloName        = "localhost"
	tracerName       = "github.com/kana-consultant/kantor/backend/internal/mail"
)

// Credentials are the per-tenant Gmail settings for one send. Password is the
// decrypted app password; it only lives in memory for the duration of Send.
type Credentials struct {
	Username   string
	Password   string
	Port       int
	SenderName string
}

// GmailSender delivers document email through smtp.gmail.com, or — only when
// constructed with a dev address — through a local capture server such as
// Mailpit with plain SMTP.
type GmailSender struct {
	devAddr     string
	dialTimeout time.Duration
	ioTimeout   time.Duration
	now         func() time.Time

	// Test seams. They are unexported so only this package's tests can point
	// the sender at a fake listener; production always dials GmailHost.
	host     string
	dialAddr string
	rootCAs  *x509.CertPool
}

// NewGmailSender builds the document sender. devSMTPAddr must be empty in
// every environment except development (config.DocumentMailCapture enforces
// that); when set, mail goes to that host:port with plain SMTP, no TLS and no
// AUTH.
func NewGmailSender(devSMTPAddr string) *GmailSender {
	return &GmailSender{
		devAddr:     strings.TrimSpace(devSMTPAddr),
		dialTimeout: defaultDialTimeout,
		ioTimeout:   defaultIOTimeout,
		now:         time.Now,
		host:        GmailHost,
	}
}

// DevSMTPAddr returns the local capture address when the dev override is
// active, or "" in normal (Gmail) mode.
func (s *GmailSender) DevSMTPAddr() string {
	return s.devAddr
}

// Send composes msg and delivers it. Every failure is a *SendError whose
// message is a fixed category; the password never appears in it.
func (s *GmailSender) Send(ctx context.Context, creds Credentials, msg Message) error {
	mode := "starttls"
	if s.devAddr != "" {
		mode = "dev_plain"
	} else if creds.Port == PortImplicitTLS {
		mode = "implicit_tls"
	}

	ctx, span := otel.Tracer(tracerName).Start(ctx, "mail.smtp.send")
	defer span.End()
	span.SetAttributes(
		attribute.String("smtp.mode", mode),
		attribute.Int("smtp.port", creds.Port),
		attribute.Int("mail.recipients", len(msg.Recipients())),
		attribute.Int("mail.attachments", len(msg.Attachments)),
	)

	err := s.send(ctx, creds, msg)
	if err != nil {
		sendErr := classify(stageData, "", err)
		if s.devAddr != "" {
			sendErr.DevCapture = true
		}
		span.SetAttributes(attribute.String("mail.error_category", string(sendErr.Category)))
		if sendErr.Code > 0 {
			span.SetAttributes(attribute.Int("smtp.reply_code", sendErr.Code))
		}
		span.SetStatus(codes.Error, sendErr.Message())
		return sendErr
	}
	span.SetStatus(codes.Ok, "")
	return nil
}

func (s *GmailSender) send(ctx context.Context, creds Credentials, msg Message) error {
	username := strings.TrimSpace(creds.Username)
	if err := validateBareAddress(username); err != nil {
		return configError("alamat Gmail belum diisi atau tidak valid")
	}

	from := netmail.Address{Name: creds.SenderName, Address: username}
	data, err := Compose(from, msg, s.now())
	if err != nil {
		return classify(stageData, "", err)
	}

	if s.devAddr != "" {
		return s.sendPlain(ctx, s.devAddr, username, msg.Recipients(), data)
	}

	if strings.TrimSpace(creds.Password) == "" {
		return configError("app password belum diisi")
	}
	switch creds.Port {
	case PortSTARTTLS, PortImplicitTLS:
	default:
		return configError("port harus 587 atau 465")
	}

	addr := net.JoinHostPort(s.host, strconv.Itoa(creds.Port))
	dialAddr := addr
	if s.dialAddr != "" {
		dialAddr = s.dialAddr
	}

	conn, err := s.dial(ctx, dialAddr)
	if err != nil {
		return classify(stageDial, addr, err)
	}
	defer conn.Close()
	stop := s.bindDeadline(ctx, conn)
	defer stop()

	tlsConfig := &tls.Config{
		ServerName: s.host,
		MinVersion: tls.VersionTLS12,
		RootCAs:    s.rootCAs,
	}

	var client *smtp.Client
	if creds.Port == PortImplicitTLS {
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return classify(stageTLS, addr, err)
		}
		client, err = smtp.NewClient(tlsConn, s.host)
		if err != nil {
			return classify(stageHello, addr, err)
		}
	} else {
		client, err = smtp.NewClient(conn, s.host)
		if err != nil {
			return classify(stageHello, addr, err)
		}
		if err := client.Hello(helloName); err != nil {
			return classify(stageHello, addr, err)
		}
		// Never fall back to plaintext: no STARTTLS means no AUTH.
		if ok, _ := client.Extension("STARTTLS"); !ok {
			_ = client.Quit()
			return &SendError{Category: CategoryTLS, Addr: addr, Detail: "server tidak menawarkan STARTTLS"}
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			quitAfterReply(client, err)
			return classify(stageTLS, addr, err)
		}
	}
	defer client.Close()

	// PlainAuth itself refuses non-TLS connections to non-localhost hosts;
	// both paths above are TLS by the time we get here.
	if err := client.Auth(smtp.PlainAuth("", username, creds.Password, s.host)); err != nil {
		return classify(stageAuth, addr, err)
	}

	return deliver(client, addr, username, msg.Recipients(), data)
}

// sendPlain is the development-only path (Mailpit): plain SMTP, no TLS, no
// AUTH. It is reachable only when NewGmailSender got a dev address.
func (s *GmailSender) sendPlain(ctx context.Context, addr string, from string, recipients []string, data []byte) error {
	conn, err := s.dial(ctx, addr)
	if err != nil {
		return classify(stageDial, addr, err)
	}
	defer conn.Close()
	stop := s.bindDeadline(ctx, conn)
	defer stop()

	host, _, splitErr := net.SplitHostPort(addr)
	if splitErr != nil {
		host = addr
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return classify(stageHello, addr, err)
	}
	defer client.Close()
	if err := client.Hello(helloName); err != nil {
		return classify(stageHello, addr, err)
	}

	return deliver(client, addr, from, recipients, data)
}

func deliver(client *smtp.Client, addr string, from string, recipients []string, data []byte) error {
	if err := client.Mail(from); err != nil {
		quitAfterReply(client, err)
		return classify(stageMail, addr, err)
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient); err != nil {
			quitAfterReply(client, err)
			return classify(stageRcpt, addr, err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		quitAfterReply(client, err)
		return classify(stageData, addr, err)
	}
	if _, err := writer.Write(data); err != nil {
		_ = writer.Close()
		return classify(stageData, addr, err)
	}
	if err := writer.Close(); err != nil {
		quitAfterReply(client, err)
		return classify(stageData, addr, err)
	}
	// The message is accepted once DATA is closed; a failing QUIT does not
	// change the outcome.
	_ = client.Quit()
	return nil
}

// quitAfterReply sends a best-effort QUIT after the server answered a command
// with an error reply (RFC 5321 4.1.1.10: the client must not just drop the
// channel). The SMTP stream is still in sync then; after an I/O or TLS
// handshake error it is not, so nothing is sent. It is bounded by the
// connection deadline.
func quitAfterReply(client *smtp.Client, err error) {
	var protoErr *textproto.Error
	if errors.As(err, &protoErr) {
		_ = client.Quit()
	}
}

func (s *GmailSender) dial(ctx context.Context, addr string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: s.dialTimeout}
	return dialer.DialContext(ctx, "tcp", addr)
}

// bindDeadline applies the context deadline (or the default I/O timeout) to
// the connection and aborts blocked reads/writes when ctx is cancelled.
func (s *GmailSender) bindDeadline(ctx context.Context, conn net.Conn) func() bool {
	deadline := time.Now().Add(s.ioTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)
	return context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Unix(1, 0))
	})
}
