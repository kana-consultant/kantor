// Package uploads centralises file upload validation. Every uploaded file
// must satisfy both an extension allowlist and a content-type sniff so that
// polyglot files (valid magic bytes paired with an executable extension)
// cannot bypass our checks.
package uploads

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
)

// Kind identifies which upload category a file belongs to. Each kind has its
// own size cap, allowed extensions and allowed content-type prefixes.
type Kind string

const (
	KindAvatar               Kind = "avatar"
	KindReimbursement        Kind = "reimbursement"
	KindCampaignAttachment   Kind = "campaign"
)

// ErrValidation is the sentinel error returned for any user-facing validation
// failure (size, extension, mismatched content type). Callers should wrap it
// with a descriptive message and surface a 400 to the client.
var ErrValidation = errors.New("upload validation failed")

// ValidationResult is what the validator returns when a file passes. The
// caller can use the sniffed content type for storage metadata (e.g. campaign
// attachment file_type column).
type ValidationResult struct {
	ContentType string
	Extension   string
}

type kindRules struct {
	maxSize             int64
	allowedExtensions   map[string]struct{}
	allowedContentTypes []string
	allowContentType    func(string) bool
}

func (r kindRules) extensionAllowed(ext string) bool {
	_, ok := r.allowedExtensions[strings.ToLower(ext)]
	return ok
}

func (r kindRules) contentTypeAllowed(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if r.allowContentType != nil && r.allowContentType(ct) {
		return true
	}
	for _, allowed := range r.allowedContentTypes {
		if ct == allowed || strings.HasPrefix(ct, allowed+";") {
			return true
		}
	}
	return false
}

func setOf(values ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		out[strings.ToLower(v)] = struct{}{}
	}
	return out
}

var rulesByKind = map[Kind]kindRules{
	KindAvatar: {
		maxSize:           5 << 20,
		allowedExtensions: setOf(".jpg", ".jpeg", ".png", ".gif", ".webp"),
		allowContentType: func(ct string) bool {
			return strings.HasPrefix(ct, "image/")
		},
	},
	KindReimbursement: {
		maxSize:           10 << 20,
		allowedExtensions: setOf(".jpg", ".jpeg", ".png", ".gif", ".webp", ".pdf"),
		allowedContentTypes: []string{
			"application/pdf",
		},
		allowContentType: func(ct string) bool {
			return strings.HasPrefix(ct, "image/") || ct == "application/pdf"
		},
	},
	KindCampaignAttachment: {
		maxSize: 25 << 20,
		allowedExtensions: setOf(
			".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg",
			".mp4", ".mov", ".webm", ".avi",
			".pdf", ".txt",
			".zip",
			".doc", ".docx",
			".ppt", ".pptx",
			".xls", ".xlsx",
		),
		allowedContentTypes: []string{
			"application/pdf",
			"text/plain",
			"application/zip",
			"application/x-zip-compressed",
			"application/msword",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.ms-powerpoint",
			"application/vnd.openxmlformats-officedocument.presentationml.presentation",
			"application/vnd.ms-excel",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		},
		allowContentType: func(ct string) bool {
			return strings.HasPrefix(ct, "image/") || strings.HasPrefix(ct, "video/")
		},
	},
}

// MaxSize reports the size cap (bytes) for the given upload kind.
func MaxSize(kind Kind) int64 {
	if rules, ok := rulesByKind[kind]; ok {
		return rules.maxSize
	}
	return 0
}

// ValidateMultipartFile checks the multipart upload's size, file extension
// and sniffed content type against the rules for the given kind. The file's
// read offset is reset to 0 before returning so the caller can stream it to
// disk afterwards.
func ValidateMultipartFile(kind Kind, file *multipart.FileHeader) (ValidationResult, error) {
	rules, ok := rulesByKind[kind]
	if !ok {
		return ValidationResult{}, fmt.Errorf("%w: unknown upload kind %q", ErrValidation, kind)
	}

	if file == nil {
		return ValidationResult{}, fmt.Errorf("%w: missing file", ErrValidation)
	}

	if file.Size <= 0 {
		return ValidationResult{}, fmt.Errorf("%w: file is empty", ErrValidation)
	}

	if file.Size > rules.maxSize {
		return ValidationResult{}, fmt.Errorf("%w: file must be smaller than %d bytes", ErrValidation, rules.maxSize)
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext == "" {
		return ValidationResult{}, fmt.Errorf("%w: filename is missing an extension", ErrValidation)
	}
	if !rules.extensionAllowed(ext) {
		return ValidationResult{}, fmt.Errorf("%w: file extension %s is not allowed", ErrValidation, ext)
	}

	src, err := file.Open()
	if err != nil {
		return ValidationResult{}, fmt.Errorf("%w: %w", ErrValidation, err)
	}
	defer src.Close()

	sniff := make([]byte, 512)
	n, readErr := src.Read(sniff)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return ValidationResult{}, fmt.Errorf("%w: %w", ErrValidation, readErr)
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return ValidationResult{}, fmt.Errorf("%w: %w", ErrValidation, err)
	}

	contentType := http.DetectContentType(sniff[:n])
	if !rules.contentTypeAllowed(contentType) {
		return ValidationResult{}, fmt.Errorf("%w: detected content type %q is not allowed", ErrValidation, contentType)
	}

	return ValidationResult{ContentType: contentType, Extension: ext}, nil
}
