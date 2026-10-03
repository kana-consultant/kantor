package hris

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/model"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/security"
)

const (
	hrTestEmployeeID = "22222222-2222-2222-2222-222222222222"
	hrTestActorID    = "33333333-3333-3333-3333-333333333333"
	hrTestNIK        = "3273015402980001" // female, born 14-02-1998
)

type fakeHRProfilesRepo struct {
	profile    *model.EmployeeHRProfile
	logs       []string
	logErr     error
	upserts    int
	nextNumber int
}

func (r *fakeHRProfilesRepo) GetByEmployeeID(_ context.Context, employeeID string) (model.EmployeeHRProfile, error) {
	if r.profile == nil {
		return model.EmployeeHRProfile{}, hrisrepo.ErrHRProfileNotFound
	}
	return *r.profile, nil
}

func (r *fakeHRProfilesRepo) Upsert(_ context.Context, params hrisrepo.UpsertHRProfileParams) (model.EmployeeHRProfile, error) {
	r.upserts++
	profile := model.EmployeeHRProfile{EmployeeID: params.EmployeeID}
	if r.profile != nil {
		profile = *r.profile
	}
	if params.SetJobTitle {
		profile.JobTitle = params.JobTitle
	}
	if params.SetPersonalEmail {
		profile.PersonalEmail = params.PersonalEmail
	}
	if params.SetIdentity {
		profile.IdentityEncrypted = params.IdentityEncrypted
		now := time.Now()
		profile.IdentityUpdatedAt = &now
	}
	profile.UpdatedAt = time.Now()
	r.profile = &profile
	return profile, nil
}

func (r *fakeHRProfilesRepo) EnsureEmployeeCode(_ context.Context, employeeID string, formatCode func(int) string) (int, string, error) {
	if r.profile != nil && r.profile.EmployeeNumber != nil {
		return *r.profile.EmployeeNumber, *r.profile.EmployeeCode, nil
	}
	r.nextNumber++
	number, code := r.nextNumber, formatCode(r.nextNumber)
	profile := model.EmployeeHRProfile{EmployeeID: employeeID}
	if r.profile != nil {
		profile = *r.profile
	}
	profile.EmployeeNumber, profile.EmployeeCode = &number, &code
	r.profile = &profile
	return number, code, nil
}

func (r *fakeHRProfilesRepo) LogIdentityAccess(_ context.Context, userID string, employeeID string, action string) error {
	if r.logErr != nil {
		return r.logErr
	}
	r.logs = append(r.logs, action+":"+employeeID+":"+userID)
	return nil
}

type fakeHREmployees struct{ missing bool }

func (e fakeHREmployees) GetEmployeeByID(_ context.Context, employeeID string) (model.Employee, error) {
	if e.missing {
		return model.Employee{}, hrisrepo.ErrEmployeeNotFound
	}
	return model.Employee{ID: employeeID}, nil
}

func newHRProfilesTestService(t *testing.T) (*HRProfilesService, *fakeHRProfilesRepo, *security.Encrypter) {
	t.Helper()
	encrypter, err := security.NewEncrypter("hr-profile-test-secret-0123456789abc")
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeHRProfilesRepo{}
	service := NewHRProfilesService(repo, fakeHREmployees{}, encrypter)
	service.now = func() time.Time { return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC) }
	return service, repo, encrypter
}

func hrAdmin() HRProfileAccess {
	return HRProfileAccess{ActorID: hrTestActorID, CanViewIdentity: true, CanEditJobTitle: true, CanEditIdentity: true}
}

func strptr(s string) *string { return &s }

func femaleIdentity(nik string) *hrisdto.UpdateHRIdentityRequest {
	return &hrisdto.UpdateHRIdentityRequest{
		NIK:             nik,
		BirthPlace:      "Bandung",
		BirthDate:       "1998-02-14",
		Gender:          "female",
		BankAccountName: "Rina Kartika Sari",
		KTPAddress:      "Jl. Contoh No. 1\nBandung",
	}
}

func TestValidateNIK(t *testing.T) {
	born := time.Date(1998, 2, 14, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		nik    string
		date   time.Time
		gender string
		want   error
	}{
		{"female +40", hrTestNIK, born, model.GenderFemale, nil},
		{"male", "3273011402980001", born, model.GenderMale, nil},
		{"female NIK for a man", hrTestNIK, born, model.GenderMale, ErrNIKMismatch},
		{"wrong month", "3273015403980001", born, model.GenderFemale, ErrNIKMismatch},
		{"wrong year", "3273015402990001", born, model.GenderFemale, ErrNIKMismatch},
		{"wrong day", "3273015502980001", born, model.GenderFemale, ErrNIKMismatch},
		{"15 digits", "327301540298000", born, model.GenderFemale, ErrNIKInvalid},
		{"letters", "32730154029800O1", born, model.GenderFemale, ErrNIKInvalid},
		{"zero serial", "3273015402980000", born, model.GenderFemale, ErrNIKInvalid},
		{"zero region", "0000005402980001", born, model.GenderFemale, ErrNIKInvalid},
		{"2000s year", "3273010101050001", time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC), model.GenderMale, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateNIK(tc.nik, tc.date, tc.gender); !errors.Is(err, tc.want) {
				t.Fatalf("ValidateNIK = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestMaskNIK(t *testing.T) {
	if got := MaskNIK(hrTestNIK); got != "327301**********" {
		t.Fatalf("MaskNIK = %q", got)
	}
	if got := MaskNIK(""); got != "" {
		t.Fatalf("MaskNIK(empty) = %q", got)
	}
	if got := MaskNIK("12345"); got != strings.Repeat("*", 16) {
		t.Fatalf("MaskNIK(malformed) = %q", got)
	}
}

func TestHRProfileIdentityRoundTripIsMaskedAndLogged(t *testing.T) {
	service, repo, encrypter := newHRProfilesTestService(t)

	resp, changed, err := service.Update(context.Background(), hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{
		JobTitle:      strptr("  AI Engineer "),
		PersonalEmail: strptr(" Rina@Example.com "),
		Identity:      femaleIdentity(" 3273 0154 0298 0001 "),
	}, hrAdmin())
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	wantChanged := []string{"job_title", "personal_email", "nik", "birth_place", "birth_date", "gender", "bank_account_name", "ktp_address"}
	if !slices.Equal(changed, wantChanged) {
		t.Fatalf("changed = %v, want %v", changed, wantChanged)
	}
	if resp.Identity == nil || resp.Identity.NIKMasked != "327301**********" || !resp.Identity.HasNIK {
		t.Fatalf("identity = %+v", resp.Identity)
	}
	if resp.JobTitle == nil || *resp.JobTitle != "AI Engineer" || resp.PersonalEmail == nil || *resp.PersonalEmail != "rina@example.com" {
		t.Fatalf("response = %+v", resp)
	}
	if resp.Identity.KTPAddress != "Jl. Contoh No. 1\nBandung" || resp.Identity.BirthDate != "1998-02-14" || resp.Identity.Gender != "female" {
		t.Fatalf("identity fields = %+v", resp.Identity)
	}

	encoded, _ := json.Marshal(resp)
	if strings.Contains(string(encoded), hrTestNIK) || strings.Contains(string(encoded), "5402980001") {
		t.Fatalf("full NIK leaked in response: %s", encoded)
	}
	stored := *repo.profile.IdentityEncrypted
	if strings.Contains(stored, hrTestNIK) || strings.Contains(stored, "Bandung") {
		t.Fatal("identity stored in plaintext")
	}
	plain, err := encrypter.DecryptString(stored)
	if err != nil || !strings.Contains(plain, hrTestNIK) {
		t.Fatalf("stored identity does not decrypt to the NIK: %v", err)
	}

	logsBefore := len(repo.logs)
	got, err := service.Get(context.Background(), hrTestEmployeeID, hrAdmin())
	if err != nil {
		t.Fatal(err)
	}
	if got.Identity == nil || got.Identity.NIKMasked != "327301**********" {
		t.Fatalf("Get identity = %+v", got.Identity)
	}
	if len(repo.logs) != logsBefore+1 || !strings.HasPrefix(repo.logs[len(repo.logs)-1], IdentityAccessView+":"+hrTestEmployeeID) {
		t.Fatalf("identity read not logged: %v", repo.logs)
	}
}

func TestHRProfileWithoutIdentityPermission(t *testing.T) {
	service, repo, _ := newHRProfilesTestService(t)
	if _, _, err := service.Update(context.Background(), hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{
		JobTitle: strptr("Engineer"), PersonalEmail: strptr("a@b.co"), Identity: femaleIdentity(hrTestNIK),
	}, hrAdmin()); err != nil {
		t.Fatal(err)
	}
	repo.logs = nil

	viewer := HRProfileAccess{ActorID: hrTestActorID}
	resp, err := service.Get(context.Background(), hrTestEmployeeID, viewer)
	if err != nil {
		t.Fatal(err)
	}
	if resp.IdentityVisible || resp.Identity != nil || resp.PersonalEmail != nil {
		t.Fatalf("identity exposed without permission: %+v", resp)
	}
	if resp.JobTitle == nil || *resp.JobTitle != "Engineer" {
		t.Fatalf("job title missing: %+v", resp)
	}
	encoded, _ := json.Marshal(resp)
	if strings.Contains(string(encoded), "identity\":{") || strings.Contains(string(encoded), "personal_email") || strings.Contains(string(encoded), "327301") {
		t.Fatalf("identity section serialised: %s", encoded)
	}
	if len(repo.logs) != 0 {
		t.Fatalf("no identity read, no access log expected: %v", repo.logs)
	}

	editor := HRProfileAccess{ActorID: hrTestActorID, CanEditJobTitle: true}
	if _, _, err := service.Update(context.Background(), hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{Identity: femaleIdentity("")}, editor); !errors.Is(err, ErrHRProfileIdentityForbidden) {
		t.Fatalf("identity edit without permission: %v", err)
	}
	if _, _, err := service.Update(context.Background(), hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{PersonalEmail: strptr("")}, editor); !errors.Is(err, ErrHRProfileIdentityForbidden) {
		t.Fatalf("personal email edit without permission: %v", err)
	}

	identityOnly := HRProfileAccess{ActorID: hrTestActorID, CanEditIdentity: true}
	if _, _, err := service.Update(context.Background(), hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{JobTitle: strptr("CTO")}, identityOnly); !errors.Is(err, ErrHRProfileJobTitleForbidden) {
		t.Fatalf("job title change without employee:edit: %v", err)
	}
	// Re-sending the unchanged job title together with identity is fine.
	if _, _, err := service.Update(context.Background(), hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{JobTitle: strptr("Engineer"), Identity: femaleIdentity(hrTestNIK)}, identityOnly); err != nil {
		t.Fatalf("unchanged job title rejected: %v", err)
	}
}

// TestHRProfileEditWithoutViewIsNotAnOracle: an editor without
// identity:view must not learn the stored birth date or gender (NIK digits
// 7-12) or any other stored identity value from the answers to its saves.
func TestHRProfileEditWithoutViewIsNotAnOracle(t *testing.T) {
	service, repo, encrypter := newHRProfilesTestService(t)
	ctx := context.Background()
	if _, _, err := service.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{
		PersonalEmail: strptr("rina@example.com"), Identity: femaleIdentity(hrTestNIK),
	}, hrAdmin()); err != nil {
		t.Fatal(err)
	}
	blind := HRProfileAccess{ActorID: hrTestActorID, CanEditIdentity: true}

	// Probing with an empty NIK (keep the stored one) gives the same answer
	// whatever birth data is guessed, and writes nothing.
	upserts := repo.upserts
	for _, guess := range []struct{ date, gender string }{
		{"1998-02-15", "female"}, {"1998-02-14", "male"}, {"1998-02-14", "female"},
	} {
		probe := femaleIdentity("")
		probe.BirthDate, probe.Gender = guess.date, guess.gender
		_, _, err := service.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{Identity: probe}, blind)
		if !errors.Is(err, ErrNIKRequired) {
			t.Fatalf("probe %v: %v, want ErrNIKRequired", guess, err)
		}
	}
	// The same answer when no identity is stored at all.
	other, otherRepo, _ := newHRProfilesTestService(t)
	if _, _, err := other.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{Identity: femaleIdentity("")}, blind); !errors.Is(err, ErrNIKRequired) {
		t.Fatalf("empty profile: %v, want ErrNIKRequired", err)
	}
	if repo.upserts != upserts || otherRepo.upserts != 0 {
		t.Fatal("rejected probes must not write")
	}

	// Errors with a NIK depend on the request only.
	mismatch := femaleIdentity("3273011402980001")
	if _, _, err := service.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{Identity: mismatch}, blind); !errors.Is(err, ErrNIKMismatch) || errors.Is(err, ErrNIKStoredMismatch) {
		t.Fatalf("mismatching NIK: %v", err)
	}

	// Re-entering exactly the stored identity and e-mail is still written,
	// so updated_at cannot tell a correct guess from a wrong one.
	resp, changed, err := service.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{
		PersonalEmail: strptr("rina@example.com"), Identity: femaleIdentity(hrTestNIK),
	}, blind)
	if err != nil {
		t.Fatal(err)
	}
	if repo.upserts != upserts+1 {
		t.Fatalf("unchanged blind save not written (upserts %d -> %d)", upserts, repo.upserts)
	}
	if !slices.Equal(changed, []string{"personal_email", "identity"}) {
		t.Fatalf("changed = %v", changed)
	}
	if resp.IdentityVisible || resp.Identity != nil || resp.PersonalEmail != nil {
		t.Fatalf("identity exposed to a blind editor: %+v", resp)
	}

	// A full replacement with a new NIK works.
	replacement := femaleIdentity("3273015502980002")
	replacement.BirthDate = "1998-02-15"
	if _, _, err := service.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{Identity: replacement}, blind); err != nil {
		t.Fatalf("full replacement: %v", err)
	}
	plain, _ := encrypter.DecryptString(*repo.profile.IdentityEncrypted)
	if !strings.Contains(plain, "3273015502980002") {
		t.Fatal("replacement NIK not stored")
	}
}

func TestHRProfileNIKIsReplaceOnly(t *testing.T) {
	service, repo, encrypter := newHRProfilesTestService(t)
	ctx := context.Background()
	if _, _, err := service.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{Identity: femaleIdentity(hrTestNIK)}, hrAdmin()); err != nil {
		t.Fatal(err)
	}

	// Empty NIK keeps the stored one while other fields change.
	edit := femaleIdentity("")
	edit.BirthPlace = "Cimahi"
	_, changed, err := service.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{Identity: edit}, hrAdmin())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changed, []string{"birth_place"}) {
		t.Fatalf("changed = %v", changed)
	}
	plain, _ := encrypter.DecryptString(*repo.profile.IdentityEncrypted)
	if !strings.Contains(plain, hrTestNIK) || !strings.Contains(plain, "Cimahi") {
		t.Fatal("stored NIK was not kept")
	}

	// Changing the birth date without a new NIK is refused.
	edit = femaleIdentity("")
	edit.BirthDate = "1998-02-15"
	if _, _, err := service.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{Identity: edit}, hrAdmin()); !errors.Is(err, ErrNIKStoredMismatch) {
		t.Fatalf("stored NIK vs new birth date: %v", err)
	}
	// ... but accepted with a matching replacement NIK.
	if _, _, err := service.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{Identity: func() *hrisdto.UpdateHRIdentityRequest {
		e := femaleIdentity("3273015502980002")
		e.BirthDate = "1998-02-15"
		return e
	}()}, hrAdmin()); err != nil {
		t.Fatalf("replacement NIK rejected: %v", err)
	}

	// A new NIK whose digits 7-12 do not match is refused and nothing is written.
	upserts := repo.upserts
	if _, _, err := service.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{Identity: femaleIdentity("3273011402980001")}, hrAdmin()); !errors.Is(err, ErrNIKMismatch) {
		t.Fatalf("mismatching NIK: %v", err)
	}
	if repo.upserts != upserts {
		t.Fatal("a rejected identity must not be written")
	}

	// NIK without birth data.
	noBirth := &hrisdto.UpdateHRIdentityRequest{NIK: hrTestNIK}
	if _, _, err := service.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{Identity: noBirth}, hrAdmin()); !errors.Is(err, ErrNIKNeedsBirthData) {
		t.Fatalf("NIK without birth data: %v", err)
	}
	future := femaleIdentity("")
	future.BirthDate = "2030-01-01"
	if _, _, err := service.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{Identity: future}, hrAdmin()); !errors.Is(err, ErrBirthDateInvalid) {
		t.Fatalf("future birth date: %v", err)
	}
}

func TestHRProfileIdentityLogIsFailClosed(t *testing.T) {
	service, repo, _ := newHRProfilesTestService(t)
	ctx := context.Background()
	if _, _, err := service.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{Identity: femaleIdentity(hrTestNIK)}, hrAdmin()); err != nil {
		t.Fatal(err)
	}

	repo.logErr = errors.New("audit insert failed")
	if resp, err := service.Get(ctx, hrTestEmployeeID, hrAdmin()); err == nil || resp.Identity != nil {
		t.Fatalf("Get must fail without an access log, got %+v, %v", resp, err)
	}
	upserts := repo.upserts
	edit := femaleIdentity("")
	edit.BirthPlace = "Cimahi"
	if _, _, err := service.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{Identity: edit}, hrAdmin()); err == nil {
		t.Fatal("Update returning identity must fail without an access log")
	}
	if repo.upserts != upserts {
		t.Fatal("nothing may be written when the access log fails")
	}
	if _, _, err := service.IdentityForDocument(ctx, hrTestEmployeeID, hrTestActorID); err == nil {
		t.Fatal("IdentityForDocument must fail without an access log")
	}

	// A caller without identity:view reads nothing sensitive and needs no log.
	if _, err := service.Get(ctx, hrTestEmployeeID, HRProfileAccess{ActorID: hrTestActorID}); err != nil {
		t.Fatalf("Get without identity: %v", err)
	}
}

func TestHRProfileMisc(t *testing.T) {
	service, repo, _ := newHRProfilesTestService(t)
	ctx := context.Background()

	if _, err := service.Get(ctx, "not-a-uuid", hrAdmin()); !errors.Is(err, ErrEmployeeNotFound) {
		t.Fatalf("invalid id: %v", err)
	}
	missing := NewHRProfilesService(repo, fakeHREmployees{missing: true}, service.encrypter)
	if _, err := missing.Get(ctx, hrTestEmployeeID, hrAdmin()); !errors.Is(err, ErrEmployeeNotFound) {
		t.Fatalf("missing employee: %v", err)
	}

	empty, err := service.Get(ctx, hrTestEmployeeID, hrAdmin())
	if err != nil {
		t.Fatal(err)
	}
	if empty.EmployeeCode != nil || empty.Identity == nil || empty.Identity.HasNIK || empty.PersonalEmail == nil || *empty.PersonalEmail != "" {
		t.Fatalf("empty profile = %+v", empty)
	}

	if _, _, err := service.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{PersonalEmail: strptr("not an email")}, hrAdmin()); !errors.Is(err, ErrPersonalEmailInvalid) {
		t.Fatalf("bad email: %v", err)
	}
	_, changed, err := service.Update(ctx, hrTestEmployeeID, hrisdto.UpdateHRProfileRequest{JobTitle: strptr("   ")}, hrAdmin())
	if err != nil || len(changed) != 0 {
		t.Fatalf("blank job title on an empty profile: %v, %v", changed, err)
	}

	number, code, err := service.EnsureEmployeeCode(ctx, hrTestEmployeeID, "ctn")
	if err != nil || number != 1 || code != "CTN-0001" {
		t.Fatalf("EnsureEmployeeCode = %d %q %v", number, code, err)
	}
	if _, again, _ := service.EnsureEmployeeCode(ctx, hrTestEmployeeID, "XYZ"); again != "CTN-0001" {
		t.Fatalf("code must be assigned once, got %q", again)
	}
	if got := FormatEmployeeCode("", 21); got != "0021" {
		t.Fatalf("FormatEmployeeCode = %q", got)
	}
}
