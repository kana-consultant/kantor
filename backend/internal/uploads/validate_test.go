package uploads

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"
)

// pngMagic is enough bytes to make http.DetectContentType return "image/png".
var pngMagic = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

func writeMultipartFile(t *testing.T, filename string, body []byte) *multipart.FileHeader {
	t.Helper()
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)

	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create part: %v", err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	reader := multipart.NewReader(buf, writer.Boundary())
	form, err := reader.ReadForm(int64(len(body)) + 1024)
	if err != nil {
		t.Fatalf("read form: %v", err)
	}
	files := form.File["file"]
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	return files[0]
}

func TestValidateMultipartFile_AvatarHappyPath(t *testing.T) {
	fh := writeMultipartFile(t, "selfie.png", append(pngMagic, bytes.Repeat([]byte{0x00}, 256)...))
	res, err := ValidateMultipartFile(KindAvatar, fh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Extension != ".png" {
		t.Fatalf("expected .png, got %q", res.Extension)
	}
	if !strings.HasPrefix(res.ContentType, "image/png") {
		t.Fatalf("expected image/png content type, got %q", res.ContentType)
	}
}

func TestValidateMultipartFile_RejectsExeExtension(t *testing.T) {
	// Polyglot: image magic bytes paired with an executable extension. The
	// extension allowlist must reject this even though DetectContentType
	// would happily return image/png.
	fh := writeMultipartFile(t, "innocent.exe", append(pngMagic, bytes.Repeat([]byte{0x00}, 256)...))
	if _, err := ValidateMultipartFile(KindAvatar, fh); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestValidateMultipartFile_RejectsMismatchedContentType(t *testing.T) {
	// Plain text body but .png extension — content-type sniff catches it.
	fh := writeMultipartFile(t, "fake.png", []byte("just plain text content"))
	if _, err := ValidateMultipartFile(KindAvatar, fh); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestValidateMultipartFile_RejectsMissingExtension(t *testing.T) {
	fh := writeMultipartFile(t, "noext", append(pngMagic, bytes.Repeat([]byte{0x00}, 256)...))
	if _, err := ValidateMultipartFile(KindAvatar, fh); !errors.Is(err, ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestValidateMultipartFile_ReimbursementAcceptsPDF(t *testing.T) {
	pdf := []byte("%PDF-1.4\n")
	pdf = append(pdf, bytes.Repeat([]byte{0x20}, 200)...)
	fh := writeMultipartFile(t, "receipt.pdf", pdf)
	res, err := ValidateMultipartFile(KindReimbursement, fh)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ContentType != "application/pdf" {
		t.Fatalf("expected application/pdf, got %q", res.ContentType)
	}
}

func TestValidateMultipartFile_LeavesReadOffsetAtZero(t *testing.T) {
	fh := writeMultipartFile(t, "selfie.png", append(pngMagic, bytes.Repeat([]byte{0x00}, 256)...))
	if _, err := ValidateMultipartFile(KindAvatar, fh); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	src, err := fh.Open()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer src.Close()
	got, err := io.ReadAll(src)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.HasPrefix(got, pngMagic) {
		t.Fatalf("read offset not reset; first bytes: %x", got[:8])
	}
}
