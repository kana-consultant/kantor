package mail

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net/textproto"
	"os"
	"testing"
)

func TestClassifyErrorCategories(t *testing.T) {
	const addr = "smtp.gmail.com:587"

	tests := []struct {
		name         string
		stage        stage
		err          error
		wantCategory ErrorCategory
		wantMessage  string
		wantCode     int
	}{
		{
			name:         "auth 535",
			stage:        stageAuth,
			err:          &textproto.Error{Code: 535, Msg: "5.7.8 Username and Password not accepted"},
			wantCategory: CategoryAuth,
			wantMessage:  "Autentikasi ditolak (535)",
			wantCode:     535,
		},
		{
			name:         "auth 534 multi-line",
			stage:        stageAuth,
			err:          &textproto.Error{Code: 534, Msg: "5.7.9 Application-specific password required.\n5.7.9 Learn more"},
			wantCategory: CategoryAuth,
			wantMessage:  "Autentikasi ditolak (534)",
			wantCode:     534,
		},
		{
			name:         "auth 454 too many login attempts is transient",
			stage:        stageAuth,
			err:          &textproto.Error{Code: 454, Msg: "4.7.0 Too many login attempts, please try again later"},
			wantCategory: CategoryTemporary,
			wantMessage:  "Server email menolak sementara (454), coba lagi nanti",
			wantCode:     454,
		},
		{
			name:         "auth 421 try again later is transient",
			stage:        stageAuth,
			err:          &textproto.Error{Code: 421, Msg: "4.7.0 Try again later, closing connection"},
			wantCategory: CategoryTemporary,
			wantMessage:  "Server email menolak sementara (421), coba lagi nanti",
			wantCode:     421,
		},
		{
			name:         "mail 451 is transient",
			stage:        stageMail,
			err:          &textproto.Error{Code: 451, Msg: "4.3.0 Temporary server error"},
			wantCategory: CategoryTemporary,
			wantMessage:  "Server email menolak sementara (451), coba lagi nanti",
			wantCode:     451,
		},
		{
			name:         "starttls refused keeps the SMTP code",
			stage:        stageTLS,
			err:          &textproto.Error{Code: 454, Msg: "4.7.0 TLS not available due to temporary reason"},
			wantCategory: CategoryTLS,
			wantMessage:  "TLS gagal saat terhubung ke smtp.gmail.com:587 (454)",
			wantCode:     454,
		},
		{
			name:         "auth without SMTP code",
			stage:        stageAuth,
			err:          errors.New("unencrypted connection"),
			wantCategory: CategoryAuth,
			wantMessage:  "Autentikasi ditolak",
		},
		{
			name:         "dial refused",
			stage:        stageDial,
			err:          errors.New("dial tcp 142.250.4.108:587: connect: connection refused"),
			wantCategory: CategoryConnect,
			wantMessage:  "Tidak bisa terhubung ke smtp.gmail.com:587",
		},
		{
			name:         "dial timeout is still a connect failure",
			stage:        stageDial,
			err:          fmt.Errorf("dial: %w", os.ErrDeadlineExceeded),
			wantCategory: CategoryConnect,
			wantMessage:  "Tidak bisa terhubung ke smtp.gmail.com:587",
		},
		{
			name:         "tls handshake with unknown authority",
			stage:        stageTLS,
			err:          x509.UnknownAuthorityError{},
			wantCategory: CategoryTLS,
			wantMessage:  "TLS gagal saat terhubung ke smtp.gmail.com:587",
		},
		{
			name:         "tls error surfacing in a later stage",
			stage:        stageData,
			err:          fmt.Errorf("write: %w", x509.HostnameError{Host: "evil.example"}),
			wantCategory: CategoryTLS,
			wantMessage:  "TLS gagal saat terhubung ke smtp.gmail.com:587",
		},
		{
			name:         "recipient rejected",
			stage:        stageRcpt,
			err:          &textproto.Error{Code: 550, Msg: "5.1.1 The email account that you tried to reach does not exist"},
			wantCategory: CategoryRecipient,
			wantMessage:  "Alamat penerima ditolak (550)",
			wantCode:     550,
		},
		{
			name:         "message rejected",
			stage:        stageData,
			err:          &textproto.Error{Code: 552, Msg: "5.3.4 Message size exceeds fixed limit"},
			wantCategory: CategoryRejected,
			wantMessage:  "Server email menolak pengiriman (552)",
			wantCode:     552,
		},
		{
			name:         "io timeout",
			stage:        stageData,
			err:          fmt.Errorf("read: %w", os.ErrDeadlineExceeded),
			wantCategory: CategoryTimeout,
			wantMessage:  "Waktu habis saat mengirim melalui smtp.gmail.com:587",
		},
		{
			name:         "context deadline",
			stage:        stageMail,
			err:          context.DeadlineExceeded,
			wantCategory: CategoryTimeout,
			wantMessage:  "Waktu habis saat mengirim melalui smtp.gmail.com:587",
		},
		{
			name:         "invalid message",
			stage:        stageData,
			err:          fmt.Errorf("%w: to: address is empty", errInvalidMessage),
			wantCategory: CategoryMessage,
			wantMessage:  "Isi email tidak valid (alamat, subjek, atau lampiran)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classify(tc.stage, addr, tc.err)
			if got.Category != tc.wantCategory {
				t.Fatalf("category = %q, want %q", got.Category, tc.wantCategory)
			}
			if got.Message() != tc.wantMessage {
				t.Fatalf("message = %q, want %q", got.Message(), tc.wantMessage)
			}
			if got.Error() != tc.wantMessage {
				t.Fatalf("Error() = %q, want the fixed category message", got.Error())
			}
			if got.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d", got.Code, tc.wantCode)
			}
			if !errors.Is(got, tc.err) {
				t.Fatal("the cause must stay reachable through Unwrap for server-side logs")
			}
		})
	}
}

func TestClassifyKeepsExistingSendError(t *testing.T) {
	original := &SendError{Category: CategoryTLS, Addr: "smtp.gmail.com:587", Detail: "server tidak menawarkan STARTTLS"}
	wrapped := fmt.Errorf("send: %w", original)
	if got := classify(stageData, "other:1", wrapped); got != original {
		t.Fatalf("classify must return the existing *SendError, got %#v", got)
	}
}

func TestSendErrorNeverIncludesCause(t *testing.T) {
	secret := "abcdefghijklmnop"
	err := classify(stageAuth, "smtp.gmail.com:465", fmt.Errorf("AUTH PLAIN %s rejected", secret))
	if got := err.Error(); got != "Autentikasi ditolak" {
		t.Fatalf("Error() = %q, must be the fixed category text only", got)
	}
}
