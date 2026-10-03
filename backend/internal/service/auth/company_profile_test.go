package auth

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/dto"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

const companyTestTenantID = "00000000-0000-0000-0000-000000000001"

type fakeCompanyProfileRepo struct {
	record  authrepo.CompanyProfileRecord
	updates int
}

func (r *fakeCompanyProfileRepo) GetCompanyProfileRecord(context.Context) (authrepo.CompanyProfileRecord, error) {
	return r.record, nil
}

func (r *fakeCompanyProfileRepo) UpdateCompanyProfile(_ context.Context, _ string, record authrepo.CompanyProfileRecord) error {
	r.updates++
	logo := r.record.LogoUpdatedAt
	r.record = authrepo.NormalizeCompanyProfileRecord(record)
	r.record.LogoUpdatedAt = logo
	return nil
}

func (r *fakeCompanyProfileRepo) SetCompanyLogoUpdatedAt(_ context.Context, _ string, at *time.Time) error {
	r.record.LogoUpdatedAt = at
	return nil
}

func newCompanyProfileTestService(t *testing.T) (*CompanyProfileService, *fakeCompanyProfileRepo, string, context.Context) {
	t.Helper()
	repo := &fakeCompanyProfileRepo{record: authrepo.DefaultCompanyProfileRecord()}
	uploads := t.TempDir()
	service := NewCompanyProfileService(repo, uploads)
	service.now = func() time.Time { return time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC) }
	ctx := tenant.WithInfo(context.Background(), tenant.Info{ID: companyTestTenantID, Slug: "default"})
	return service, repo, uploads, ctx
}

// logoPNG draws a dark 200x40 bar centred on a transparent 400x200 canvas.
func logoPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 400, 200))
	for y := 80; y < 120; y++ {
		for x := 100; x < 300; x++ {
			img.Set(x, y, color.NRGBA{R: 20, G: 40, B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// hugePNGHeader is a PNG whose IHDR claims 20000x20000 pixels. It must be
// rejected from the header alone, before any pixel is decoded.
func hugePNGHeader() []byte {
	var buf bytes.Buffer
	buf.Write([]byte("\x89PNG\r\n\x1a\n"))
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], 20000)
	binary.BigEndian.PutUint32(ihdr[4:], 20000)
	ihdr[8], ihdr[9] = 8, 6 // 8-bit RGBA
	chunk := append([]byte("IHDR"), ihdr...)
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(ihdr)))
	buf.Write(chunk)
	_ = binary.Write(&buf, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return buf.Bytes()
}

func TestCompanyProfileDefaultsAndUpdateDiff(t *testing.T) {
	service, repo, _, ctx := newCompanyProfileTestService(t)

	profile, err := service.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if profile.PaydayDay != 25 || profile.AnnualLeaveDays != 12 || profile.HasLogo || profile.LogoUpdatedAt != nil || profile.DocCode != "" {
		t.Fatalf("defaults = %+v", profile)
	}

	input := dto.UpdateCompanyProfileRequest{
		LegalName: " PT Contoh Teknologi Nusantara ", City: "Jakarta", SignerName: "Rudi Hartono",
		SignerTitle: "Direktur", HRContactEmail: " HR@Contoh.co.id ", DocCode: " ctn ",
		PaydayDay: 25, AnnualLeaveDays: 14,
	}
	updated, change, err := service.Update(ctx, "actor", input)
	if err != nil {
		t.Fatal(err)
	}
	if updated.LegalName != "PT Contoh Teknologi Nusantara" || updated.HRContactEmail != "hr@contoh.co.id" || updated.DocCode != "CTN" {
		t.Fatalf("normalised profile = %+v", updated)
	}
	wantFields := []string{"legal_name", "city", "signer_name", "signer_title", "hr_contact_email", "doc_code", "annual_leave_days"}
	if !slices.Equal(change.Fields, wantFields) {
		t.Fatalf("changed = %v, want %v", change.Fields, wantFields)
	}
	if change.Old["annual_leave_days"] != 12 || change.New["annual_leave_days"] != 14 {
		t.Fatalf("diff = %+v -> %+v", change.Old, change.New)
	}
	if _, ok := change.New["payday_day"]; ok {
		t.Fatal("unchanged fields must not be in the diff")
	}

	updates := repo.updates
	if _, change, err := service.Update(ctx, "actor", input); err != nil || len(change.Fields) != 0 || repo.updates != updates {
		t.Fatalf("no-op update wrote: %v %v", change.Fields, err)
	}

	input.DocCode = "P/10"
	if _, _, err := service.Update(ctx, "actor", input); !errors.Is(err, ErrCompanyDocCodeInvalid) {
		t.Fatalf("invalid doc code: %v", err)
	}
}

func TestCompanyLogoUploadGetDelete(t *testing.T) {
	service, repo, uploads, ctx := newCompanyProfileTestService(t)

	if _, err := service.LogoPNG(ctx); !errors.Is(err, ErrCompanyLogoNotFound) {
		t.Fatalf("LogoPNG before upload: %v", err)
	}

	profile, err := service.UploadLogo(ctx, "actor", logoPNG(t))
	if err != nil {
		t.Fatalf("UploadLogo: %v", err)
	}
	if !profile.HasLogo || profile.LogoUpdatedAt == nil || repo.record.LogoUpdatedAt == nil {
		t.Fatalf("profile after upload = %+v", profile)
	}

	path := filepath.Join(uploads, "branding", companyTestTenantID, "logo.png")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o640 {
		t.Fatalf("logo mode = %o, want 640", mode)
	}
	stored, err := service.LogoPNG(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(stored))
	if err != nil {
		t.Fatalf("stored logo is not a PNG: %v", err)
	}
	if cfg.Width != 200 || cfg.Height != 40 {
		t.Fatalf("stored logo = %dx%d, want the trimmed 200x40", cfg.Width, cfg.Height)
	}

	profile, removed, err := service.DeleteLogo(ctx, "actor")
	if err != nil || !removed || profile.HasLogo || profile.LogoUpdatedAt != nil {
		t.Fatalf("DeleteLogo = %+v, %v, %v", profile, removed, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("logo file still exists: %v", err)
	}
	if _, removed, err := service.DeleteLogo(ctx, "actor"); err != nil || removed {
		t.Fatalf("second DeleteLogo = %v, %v", removed, err)
	}
}

func TestCompanyLogoRejectsBadUploads(t *testing.T) {
	service, _, uploads, ctx := newCompanyProfileTestService(t)

	gif := []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;")
	cases := map[string]struct {
		data []byte
		want error
	}{
		"empty":           {nil, ErrCompanyLogoInvalid},
		"gif":             {gif, ErrCompanyLogoInvalid},
		"svg":             {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), ErrCompanyLogoInvalid},
		"html":            {[]byte("<html><script>alert(1)</script></html>"), ErrCompanyLogoInvalid},
		"truncated png":   {logoPNG(t)[:40], ErrCompanyLogoInvalid},
		"huge dimensions": {hugePNGHeader(), ErrCompanyLogoInvalid},
		"over 2 MB":       {append(logoPNG(t), make([]byte, MaxCompanyLogoBytes)...), ErrCompanyLogoTooLarge},
		"fully transparent": {func() []byte {
			var buf bytes.Buffer
			_ = png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 10, 10)))
			return buf.Bytes()
		}(), ErrCompanyLogoInvalid},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := service.UploadLogo(ctx, "actor", tc.data); !errors.Is(err, tc.want) {
				t.Fatalf("UploadLogo err = %v, want %v", err, tc.want)
			}
		})
	}
	if _, err := service.UploadLogo(ctx, "actor", hugePNGHeader()); err == nil || !strings.Contains(err.Error(), "20000x20000 px terlalu besar") {
		t.Fatalf("huge dimensions must be rejected from the header: %v", err)
	}
	if _, err := os.Stat(filepath.Join(uploads, "branding")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a rejected upload must not create files")
	}

	if _, err := service.UploadLogo(context.Background(), "actor", logoPNG(t)); !errors.Is(err, ErrCompanyTenantMissing) {
		t.Fatalf("upload without tenant: %v", err)
	}
	badTenant := tenant.WithInfo(context.Background(), tenant.Info{ID: "../../etc"})
	if _, err := service.UploadLogo(badTenant, "actor", logoPNG(t)); !errors.Is(err, ErrCompanyTenantMissing) {
		t.Fatalf("upload with a non-UUID tenant: %v", err)
	}
}
