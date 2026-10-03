package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	netmail "net/mail"
	"net/textproto"
	"strings"
	"time"
)

const (
	crlf              = "\r\n"
	base64LineLength  = 76
	defaultAttachType = "application/octet-stream"
)

// composeOptions pins the otherwise random parts of a message. Production
// callers use Compose (random boundaries + Message-ID); the golden test pins
// them so the output is byte-for-byte stable.
type composeOptions struct {
	date          time.Time
	messageID     string
	mixedBoundary string
	altBoundary   string
}

// Compose renders msg as an RFC 5322 message with CRLF line endings:
//
//	multipart/mixed
//	├── multipart/alternative
//	│   ├── text/plain; charset=UTF-8 (quoted-printable)
//	│   └── text/html;  charset=UTF-8 (quoted-printable)
//	└── attachments (base64, wrapped at 76 columns)
//
// Non-ASCII header text (Subject, display name) is RFC 2047 encoded.
func Compose(from netmail.Address, msg Message, now time.Time) ([]byte, error) {
	messageID, err := newMessageID(from.Address)
	if err != nil {
		return nil, err
	}
	return compose(from, msg, composeOptions{date: now, messageID: messageID})
}

func compose(from netmail.Address, msg Message, opts composeOptions) ([]byte, error) {
	if err := validateBareAddress(from.Address); err != nil {
		return nil, fmt.Errorf("%w: from: %v", errInvalidMessage, err)
	}
	if err := msg.validate(); err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	mixed := multipart.NewWriter(&buf)
	if opts.mixedBoundary != "" {
		if err := mixed.SetBoundary(opts.mixedBoundary); err != nil {
			return nil, err
		}
	}

	var header bytes.Buffer
	sender := netmail.Address{Name: sanitizeHeaderText(from.Name), Address: strings.TrimSpace(from.Address)}
	writeHeader(&header, "From", sender.String())
	writeHeader(&header, "To", formatAddress(msg.To))
	if cc := formatAddressList(msg.Cc); cc != "" {
		writeHeader(&header, "Cc", cc)
	}
	if strings.TrimSpace(msg.ReplyTo) != "" {
		writeHeader(&header, "Reply-To", formatAddress(msg.ReplyTo))
	}
	writeHeader(&header, "Subject", encodeHeaderText(msg.Subject))
	writeHeader(&header, "Date", opts.date.Format(time.RFC1123Z))
	if opts.messageID != "" {
		writeHeader(&header, "Message-ID", opts.messageID)
	}
	writeHeader(&header, "MIME-Version", "1.0")
	writeHeader(&header, "Content-Type", mime.FormatMediaType("multipart/mixed", map[string]string{"boundary": mixed.Boundary()}))
	header.WriteString(crlf)

	if err := writeAlternative(mixed, msg, opts.altBoundary); err != nil {
		return nil, err
	}
	for _, attachment := range msg.Attachments {
		if err := writeAttachment(mixed, attachment); err != nil {
			return nil, err
		}
	}
	if err := mixed.Close(); err != nil {
		return nil, err
	}

	out := make([]byte, 0, header.Len()+buf.Len())
	out = append(out, header.Bytes()...)
	out = append(out, buf.Bytes()...)
	return out, nil
}

func writeAlternative(mixed *multipart.Writer, msg Message, boundary string) error {
	var altBody bytes.Buffer
	alt := multipart.NewWriter(&altBody)
	if boundary != "" {
		if err := alt.SetBoundary(boundary); err != nil {
			return err
		}
	}

	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Type", mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": alt.Boundary()}))
	part, err := mixed.CreatePart(partHeader)
	if err != nil {
		return err
	}

	text := msg.Text
	if strings.TrimSpace(text) == "" {
		text = htmlToPlainFallback(msg.HTML)
	}
	if err := writeQuotedPrintable(alt, "text/plain", text); err != nil {
		return err
	}
	if strings.TrimSpace(msg.HTML) != "" {
		if err := writeQuotedPrintable(alt, "text/html", msg.HTML); err != nil {
			return err
		}
	}
	if err := alt.Close(); err != nil {
		return err
	}

	_, err = part.Write(altBody.Bytes())
	return err
}

func writeQuotedPrintable(writer *multipart.Writer, mediaType string, body string) error {
	header := textproto.MIMEHeader{}
	header.Set("Content-Type", mime.FormatMediaType(mediaType, map[string]string{"charset": "UTF-8"}))
	header.Set("Content-Transfer-Encoding", "quoted-printable")
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}

	qp := quotedprintable.NewWriter(part)
	// quotedprintable emits CRLF for every line break in text mode; normalise
	// the input so "\r\n" and "\n" both become a single hard break.
	normalized := strings.ReplaceAll(body, "\r\n", "\n")
	if !strings.HasSuffix(normalized, "\n") {
		normalized += "\n"
	}
	if _, err := io.WriteString(qp, normalized); err != nil {
		return err
	}
	return qp.Close()
}

func writeAttachment(writer *multipart.Writer, attachment Attachment) error {
	filename := sanitizeFilename(attachment.Filename)
	contentType := strings.TrimSpace(attachment.ContentType)
	if contentType == "" {
		contentType = defaultAttachType
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("%w: attachment content type: %v", errInvalidMessage, err)
	}
	params["name"] = filename

	header := textproto.MIMEHeader{}
	header.Set("Content-Type", mime.FormatMediaType(mediaType, params))
	header.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	header.Set("Content-Transfer-Encoding", "base64")
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}

	encoded := base64.StdEncoding.EncodeToString(attachment.Data)
	for start := 0; start < len(encoded); start += base64LineLength {
		end := start + base64LineLength
		if end > len(encoded) {
			end = len(encoded)
		}
		if _, err := io.WriteString(part, encoded[start:end]+crlf); err != nil {
			return err
		}
	}
	return nil
}

// maxHeaderLine is the RFC 5322 section 2.1.1 recommended line length.
const maxHeaderLine = 78

// writeHeader writes "Name: value" and folds it at existing spaces so every
// physical line stays within 78 characters where the value allows it. Every
// header we write has a legal folding point at each space: between words of
// unstructured text (Subject), between RFC 2047 encoded-words, inside a
// quoted display name (FWS is allowed in a quoted-string), after the commas
// of an address list, and between MIME parameters. A single token longer than
// the limit is left intact (still far below the 998 hard limit).
func writeHeader(buf *bytes.Buffer, name string, value string) {
	buf.WriteString(name)
	buf.WriteString(":")
	lineLength := len(name) + 1
	for _, token := range strings.Split(value, " ") {
		piece := " " + token
		// Fold before a token that does not fit (for the first token that
		// means starting the value on a continuation line), but never before
		// an empty token from repeated spaces: a folded line must not be
		// whitespace only.
		if lineLength+len(piece) > maxHeaderLine && token != "" {
			buf.WriteString(crlf)
			lineLength = 0
		}
		buf.WriteString(piece)
		lineLength += len(piece)
	}
	buf.WriteString(crlf)
}

func formatAddress(address string) string {
	return (&netmail.Address{Address: strings.TrimSpace(address)}).String()
}

func formatAddressList(addresses []string) string {
	formatted := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if strings.TrimSpace(address) == "" {
			continue
		}
		formatted = append(formatted, formatAddress(address))
	}
	return strings.Join(formatted, ", ")
}

// encodeHeaderText RFC 2047 encodes non-ASCII text (UTF-8, Q encoding). A
// long result is a series of encoded-words separated by spaces, which
// writeHeader folds (whitespace between encoded-words is ignored by readers).
func encodeHeaderText(value string) string {
	return mime.QEncoding.Encode("UTF-8", sanitizeHeaderText(value))
}

// htmlToPlainFallback is only used when a caller supplies HTML without a
// text part; it strips tags so the text/plain alternative is never empty.
func htmlToPlainFallback(html string) string {
	var builder strings.Builder
	inTag := false
	for _, char := range html {
		switch {
		case char == '<':
			inTag = true
		case char == '>':
			inTag = false
		case !inTag:
			builder.WriteRune(char)
		}
	}
	return strings.TrimSpace(builder.String())
}

func newMessageID(fromAddress string) (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	domain := "localhost"
	if at := strings.LastIndex(fromAddress, "@"); at >= 0 && at < len(fromAddress)-1 {
		domain = strings.TrimSpace(fromAddress[at+1:])
	}
	return "<" + hex.EncodeToString(random) + "@" + domain + ">", nil
}
