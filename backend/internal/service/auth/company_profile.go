package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kana-consultant/kantor/backend/internal/docgen"
	"github.com/kana-consultant/kantor/backend/internal/dto"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
	"github.com/kana-consultant/kantor/backend/internal/uploads"
)

// MaxCompanyLogoBytes is the upload limit for the tenant logo.
const MaxCompanyLogoBytes = 2 << 20

const (
	companyLogoDir      = "branding"
	companyLogoFilename = "logo.png"
	companyLogoFileMode = 0o640
	companyLogoDirMode  = 0o750
)

var (
	ErrCompanyDocCodeInvalid = errors.New("kode dokumen hanya boleh berisi huruf, angka, dan tanda hubung (maksimal 20 karakter)")
	ErrCompanyLogoInvalid    = errors.New("logo harus berupa gambar PNG atau JPEG yang valid")
	ErrCompanyLogoTooLarge   = errors.New("ukuran logo maksimal 2 MB")
	ErrCompanyLogoNotFound   = errors.New("logo perusahaan belum diunggah")
	ErrCompanyTenantMissing  = errors.New("tenant tidak ditemukan pada konteks permintaan")
)

var companyDocCodePattern = regexp.MustCompile(`^[A-Z0-9](?:[A-Z0-9-]{0,19})$`)

type companyProfileRepository interface {
	GetCompanyProfileRecord(ctx context.Context) (authrepo.CompanyProfileRecord, error)
	UpdateCompanyProfile(ctx context.Context, updatedBy string, record authrepo.CompanyProfileRecord) error
	SetCompanyLogoUpdatedAt(ctx context.Context, updatedBy string, at *time.Time) error
}

// CompanyProfileService owns the tenant's 'company_profile' setting and the
// tenant logo used on generated documents. The logo is processed once on
// upload (docgen.ProcessLogo: trimmed, downscaled, re-encoded as PNG) and
// stored as UPLOADS_DIR/branding/<tenant_id>/logo.png with mode 0640.
type CompanyProfileService struct {
	repo       companyProfileRepository
	uploadsDir string
	now        func() time.Time
}

func NewCompanyProfileService(repo companyProfileRepository, uploadsDir string) *CompanyProfileService {
	return &CompanyProfileService{repo: repo, uploadsDir: uploadsDir, now: time.Now}
}

// CompanyProfileChange lists the fields a profile update changed, with their
// old and new values (for the audit diff).
type CompanyProfileChange struct {
	Fields []string
	Old    map[string]any
	New    map[string]any
}

// Profile returns the stored profile record, for document generation.
func (s *CompanyProfileService) Profile(ctx context.Context) (authrepo.CompanyProfileRecord, error) {
	return s.repo.GetCompanyProfileRecord(ctx)
}

// Get returns the profile plus the logo state.
func (s *CompanyProfileService) Get(ctx context.Context) (dto.CompanyProfileResponse, error) {
	record, err := s.repo.GetCompanyProfileRecord(ctx)
	if err != nil {
		return dto.CompanyProfileResponse{}, err
	}
	return s.toResponse(ctx, record)
}

// Update replaces the profile fields and reports what changed.
func (s *CompanyProfileService) Update(ctx context.Context, actorID string, input dto.UpdateCompanyProfileRequest) (dto.CompanyProfileResponse, CompanyProfileChange, error) {
	existing, err := s.repo.GetCompanyProfileRecord(ctx)
	if err != nil {
		return dto.CompanyProfileResponse{}, CompanyProfileChange{}, err
	}

	updated := authrepo.NormalizeCompanyProfileRecord(authrepo.CompanyProfileRecord{
		LegalName:       input.LegalName,
		Address:         input.Address,
		BusinessType:    input.BusinessType,
		City:            input.City,
		SignerName:      input.SignerName,
		SignerTitle:     input.SignerTitle,
		HRContactEmail:  input.HRContactEmail,
		DocCode:         input.DocCode,
		PaydayDay:       input.PaydayDay,
		AnnualLeaveDays: input.AnnualLeaveDays,
		LogoUpdatedAt:   existing.LogoUpdatedAt,
	})
	if updated.DocCode != "" && !companyDocCodePattern.MatchString(updated.DocCode) {
		return dto.CompanyProfileResponse{}, CompanyProfileChange{}, ErrCompanyDocCodeInvalid
	}

	change := diffCompanyProfile(existing, updated)
	if len(change.Fields) > 0 {
		if err := s.repo.UpdateCompanyProfile(ctx, actorID, updated); err != nil {
			return dto.CompanyProfileResponse{}, CompanyProfileChange{}, err
		}
	}

	record, err := s.repo.GetCompanyProfileRecord(ctx)
	if err != nil {
		return dto.CompanyProfileResponse{}, CompanyProfileChange{}, err
	}
	response, err := s.toResponse(ctx, record)
	if err != nil {
		return dto.CompanyProfileResponse{}, CompanyProfileChange{}, err
	}
	return response, change, nil
}

// UploadLogo validates, processes and stores the tenant logo. data is the
// raw upload (at most MaxCompanyLogoBytes). The content is sniffed: only PNG
// and JPEG are accepted whatever the file name or declared type says, and
// docgen.ProcessLogo checks the pixel dimensions before decoding.
func (s *CompanyProfileService) UploadLogo(ctx context.Context, actorID string, data []byte) (dto.CompanyProfileResponse, error) {
	if len(data) > MaxCompanyLogoBytes {
		return dto.CompanyProfileResponse{}, ErrCompanyLogoTooLarge
	}
	if len(data) == 0 {
		return dto.CompanyProfileResponse{}, ErrCompanyLogoInvalid
	}
	switch http.DetectContentType(data) {
	case "image/png", "image/jpeg":
	default:
		return dto.CompanyProfileResponse{}, ErrCompanyLogoInvalid
	}

	processed, err := docgen.ProcessLogo(data)
	if err != nil {
		if errors.Is(err, docgen.ErrInvalidLogo) {
			// docgen's message starts with the same sentence; keep only its
			// detail (e.g. the oversized dimensions).
			detail := strings.TrimPrefix(err.Error(), docgen.ErrInvalidLogo.Error())
			return dto.CompanyProfileResponse{}, fmt.Errorf("%w%s", ErrCompanyLogoInvalid, detail)
		}
		return dto.CompanyProfileResponse{}, err
	}

	path, err := s.logoPath(ctx)
	if err != nil {
		return dto.CompanyProfileResponse{}, err
	}
	if err := uploads.WriteFileAtomic(path, processed, companyLogoFileMode, companyLogoDirMode); err != nil {
		return dto.CompanyProfileResponse{}, fmt.Errorf("store company logo: %w", err)
	}

	now := s.now().UTC()
	if err := s.repo.SetCompanyLogoUpdatedAt(ctx, actorID, &now); err != nil {
		return dto.CompanyProfileResponse{}, err
	}
	return s.Get(ctx)
}

// DeleteLogo removes the tenant logo. removed is false when there was none.
func (s *CompanyProfileService) DeleteLogo(ctx context.Context, actorID string) (dto.CompanyProfileResponse, bool, error) {
	path, err := s.logoPath(ctx)
	if err != nil {
		return dto.CompanyProfileResponse{}, false, err
	}
	removed := true
	if err := os.Remove(path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return dto.CompanyProfileResponse{}, false, fmt.Errorf("remove company logo: %w", err)
		}
		removed = false
	}
	if err := s.repo.SetCompanyLogoUpdatedAt(ctx, actorID, nil); err != nil {
		return dto.CompanyProfileResponse{}, false, err
	}
	response, err := s.Get(ctx)
	return response, removed, err
}

// LogoPNG returns the processed tenant logo, or ErrCompanyLogoNotFound.
// Document rendering passes it to docgen.WithLogo.
func (s *CompanyProfileService) LogoPNG(ctx context.Context) ([]byte, error) {
	path, err := s.logoPath(ctx)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrCompanyLogoNotFound
		}
		return nil, fmt.Errorf("read company logo: %w", err)
	}
	return data, nil
}

func (s *CompanyProfileService) toResponse(ctx context.Context, record authrepo.CompanyProfileRecord) (dto.CompanyProfileResponse, error) {
	hasLogo, err := s.hasLogo(ctx)
	if err != nil {
		return dto.CompanyProfileResponse{}, err
	}
	response := dto.CompanyProfileResponse{
		LegalName:       record.LegalName,
		Address:         record.Address,
		BusinessType:    record.BusinessType,
		City:            record.City,
		SignerName:      record.SignerName,
		SignerTitle:     record.SignerTitle,
		HRContactEmail:  record.HRContactEmail,
		DocCode:         record.DocCode,
		PaydayDay:       record.PaydayDay,
		AnnualLeaveDays: record.AnnualLeaveDays,
		HasLogo:         hasLogo,
	}
	if hasLogo {
		response.LogoUpdatedAt = record.LogoUpdatedAt
	}
	return response, nil
}

func (s *CompanyProfileService) hasLogo(ctx context.Context) (bool, error) {
	path, err := s.logoPath(ctx)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat company logo: %w", err)
	}
	return info.Mode().IsRegular() && info.Size() > 0, nil
}

func (s *CompanyProfileService) logoPath(ctx context.Context) (string, error) {
	info, ok := tenant.FromContext(ctx)
	if !ok {
		return "", ErrCompanyTenantMissing
	}
	// The tenant id comes from the tenants table, but it becomes a path
	// segment: accept only a canonical UUID.
	parsed, err := uuid.Parse(info.ID)
	if err != nil {
		return "", ErrCompanyTenantMissing
	}
	return filepath.Join(s.uploadsDir, companyLogoDir, parsed.String(), companyLogoFilename), nil
}

func diffCompanyProfile(old, updated authrepo.CompanyProfileRecord) CompanyProfileChange {
	change := CompanyProfileChange{Old: map[string]any{}, New: map[string]any{}}
	add := func(field string, before, after any) {
		if before == after {
			return
		}
		change.Fields = append(change.Fields, field)
		change.Old[field] = before
		change.New[field] = after
	}
	add("legal_name", old.LegalName, updated.LegalName)
	add("address", old.Address, updated.Address)
	add("business_type", old.BusinessType, updated.BusinessType)
	add("city", old.City, updated.City)
	add("signer_name", old.SignerName, updated.SignerName)
	add("signer_title", old.SignerTitle, updated.SignerTitle)
	add("hr_contact_email", old.HRContactEmail, updated.HRContactEmail)
	add("doc_code", old.DocCode, updated.DocCode)
	add("payday_day", old.PaydayDay, updated.PaydayDay)
	add("annual_leave_days", old.AnnualLeaveDays, updated.AnnualLeaveDays)
	return change
}
