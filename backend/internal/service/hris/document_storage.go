package hris

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/kana-consultant/kantor/backend/internal/security"
	"github.com/kana-consultant/kantor/backend/internal/uploads"
)

// Encrypted storage for generated PDFs. Only the PDF is stored (the DOCX is
// re-rendered on demand from the encrypted snapshot), encrypted with
// security.Encrypter.EncryptBytes, under
//
//	UPLOADS_DIR/documents/<tenant_id>/<kind>/<id>-<part>-<nonce>.pdf.enc
//
// with mode 0640 (directories 0750). The random nonce gives every render its
// own file, so a newer render never overwrites the file a download may be
// reading, and the superseded file can be deleted once the row points at the
// new one.
const (
	documentsRootDir     = "documents"
	documentFileMode     = 0o640
	documentDirMode      = 0o750
	documentFileSuffix   = ".pdf.enc"
	documentNonceBytes   = 6
	maxStoredDocumentLen = 64 << 20
)

var (
	ErrDocumentPathInvalid  = errors.New("invalid stored document path")
	ErrDocumentNotFound     = errors.New("stored document not found")
	ErrDocumentHashMismatch = errors.New("stored document does not match its recorded sha256")
)

var documentNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

// StoredDocument is one encrypted PDF written by DocumentStore.Write. Path is
// relative to UPLOADS_DIR (slash separated) and is what the document row
// records; SHA256 is the hex digest of the plaintext PDF.
type StoredDocument struct {
	Part   string
	Path   string
	SHA256 string
	Size   int
	// Pages is the page count of the PDF (0: unknown), set by the worker.
	Pages int
}

type DocumentStore struct {
	uploadsDir string
	encrypter  *security.Encrypter
	random     io.Reader
}

func NewDocumentStore(uploadsDir string, encrypter *security.Encrypter) *DocumentStore {
	return &DocumentStore{uploadsDir: uploadsDir, encrypter: encrypter, random: rand.Reader}
}

// Write encrypts pdf and stores it for (tenantID, kind, id, part).
func (s *DocumentStore) Write(tenantID string, kind string, id string, part string, pdf []byte) (StoredDocument, error) {
	tenantUUID, err := uuid.Parse(tenantID)
	if err != nil {
		return StoredDocument{}, fmt.Errorf("%w: tenant id", ErrDocumentPathInvalid)
	}
	docUUID, err := uuid.Parse(id)
	if err != nil {
		return StoredDocument{}, fmt.Errorf("%w: document id", ErrDocumentPathInvalid)
	}
	if !documentNamePattern.MatchString(kind) || !documentNamePattern.MatchString(part) {
		return StoredDocument{}, fmt.Errorf("%w: kind or part", ErrDocumentPathInvalid)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-")) {
		return StoredDocument{}, errors.New("refusing to store a document that is not a PDF")
	}

	nonce := make([]byte, documentNonceBytes)
	if _, err := io.ReadFull(s.random, nonce); err != nil {
		return StoredDocument{}, fmt.Errorf("document nonce: %w", err)
	}
	rel := path.Join(documentsRootDir, tenantUUID.String(), kind,
		fmt.Sprintf("%s-%s-%s%s", docUUID.String(), part, hex.EncodeToString(nonce), documentFileSuffix))

	cipher, err := s.encrypter.EncryptBytes(pdf)
	if err != nil {
		return StoredDocument{}, fmt.Errorf("encrypt document: %w", err)
	}
	abs, err := s.resolve(tenantUUID.String(), rel)
	if err != nil {
		return StoredDocument{}, err
	}
	if err := uploads.WriteFileAtomic(abs, cipher, documentFileMode, documentDirMode); err != nil {
		return StoredDocument{}, fmt.Errorf("write document: %w", err)
	}

	sum := sha256.Sum256(pdf)
	return StoredDocument{Part: part, Path: rel, SHA256: hex.EncodeToString(sum[:]), Size: len(pdf)}, nil
}

// Read decrypts a stored PDF of the given tenant. When wantSHA256 is
// non-empty the plaintext must match it.
func (s *DocumentStore) Read(tenantID string, rel string, wantSHA256 string) ([]byte, error) {
	abs, err := s.resolve(tenantID, rel)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrDocumentNotFound
		}
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxStoredDocumentLen {
		return nil, ErrDocumentPathInvalid
	}
	cipher, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	pdf, err := s.encrypter.DecryptBytes(cipher)
	if err != nil {
		return nil, fmt.Errorf("decrypt document: %w", err)
	}
	if wantSHA256 != "" {
		sum := sha256.Sum256(pdf)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), strings.TrimSpace(wantSHA256)) {
			return nil, ErrDocumentHashMismatch
		}
	}
	return pdf, nil
}

// Delete removes a stored PDF of the given tenant. A file that is already
// gone is not an error.
func (s *DocumentStore) Delete(tenantID string, rel string) error {
	abs, err := s.resolve(tenantID, rel)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// resolve maps a stored relative path to an absolute one, accepting only
// paths of the documents/<tenant>/<kind>/<file>.pdf.enc shape that belong to
// tenantID.
func (s *DocumentStore) resolve(tenantID string, rel string) (string, error) {
	if rel == "" || strings.Contains(rel, "\\") || strings.HasPrefix(rel, "/") {
		return "", ErrDocumentPathInvalid
	}
	cleaned := path.Clean(rel)
	if cleaned != rel {
		return "", ErrDocumentPathInvalid
	}
	segments := strings.Split(cleaned, "/")
	if len(segments) != 4 || segments[0] != documentsRootDir {
		return "", ErrDocumentPathInvalid
	}
	pathTenant, err := uuid.Parse(segments[1])
	if err != nil || segments[1] != pathTenant.String() {
		return "", ErrDocumentPathInvalid
	}
	wantTenant, err := uuid.Parse(tenantID)
	if err != nil || wantTenant != pathTenant {
		return "", ErrDocumentPathInvalid
	}
	if !documentNamePattern.MatchString(segments[2]) {
		return "", ErrDocumentPathInvalid
	}
	name := segments[3]
	if !strings.HasSuffix(name, documentFileSuffix) || strings.HasPrefix(name, ".") || name == ".." {
		return "", ErrDocumentPathInvalid
	}
	return filepath.Join(s.uploadsDir, filepath.FromSlash(cleaned)), nil
}
