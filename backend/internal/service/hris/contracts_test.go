package hris

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kana-consultant/kantor/backend/internal/docgen"
	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/model"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
	"github.com/kana-consultant/kantor/backend/internal/security"
	"github.com/kana-consultant/kantor/backend/internal/tenant"
)

// ---------------------------------------------------------------------------
// Pure helpers

func day(year int, month time.Month, d int) time.Time {
	return time.Date(year, month, d, 0, 0, 0, 0, time.UTC)
}

func TestFormatContractNumbers(t *testing.T) {
	pkwt, nda := FormatContractNumbers(21, "ctn", day(2026, 10, 1))
	if pkwt != "021/PKWT/CTN/X/2026" || nda != "021/NDA-HKI/CTN/X/2026" {
		t.Errorf("numbers = %q %q", pkwt, nda)
	}
	pkwt, nda = FormatContractNumbers(1, "", day(2027, 2, 14))
	if pkwt != "001/PKWT/II/2027" || nda != "001/NDA-HKI/II/2027" {
		t.Errorf("numbers without code = %q %q", pkwt, nda)
	}
	if key := contractPeriodKey(day(2026, 10, 2)); key != "2026-10" {
		t.Errorf("period key = %q", key)
	}
}

func TestContractSpanDurationAndRenewal(t *testing.T) {
	months, days := contractSpan(day(2026, 10, 1), day(2027, 9, 30))
	if months != 12 || days != 0 {
		t.Errorf("span = %d months %d days", months, days)
	}
	if text := durationMonthsText(day(2026, 10, 1), day(2027, 9, 30)); text != "12" {
		t.Errorf("durasi_bulan = %q", text)
	}
	months, days = contractSpan(day(2026, 10, 1), day(2027, 4, 15))
	if months != 6 || days != 15 {
		t.Errorf("partial span = %d months %d days", months, days)
	}
	start, end := renewalDates(day(2026, 10, 1), day(2027, 9, 30))
	if !start.Equal(day(2027, 10, 1)) || !end.Equal(day(2028, 9, 30)) {
		t.Errorf("renewal = %s - %s", start, end)
	}
	start, end = renewalDates(day(2026, 10, 1), day(2026, 10, 20))
	if !start.Equal(day(2026, 10, 21)) || !end.Equal(day(2026, 11, 9)) {
		t.Errorf("day-based renewal = %s - %s", start, end)
	}
}

// A synthetic renewal chain: 24 + 24 + 18 months of PKWT = 66 > 60 warns;
// cancelled links and other types do not count; an early end counts until
// ended_at.
func TestChainPKWTMonthsFiveYearCheck(t *testing.T) {
	link := func(contractType string, status string, start time.Time, end time.Time) hrisrepo.ContractChainLink {
		return hrisrepo.ContractChainLink{ContractType: contractType, Status: status, StartDate: start, EndDate: &end}
	}
	chain := []hrisrepo.ContractChainLink{
		link(model.ContractTypePKWT, model.ContractStatusDraft, day(2028, 10, 1), day(2030, 3, 31)),  // 18
		link(model.ContractTypePKWT, model.ContractStatusSigned, day(2026, 10, 1), day(2028, 9, 30)), // 24
		link(model.ContractTypePKWT, model.ContractStatusEnded, day(2024, 10, 1), day(2026, 9, 30)),  // 24
		link(model.ContractTypePKWT, model.ContractStatusCancelled, day(2020, 1, 1), day(2024, 9, 30)),
		link(model.ContractTypeMagang, model.ContractStatusEnded, day(2024, 1, 1), day(2024, 9, 30)),
	}
	if months := chainPKWTMonths(chain); months != 66 {
		t.Fatalf("chain months = %v, want 66", months)
	}
	contract := model.EmploymentContract{ContractType: model.ContractTypePKWT, StartDate: day(2028, 10, 1), EndDate: timePtr(day(2030, 3, 31))}
	if !hasWarning(contractWarnings(contract, "active", chainPKWTMonths(chain)), "pkwt_chain_over_5_years") {
		t.Error("66 months must warn")
	}

	// Exactly 60 months does not warn.
	if hasWarning(contractWarnings(contract, "active", chainPKWTMonths(chain[1:3])+12), "pkwt_chain_over_5_years") {
		t.Error("60 months must not warn")
	}

	// Ended early: counted until ended_at (12 of 24 months).
	early := link(model.ContractTypePKWT, model.ContractStatusEnded, day(2024, 10, 1), day(2026, 9, 30))
	early.EndedAt = timePtr(day(2025, 9, 30))
	if months := chainPKWTMonths([]hrisrepo.ContractChainLink{early}); months != 12 {
		t.Errorf("early end months = %v", months)
	}
}

func hasWarning(warnings []hrisdto.ContractWarning, code string) bool {
	for _, item := range warnings {
		if item.Code == code {
			return true
		}
	}
	return false
}

func TestContractWarningsProbationAndPartialMonths(t *testing.T) {
	contract := model.EmploymentContract{ContractType: model.ContractTypePKWT, StartDate: day(2026, 10, 1), EndDate: timePtr(day(2027, 9, 30))}
	warnings := contractWarnings(contract, "probation", 12)
	if !hasWarning(warnings, "employee_probation") || warnings[0].Action != "set_employee_active" {
		t.Errorf("probation warning = %+v", warnings)
	}
	if hasWarning(contractWarnings(contract, "active", 12), "employee_probation") {
		t.Error("active employee must not warn")
	}
	contract.EndDate = timePtr(day(2027, 4, 15))
	if !hasWarning(contractWarnings(contract, "active", 6), "duration_not_whole_months") {
		t.Error("partial months must warn")
	}
}

// end 30 Sep 2027, notice 30 days: deadline 31 Aug 2027, badge from 17 Aug
// (44 days before the end), expired from 1 Oct.
func TestContractNoticeDeadlineAndExpired(t *testing.T) {
	contract := model.EmploymentContract{Status: model.ContractStatusSigned, NoticeDays: 30, EndDate: timePtr(day(2027, 9, 30))}
	deadline, alert, expired := contractNotice(contract, day(2027, 8, 16))
	if deadline == nil || !deadline.Equal(day(2027, 8, 31)) || alert || expired {
		t.Errorf("16 Aug: %v %v %v", deadline, alert, expired)
	}
	if _, alert, _ := contractNotice(contract, day(2027, 8, 17)); !alert {
		t.Error("17 Aug must show the notice badge")
	}
	if _, alert, expired := contractNotice(contract, day(2027, 10, 1)); alert || !expired {
		t.Errorf("1 Oct: alert %v expired %v", alert, expired)
	}
	contract.Status = model.ContractStatusEnded
	if _, alert, expired := contractNotice(contract, day(2027, 10, 1)); alert || expired {
		t.Error("an ended contract gets no badge")
	}
	if deadline, _, _ := contractNotice(model.EmploymentContract{}, day(2027, 1, 1)); deadline != nil {
		t.Error("no end date, no deadline")
	}
}

func TestNormalizeContractCc(t *testing.T) {
	cc, err := normalizeContractCc([]string{" Legal@Contoh.co.id ", "legal@contoh.co.id", "budi@contoh.co.id", ""}, "contoh.co.id", "budi@contoh.co.id")
	if err != nil || len(cc) != 1 || cc[0] != "legal@contoh.co.id" {
		t.Errorf("cc = %v, %v", cc, err)
	}
	if _, err := normalizeContractCc([]string{"x@gmail.com"}, "contoh.co.id", ""); !errors.Is(err, ErrContractCcDomain) {
		t.Errorf("other domain err = %v", err)
	}
	if _, err := normalizeContractCc([]string{"x@sub.contoh.co.id"}, "contoh.co.id", ""); !errors.Is(err, ErrContractCcDomain) {
		t.Errorf("subdomain err = %v", err)
	}
	if _, err := normalizeContractCc([]string{"x@contoh.co.id"}, "", ""); !errors.Is(err, ErrContractCcDomain) {
		t.Errorf("no HR domain err = %v", err)
	}
	if _, err := normalizeContractCc([]string{"not an address"}, "contoh.co.id", ""); !errors.Is(err, ErrContractCcInvalid) {
		t.Errorf("invalid err = %v", err)
	}
	if domain := contractCcDomain(" HR@Contoh.co.id "); domain != "contoh.co.id" {
		t.Errorf("domain = %q", domain)
	}
}

func TestNormalizeContractTerms(t *testing.T) {
	end := "2027-09-30"
	terms, err := normalizeContractTerms(hrisdto.ContractFields{ContractType: "PKWT", StartDate: "2026-10-01", EndDate: &end, JobTitle: " Backend\nEngineer "})
	if err != nil {
		t.Fatal(err)
	}
	if terms.JobTitle != "Backend Engineer" || terms.WeeklyHours != 40 || terms.NoticeDays != 30 || terms.WorkMode != "wfo" ||
		terms.IncidentReportHours != 24 || terms.NonSolicitMonths != 12 || terms.ConfidentialityYears != 3 || terms.WorkDays == "" || terms.WorkHours == "" {
		t.Errorf("defaults = %+v", terms)
	}
	if _, err := normalizeContractTerms(hrisdto.ContractFields{ContractType: "PKWT", StartDate: "2026-10-01", JobTitle: "X"}); !errors.Is(err, ErrContractEndDateRequired) {
		t.Errorf("PKWT without end err = %v", err)
	}
	before := "2026-09-01"
	if _, err := normalizeContractTerms(hrisdto.ContractFields{ContractType: "PKWT", StartDate: "2026-10-01", EndDate: &before, JobTitle: "X"}); !errors.Is(err, ErrContractDatesInvalid) {
		t.Errorf("end before start err = %v", err)
	}
	terms, err = normalizeContractTerms(hrisdto.ContractFields{ContractType: "PKWTT", StartDate: "2020-01-06", JobTitle: "Lead"})
	if err != nil || !terms.IsRecordOnly || terms.EndDate != nil {
		t.Errorf("PKWTT = %+v, %v", terms, err)
	}
	terms, err = normalizeContractTerms(hrisdto.ContractFields{ContractType: "MAGANG", StartDate: "2026-07-01", JobTitle: "Intern", IsRecordOnly: false})
	if err != nil || !terms.IsRecordOnly {
		t.Errorf("Magang must be record-only: %+v, %v", terms, err)
	}
}

func TestContractEditTargetAndStatusSources(t *testing.T) {
	cases := []struct {
		contract model.EmploymentContract
		status   string
		revision int
		err      error
	}{
		{model.EmploymentContract{Status: model.ContractStatusDraft}, model.ContractStatusDraft, 0, nil},
		{model.EmploymentContract{Status: model.ContractStatusGenerated, Revision: 1}, model.ContractStatusDraft, 1, nil},
		{model.EmploymentContract{Status: model.ContractStatusSent}, model.ContractStatusDraft, 1, nil},
		{model.EmploymentContract{Status: model.ContractStatusSent, SignedAt: timePtr(day(2026, 10, 5))}, "", 0, ErrContractNotEditable},
		{model.EmploymentContract{Status: model.ContractStatusSigned, SignedAt: timePtr(day(2026, 10, 5))}, "", 0, ErrContractNotEditable},
		{model.EmploymentContract{Status: model.ContractStatusCancelled}, "", 0, ErrContractNotEditable},
	}
	for _, tc := range cases {
		status, revision, err := contractEditTarget(tc.contract)
		if status != tc.status || revision != tc.revision || !errors.Is(err, tc.err) {
			t.Errorf("%s: %q %d %v", tc.contract.Status, status, revision, err)
		}
	}

	documents := model.EmploymentContract{ContractType: model.ContractTypePKWT}
	if got := contractStatusSources(documents, model.ContractStatusSigned); strings.Join(got, ",") != "generated,sent" {
		t.Errorf("signed from = %v", got)
	}
	if got := contractStatusSources(documents, model.ContractStatusEnded); strings.Join(got, ",") != "signed" {
		t.Errorf("ended from = %v", got)
	}
	record := model.EmploymentContract{ContractType: model.ContractTypePKWTT, IsRecordOnly: true}
	if got := contractStatusSources(record, model.ContractStatusSigned); strings.Join(got, ",") != "draft" {
		t.Errorf("record-only signed from = %v", got)
	}
	if got := contractStatusSources(record, model.ContractStatusEnded); strings.Join(got, ",") != "draft,signed" {
		t.Errorf("record-only ended from = %v", got)
	}
}

func TestContractSubjectAndFilename(t *testing.T) {
	contract := model.EmploymentContract{DocNumber: strPtr("001/PKWT/CTN/X/2026"), NDADocNumber: strPtr("001/NDA-HKI/CTN/X/2026")}
	if subject := contractSubject(contract); subject != "Kontrak Kerja (PKWT) & NDA/HKI — 001/PKWT/CTN/X/2026" {
		t.Errorf("subject = %q", subject)
	}
	contract.Revision = 1
	if subject := contractSubject(contract); !strings.HasSuffix(subject, "(Revisi 1)") {
		t.Errorf("revised subject = %q", subject)
	}
	if name := contractFilename(contract, ContractPartNDA, "Budi Santoso"); name != "NDA-HKI_001-NDA-HKI-CTN-X-2026_Budi_Santoso_Revisi1" {
		t.Errorf("filename = %q", name)
	}
	if name := contractFilename(contract, ContractPartPKWT, "Budi Santoso"); name != "PKWT_001-PKWT-CTN-X-2026_Budi_Santoso_Revisi1" {
		t.Errorf("filename = %q", name)
	}
}

// ---------------------------------------------------------------------------
// Payloads against the real templates

func contractTestCompany() authrepo.CompanyProfileRecord {
	return authrepo.CompanyProfileRecord{
		LegalName:       "PT Contoh Teknologi Nusantara",
		Address:         "Jl. Contoh Raya No. 10, Jakarta Selatan",
		BusinessType:    "pengembangan perangkat lunak",
		City:            "Jakarta",
		SignerName:      "Rudi Hartono",
		SignerTitle:     "Direktur",
		HRContactEmail:  "hr@contoh.co.id",
		DocCode:         "CTN",
		PaydayDay:       25,
		AnnualLeaveDays: 12,
	}
}

func contractTestIdentity() model.EmployeeIdentity {
	return model.EmployeeIdentity{
		NIK:             "3273011503950001",
		BirthPlace:      "Bandung",
		BirthDate:       "1995-03-15",
		Gender:          model.GenderMale,
		BankAccountName: "Budi Santoso",
		KTPAddress:      "Jl. Melati No. 5, Bandung",
	}
}

func contractTestEmployee(id string) hrisrepo.ContractEmployee {
	return hrisrepo.ContractEmployee{
		ID:                id,
		FullName:          "Budi Santoso",
		Email:             "budi@contoh.co.id",
		Phone:             strPtr("0812-0000-1111"),
		Position:          "Full Time",
		Department:        strPtr("Engineering"),
		EmploymentStatus:  "active",
		BankAccountNumber: strPtr("9990000000002"),
		BankName:          strPtr("Mandiri"),
		JobTitle:          strPtr("Backend Engineer"),
		DepartmentHead:    strPtr("Andi Wijaya"),
	}
}

func contractTestTerms() model.EmploymentContract {
	return model.EmploymentContract{
		ContractType:         model.ContractTypePKWT,
		Status:               model.ContractStatusDraft,
		StartDate:            day(2026, 10, 1),
		EndDate:              timePtr(day(2027, 9, 30)),
		JobTitle:             "Backend Engineer",
		Department:           strPtr("Engineering"),
		SupervisorName:       strPtr("Andi Wijaya"),
		WorkLocation:         "Jakarta",
		WorkMode:             model.ContractWorkModeHybrid,
		WorkModeDetail:       strPtr("3 hari WFO, 2 hari WFH"),
		PKWTBasis:            strPtr("pekerjaan pengembangan sistem untuk proyek klien"),
		JobDescription:       "membangun dan memelihara layanan backend",
		WorkDays:             contractDefaultWorkDays,
		WorkHours:            contractDefaultWorkHours,
		WeeklyHours:          40,
		NoticeDays:           30,
		IncidentReportHours:  24,
		NonSolicitMonths:     12,
		ConfidentialityYears: 3,
		Benefits: []model.ContractBenefit{
			{Name: "Laptop kerja / Work laptop", Value: "Pinjam pakai", Notes: "Dikembalikan saat kontrak berakhir"},
			{Name: "Tunjangan internet / Internet allowance", Value: "Rp300.000/bulan", Notes: "Dibayar bersama gaji"},
		},
		PriorWorks: []model.ContractPriorWork{{Title: "indo-tokenizer", Description: "Library open-source", Year: "2024"}},
	}
}

func docxText(t *testing.T, docx []byte) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	for _, file := range reader.File {
		if !strings.HasPrefix(file.Name, "word/") || !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		out.Write(data)
	}
	return out.String()
}

// Every variable of 01 and 02 in the sanitized kamus has a value, both
// templates render without a missing variable or a leftover tag, the full
// NIK and account number appear only in the PKWT, and the NDA cites the
// PKWT number.
func TestBuildContractPayloadsCoverTemplates(t *testing.T) {
	pkwtNumber, ndaNumber := FormatContractNumbers(1, "CTN", day(2026, 10, 2))
	pkwt, nda := buildContractPayloads(contractPayloadInput{
		Contract:     contractTestTerms(),
		Employee:     contractTestEmployee(uuid.NewString()),
		Identity:     contractTestIdentity(),
		Company:      contractTestCompany(),
		Compensation: model.ContractCompensation{BaseSalary: 12_500_000, FixedAllowance: 2_150_000},
		DocNumber:    pkwtNumber,
		NDADocNumber: ndaNumber,
		DocumentDate: day(2026, 10, 2),
		DocumentCity: "Jakarta",
	})

	raw, err := os.ReadFile("../../docgen/testdata/kamus_variabel.json")
	if err != nil {
		t.Fatal(err)
	}
	var kamus struct {
		Templates map[string]struct {
			Variabel map[string]json.RawMessage `json:"variabel"`
		} `json:"templates"`
	}
	if err := json.Unmarshal(raw, &kamus); err != nil {
		t.Fatal(err)
	}
	for file, payload := range map[string]docgen.Payload{string(docgen.TemplatePKWT): pkwt, string(docgen.TemplateNDA): nda} {
		entry, ok := kamus.Templates[file]
		if !ok {
			t.Fatalf("kamus has no %s", file)
		}
		for name := range entry.Variabel {
			value, ok := payload[name]
			if !ok {
				t.Errorf("%s: variable %s has no value", file, name)
				continue
			}
			if text, isText := value.(string); isText && strings.TrimSpace(text) == "" {
				t.Errorf("%s: variable %s is empty", file, name)
			}
		}
	}

	if pkwt["karyawan_nik"] != "3273011503950001" || pkwt["karyawan_norek"] != "9990000000002" {
		t.Errorf("PKWT identity = %v / %v", pkwt["karyawan_nik"], pkwt["karyawan_norek"])
	}
	if nda["karyawan_nik"] != "327301**********" {
		t.Errorf("NDA NIK must be masked: %v", nda["karyawan_nik"])
	}
	if _, has := nda["karyawan_norek"]; has {
		t.Error("NDA must not carry the account number")
	}
	if nda["nomor_kontrak_kerja"] != pkwtNumber || pkwt["nomor_nda_hki"] != ndaNumber || pkwt["dokumen_nomor"] != pkwtNumber || nda["dokumen_nomor"] != ndaNumber {
		t.Errorf("cross references: %v %v", nda["nomor_kontrak_kerja"], pkwt["nomor_nda_hki"])
	}
	if pkwt["durasi_bulan"] != "12" || pkwt["mode_kerja"] != "hybrid (3 hari WFO, 2 hari WFH)" || pkwt["gaji_pokok"] != "12.500.000" ||
		pkwt["gaji_pokok_terbilang"] != "dua belas juta lima ratus ribu rupiah" || pkwt["tunjangan_tetap"] != "2.150.000" ||
		pkwt["karyawan_jenis_kelamin"] != "Laki-laki / Male" || pkwt["karyawan_tanggal_lahir"] != "15-03-1995" ||
		pkwt["dokumen_tanggal"] != "2 Oktober 2026" || pkwt["dokumen_tanggal_en"] != "2 October 2026" {
		t.Errorf("formatted values: %v", pkwt)
	}

	for name, item := range map[string]struct {
		id      docgen.TemplateID
		payload docgen.Payload
	}{"pkwt": {docgen.TemplatePKWT, pkwt}, "nda": {docgen.TemplateNDA, nda}} {
		tpl, err := docgen.Load(item.id)
		if err != nil {
			t.Fatal(err)
		}
		docx, err := docgen.Render(tpl, item.payload)
		if err != nil {
			t.Fatalf("%s render: %v", name, err)
		}
		text := docxText(t, docx)
		if strings.Contains(text, "{{") || strings.Contains(text, "}}") {
			t.Errorf("%s: leftover tag in the rendered document", name)
		}
		hasFullNIK := strings.Contains(text, "3273011503950001")
		if (name == "pkwt") != hasFullNIK {
			t.Errorf("%s: full NIK present = %v", name, hasFullNIK)
		}
		if name == "nda" && (!strings.Contains(text, pkwtNumber) || strings.Contains(text, "9990000000002")) {
			t.Error("NDA must cite the PKWT number and never the account number")
		}
	}

	// Empty benefit and prior-work lists still render one row.
	terms := contractTestTerms()
	terms.Benefits, terms.PriorWorks = nil, nil
	pkwt, nda = buildContractPayloads(contractPayloadInput{Contract: terms, Company: contractTestCompany(), DocumentDate: day(2026, 10, 2)})
	if rows := pkwt["benefit"].([]map[string]string); len(rows) != 1 {
		t.Errorf("empty benefit rows = %v", rows)
	}
	if rows := nda["karya_terdahulu"].([]map[string]string); len(rows) != 1 || rows[0]["judul"] != "Tidak ada / None" {
		t.Errorf("empty prior work rows = %v", rows)
	}
}

func TestContractMissingFields(t *testing.T) {
	complete := contractCompleteness{
		Contract:        contractTestTerms(),
		Employee:        contractTestEmployee(uuid.NewString()),
		Identity:        contractTestIdentity(),
		Company:         contractTestCompany(),
		HasCompensation: true,
	}
	if missing := contractMissingFields(complete); len(missing) != 0 {
		t.Errorf("complete contract misses %v", missing)
	}
	noIdentity := complete
	noIdentity.Identity = model.EmployeeIdentity{}
	noIdentity.HasCompensation = false
	missing := contractMissingFields(noIdentity)
	fields := []string{}
	for _, item := range missing {
		fields = append(fields, item.Scope+"."+item.Field)
	}
	sort.Strings(fields)
	want := "contract.compensation,identity.bank_account_name,identity.birth_date,identity.birth_place,identity.gender,identity.ktp_address,identity.nik"
	if strings.Join(fields, ",") != want {
		t.Errorf("missing = %v", fields)
	}
}

// ---------------------------------------------------------------------------
// Service with in-memory fakes

type fakeContractRepo struct {
	mu        sync.Mutex
	contracts map[string]model.EmploymentContract
	employees map[string]hrisrepo.ContractEmployee
	order     []string
	now       func() time.Time
}

func newFakeContractRepo() *fakeContractRepo {
	return &fakeContractRepo{contracts: map[string]model.EmploymentContract{}, employees: map[string]hrisrepo.ContractEmployee{}, now: time.Now}
}

func (r *fakeContractRepo) GetEmployee(_ context.Context, employeeID string) (hrisrepo.ContractEmployee, error) {
	employee, ok := r.employees[employeeID]
	if !ok {
		return hrisrepo.ContractEmployee{}, hrisrepo.ErrEmployeeNotFound
	}
	return employee, nil
}

func (r *fakeContractRepo) GetByID(_ context.Context, id string) (model.EmploymentContract, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	contract, ok := r.contracts[id]
	if !ok {
		return model.EmploymentContract{}, hrisrepo.ErrContractNotFound
	}
	return contract, nil
}

func (r *fakeContractRepo) List(_ context.Context, filter hrisrepo.ContractListFilter) ([]hrisrepo.ContractListRow, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := []hrisrepo.ContractListRow{}
	for _, id := range r.order {
		contract := r.contracts[id]
		if filter.EmployeeID != "" && contract.EmployeeID != filter.EmployeeID {
			continue
		}
		rows = append(rows, hrisrepo.ContractListRow{Contract: contract, EmployeeName: r.employees[contract.EmployeeID].FullName})
	}
	return rows, nil
}

func applyTerms(contract *model.EmploymentContract, terms hrisrepo.ContractTerms) {
	contract.ContractType, contract.IsRecordOnly = terms.ContractType, terms.IsRecordOnly
	contract.StartDate, contract.EndDate = terms.StartDate, terms.EndDate
	contract.JobTitle, contract.Department, contract.SupervisorName = terms.JobTitle, terms.Department, terms.SupervisorName
	contract.WorkLocation, contract.WorkMode, contract.WorkModeDetail = terms.WorkLocation, terms.WorkMode, terms.WorkModeDetail
	contract.PKWTBasis, contract.JobDescription = terms.PKWTBasis, terms.JobDescription
	contract.WorkDays, contract.WorkHours, contract.WeeklyHours, contract.NoticeDays = terms.WorkDays, terms.WorkHours, terms.WeeklyHours, terms.NoticeDays
	contract.CompensationEncrypted, contract.Benefits = terms.CompensationEncrypted, terms.Benefits
	contract.IncidentReportHours, contract.NonSolicitMonths, contract.ConfidentialityYears = terms.IncidentReportHours, terms.NonSolicitMonths, terms.ConfidentialityYears
	contract.PriorWorks, contract.DocumentDate, contract.DocumentCity = terms.PriorWorks, terms.DocumentDate, terms.DocumentCity
}

func (r *fakeContractRepo) Create(_ context.Context, params hrisrepo.CreateContractParams) (model.EmploymentContract, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.employees[params.EmployeeID]; !ok {
		return model.EmploymentContract{}, hrisrepo.ErrEmployeeNotFound
	}
	contract := model.EmploymentContract{
		ID:                 uuid.NewString(),
		EmployeeID:         params.EmployeeID,
		PreviousContractID: params.PreviousContractID,
		Status:             model.ContractStatusDraft,
		RenderStatus:       model.DocumentRenderNone,
		CreatedAt:          r.now(),
		UpdatedAt:          r.now(),
	}
	applyTerms(&contract, params.Terms)
	r.contracts[contract.ID] = contract
	r.order = append(r.order, contract.ID)
	return contract, nil
}

func (r *fakeContractRepo) UpdateTerms(_ context.Context, id string, params hrisrepo.UpdateContractTermsParams) (model.EmploymentContract, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	contract, ok := r.contracts[id]
	if !ok {
		return model.EmploymentContract{}, hrisrepo.ErrContractNotFound
	}
	if contract.Status != params.ExpectStatus || contract.Revision != params.ExpectRevision || contract.SignedAt != nil {
		return model.EmploymentContract{}, hrisrepo.ErrContractStateChanged
	}
	if contract.Status == model.ContractStatusGenerated || contract.Status == model.ContractStatusSent {
		contract.RenderStatus = model.DocumentRenderNone
	}
	contract.Status, contract.Revision = params.Status, params.Revision
	applyTerms(&contract, params.Terms)
	r.contracts[id] = contract
	return contract, nil
}

func (r *fakeContractRepo) RecordSnapshot(_ context.Context, id string, params hrisrepo.ContractSnapshotParams) (model.EmploymentContract, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	contract := r.contracts[id]
	if contract.Status != params.ExpectStatus || contract.Revision != params.ExpectRevision || contract.IsRecordOnly {
		return model.EmploymentContract{}, hrisrepo.ErrContractStateChanged
	}
	if contract.SeqNo == nil {
		seq, doc, nda := params.SeqNo, params.DocNumber, params.NDADocNumber
		contract.SeqNo, contract.DocNumber, contract.NDADocNumber = &seq, &doc, &nda
	}
	documentDate, city, payload, version, by, at := params.DocumentDate, params.DocumentCity, params.PayloadEncrypted, params.TemplateVersion, params.GeneratedBy, r.now()
	contract.DocumentDate, contract.DocumentCity, contract.PayloadEncrypted, contract.TemplateVersion = &documentDate, &city, &payload, &version
	contract.GeneratedBy, contract.GeneratedAt = &by, &at
	contract.Status = model.ContractStatusGenerated
	contract.RenderStatus = params.RenderStatus
	r.contracts[id] = contract
	return contract, nil
}

func (r *fakeContractRepo) setRenderReady(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	contract := r.contracts[id]
	pkwt, nda := "documents/t/contract/"+id+"-pkwt.pdf.enc", "documents/t/contract/"+id+"-nda.pdf.enc"
	contract.PKWTPDFPath, contract.NDAPDFPath = &pkwt, &nda
	contract.PDFSHA256 = []string{"sha-pkwt-" + id, "sha-nda-" + id}
	contract.RenderStatus = model.DocumentRenderReady
	r.contracts[id] = contract
}

func (r *fakeContractRepo) SetStatus(_ context.Context, id string, from []string, status string, signedAt *time.Time, endedAt *time.Time, endNotes *string) (model.EmploymentContract, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	contract := r.contracts[id]
	if !containsStatus(from, contract.Status) {
		return model.EmploymentContract{}, hrisrepo.ErrContractStateChanged
	}
	contract.Status = status
	if signedAt != nil {
		contract.SignedAt = signedAt
	}
	if endedAt != nil {
		contract.EndedAt = endedAt
	}
	if endNotes != nil {
		contract.EndNotes = endNotes
	}
	r.contracts[id] = contract
	return contract, nil
}

func (r *fakeContractRepo) MarkSent(_ context.Context, id string, at time.Time, sent hrisrepo.ContractSentSnapshot) (model.EmploymentContract, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	contract := r.contracts[id]
	if contract.Revision != sent.Revision || optionalText(contract.PayloadEncrypted) != sent.PayloadEncrypted {
		return model.EmploymentContract{}, hrisrepo.ErrContractStateChanged
	}
	if contract.Status != model.ContractStatusSigned && contract.Status != model.ContractStatusEnded {
		contract.Status = model.ContractStatusSent
	}
	contract.LastSentAt = &at
	r.contracts[id] = contract
	return contract, nil
}

func (r *fakeContractRepo) GetForUpdate(ctx context.Context, id string) (model.EmploymentContract, error) {
	return r.GetByID(ctx, id)
}

func (r *fakeContractRepo) ChainLinks(_ context.Context, employeeID string) ([]hrisrepo.ContractChainLink, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	links := []hrisrepo.ContractChainLink{}
	for _, id := range r.order {
		contract := r.contracts[id]
		if contract.EmployeeID != employeeID {
			continue
		}
		links = append(links, hrisrepo.ContractChainLink{ID: contract.ID, PreviousContractID: contract.PreviousContractID, ContractType: contract.ContractType, Status: contract.Status, StartDate: contract.StartDate, EndDate: contract.EndDate, EndedAt: contract.EndedAt})
	}
	return links, nil
}

func (r *fakeContractRepo) RenewalOf(_ context.Context, id string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, contract := range r.contracts {
		if optionalText(contract.PreviousContractID) == id && contract.Status != model.ContractStatusCancelled {
			return contract.ID, nil
		}
	}
	return "", nil
}

func (r *fakeContractRepo) ActiveForPeriod(_ context.Context, employeeID string, start time.Time, end time.Time) (model.EmploymentContract, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var found *model.EmploymentContract
	for _, contract := range r.contracts {
		if contract.EmployeeID != employeeID || (contract.Status != model.ContractStatusSent && contract.Status != model.ContractStatusSigned && contract.Status != model.ContractStatusEnded) {
			continue
		}
		if contract.Status == model.ContractStatusEnded && contract.EndedAt != nil && contract.EndedAt.Before(start) {
			continue
		}
		if contract.StartDate.After(end) || (contract.EndDate != nil && contract.EndDate.Before(start)) {
			continue
		}
		if found == nil || contract.StartDate.After(found.StartDate) {
			item := contract
			found = &item
		}
	}
	if found == nil {
		return model.EmploymentContract{}, false, nil
	}
	return *found, true, nil
}

func (r *fakeContractRepo) LatestDeliveries(context.Context, []string, bool) (map[string]model.EmailDelivery, error) {
	return map[string]model.EmailDelivery{}, nil
}

func (r *fakeContractRepo) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type fakeContractSequences struct{ calls []string }

func (f *fakeContractSequences) Next(_ context.Context, docType string, periodKey string) (int, error) {
	f.calls = append(f.calls, docType+"/"+periodKey)
	return len(f.calls), nil
}

type fakeContractIdentity struct {
	identities map[string]model.EmployeeIdentity
	log        []string
}

func (f *fakeContractIdentity) IdentityWithAccess(_ context.Context, employeeID string, _ string, action string) (model.EmployeeIdentity, bool, error) {
	f.log = append(f.log, action)
	identity, ok := f.identities[employeeID]
	return identity, ok, nil
}

func (f *fakeContractIdentity) LogIdentityAccess(_ context.Context, _ string, _ string, action string) error {
	f.log = append(f.log, action)
	return nil
}

func (f *fakeContractIdentity) EnsureEmployeeCode(context.Context, string, string) (int, string, error) {
	return 1, "CTN-0001", nil
}

type contractFixture struct {
	service      *ContractsService
	repo         *fakeContractRepo
	sequences    *fakeContractSequences
	identity     *fakeContractIdentity
	compensation *fakePayslipCompensation
	queue        *fakePayslipQueue
	sender       *fakeMailSender
	budi         string
	andi         string
	ctx          context.Context
}

var contractHR = ContractViewer{DocumentViewer: DocumentViewer{ActorID: payslipTestActor, CanViewIdentity: true}, CanViewSalary: true, CanViewContract: true}

func newContractFixture(t *testing.T) *contractFixture {
	t.Helper()
	encrypter, err := security.NewEncrypter("contract-test-secret-0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	repo := newFakeContractRepo()
	budi, andi := uuid.NewString(), uuid.NewString()
	repo.employees[budi] = contractTestEmployee(budi)
	andiEmployee := contractTestEmployee(andi)
	andiEmployee.FullName, andiEmployee.Email = "Andi Wijaya", "andi@contoh.co.id"
	repo.employees[andi] = andiEmployee

	// The mailer resolves recipients through the payslip fakes.
	payslipRepo := newFakePayslipRepo()
	budiRecipient := demoEmployee()
	budiRecipient.ID = budi
	payslipRepo.employees[budi] = budiRecipient

	compensation := &fakePayslipCompensation{salaries: map[string]model.SalaryRecord{budi: *budiSalary()}}
	identity := &fakeContractIdentity{identities: map[string]model.EmployeeIdentity{budi: contractTestIdentity()}}
	queue := &fakePayslipQueue{}
	deliveries := &fakeMailDeliveries{inFlight: map[string]bool{}}
	sender := &fakeMailSender{deliveries: deliveries}
	recipients := &fakeMailRecipients{repo: payslipRepo, last: map[string]string{}, lastSource: map[string]string{}}
	company := fakePayslipCompany{record: contractTestCompany()}
	mailer := NewDocumentMailer(sender, deliveries, recipients, company, nil, false)
	sequences := &fakeContractSequences{}
	service := NewContractsService(repo, sequences, compensation, identity, company, queue, &fakePayslipStore{}, mailer, encrypter)
	service.now = func() time.Time { return wib(2026, 10, 2, 12) }
	ctx := tenant.WithInfo(context.Background(), tenant.Info{ID: "00000000-0000-0000-0000-000000000001", Slug: "default"})
	return &contractFixture{service: service, repo: repo, sequences: sequences, identity: identity, compensation: compensation, queue: queue, sender: sender, budi: budi, andi: andi, ctx: ctx}
}

func budiPKWTRequest(employeeID string) hrisdto.CreateContractRequest {
	end := "2027-09-30"
	return hrisdto.CreateContractRequest{
		EmployeeID: employeeID,
		ContractFields: hrisdto.ContractFields{
			ContractType:   "PKWT",
			StartDate:      "2026-10-01",
			EndDate:        &end,
			JobTitle:       "Backend Engineer",
			WorkLocation:   "Jakarta",
			WorkMode:       "hybrid",
			WorkModeDetail: strPtr("3 hari WFO, 2 hari WFH"),
			PKWTBasis:      strPtr("pekerjaan pengembangan sistem untuk proyek klien"),
			JobDescription: "membangun dan memelihara layanan backend",
			Benefits: []hrisdto.ContractBenefitRequest{
				{Name: "Laptop kerja / Work laptop", Value: "Pinjam pakai"},
				{Name: "Tunjangan internet / Internet allowance", Value: "Rp300.000/bulan"},
			},
		},
	}
}

func (f *contractFixture) send(t *testing.T, id string, cc ...string) hrisdto.ContractSendResponse {
	t.Helper()
	result, err := f.service.Send(f.ctx, contractHR, id, hrisdto.SendContractRequest{Cc: cc})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !result.Response.Sent {
		t.Fatalf("send not sent: %+v", result.Response)
	}
	return result.Response
}

// Create -> preflight -> generate (numbers once) -> send (one mail, both
// PDFs, CC on the company domain) -> edit after send (draft, revision 1,
// same numbers) -> regenerate + send with '(Revisi 1)' -> signed (no more
// edits) -> renew (linked draft from the day after the end).
func TestContractLifecycleRevisionSendRenew(t *testing.T) {
	f := newContractFixture(t)
	created, audit, err := f.service.Create(f.ctx, contractHR, budiPKWTRequest(f.budi))
	if err != nil {
		t.Fatal(err)
	}
	if created.Compensation == nil || created.Compensation.BaseSalary != 12_500_000 || created.Compensation.FixedAllowance != 2_150_000 || audit.Values["compensation_prefilled"] != true {
		t.Fatalf("compensation prefill = %+v %v", created.Compensation, audit.Values)
	}
	if optionalText(created.Department) != "Engineering" || optionalText(created.SupervisorName) != "Andi Wijaya" {
		t.Errorf("department / atasan defaults = %v / %v", created.Department, created.SupervisorName)
	}

	preflight, err := f.service.Preflight(f.ctx, contractHR, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !preflight.Ready || len(preflight.Missing) != 0 || preflight.ChainPKWTMonths != 12 {
		t.Fatalf("preflight = %+v", preflight)
	}

	generated, _, err := f.service.Generate(f.ctx, contractHR, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if optionalText(generated.DocNumber) != "001/PKWT/CTN/X/2026" || optionalText(generated.NDADocNumber) != "001/NDA-HKI/CTN/X/2026" ||
		generated.Status != model.ContractStatusGenerated || optionalText(generated.DocumentDate) != "2026-10-02" || optionalText(generated.DocumentCity) != "Jakarta" {
		t.Fatalf("generated = %+v", generated)
	}
	if strings.Join(f.sequences.calls, ",") != "EMPLOYMENT/2026-10" || len(f.queue.jobs) != 1 || f.queue.jobs[0].Kind != ContractDocumentKind {
		t.Fatalf("sequence calls %v, jobs %v", f.sequences.calls, f.queue.jobs)
	}
	if _, err := f.service.Send(f.ctx, contractHR, created.ID, hrisdto.SendContractRequest{}); !errors.Is(err, ErrContractPDFNotReady) {
		t.Errorf("send before the render err = %v", err)
	}
	f.repo.setRenderReady(created.ID)

	if _, err := f.service.Send(f.ctx, contractHR, created.ID, hrisdto.SendContractRequest{Cc: []string{"boss@gmail.com"}}); !errors.Is(err, ErrContractCcDomain) {
		t.Errorf("foreign CC err = %v", err)
	}
	sent := f.send(t, created.ID, "Legal@Contoh.co.id")
	if sent.Contract.Status != model.ContractStatusSent || len(f.sender.delivered) != 1 {
		t.Fatalf("after send: %s, %d mails", sent.Contract.Status, len(f.sender.delivered))
	}
	mail := f.sender.delivered[0]
	if len(mail.Attachments) != 2 || !strings.HasPrefix(mail.Attachments[0].Filename, "PKWT_001-PKWT-CTN-X-2026") || !strings.HasPrefix(mail.Attachments[1].Filename, "NDA-HKI_001-NDA-HKI-CTN-X-2026") {
		t.Fatalf("attachments = %+v", mail.Attachments)
	}
	if strings.Join(mail.Cc, ",") != "legal@contoh.co.id" || mail.Recipient != "staff.ops@kantor.local" || strings.Contains(mail.Subject, "Revisi") {
		t.Errorf("mail = to %s cc %v subject %q", mail.Recipient, mail.Cc, mail.Subject)
	}
	if strings.Contains(mail.Text, "12.500.000") || strings.Contains(mail.Text, "3273011503950001") {
		t.Error("the mail body must not carry amounts or the NIK")
	}

	edit := budiPKWTRequest(f.budi).ContractFields
	edit.Department, edit.SupervisorName = strPtr("Engineering"), strPtr("Andi Wijaya")
	edit.JobDescription = "membangun layanan backend dan API"
	edit.PriorWorks = []hrisdto.ContractPriorWorkRequest{{Title: "indo-tokenizer", Year: "2024"}}
	revised, changed, _, err := f.service.Update(f.ctx, contractHR, created.ID, hrisdto.UpdateContractRequest{ContractFields: edit})
	if err != nil {
		t.Fatal(err)
	}
	if revised.Status != model.ContractStatusDraft || revised.Revision != 1 || optionalText(revised.DocNumber) != "001/PKWT/CTN/X/2026" || strings.Join(changed, ",") != "job_description,prior_works" {
		t.Fatalf("revision = %s rev %d %v changed %v", revised.Status, revised.Revision, revised.DocNumber, changed)
	}
	if _, _, err := f.service.Generate(f.ctx, contractHR, created.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.sequences.calls) != 1 {
		t.Errorf("a revision must keep the numbers: %v", f.sequences.calls)
	}
	f.repo.setRenderReady(created.ID)
	f.send(t, created.ID)
	if subject := f.sender.delivered[1].Subject; !strings.HasSuffix(subject, "(Revisi 1)") {
		t.Errorf("revised subject = %q", subject)
	}

	signed, _, err := f.service.SetStatus(f.ctx, contractHR, created.ID, hrisdto.UpdateContractStatusRequest{Status: "signed", SignedAt: strPtr("2026-10-02")})
	if err != nil || signed.Status != model.ContractStatusSigned || optionalText(signed.SignedAt) != "2026-10-02" {
		t.Fatalf("signed = %+v, %v", signed.Status, err)
	}
	if _, _, _, err := f.service.Update(f.ctx, contractHR, created.ID, hrisdto.UpdateContractRequest{ContractFields: edit}); !errors.Is(err, ErrContractNotEditable) {
		t.Errorf("edit after signing err = %v", err)
	}
	if _, _, err := f.service.SetStatus(f.ctx, contractHR, created.ID, hrisdto.UpdateContractStatusRequest{Status: "signed", SignedAt: strPtr("2026-12-01")}); !errors.Is(err, ErrContractStatusTransition) {
		t.Errorf("signing twice err = %v", err)
	}
	// A copy re-sent after signing keeps the signed status.
	resent := f.send(t, created.ID)
	if resent.Contract.Status != model.ContractStatusSigned {
		t.Errorf("re-send changed the status to %s", resent.Contract.Status)
	}

	renewal, renewAudit, err := f.service.Renew(f.ctx, contractHR, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if renewal.StartDate != "2027-10-01" || optionalText(renewal.EndDate) != "2028-09-30" || optionalText(renewal.PreviousContractID) != created.ID ||
		renewal.Status != model.ContractStatusDraft || renewal.DocNumber != nil || renewal.JobTitle != "Backend Engineer" || len(renewal.PriorWorks) != 1 {
		t.Fatalf("renewal = %+v", renewal)
	}
	if renewAudit.Values["chain_pkwt_months"] != 24.0 {
		t.Errorf("chain months = %v", renewAudit.Values["chain_pkwt_months"])
	}
	if _, _, err := f.service.Renew(f.ctx, contractHR, created.ID); !errors.Is(err, ErrContractAlreadyRenewed) {
		t.Errorf("second renewal err = %v", err)
	}

	// Payslip hook: the signed contract is active in October 2026.
	info, err := f.service.ActiveContractForPayslip(f.ctx, f.budi, wib(2026, 10, 31, 0))
	if err != nil || info == nil || info.StatusKerja != "PKWT" || info.JobTitle != "Backend Engineer" || info.ContractID != created.ID {
		t.Errorf("payslip contract = %+v, %v", info, err)
	}
	if info, _ := f.service.ActiveContractForPayslip(f.ctx, f.budi, wib(2026, 9, 30, 0)); info != nil {
		t.Errorf("September has no active contract: %+v", info)
	}

	// Identity reads were recorded: preflight, generate (x2), send (x3).
	if !strings.Contains(strings.Join(f.identity.log, ","), IdentityAccessPreflight) || countOf(f.identity.log, contractAccessSend) != 3 {
		t.Errorf("identity log = %v", f.identity.log)
	}
}

func countOf(values []string, value string) int {
	count := 0
	for _, item := range values {
		if item == value {
			count++
		}
	}
	return count
}

func TestContractCompensationNeedsSalaryView(t *testing.T) {
	f := newContractFixture(t)
	noSalary := ContractViewer{DocumentViewer: DocumentViewer{ActorID: payslipTestActor}, CanViewContract: true}
	// Without hris:salary:view nothing is prefilled: the PKWT would print a
	// salary (of any date, through start_date) the caller may not read.
	draft, audit, err := f.service.Create(f.ctx, noSalary, budiPKWTRequest(f.budi))
	if err != nil {
		t.Fatal(err)
	}
	if draft.Compensation != nil || draft.HasCompensation || draft.CompensationVisible || audit.Values["compensation_prefilled"] != false {
		t.Errorf("compensation prefilled without salary:view: %+v %v", draft.Compensation, audit.Values)
	}
	preflight, err := f.service.Preflight(f.ctx, noSalary, draft.ID)
	if err != nil || preflight.Ready || !hasMissing(preflight.Missing, "contract.compensation") {
		t.Errorf("preflight without compensation = %+v, %v", preflight.Missing, err)
	}

	created, _, err := f.service.Create(f.ctx, contractHR, budiPKWTRequest(f.budi))
	if err != nil {
		t.Fatal(err)
	}
	detail, err := f.service.Get(f.ctx, noSalary, created.ID)
	if err != nil || detail.Compensation != nil || !detail.HasCompensation || detail.CompensationVisible {
		t.Errorf("detail compensation = %+v, %v", detail.Compensation, err)
	}
	withCompensation := budiPKWTRequest(f.budi)
	withCompensation.Compensation = &hrisdto.ContractCompensationRequest{BaseSalary: 1}
	if _, _, err := f.service.Create(f.ctx, noSalary, withCompensation); !errors.Is(err, ErrContractCompensationForbidden) {
		t.Errorf("setting compensation without salary:view err = %v", err)
	}
	before := countOf(f.compensation.accessLog, contractAccessView+":"+f.budi)
	if _, err := f.service.Get(f.ctx, noSalary, created.ID); err != nil {
		t.Fatal(err)
	}
	if countOf(f.compensation.accessLog, contractAccessView+":"+f.budi) != before {
		t.Error("no salary access row when nothing is shown")
	}
	if _, err := f.service.Get(f.ctx, contractHR, created.ID); err != nil {
		t.Fatal(err)
	}
	if countOf(f.compensation.accessLog, contractAccessView+":"+f.budi) != before+1 {
		t.Errorf("salary access log = %v", f.compensation.accessLog)
	}
}

func hasMissing(missing []hrisdto.ContractMissingField, key string) bool {
	for _, item := range missing {
		if item.Scope+"."+item.Field == key {
			return true
		}
	}
	return false
}

// A generated contract is marked signed only once its PDFs exist (the
// worker never renders a signed contract); a DOCX is only served for the
// snapshot of the current terms; renewing needs a signed contract and an
// end date not before the signing; the send response carries the detail
// only for contract viewers.
func TestContractStatusAndDocumentGuards(t *testing.T) {
	f := newContractFixture(t)
	created, _, err := f.service.Create(f.ctx, contractHR, budiPKWTRequest(f.budi))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Docx(f.ctx, payslipTestActor, created.ID, ContractPartPKWT); !errors.Is(err, ErrContractDocumentNotCurrent) {
		t.Errorf("docx of a draft err = %v", err)
	}
	if _, _, err := f.service.Generate(f.ctx, contractHR, created.ID); err != nil {
		t.Fatal(err)
	}
	sign := hrisdto.UpdateContractStatusRequest{Status: "signed", SignedAt: strPtr("2026-10-02")}
	if _, _, err := f.service.SetStatus(f.ctx, contractHR, created.ID, sign); !errors.Is(err, ErrContractSignBeforePDF) {
		t.Errorf("sign while the PDF renders err = %v", err)
	}
	if _, err := f.service.Docx(f.ctx, payslipTestActor, created.ID, ContractPartNDA); err != nil {
		t.Errorf("docx of a generated contract: %v", err)
	}
	f.repo.setRenderReady(created.ID)

	noView := ContractViewer{DocumentViewer: DocumentViewer{ActorID: payslipTestActor}}
	sent, err := f.service.Send(f.ctx, noView, created.ID, hrisdto.SendContractRequest{})
	if err != nil || !sent.Response.Sent || sent.Response.Contract != nil {
		t.Fatalf("send without contract:view = %+v, %v", sent.Response.Contract, err)
	}
	if _, _, err := f.service.Renew(f.ctx, contractHR, created.ID); !errors.Is(err, ErrContractNotRenewable) {
		t.Errorf("renew of an unsigned (sent) contract err = %v", err)
	}

	// Edit after send: the snapshot is the previous revision's.
	edit := budiPKWTRequest(f.budi).ContractFields
	edit.Department, edit.SupervisorName = strPtr("Engineering"), strPtr("Andi Wijaya")
	edit.JobTitle = "Senior Backend Engineer"
	if _, _, _, err := f.service.Update(f.ctx, contractHR, created.ID, hrisdto.UpdateContractRequest{ContractFields: edit}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Docx(f.ctx, payslipTestActor, created.ID, ContractPartPKWT); !errors.Is(err, ErrContractDocumentNotCurrent) {
		t.Errorf("docx of a revision draft err = %v", err)
	}
	if _, _, err := f.service.Generate(f.ctx, contractHR, created.ID); err != nil {
		t.Fatal(err)
	}
	f.repo.setRenderReady(created.ID)
	if _, _, err := f.service.SetStatus(f.ctx, contractHR, created.ID, sign); err != nil {
		t.Fatalf("sign with the PDFs ready: %v", err)
	}
	if _, _, err := f.service.SetStatus(f.ctx, contractHR, created.ID, hrisdto.UpdateContractStatusRequest{Status: "ended", EndedAt: strPtr("2025-01-01")}); !errors.Is(err, ErrContractStatusDateInvalid) {
		t.Errorf("ended before the signing err = %v", err)
	}
	ended, _, err := f.service.SetStatus(f.ctx, contractHR, created.ID, hrisdto.UpdateContractStatusRequest{Status: "ended", EndedAt: strPtr("2026-10-02")})
	if err != nil || ended.Status != model.ContractStatusEnded {
		t.Fatalf("ended = %v, %v", ended.Status, err)
	}
	// The final slip of the month the contract ended in still shows it.
	info, err := f.service.ActiveContractForPayslip(f.ctx, f.budi, wib(2026, 10, 31, 0))
	if err != nil || info == nil || info.ContractID != created.ID || info.StatusKerja != "PKWT" {
		t.Errorf("payslip contract after Akhiri = %+v, %v", info, err)
	}
	if info, _ := f.service.ActiveContractForPayslip(f.ctx, f.budi, wib(2026, 11, 30, 0)); info != nil {
		t.Errorf("the month after Akhiri has a contract: %+v", info)
	}
}

// Renewing a record-only PKWT gives a PKWT with documents, and record-only
// history entered with 'Buat Kontrak' (no Perpanjang link) counts toward the
// 5-year check when the next PKWT starts right after it.
func TestContractRecordOnlyHistoryCountsInChain(t *testing.T) {
	f := newContractFixture(t)
	historyEnd := "2026-09-30"
	history, _, err := f.service.Create(f.ctx, contractHR, hrisdto.CreateContractRequest{
		EmployeeID: f.budi,
		ContractFields: hrisdto.ContractFields{
			ContractType: "PKWT", IsRecordOnly: true, StartDate: "2022-10-01", EndDate: &historyEnd, JobTitle: "Backend Engineer",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.service.SetStatus(f.ctx, contractHR, history.ID, hrisdto.UpdateContractStatusRequest{Status: "signed", SignedAt: strPtr("2022-09-25")}); err != nil {
		t.Fatal(err)
	}
	request := budiPKWTRequest(f.budi)
	end := "2028-09-30"
	request.EndDate = &end
	next, _, err := f.service.Create(f.ctx, contractHR, request)
	if err != nil {
		t.Fatal(err)
	}
	preflight, err := f.service.Preflight(f.ctx, contractHR, next.ID)
	if err != nil || preflight.ChainPKWTMonths != 72 || !hasWarning(preflight.Warnings, "pkwt_chain_over_5_years") {
		t.Fatalf("chain over record-only history = %v %v, %v", preflight.ChainPKWTMonths, preflight.Warnings, err)
	}

	renewal, _, err := f.service.Renew(f.ctx, contractHR, history.ID)
	if err != nil {
		t.Fatal(err)
	}
	if renewal.IsRecordOnly || renewal.ContractType != model.ContractTypePKWT || optionalText(renewal.PreviousContractID) != history.ID {
		t.Errorf("renewal of a record-only PKWT = record-only %v, type %s", renewal.IsRecordOnly, renewal.ContractType)
	}
}

func TestContractChainFollowsLinksAndContiguousPKWT(t *testing.T) {
	link := func(id string, previous string, contractType string, status string, start time.Time, end time.Time) hrisrepo.ContractChainLink {
		item := hrisrepo.ContractChainLink{ID: id, ContractType: contractType, Status: status, StartDate: start, EndDate: &end}
		if previous != "" {
			item.PreviousContractID = &previous
		}
		return item
	}
	links := []hrisrepo.ContractChainLink{
		link("a", "", model.ContractTypePKWT, model.ContractStatusEnded, day(2020, 1, 1), day(2020, 12, 31)),
		// Starts 61 days after a ended: a new chain.
		link("b", "", model.ContractTypePKWT, model.ContractStatusSigned, day(2021, 3, 2), day(2022, 3, 1)),
		// Contiguous with b, no link (record-only history).
		link("c", "", model.ContractTypePKWT, model.ContractStatusSigned, day(2022, 3, 15), day(2023, 3, 14)),
		link("x", "", model.ContractTypePKWT, model.ContractStatusCancelled, day(2023, 3, 15), day(2024, 3, 14)),
		// Linked renewal of c.
		link("d", "c", model.ContractTypePKWT, model.ContractStatusDraft, day(2023, 3, 15), day(2024, 3, 14)),
	}
	ids := func(chain []hrisrepo.ContractChainLink) string {
		out := []string{}
		for _, item := range chain {
			out = append(out, item.ID)
		}
		return strings.Join(out, ",")
	}
	if got := ids(contractChain("d", links)); got != "d,c,b" {
		t.Errorf("chain of d = %s", got)
	}
	if got := ids(contractChain("b", links)); got != "b" {
		t.Errorf("chain of b = %s (a ended more than 30 days before)", got)
	}
	if got := chainPKWTMonths(contractChain("d", links)); got != 36 {
		t.Errorf("months of d = %v", got)
	}
	if contractChain("missing", links) != nil {
		t.Error("unknown id must give no chain")
	}
}

func TestContractCcDomainRefusesWebmail(t *testing.T) {
	for input, want := range map[string]string{
		"hr@contoh.co.id":       "contoh.co.id",
		"HR.PTContoh@Gmail.com": "",
		"hr@yahoo.co.uk":        "",
		"hr@outlook.com":        "",
		"":                      "",
	} {
		if got := contractCcDomain(input); got != want {
			t.Errorf("cc domain of %q = %q, want %q", input, got, want)
		}
	}
	if _, err := normalizeContractCc([]string{"siapa.saja@gmail.com"}, contractCcDomain("hr.ptcontoh@gmail.com"), "budi@contoh.co.id"); !errors.Is(err, ErrContractCcDomain) {
		t.Errorf("webmail CC err = %v", err)
	}
}

// The atasan defaults to the company signer when the employee heads the
// department; an emptied document date / city is cleared on an unnumbered
// draft; a list filtered by a malformed employee id still reports the
// converter and mail readiness.
func TestContractDefaultsAndClearing(t *testing.T) {
	f := newContractFixture(t)
	head := f.repo.employees[f.budi]
	head.DepartmentHead = nil
	f.repo.employees[f.budi] = head
	request := budiPKWTRequest(f.budi)
	request.DocumentDate, request.DocumentCity = strPtr("2026-09-15"), strPtr("Bandung")
	created, _, err := f.service.Create(f.ctx, contractHR, request)
	if err != nil {
		t.Fatal(err)
	}
	if optionalText(created.SupervisorName) != "Rudi Hartono" || optionalText(created.DocumentDate) != "2026-09-15" {
		t.Errorf("atasan = %v, document date = %v", created.SupervisorName, created.DocumentDate)
	}
	edit := request.ContractFields
	edit.Department, edit.SupervisorName = created.Department, created.SupervisorName
	edit.DocumentDate, edit.DocumentCity = strPtr(""), strPtr("")
	cleared, changed, _, err := f.service.Update(f.ctx, contractHR, created.ID, hrisdto.UpdateContractRequest{ContractFields: edit})
	if err != nil || cleared.DocumentDate != nil || cleared.DocumentCity != nil || strings.Join(changed, ",") != "document_date,document_city" {
		t.Fatalf("cleared = %v %v changed %v, %v", cleared.DocumentDate, cleared.DocumentCity, changed, err)
	}
	edit.DocumentDate = strPtr("2026-13-45")
	if _, _, _, err := f.service.Update(f.ctx, contractHR, created.ID, hrisdto.UpdateContractRequest{ContractFields: edit}); !errors.Is(err, ErrContractDocumentDateInvalid) {
		t.Errorf("malformed document date err = %v", err)
	}
	edit.DocumentDate, edit.DocumentCity = nil, nil
	kept, changed, _, err := f.service.Update(f.ctx, contractHR, created.ID, hrisdto.UpdateContractRequest{ContractFields: edit})
	if err != nil || kept.DocumentDate != nil || len(changed) != 0 {
		t.Errorf("omitted date/city = %v changed %v, %v", kept.DocumentDate, changed, err)
	}

	list, err := f.service.List(f.ctx, contractHR, hrisrepo.ContractListFilter{EmployeeID: "abc"})
	if err != nil || len(list.Items) != 0 || !list.PDFAvailable {
		t.Errorf("list with a malformed employee id = %+v, %v", list, err)
	}
}

func TestContractRecordOnlyAndIncomplete(t *testing.T) {
	f := newContractFixture(t)
	record, _, err := f.service.Create(f.ctx, contractHR, hrisdto.CreateContractRequest{
		EmployeeID:     f.andi,
		ContractFields: hrisdto.ContractFields{ContractType: "PKWTT", StartDate: "2020-01-06", JobTitle: "Engineering Lead"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !record.IsRecordOnly {
		t.Fatal("PKWTT must be record-only")
	}
	if _, _, err := f.service.Generate(f.ctx, contractHR, record.ID); !errors.Is(err, ErrContractRecordOnly) {
		t.Errorf("generate record-only err = %v", err)
	}
	if _, err := f.service.Send(f.ctx, contractHR, record.ID, hrisdto.SendContractRequest{}); !errors.Is(err, ErrContractRecordOnly) {
		t.Errorf("send record-only err = %v", err)
	}
	preflight, err := f.service.Preflight(f.ctx, contractHR, record.ID)
	if err != nil || preflight.Documents || preflight.Ready {
		t.Errorf("record-only preflight = %+v, %v", preflight, err)
	}
	signed, _, err := f.service.SetStatus(f.ctx, contractHR, record.ID, hrisdto.UpdateContractStatusRequest{Status: "signed", SignedAt: strPtr("2020-01-06")})
	if err != nil || signed.Status != model.ContractStatusSigned {
		t.Fatalf("record-only signed = %v, %v", signed.Status, err)
	}
	info, err := f.service.ActiveContractForPayslip(f.ctx, f.andi, wib(2026, 10, 31, 0))
	if err != nil || info == nil || info.StatusKerja != "PKWTT" || info.JobTitle != "Engineering Lead" {
		t.Errorf("record-only payslip info = %+v, %v", info, err)
	}
	ended, _, err := f.service.SetStatus(f.ctx, contractHR, record.ID, hrisdto.UpdateContractStatusRequest{Status: "ended", EndNotes: strPtr("Pindah divisi")})
	if err != nil || ended.Status != model.ContractStatusEnded || optionalText(ended.EndedAt) != "2026-10-02" {
		t.Fatalf("record-only ended = %v %v, %v", ended.Status, ended.EndedAt, err)
	}
	if _, _, err := f.service.SetStatus(f.ctx, contractHR, record.ID, hrisdto.UpdateContractStatusRequest{Status: "signed", SignedAt: strPtr("2030-01-01")}); !errors.Is(err, ErrContractStatusTransition) {
		t.Errorf("ended -> signed err = %v", err)
	}

	// A PKWT for Andi, whose identity is empty, cannot be generated.
	pkwt, _, err := f.service.Create(f.ctx, contractHR, budiPKWTRequest(f.andi))
	if err != nil {
		t.Fatal(err)
	}
	preflight, err = f.service.Preflight(f.ctx, contractHR, pkwt.ID)
	if err != nil || preflight.Ready || len(preflight.Missing) == 0 {
		t.Fatalf("incomplete preflight = %+v, %v", preflight, err)
	}
	_, _, err = f.service.Generate(f.ctx, contractHR, pkwt.ID)
	var incomplete *ContractIncompleteError
	if !errors.As(err, &incomplete) || len(incomplete.Missing) != len(preflight.Missing) {
		t.Errorf("generate incomplete err = %v", err)
	}
	if len(f.sequences.calls) != 0 {
		t.Error("an incomplete contract must not take a number")
	}
}

func TestContractRenderHandlerPartsAndCompletion(t *testing.T) {
	encrypter, err := security.NewEncrypter("contract-test-secret-0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	pkwt, nda := buildContractPayloads(contractPayloadInput{Contract: contractTestTerms(), Company: contractTestCompany(), DocumentDate: day(2026, 10, 2)})
	raw, _ := json.Marshal(contractPayloads{PKWT: pkwt, NDA: nda})
	cipher, err := encrypter.EncryptString(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeContractRenderRepo{contract: model.EmploymentContract{ID: uuid.NewString(), PayloadEncrypted: &cipher, GeneratedBy: strPtr(payslipTestActor)}, superseded: []string{"old-pkwt", "old-nda"}}
	handler := NewContractRenderHandler(repo, encrypter)
	if handler.Kind() != "contract" {
		t.Fatal(handler.Kind())
	}
	prepared, err := handler.Prepare(context.Background(), repo.contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Parts) != 2 || prepared.Parts[0].Name != "pkwt" || prepared.Parts[0].Template != docgen.TemplatePKWT || prepared.Parts[1].Name != "nda" || prepared.Parts[1].Template != docgen.TemplateNDA || prepared.ActorID != payslipTestActor {
		t.Fatalf("parts = %+v", prepared.Parts)
	}
	for _, part := range prepared.Parts {
		if _, err := renderDocumentPart(part); err != nil {
			t.Errorf("%s: %v", part.Name, err)
		}
	}
	superseded, err := handler.Complete(context.Background(), repo.contract.ID, prepared.Claim, []StoredDocument{
		{Part: "nda", Path: "new-nda", SHA256: "b", Pages: 5},
		{Part: "pkwt", Path: "new-pkwt", SHA256: "a", Pages: 5},
	})
	if err != nil || strings.Join(superseded, ",") != "old-pkwt,old-nda" {
		t.Fatalf("superseded = %v, %v", superseded, err)
	}
	if repo.completed != "new-pkwt|new-nda|a,b" {
		t.Errorf("complete args = %q", repo.completed)
	}
	if _, err := handler.Complete(context.Background(), repo.contract.ID, prepared.Claim, []StoredDocument{{Part: "pkwt", Path: "x"}}); err == nil {
		t.Error("a render without the NDA must fail")
	}
	repo.stateChanged = true
	if _, err := handler.Complete(context.Background(), repo.contract.ID, prepared.Claim, []StoredDocument{{Part: "pkwt"}, {Part: "nda"}}); !errors.Is(err, ErrDocumentJobSuperseded) {
		t.Errorf("superseded render err = %v", err)
	}
}

type fakeContractRenderRepo struct {
	contract     model.EmploymentContract
	superseded   []string
	completed    string
	stateChanged bool
}

func (r *fakeContractRenderRepo) ListRenderPending(context.Context, time.Time) ([]string, error) {
	return []string{r.contract.ID}, nil
}

func (r *fakeContractRenderRepo) ClaimRender(context.Context, string, time.Time) (string, model.EmploymentContract, bool, error) {
	return "claim-1", r.contract, true, nil
}

func (r *fakeContractRenderRepo) CompleteRender(_ context.Context, _ string, claim string, pkwtPath string, ndaPath string, digests []string, _ string) ([]string, error) {
	if r.stateChanged || claim != "claim-1" {
		return nil, hrisrepo.ErrContractStateChanged
	}
	r.completed = pkwtPath + "|" + ndaPath + "|" + strings.Join(digests, ",")
	return r.superseded, nil
}

func (r *fakeContractRenderRepo) FailRender(context.Context, string, string, string) error {
	return nil
}

// The worker renders the PKWT and the NDA of a contract in ONE converter
// call, stores both encrypted and deletes the PDFs of the previous render.
func TestContractRenderedInOneConversionWithSupersededDeleted(t *testing.T) {
	h := newWorkerHarness(t, true)
	encrypter, err := security.NewEncrypter("contract-test-secret-0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	pkwt, nda := buildContractPayloads(contractPayloadInput{Contract: contractTestTerms(), Company: contractTestCompany(), DocumentDate: day(2026, 10, 2)})
	raw, _ := json.Marshal(contractPayloads{PKWT: pkwt, NDA: nda})
	cipher, err := encrypter.EncryptString(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	id := docC
	oldPKWT, err := h.store.Write(workerTestTenantID, ContractDocumentKind, id, ContractPartPKWT, []byte("%PDF-old-pkwt"))
	if err != nil {
		t.Fatal(err)
	}
	oldNDA, err := h.store.Write(workerTestTenantID, ContractDocumentKind, id, ContractPartNDA, []byte("%PDF-old-nda"))
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeContractRenderRepo{contract: model.EmploymentContract{ID: id, PayloadEncrypted: &cipher}, superseded: []string{oldPKWT.Path, oldNDA.Path}}
	if err := h.worker.Register(NewContractRenderHandler(repo, encrypter)); err != nil {
		t.Fatal(err)
	}

	h.worker.ProcessTenantBatch(context.Background(), workerTestTenant, jobsFor(ContractDocumentKind, id))

	if h.converter.callCount() != 1 || len(h.converter.calls[0]) != 2 {
		t.Fatalf("converter calls = %v, want one call with both documents", h.converter.calls)
	}
	parts := strings.Split(repo.completed, "|")
	if len(parts) != 3 || !strings.Contains(parts[0], "-pkwt-") || !strings.Contains(parts[1], "-nda-") {
		t.Fatalf("completed = %q", repo.completed)
	}
	files := storedFiles(t, h.uploads)
	sort.Strings(files)
	want := []string{parts[0], parts[1]}
	sort.Strings(want)
	if strings.Join(files, ",") != strings.Join(want, ",") {
		t.Errorf("stored files = %v, want only the new render %v", files, want)
	}
	data, err := h.store.Read(workerTestTenantID, parts[0], strings.Split(parts[2], ",")[0])
	if err != nil || !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Errorf("stored PKWT read = %v", err)
	}
}
