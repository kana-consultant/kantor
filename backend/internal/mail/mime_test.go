package mail

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	netmail "net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata golden files")

const goldenSubject = "Slip Gaji — Oktober 2026 (Revisi 1)"

func goldenMessage() (netmail.Address, Message, composeOptions) {
	pdf := make([]byte, 0, 300)
	pdf = append(pdf, []byte("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")...)
	for i := 0; len(pdf) < 300; i++ {
		pdf = append(pdf, byte(i*7+3))
	}

	text, html, err := RenderDocumentEmail(DocumentEmail{
		RecipientName: "Budi Santoso",
		ParagraphsID: []string{
			"Terlampir slip gaji Anda untuk periode Oktober 2026 (dokumen revisi 1).",
			"Dokumen ini bersifat rahasia — mohon tidak diteruskan kepada pihak lain.",
		},
		ParagraphsEN: []string{
			"Please find attached your payslip for October 2026 (revision 1).",
			"This document is confidential — please do not forward it.",
		},
		Signature: "HR Kantor",
	})
	if err != nil {
		panic(err)
	}

	return netmail.Address{Name: "HR Kantor", Address: "slip@example.com"},
		Message{
			To:          "budi.santoso@example.com",
			Cc:          []string{"hr@example.com"},
			ReplyTo:     "hr@example.com",
			Subject:     goldenSubject,
			Text:        text,
			HTML:        html,
			Attachments: []Attachment{{Filename: "Slip_Gaji_Oktober_2026.pdf", ContentType: "application/pdf", Data: pdf}},
		},
		composeOptions{
			date:          time.Date(2026, 10, 23, 9, 30, 0, 0, time.FixedZone("WIB", 7*60*60)),
			messageID:     "<0123456789abcdef@example.com>",
			mixedBoundary: "kantor-mixed-0001",
			altBoundary:   "kantor-alt-0001",
		}
}

func TestComposeGolden(t *testing.T) {
	from, msg, opts := goldenMessage()
	out, err := compose(from, msg, opts)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}

	assertCRLFOnly(t, out)

	normalized := strings.ReplaceAll(string(out), "\r\n", "\n")
	goldenPath := filepath.Join("testdata", "document_email.golden")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, []byte(normalized), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if normalized != string(want) {
		t.Fatalf("composed message differs from %s (run go test ./internal/mail -run TestComposeGolden -update to accept)\n--- got ---\n%s", goldenPath, normalized)
	}
}

func TestComposeStructure(t *testing.T) {
	from, msg, opts := goldenMessage()
	out, err := compose(from, msg, opts)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}

	parsed, err := netmail.ReadMessage(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	rawSubject := parsed.Header.Get("Subject")
	if !strings.HasPrefix(rawSubject, "=?UTF-8?q?") {
		t.Fatalf("Subject must be RFC 2047 encoded, got %q", rawSubject)
	}
	decoder := new(mime.WordDecoder)
	subject, err := decoder.DecodeHeader(rawSubject)
	if err != nil {
		t.Fatalf("decode subject: %v", err)
	}
	if subject != goldenSubject {
		t.Fatalf("Subject = %q, want %q", subject, goldenSubject)
	}
	fromHeader, err := parsed.Header.AddressList("From")
	if err != nil || len(fromHeader) != 1 || fromHeader[0].Name != "HR Kantor" || fromHeader[0].Address != "slip@example.com" {
		t.Fatalf("From = %v (%v)", fromHeader, err)
	}
	if got := parsed.Header.Get("Reply-To"); got != "<hr@example.com>" {
		t.Fatalf("Reply-To = %q", got)
	}
	if got := parsed.Header.Get("Cc"); got != "<hr@example.com>" {
		t.Fatalf("Cc = %q", got)
	}

	mediaType, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/mixed" {
		t.Fatalf("top-level Content-Type = %q (%v)", parsed.Header.Get("Content-Type"), err)
	}
	mixed := multipart.NewReader(parsed.Body, params["boundary"])

	altPart, err := mixed.NextPart()
	if err != nil {
		t.Fatalf("first mixed part: %v", err)
	}
	altType, altParams, err := mime.ParseMediaType(altPart.Header.Get("Content-Type"))
	if err != nil || altType != "multipart/alternative" {
		t.Fatalf("first part Content-Type = %q", altPart.Header.Get("Content-Type"))
	}
	alt := multipart.NewReader(altPart, altParams["boundary"])
	for _, want := range []struct {
		mediaType string
		body      string
	}{
		{"text/plain", msg.Text},
		{"text/html", msg.HTML},
	} {
		part, err := alt.NextPart()
		if err != nil {
			t.Fatalf("alternative part %s: %v", want.mediaType, err)
		}
		partType, partParams, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		if partType != want.mediaType || !strings.EqualFold(partParams["charset"], "UTF-8") {
			t.Fatalf("part Content-Type = %q, want %s; charset=UTF-8", part.Header.Get("Content-Type"), want.mediaType)
		}
		// multipart.Reader transparently decodes quoted-printable.
		body, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read %s: %v", want.mediaType, err)
		}
		got := strings.ReplaceAll(string(body), "\r\n", "\n")
		if strings.TrimRight(got, "\n") != strings.TrimRight(want.body, "\n") {
			t.Fatalf("%s body round-trip mismatch:\n%s", want.mediaType, got)
		}
	}
	if _, err := alt.NextPart(); err != io.EOF {
		t.Fatalf("expected exactly two alternative parts, got err=%v", err)
	}

	attachment, err := mixed.NextPart()
	if err != nil {
		t.Fatalf("attachment part: %v", err)
	}
	if attachment.Header.Get("Content-Transfer-Encoding") != "base64" {
		t.Fatalf("attachment encoding = %q", attachment.Header.Get("Content-Transfer-Encoding"))
	}
	if attachment.FileName() != "Slip_Gaji_Oktober_2026.pdf" {
		t.Fatalf("attachment filename = %q", attachment.FileName())
	}
	rawAttachment, err := io.ReadAll(attachment)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimRight(string(rawAttachment), "\r\n"), "\r\n") {
		if len(line) > 76 {
			t.Fatalf("base64 line longer than 76 columns: %d", len(line))
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(rawAttachment), "\r\n", ""))
	if err != nil {
		t.Fatalf("decode attachment: %v", err)
	}
	if !bytes.Equal(decoded, msg.Attachments[0].Data) {
		t.Fatal("attachment bytes do not round-trip")
	}
	if _, err := mixed.NextPart(); err != io.EOF {
		t.Fatalf("expected no further mixed parts, got err=%v", err)
	}
}

func TestComposeLongSubjectIsFolded(t *testing.T) {
	from, msg, opts := goldenMessage()
	msg.Subject = "Kontrak Kerja Waktu Tertentu (PKWT) dan Perjanjian Kerahasiaan — Budi Santoso — 021/PKWT/P10/X/2026 (Revisi 2)"
	out, err := compose(from, msg, opts)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	headerEnd := bytes.Index(out, []byte("\r\n\r\n"))
	for _, line := range strings.Split(string(out[:headerEnd]), "\r\n") {
		if len(line) > 78 {
			t.Fatalf("header line longer than 78 characters: %q", line)
		}
	}
	parsed, err := netmail.ReadMessage(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
	if err != nil || subject != msg.Subject {
		t.Fatalf("folded subject = %q (%v), want %q", subject, err, msg.Subject)
	}
}

// Every header line must fold to 78 characters, not only non-ASCII subjects:
// ASCII subjects, long (non-)ASCII display names and Cc lists too.
func TestComposeFoldsEveryHeader(t *testing.T) {
	cc := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		cc = append(cc, fmt.Sprintf("hr.staff%02d@perusahaan.co.id", i))
	}
	for _, tc := range []struct {
		name    string
		from    netmail.Address
		subject string
		cc      []string
	}{
		{
			name:    "ascii subject",
			from:    netmail.Address{Name: "HR Kantor", Address: "slip@example.com"},
			subject: "Kontrak Kerja Waktu Tertentu (PKWT) dan Perjanjian Kerahasiaan - Budi Santoso - 021/PKWT/P10/X/2026 (Revisi 2)",
		},
		{
			name:    "120-rune non-ASCII sender name",
			from:    netmail.Address{Name: strings.Repeat("人", 120), Address: "slip@example.com"},
			subject: goldenSubject,
		},
		{
			name:    "120-char ASCII sender name",
			from:    netmail.Address{Name: strings.TrimSpace(strings.Repeat("Divisi SDM ", 11)), Address: "slip@example.com"},
			subject: goldenSubject,
		},
		{
			name:    "many Cc addresses",
			from:    netmail.Address{Name: "HR Kantor", Address: "slip@example.com"},
			subject: goldenSubject,
			cc:      cc,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, msg, opts := goldenMessage()
			msg.Subject = tc.subject
			if tc.cc != nil {
				msg.Cc = tc.cc
			}
			out, err := compose(tc.from, msg, opts)
			if err != nil {
				t.Fatalf("compose: %v", err)
			}
			assertCRLFOnly(t, out)
			headerEnd := bytes.Index(out, []byte("\r\n\r\n"))
			for _, line := range strings.Split(string(out[:headerEnd]), "\r\n") {
				if len(line) > 78 {
					t.Fatalf("header line longer than 78 characters (%d): %q", len(line), line)
				}
				if strings.TrimSpace(line) == "" {
					t.Fatal("a folded header line must not be whitespace only")
				}
			}

			parsed, err := netmail.ReadMessage(bytes.NewReader(out))
			if err != nil {
				t.Fatal(err)
			}
			subject, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
			if err != nil || subject != tc.subject {
				t.Fatalf("subject = %q (%v), want %q", subject, err, tc.subject)
			}
			from, err := parsed.Header.AddressList("From")
			if err != nil || len(from) != 1 || from[0].Name != tc.from.Name || from[0].Address != tc.from.Address {
				t.Fatalf("From = %v (%v), want %q <%s>", from, err, tc.from.Name, tc.from.Address)
			}
			ccList, err := parsed.Header.AddressList("Cc")
			if err != nil || len(ccList) != len(msg.Cc) {
				t.Fatalf("Cc = %d addresses (%v), want %d", len(ccList), err, len(msg.Cc))
			}
			for index, address := range ccList {
				if address.Address != msg.Cc[index] {
					t.Fatalf("Cc[%d] = %q, want %q", index, address.Address, msg.Cc[index])
				}
			}
		})
	}
}

func TestComposeRejectsHeaderInjection(t *testing.T) {
	from, msg, opts := goldenMessage()

	injected := msg
	injected.To = "budi@example.com\r\nBcc: attacker@example.com"
	if _, err := compose(from, injected, opts); err == nil {
		t.Fatal("expected CRLF in To to be rejected")
	}

	named := msg
	named.To = "Budi <budi@example.com>"
	if _, err := compose(from, named, opts); err == nil {
		t.Fatal("expected a display-name recipient to be rejected")
	}

	subject := msg
	subject.Subject = "Slip\r\nBcc: attacker@example.com"
	out, err := compose(from, subject, opts)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if bytes.Contains(out, []byte("\r\nBcc:")) {
		t.Fatal("CR/LF in the subject must not start a new header")
	}
}

func assertCRLFOnly(t *testing.T, out []byte) {
	t.Helper()
	for index, char := range out {
		if char == '\n' && (index == 0 || out[index-1] != '\r') {
			t.Fatalf("bare LF at byte %d", index)
		}
		if char == '\r' && (index+1 >= len(out) || out[index+1] != '\n') {
			t.Fatalf("bare CR at byte %d", index)
		}
	}
	if !bytes.HasSuffix(out, []byte("\r\n")) {
		t.Fatal("message must end with CRLF")
	}
}
