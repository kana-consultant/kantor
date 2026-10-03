package mail

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/textproto"
)

// ErrorCategory is the fixed, user-facing class of a send failure. The UI
// shows the category message instead of raw SMTP text; app passwords get
// revoked on every Google password change, so "auth" is the common one.
type ErrorCategory string

const (
	CategoryConfig    ErrorCategory = "config"
	CategoryMessage   ErrorCategory = "message"
	CategoryConnect   ErrorCategory = "connect"
	CategoryTLS       ErrorCategory = "tls"
	CategoryAuth      ErrorCategory = "auth"
	CategoryRecipient ErrorCategory = "recipient"
	CategoryRejected  ErrorCategory = "rejected"
	// CategoryTemporary is a 4xx reply (e.g. Gmail's "454 4.7.0 Too many
	// login attempts" or "421 4.7.0 Try again later"): the account and
	// password may be fine, the server only refuses for now.
	CategoryTemporary ErrorCategory = "temporary"
	CategoryTimeout   ErrorCategory = "timeout"
)

// stage is where in the SMTP conversation a failure happened; it drives the
// category mapping.
type stage string

const (
	stageDial  stage = "dial"
	stageTLS   stage = "tls"
	stageAuth  stage = "auth"
	stageMail  stage = "mail"
	stageRcpt  stage = "rcpt"
	stageData  stage = "data"
	stageQuit  stage = "quit"
	stageHello stage = "hello"
)

// SendError is returned by Sender.Send for every failure. Error() returns only
// the fixed category message (plus SMTP code / host:port), never the
// credentials. The underlying cause stays available through Unwrap for
// server-side logging.
type SendError struct {
	Category ErrorCategory
	// Code is the SMTP reply code when the server answered (e.g. 535).
	Code int
	// Addr is the host:port that was dialled.
	Addr string
	// Detail is an extra fixed hint (e.g. "STARTTLS tidak tersedia").
	Detail string
	// DevCapture marks a failure on the development capture path (Mailpit),
	// so an unreachable capture server gets a message that says what to do.
	DevCapture bool
	cause      error
}

func (e *SendError) Error() string {
	return e.Message()
}

func (e *SendError) Unwrap() error {
	return e.cause
}

// Message is the fixed Indonesian text shown to admins and stored in
// email_deliveries.error.
func (e *SendError) Message() string {
	withCode := func(base string) string {
		if e.Code > 0 {
			return fmt.Sprintf("%s (%d)", base, e.Code)
		}
		return base
	}
	if e.DevCapture && (e.Category == CategoryConnect || e.Category == CategoryTimeout) {
		return devCaptureUnreachableMessage(e.Addr)
	}
	switch e.Category {
	case CategoryConfig:
		if e.Detail != "" {
			return "Konfigurasi email dokumen tidak valid: " + e.Detail
		}
		return "Konfigurasi email dokumen tidak valid"
	case CategoryMessage:
		return "Isi email tidak valid (alamat, subjek, atau lampiran)"
	case CategoryConnect:
		return withCode("Tidak bisa terhubung ke " + e.Addr)
	case CategoryTLS:
		if e.Detail != "" {
			return withCode("TLS gagal: " + e.Detail)
		}
		return withCode("TLS gagal saat terhubung ke " + e.Addr)
	case CategoryAuth:
		return withCode("Autentikasi ditolak")
	case CategoryRecipient:
		return withCode("Alamat penerima ditolak")
	case CategoryTemporary:
		if e.Code > 0 {
			return fmt.Sprintf("Server email menolak sementara (%d), coba lagi nanti", e.Code)
		}
		return "Server email menolak sementara, coba lagi nanti"
	case CategoryTimeout:
		return "Waktu habis saat mengirim melalui " + e.Addr
	default:
		return withCode("Server email menolak pengiriman")
	}
}

// devCaptureUnreachableMessage is the fixed text for a development capture
// server that cannot be reached. It names only the capture address: no
// credentials, no recipients.
func devCaptureUnreachableMessage(addr string) string {
	return "Mode development: email dokumen diarahkan ke Mailpit di " + addr +
		", tetapi server itu tidak bisa dihubungi. Jalankan Mailpit (mis. docker run -d -p 1025:1025 -p 8025:8025 axllent/mailpit) " +
		"atau set DOCUMENT_MAIL_DEV_SMTP_ADDR=off untuk mengirim lewat Gmail."
}

// AsSendError extracts a *SendError from err.
func AsSendError(err error) (*SendError, bool) {
	var sendErr *SendError
	if errors.As(err, &sendErr) {
		return sendErr, true
	}
	return nil, false
}

func configError(detail string) *SendError {
	return &SendError{Category: CategoryConfig, Detail: detail}
}

// classify maps a raw error from a given stage to a SendError.
func classify(st stage, addr string, err error) *SendError {
	if err == nil {
		return nil
	}
	if existing, ok := AsSendError(err); ok {
		return existing
	}
	if errors.Is(err, errInvalidMessage) {
		return &SendError{Category: CategoryMessage, Addr: addr, cause: err}
	}

	if st == stageDial {
		return &SendError{Category: CategoryConnect, Addr: addr, cause: err}
	}

	var protoErr *textproto.Error
	if errors.As(err, &protoErr) {
		code := protoErr.Code
		switch {
		case st == stageTLS:
			return &SendError{Category: CategoryTLS, Code: code, Addr: addr, cause: err}
		case code >= 400 && code < 500:
			// Transient: never report a 4xx at AUTH as a rejected login,
			// which would send the admin off to regenerate a valid app
			// password.
			return &SendError{Category: CategoryTemporary, Code: code, Addr: addr, cause: err}
		case st == stageAuth:
			return &SendError{Category: CategoryAuth, Code: code, Addr: addr, cause: err}
		case st == stageRcpt && code >= 500:
			return &SendError{Category: CategoryRecipient, Code: code, Addr: addr, cause: err}
		default:
			return &SendError{Category: CategoryRejected, Code: code, Addr: addr, cause: err}
		}
	}

	if isTimeout(err) {
		return &SendError{Category: CategoryTimeout, Addr: addr, cause: err}
	}
	if st == stageTLS || isTLSError(err) {
		return &SendError{Category: CategoryTLS, Addr: addr, cause: err}
	}
	if st == stageAuth {
		return &SendError{Category: CategoryAuth, Addr: addr, cause: err}
	}
	if st == stageHello {
		return &SendError{Category: CategoryConnect, Addr: addr, cause: err}
	}
	return &SendError{Category: CategoryRejected, Addr: addr, cause: err}
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func isTLSError(err error) bool {
	var recordErr tls.RecordHeaderError
	var alertErr tls.AlertError
	var certErr *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	var invalidErr x509.CertificateInvalidError
	return errors.As(err, &recordErr) ||
		errors.As(err, &alertErr) ||
		errors.As(err, &certErr) ||
		errors.As(err, &unknownAuthority) ||
		errors.As(err, &hostnameErr) ||
		errors.As(err, &invalidErr)
}
