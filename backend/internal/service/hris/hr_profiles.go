package hris

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

// Access-log actions written (fail-closed) for identity reads.
const (
	IdentityAccessView     = "identity_view"
	IdentityAccessDocument = "identity_document"
	// IdentityAccessPreflight: the identity was decrypted to report which
	// fields a contract still misses (no value is returned).
	IdentityAccessPreflight = "identity_preflight"
)

var (
	ErrHRProfileJobTitleForbidden = errors.New("anda tidak memiliki izin mengubah jabatan karyawan")
	ErrHRProfileIdentityForbidden = errors.New("anda tidak memiliki izin mengubah data identitas karyawan")
	ErrNIKInvalid                 = errors.New("NIK harus 16 digit angka")
	ErrNIKMismatch                = errors.New("NIK tidak cocok dengan tanggal lahir dan jenis kelamin (digit 7-12: tanggal lahir, +40 untuk perempuan, bulan, dua digit tahun)")
	ErrNIKStoredMismatch          = errors.New("NIK yang tersimpan tidak cocok dengan tanggal lahir atau jenis kelamin yang baru; masukkan NIK baru")
	ErrNIKNeedsBirthData          = errors.New("tanggal lahir dan jenis kelamin wajib diisi bersama NIK")
	ErrNIKRequired                = errors.New("NIK wajib diisi: tanpa izin melihat data identitas, data identitas hanya dapat diganti seluruhnya termasuk NIK")
	ErrBirthDateInvalid           = errors.New("tanggal lahir tidak valid")
	ErrGenderInvalid              = errors.New("jenis kelamin harus male atau female")
	ErrPersonalEmailInvalid       = errors.New("email pribadi tidak valid")
)

type hrProfilesRepository interface {
	GetByEmployeeID(ctx context.Context, employeeID string) (model.EmployeeHRProfile, error)
	Upsert(ctx context.Context, params hrisrepo.UpsertHRProfileParams) (model.EmployeeHRProfile, error)
	EnsureEmployeeCode(ctx context.Context, employeeID string, formatCode func(number int) string) (int, string, error)
	LogIdentityAccess(ctx context.Context, userID string, employeeID string, action string) error
}

type hrProfilesEmployeesRepository interface {
	GetEmployeeByID(ctx context.Context, employeeID string) (model.Employee, error)
}

// HRProfileAccess is what the caller may see and change, resolved from its
// permissions by the handler.
type HRProfileAccess struct {
	ActorID         string
	CanViewIdentity bool // hris:employee_identity:view
	CanEditJobTitle bool // hris:employee:edit
	CanEditIdentity bool // hris:employee_identity:edit
}

// HRProfilesService manages employee_hr_profiles: job title, personal email,
// the lazily assigned employee code and the encrypted identity blob.
type HRProfilesService struct {
	repo      hrProfilesRepository
	employees hrProfilesEmployeesRepository
	encrypter *security.Encrypter
	now       func() time.Time
}

func NewHRProfilesService(repo hrProfilesRepository, employees hrProfilesEmployeesRepository, encrypter *security.Encrypter) *HRProfilesService {
	return &HRProfilesService{repo: repo, employees: employees, encrypter: encrypter, now: time.Now}
}

// Get returns the profile. The identity section is included only with
// CanViewIdentity, and only after the read was recorded (fail-closed).
func (s *HRProfilesService) Get(ctx context.Context, employeeID string, access HRProfileAccess) (hrisdto.HRProfileResponse, error) {
	profile, err := s.load(ctx, employeeID)
	if err != nil {
		return hrisdto.HRProfileResponse{}, err
	}
	if access.CanViewIdentity {
		if err := s.repo.LogIdentityAccess(ctx, access.ActorID, employeeID, IdentityAccessView); err != nil {
			return hrisdto.HRProfileResponse{}, fmt.Errorf("log identity access: %w", err)
		}
	}
	return s.toResponse(profile, access.CanViewIdentity)
}

// Update applies the present groups and returns the new view plus the names
// of the changed fields (for the audit entry; never values).
func (s *HRProfilesService) Update(ctx context.Context, employeeID string, input hrisdto.UpdateHRProfileRequest, access HRProfileAccess) (hrisdto.HRProfileResponse, []string, error) {
	current, err := s.load(ctx, employeeID)
	if err != nil {
		return hrisdto.HRProfileResponse{}, nil, err
	}

	params := hrisrepo.UpsertHRProfileParams{EmployeeID: employeeID, UpdatedBy: access.ActorID}
	var changed []string

	if input.JobTitle != nil {
		jobTitle := normalizeOptionalText(*input.JobTitle)
		if !equalOptionalText(current.JobTitle, jobTitle) {
			if !access.CanEditJobTitle {
				return hrisdto.HRProfileResponse{}, nil, ErrHRProfileJobTitleForbidden
			}
			params.SetJobTitle = true
			params.JobTitle = jobTitle
			changed = append(changed, "job_title")
		}
	}

	// Identity and personal e-mail are checked on presence, not on change:
	// comparing them for a caller who may not read them would be an oracle.
	if (input.PersonalEmail != nil || input.Identity != nil) && !access.CanEditIdentity {
		return hrisdto.HRProfileResponse{}, nil, ErrHRProfileIdentityForbidden
	}
	// The same holds for an editor who may not read them (identity:edit
	// without identity:view): nothing they get back may depend on the
	// stored values. Their identity save is a full replacement that must
	// carry a new NIK, validated only against the request (buildIdentity),
	// and a present group is always written, because skipping an unchanged
	// write would show in updated_at whether a guess matched.
	blindEdit := !access.CanViewIdentity

	if input.PersonalEmail != nil {
		email := normalizeOptionalText(strings.ToLower(*input.PersonalEmail))
		if email != nil {
			if err := validatePersonalEmail(*email); err != nil {
				return hrisdto.HRProfileResponse{}, nil, err
			}
		}
		if blindEdit || !equalOptionalText(current.PersonalEmail, email) {
			params.SetPersonalEmail = true
			params.PersonalEmail = email
			changed = append(changed, "personal_email")
		}
	}

	if input.Identity != nil {
		existing, _, err := s.decryptIdentity(current)
		if err != nil {
			return hrisdto.HRProfileResponse{}, nil, err
		}
		next, err := buildIdentity(existing, *input.Identity, s.now(), !blindEdit)
		if err != nil {
			return hrisdto.HRProfileResponse{}, nil, err
		}
		identityChanged := diffIdentity(existing, next)
		if blindEdit && len(identityChanged) == 0 {
			// Re-entered unchanged; still written (see above) and audited.
			identityChanged = []string{"identity"}
		}
		if len(identityChanged) > 0 {
			params.SetIdentity = true
			if !identityIsEmpty(next) {
				raw, err := json.Marshal(next)
				if err != nil {
					return hrisdto.HRProfileResponse{}, nil, err
				}
				cipher, err := s.encrypter.EncryptString(string(raw))
				if err != nil {
					return hrisdto.HRProfileResponse{}, nil, err
				}
				params.IdentityEncrypted = &cipher
			}
			changed = append(changed, identityChanged...)
		}
	}

	if access.CanViewIdentity {
		// The response shows the (masked) identity: record the read before
		// anything is written, so a failed log leaves nothing half-done.
		if err := s.repo.LogIdentityAccess(ctx, access.ActorID, employeeID, IdentityAccessView); err != nil {
			return hrisdto.HRProfileResponse{}, nil, fmt.Errorf("log identity access: %w", err)
		}
	}

	if len(changed) > 0 {
		updated, err := s.repo.Upsert(ctx, params)
		if err != nil {
			return hrisdto.HRProfileResponse{}, nil, mapEmployeeError(err)
		}
		current = updated
	}

	response, err := s.toResponse(current, access.CanViewIdentity)
	if err != nil {
		return hrisdto.HRProfileResponse{}, nil, err
	}
	return response, changed, nil
}

// IdentityForDocument returns the full decrypted identity for building a
// document snapshot (e.g. the PKWT, which prints the full NIK). The access is
// recorded first and an error aborts (fail-closed). ok is false when no
// identity has been filled in.
func (s *HRProfilesService) IdentityForDocument(ctx context.Context, employeeID string, actorID string) (model.EmployeeIdentity, bool, error) {
	return s.IdentityWithAccess(ctx, employeeID, actorID, IdentityAccessDocument)
}

// IdentityWithAccess decrypts the identity after recording the read under
// action (fail-closed), e.g. IdentityAccessPreflight for the contract
// completeness check.
func (s *HRProfilesService) IdentityWithAccess(ctx context.Context, employeeID string, actorID string, action string) (model.EmployeeIdentity, bool, error) {
	profile, err := s.load(ctx, employeeID)
	if err != nil {
		return model.EmployeeIdentity{}, false, err
	}
	if err := s.repo.LogIdentityAccess(ctx, actorID, employeeID, action); err != nil {
		return model.EmployeeIdentity{}, false, fmt.Errorf("log identity access: %w", err)
	}
	return s.decryptIdentity(profile)
}

// LogIdentityAccess records a read of identity data that does not go
// through this service (e.g. a contract PDF, which prints the full NIK).
// Callers treat an error as fatal (fail-closed).
func (s *HRProfilesService) LogIdentityAccess(ctx context.Context, actorID string, employeeID string, action string) error {
	return s.repo.LogIdentityAccess(ctx, actorID, employeeID, action)
}

// Profile returns the raw profile row (identity still encrypted), or an empty
// profile when none exists yet.
func (s *HRProfilesService) Profile(ctx context.Context, employeeID string) (model.EmployeeHRProfile, error) {
	return s.load(ctx, employeeID)
}

// EnsureEmployeeCode returns the employee's number and code, assigning the
// next number on first use ("<doc_code>-0001", or "0001" without a document
// code). Called at the first payslip or contract generation.
func (s *HRProfilesService) EnsureEmployeeCode(ctx context.Context, employeeID string, docCode string) (int, string, error) {
	if _, err := s.load(ctx, employeeID); err != nil {
		return 0, "", err
	}
	number, code, err := s.repo.EnsureEmployeeCode(ctx, employeeID, func(n int) string {
		return FormatEmployeeCode(docCode, n)
	})
	if err != nil {
		return 0, "", mapEmployeeError(err)
	}
	return number, code, nil
}

// FormatEmployeeCode renders an employee code: "<DOC_CODE>-0021" or "0021".
func FormatEmployeeCode(docCode string, number int) string {
	docCode = strings.ToUpper(strings.TrimSpace(docCode))
	if docCode == "" {
		return fmt.Sprintf("%04d", number)
	}
	return fmt.Sprintf("%s-%04d", docCode, number)
}

func (s *HRProfilesService) load(ctx context.Context, employeeID string) (model.EmployeeHRProfile, error) {
	if _, err := uuid.Parse(employeeID); err != nil {
		return model.EmployeeHRProfile{}, ErrEmployeeNotFound
	}
	if _, err := s.employees.GetEmployeeByID(ctx, employeeID); err != nil {
		return model.EmployeeHRProfile{}, mapEmployeeError(err)
	}
	profile, err := s.repo.GetByEmployeeID(ctx, employeeID)
	if err != nil {
		if errors.Is(err, hrisrepo.ErrHRProfileNotFound) {
			return model.EmployeeHRProfile{EmployeeID: employeeID}, nil
		}
		return model.EmployeeHRProfile{}, err
	}
	return profile, nil
}

func (s *HRProfilesService) decryptIdentity(profile model.EmployeeHRProfile) (model.EmployeeIdentity, bool, error) {
	if profile.IdentityEncrypted == nil || strings.TrimSpace(*profile.IdentityEncrypted) == "" {
		return model.EmployeeIdentity{}, false, nil
	}
	plain, err := s.encrypter.DecryptString(*profile.IdentityEncrypted)
	if err != nil {
		return model.EmployeeIdentity{}, false, fmt.Errorf("decrypt employee identity: %w", err)
	}
	var identity model.EmployeeIdentity
	if err := json.Unmarshal([]byte(plain), &identity); err != nil {
		return model.EmployeeIdentity{}, false, fmt.Errorf("decode employee identity: %w", err)
	}
	return identity, true, nil
}

func (s *HRProfilesService) toResponse(profile model.EmployeeHRProfile, identityVisible bool) (hrisdto.HRProfileResponse, error) {
	response := hrisdto.HRProfileResponse{
		EmployeeID:      profile.EmployeeID,
		EmployeeNumber:  profile.EmployeeNumber,
		EmployeeCode:    profile.EmployeeCode,
		JobTitle:        profile.JobTitle,
		IdentityVisible: identityVisible,
	}
	if !profile.UpdatedAt.IsZero() {
		updatedAt := profile.UpdatedAt
		response.UpdatedAt = &updatedAt
	}
	if !identityVisible {
		return response, nil
	}

	email := ""
	if profile.PersonalEmail != nil {
		email = *profile.PersonalEmail
	}
	response.PersonalEmail = &email

	identity, _, err := s.decryptIdentity(profile)
	if err != nil {
		return hrisdto.HRProfileResponse{}, err
	}
	response.Identity = &hrisdto.HRIdentityResponse{
		HasNIK:          identity.NIK != "",
		NIKMasked:       MaskNIK(identity.NIK),
		BirthPlace:      identity.BirthPlace,
		BirthDate:       identity.BirthDate,
		Gender:          identity.Gender,
		BankAccountName: identity.BankAccountName,
		KTPAddress:      identity.KTPAddress,
		UpdatedAt:       profile.IdentityUpdatedAt,
	}
	return response, nil
}

// buildIdentity merges an identity edit into the stored identity. Every field
// is replaced except the NIK, which is kept when the request leaves it empty
// and keepStoredNIK is set (the caller may read the identity). Without
// keepStoredNIK a NIK is required, so no error can depend on the stored
// identity. A NIK (new or kept) must match the birth date and gender.
func buildIdentity(existing model.EmployeeIdentity, input hrisdto.UpdateHRIdentityRequest, now time.Time, keepStoredNIK bool) (model.EmployeeIdentity, error) {
	next := model.EmployeeIdentity{
		BirthPlace:      singleLine(input.BirthPlace),
		BirthDate:       strings.TrimSpace(input.BirthDate),
		Gender:          strings.ToLower(strings.TrimSpace(input.Gender)),
		BankAccountName: singleLine(input.BankAccountName),
		KTPAddress:      strings.TrimSpace(input.KTPAddress),
	}

	var birthDate time.Time
	if next.BirthDate != "" {
		parsed, err := time.Parse("2006-01-02", next.BirthDate)
		if err != nil || parsed.Year() < 1900 || parsed.After(now) {
			return model.EmployeeIdentity{}, ErrBirthDateInvalid
		}
		birthDate = parsed
	}
	if next.Gender != "" && next.Gender != model.GenderMale && next.Gender != model.GenderFemale {
		return model.EmployeeIdentity{}, ErrGenderInvalid
	}

	nik := strings.Join(strings.Fields(input.NIK), "")
	keptNIK := false
	if nik == "" {
		if !keepStoredNIK {
			return model.EmployeeIdentity{}, ErrNIKRequired
		}
		nik = existing.NIK
		keptNIK = nik != ""
	}
	next.NIK = nik
	if nik == "" {
		return next, nil
	}

	if !isNIKFormat(nik) {
		return model.EmployeeIdentity{}, ErrNIKInvalid
	}
	if next.BirthDate == "" || next.Gender == "" {
		return model.EmployeeIdentity{}, ErrNIKNeedsBirthData
	}
	if err := ValidateNIK(nik, birthDate, next.Gender); err != nil {
		if keptNIK && errors.Is(err, ErrNIKMismatch) {
			return model.EmployeeIdentity{}, ErrNIKStoredMismatch
		}
		return model.EmployeeIdentity{}, err
	}
	return next, nil
}

// ValidateNIK checks a 16-digit NIK against the holder's birth date and
// gender. Digits 7-8 are the birth day (plus 40 for women), 9-10 the month and
// 11-12 the last two digits of the year; the serial (13-16) cannot be 0000.
func ValidateNIK(nik string, birthDate time.Time, gender string) error {
	if !isNIKFormat(nik) {
		return ErrNIKInvalid
	}
	day := birthDate.Day()
	switch gender {
	case model.GenderFemale:
		day += 40
	case model.GenderMale:
	default:
		return ErrNIKMismatch
	}
	want := fmt.Sprintf("%02d%02d%02d", day, int(birthDate.Month()), birthDate.Year()%100)
	if nik[6:12] != want {
		return ErrNIKMismatch
	}
	return nil
}

// MaskNIK keeps the six region digits and hides the rest:
// "3273015402980001" -> "327301**********". Anything that is not a
// well-formed NIK is hidden entirely.
func MaskNIK(nik string) string {
	if nik == "" {
		return ""
	}
	if !isNIKFormat(nik) {
		return strings.Repeat("*", 16)
	}
	return nik[:6] + strings.Repeat("*", len(nik)-6)
}

func isNIKFormat(nik string) bool {
	if len(nik) != 16 {
		return false
	}
	for _, c := range nik {
		if c < '0' || c > '9' {
			return false
		}
	}
	return nik[:6] != "000000" && nik[12:] != "0000"
}

func diffIdentity(old, next model.EmployeeIdentity) []string {
	var fields []string
	add := func(name string, a, b string) {
		if a != b {
			fields = append(fields, name)
		}
	}
	add("nik", old.NIK, next.NIK)
	add("birth_place", old.BirthPlace, next.BirthPlace)
	add("birth_date", old.BirthDate, next.BirthDate)
	add("gender", old.Gender, next.Gender)
	add("bank_account_name", old.BankAccountName, next.BankAccountName)
	add("ktp_address", old.KTPAddress, next.KTPAddress)
	return fields
}

func identityIsEmpty(identity model.EmployeeIdentity) bool {
	return identity == model.EmployeeIdentity{}
}

func validatePersonalEmail(email string) error {
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Name != "" || parsed.Address != email || !strings.Contains(email, "@") {
		return ErrPersonalEmailInvalid
	}
	local, domain, _ := strings.Cut(email, "@")
	if local == "" || !strings.Contains(domain, ".") || strings.ContainsAny(email, " \t\r\n\"<>") {
		return ErrPersonalEmailInvalid
	}
	return nil
}

func normalizeOptionalText(value string) *string {
	trimmed := singleLine(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func equalOptionalText(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// singleLine trims and collapses any run of whitespace (including newlines)
// into one space.
func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
