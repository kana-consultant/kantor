package security

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
)

// Binary encryption for stored files (generated PDFs).
//
// Format: "v<version>:" followed by the raw AES-256-GCM output
// (nonce || ciphertext || tag). It shares the key versions of
// EncryptString/DecryptString but skips base64, so an encrypted file is only
// ~31 bytes larger than the plaintext.

// EncryptBytes encrypts plaintext with the primary key.
func (e *Encrypter) EncryptBytes(plaintext []byte) ([]byte, error) {
	sealed, err := encrypt(e.primaryKey, plaintext)
	if err != nil {
		return nil, err
	}
	header := "v" + strconv.Itoa(e.primaryVersion) + ":"
	out := make([]byte, 0, len(header)+len(sealed))
	out = append(out, header...)
	return append(out, sealed...), nil
}

// DecryptBytes decrypts data produced by EncryptBytes with the key version
// named in its header.
func (e *Encrypter) DecryptBytes(data []byte) ([]byte, error) {
	if len(data) < 3 || data[0] != 'v' {
		return nil, errors.New("decrypt bytes: missing version header")
	}
	colon := bytes.IndexByte(data[:min(len(data), 12)], ':')
	if colon < 2 {
		return nil, errors.New("decrypt bytes: missing version header")
	}
	version, err := strconv.Atoi(string(data[1:colon]))
	if err != nil || version <= 0 {
		return nil, errors.New("decrypt bytes: invalid version header")
	}
	entry, ok := e.keys[version]
	if !ok {
		return nil, fmt.Errorf("unknown encryption key version: %d", version)
	}
	plaintext, err := decrypt(entry.primary, data[colon+1:])
	if err != nil {
		return nil, fmt.Errorf("decrypt bytes (v%d): %w", version, err)
	}
	return plaintext, nil
}
