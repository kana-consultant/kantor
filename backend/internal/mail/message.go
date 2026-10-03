// Package mail is the documents-only SMTP path (payslips, contracts and the
// admin test email). It is stdlib only and talks to exactly one host,
// smtp.gmail.com; password-reset and notification mail stay on the Resend
// clients in service/auth and service/notifications.
package mail

import (
	"errors"
	"fmt"
	"mime"
	netmail "net/mail"
	"path"
	"strings"
)

// Attachment is one file attached to a Message. Data is sent base64 encoded.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// Message is a single outgoing document email. From is not part of the
// message: the sender always uses the configured Gmail username (Gmail
// rewrites any other From), with the configured display name.
type Message struct {
	To          string
	Cc          []string
	ReplyTo     string
	Subject     string
	HTML        string
	Text        string
	Attachments []Attachment
}

var errInvalidMessage = errors.New("invalid message")

// Recipients returns To followed by every Cc address (the SMTP RCPT list).
func (m Message) Recipients() []string {
	recipients := make([]string, 0, 1+len(m.Cc))
	recipients = append(recipients, strings.TrimSpace(m.To))
	for _, cc := range m.Cc {
		if trimmed := strings.TrimSpace(cc); trimmed != "" {
			recipients = append(recipients, trimmed)
		}
	}
	return recipients
}

// validate checks the parts that end up in headers or the envelope. Addresses
// must be bare addresses (no display names) so nothing user-controlled can
// smuggle extra header content.
func (m Message) validate() error {
	if err := validateBareAddress(m.To); err != nil {
		return fmt.Errorf("%w: to: %v", errInvalidMessage, err)
	}
	for _, cc := range m.Cc {
		if strings.TrimSpace(cc) == "" {
			continue
		}
		if err := validateBareAddress(cc); err != nil {
			return fmt.Errorf("%w: cc: %v", errInvalidMessage, err)
		}
	}
	if strings.TrimSpace(m.ReplyTo) != "" {
		if err := validateBareAddress(m.ReplyTo); err != nil {
			return fmt.Errorf("%w: reply-to: %v", errInvalidMessage, err)
		}
	}
	if strings.TrimSpace(sanitizeHeaderText(m.Subject)) == "" {
		return fmt.Errorf("%w: subject is required", errInvalidMessage)
	}
	if strings.TrimSpace(m.Text) == "" && strings.TrimSpace(m.HTML) == "" {
		return fmt.Errorf("%w: text or html body is required", errInvalidMessage)
	}
	for index, attachment := range m.Attachments {
		if sanitizeFilename(attachment.Filename) == "" {
			return fmt.Errorf("%w: attachment %d has no filename", errInvalidMessage, index)
		}
		if strings.TrimSpace(attachment.ContentType) != "" {
			if _, _, err := mime.ParseMediaType(attachment.ContentType); err != nil {
				return fmt.Errorf("%w: attachment %d content type: %v", errInvalidMessage, index, err)
			}
		}
	}
	return nil
}

// ValidateAddress reports whether value is a single bare email address
// (e.g. "slip@example.com", not "Slip <slip@example.com>").
func ValidateAddress(value string) error {
	return validateBareAddress(value)
}

func validateBareAddress(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return errors.New("address is empty")
	}
	if strings.ContainsAny(trimmed, "\r\n<>\",; ") {
		return errors.New("address contains forbidden characters")
	}
	parsed, err := netmail.ParseAddress(trimmed)
	if err != nil {
		return err
	}
	if parsed.Name != "" || !strings.EqualFold(parsed.Address, trimmed) {
		return errors.New("address must not carry a display name")
	}
	return nil
}

// sanitizeHeaderText removes CR/LF and other control characters so a value
// can never terminate a header line early.
func sanitizeHeaderText(value string) string {
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char == '\t':
			builder.WriteRune(' ')
		case char < 0x20 || char == 0x7f:
			// drop control characters, including CR and LF
		default:
			builder.WriteRune(char)
		}
	}
	return strings.TrimSpace(builder.String())
}

// sanitizeFilename keeps only the base name and strips characters that are
// unsafe inside a MIME parameter.
func sanitizeFilename(value string) string {
	cleaned := sanitizeHeaderText(value)
	cleaned = strings.ReplaceAll(cleaned, "\\", "/")
	cleaned = path.Base(cleaned)
	cleaned = strings.Map(func(char rune) rune {
		switch char {
		case '"', '/', ':', '*', '?', '<', '>', '|':
			return '_'
		}
		return char
	}, cleaned)
	if cleaned == "." || cleaned == "/" {
		return ""
	}
	return strings.TrimSpace(cleaned)
}
