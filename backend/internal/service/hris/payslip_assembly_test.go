package hris

import (
	"strings"
	"testing"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/docgen"
	"github.com/kana-consultant/kantor/backend/internal/model"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
)

func wib(year int, month time.Month, day int, hour int) time.Time {
	return time.Date(year, month, day, hour, 0, 0, 0, payslipLocation)
}

func timePtr(t time.Time) *time.Time { return &t }

func strPtr(s string) *string { return &s }

func bonus(id string, amount int64, month int, year int, status string, approvedAt *time.Time) model.BonusRecord {
	return model.BonusRecord{ID: id, Amount: amount, Reason: "Bonus " + id, PeriodMonth: month, PeriodYear: year, ApprovalStatus: status, ApprovedAt: approvedAt, CreatedAt: wib(2026, 1, 1, 0)}
}

func bonusIDs(items []model.BonusRecord) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func reimbursementIDs(items []hrisrepo.PayslipReimbursement) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func equalIDs(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ids = %v, want %v", got, want)
	}
}

// D9: without an earlier sent slip only the period's own bonuses count, so
// the first slip never sweeps up history (Budi's March THR).
func TestCarryOverBonusesFirstSlipOnlyCurrentPeriod(t *testing.T) {
	bonuses := []model.BonusRecord{
		bonus("thr-march", 11_000_000, 3, 2026, "approved", timePtr(wib(2026, 3, 20, 10))),
		bonus("sep", 3_000_000, 9, 2026, "approved", timePtr(wib(2026, 9, 28, 10))),
		bonus("sep-pending", 2_000_000, 9, 2026, "pending", nil),
		bonus("aug-rejected", 1_500_000, 8, 2026, "rejected", timePtr(wib(2026, 8, 30, 10))),
		bonus("oct", 500_000, 10, 2026, "approved", timePtr(wib(2026, 9, 29, 10))),
	}
	got := selectCarryOverBonuses(2026, 9, nil, map[string]bool{}, bonuses)
	equalIDs(t, bonusIDs(got), "sep")
}

// D9: after a sent slip L, a bonus of an older period approved after L was
// generated goes on the next slip; one approved before L is not swept up.
func TestCarryOverBonusesLateApprovalAfterSentSlip(t *testing.T) {
	anchor := &payslipAnchor{ID: "L", PeriodYear: 2026, PeriodMonth: 9, GeneratedAt: wib(2026, 9, 24, 9)}
	bonuses := []model.BonusRecord{
		bonus("thr-march", 11_000_000, 3, 2026, "approved", timePtr(wib(2026, 3, 20, 10))),
		bonus("sep-late", 2_000_000, 9, 2026, "approved", timePtr(wib(2026, 9, 26, 10))),
		bonus("aug-late", 750_000, 8, 2026, "approved", timePtr(wib(2026, 10, 2, 10))),
		bonus("oct", 1_000_000, 10, 2026, "approved", timePtr(wib(2026, 10, 15, 10))),
		bonus("on-L", 3_000_000, 9, 2026, "approved", timePtr(wib(2026, 9, 20, 10))),
	}
	consumed := map[string]bool{"on-l": true}
	got := selectCarryOverBonuses(2026, 10, anchor, consumed, bonuses)
	equalIDs(t, bonusIDs(got), "aug-late", "sep-late", "oct")
}

// D9 regression (go-live): a historical bonus approved minutes before the
// first slip L was generated was left off L by the go-live floor and must
// not reach the next slip through the read-race grace. Only a bonus of L's
// own period gets the grace; an older one needs approved_at > L.generated_at.
func TestCarryOverBonusesGraceOnlyForAnchorPeriod(t *testing.T) {
	generated := time.Date(2026, 10, 1, 20, 4, 47, 0, time.UTC)
	anchor := &payslipAnchor{ID: "L", PeriodYear: 2026, PeriodMonth: 9, GeneratedAt: generated}
	bonuses := []model.BonusRecord{
		// Budi's March THR, approved by the demo seed 60 s before L.
		bonus("thr-march", 11_000_000, 3, 2026, "approved", timePtr(generated.Add(-60*time.Second))),
		// A September bonus approved 60 s before L that L did not see
		// (committed after L's read): the grace keeps it.
		bonus("sep-race", 400_000, 9, 2026, "approved", timePtr(generated.Add(-60*time.Second))),
		// A September bonus approved well before L and still not on L stays off.
		bonus("sep-old", 300_000, 9, 2026, "approved", timePtr(generated.Add(-time.Hour))),
		// An older-period bonus approved just after L is a late approval.
		bonus("aug-after", 200_000, 8, 2026, "approved", timePtr(generated.Add(time.Second))),
		// Exactly at L.generated_at is not after it.
		bonus("jul-at", 100_000, 7, 2026, "approved", timePtr(generated)),
	}
	got := selectCarryOverBonuses(2026, 10, anchor, map[string]bool{}, bonuses)
	equalIDs(t, bonusIDs(got), "aug-after", "sep-race")
}

// D9: the reimbursement grace never reaches below the first day of L's
// period (L's own floor when it was the first slip).
func TestCarryOverReimbursementsGraceClampedToAnchorPeriod(t *testing.T) {
	generated := wib(2026, 9, 1, 0).Add(2 * time.Minute)
	anchor := &payslipAnchor{ID: "L", PeriodYear: 2026, PeriodMonth: 9, GeneratedAt: generated}
	now := wib(2026, 10, 20, 12)
	items := []hrisrepo.PayslipReimbursement{
		reimb("aug-paid", 50_000, time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC), "paid", timePtr(generated.Add(-4*time.Minute))),
		reimb("race", 60_000, time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC), "paid", timePtr(generated.Add(-time.Minute))),
	}
	got := selectCarryOverReimbursements(2026, 10, now, anchor, map[string]bool{}, items)
	equalIDs(t, reimbursementIDs(got), "race")
}

// D9: a period between L and M (missed by L) is included even when it was
// approved before L.
func TestCarryOverBonusesPeriodAfterAnchor(t *testing.T) {
	anchor := &payslipAnchor{ID: "L", PeriodYear: 2026, PeriodMonth: 7, GeneratedAt: wib(2026, 7, 24, 9)}
	bonuses := []model.BonusRecord{bonus("aug", 1_000_000, 8, 2026, "approved", timePtr(wib(2026, 7, 1, 9)))}
	got := selectCarryOverBonuses(2026, 9, anchor, map[string]bool{}, bonuses)
	equalIDs(t, bonusIDs(got), "aug")
}

func reimb(id string, amount int64, transacted time.Time, status string, paidAt *time.Time) hrisrepo.PayslipReimbursement {
	return hrisrepo.PayslipReimbursement{ID: id, Title: "R " + id, Category: "Transportasi", Amount: amount, TransactionDate: transacted, Status: status, PaidAt: paidAt}
}

// D9: the first slip's floor is the first day of the period (00:00 WIB). A
// reimbursement transacted last month but paid this month is included
// (demo R8); one paid last month is not.
func TestCarryOverReimbursementsFirstSlipFloor(t *testing.T) {
	now := wib(2026, 10, 2, 12)
	items := []hrisrepo.PayslipReimbursement{
		reimb("r5", 275_000, time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 9, 25, 10))),
		reimb("r8", 120_000, time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 9, 25, 10))),
		reimb("old", 50_000, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 8, 31, 23))),
		reimb("edge", 10_000, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 9, 1, 0))),
		reimb("a1", 95_000, time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC), "approved", nil),
		reimb("future", 1, time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 10, 5, 10))),
	}
	got := selectCarryOverReimbursements(2026, 9, now, nil, map[string]bool{}, items)
	equalIDs(t, reimbursementIDs(got), "r8", "edge", "r5")
}

// D9: after a sent slip L the floor is L's generation time, so an item paid
// after L was generated rolls to the next slip and items on L stay off.
func TestCarryOverReimbursementsAfterSentSlip(t *testing.T) {
	anchor := &payslipAnchor{ID: "L", PeriodYear: 2026, PeriodMonth: 9, GeneratedAt: wib(2026, 9, 24, 9)}
	now := wib(2026, 10, 20, 12)
	items := []hrisrepo.PayslipReimbursement{
		reimb("on-L", 185_000, time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 9, 23, 10))),
		reimb("late", 350_000, time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 9, 25, 10))),
		reimb("oct", 99_000, time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 10, 10, 10))),
	}
	got := selectCarryOverReimbursements(2026, 10, now, anchor, map[string]bool{"on-l": true}, items)
	equalIDs(t, reimbursementIDs(got), "late", "oct")
}

func demoEmployee() hrisrepo.PayslipEmployee {
	userID := "u-budi"
	return hrisrepo.PayslipEmployee{
		ID:                "e-budi",
		UserID:            &userID,
		FullName:          "Budi Santoso",
		Email:             "budi@example.com",
		LoginEmail:        strPtr("staff.ops@kantor.local"),
		Position:          "Full Time",
		Department:        strPtr("Engineering"),
		DateJoined:        time.Date(2024, 3, 4, 0, 0, 0, 0, time.UTC),
		EmploymentStatus:  "active",
		BankAccountNumber: strPtr("9990000000002"),
		BankName:          strPtr("Mandiri"),
		CreatedAt:         wib(2026, 10, 1, 9),
		JobTitle:          strPtr("Backend Engineer"),
	}
}

func budiSalary() *model.SalaryRecord {
	return &model.SalaryRecord{
		ID:         "s-budi",
		BaseSalary: 12_500_000,
		Allowances: map[string]int64{"Transport": 750_000, "Makan": 1_100_000, "Internet": 300_000},
		Deductions: map[string]int64{"BPJS Kesehatan": 120_000, "BPJS Ketenagakerjaan": 375_000, "PPh 21": 500_000},
	}
}

// Budi, September 2026 (acceptance numbers): gaji 17.650.000, potongan
// 995.000, reimbursement 535.000, diterima 17.190.000.
func TestAssemblePayslipTotalsAndPayload(t *testing.T) {
	in := payslipAssemblyInput{
		Year:     2026,
		Month:    9,
		Now:      wib(2026, 10, 2, 12),
		Employee: demoEmployee(),
		Salary:   budiSalary(),
		Bonuses: []model.BonusRecord{
			bonus("thr", 11_000_000, 3, 2026, "approved", timePtr(wib(2026, 3, 20, 10))),
			bonus("kinerja", 3_000_000, 9, 2026, "approved", timePtr(wib(2026, 10, 1, 10))),
		},
		Reimbursements: []hrisrepo.PayslipReimbursement{
			reimb("r1", 185_000, time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 9, 25, 10))),
			reimb("r2", 350_000, time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 9, 25, 10))),
		},
		Company: authrepo.CompanyProfileRecord{LegalName: "PT Contoh Teknologi Nusantara", DocCode: "CTN", HRContactEmail: "hr@contoh.co.id", PaydayDay: 25},
	}
	asm := assemblePayslip(in)
	if asm.Blocked {
		t.Fatalf("unexpected block: %+v", asm.Warnings)
	}
	want := model.PayslipTotals{TotalGaji: 17_650_000, TotalPotongan: 995_000, TotalReimbursement: 535_000, TotalDiterima: 17_190_000}
	if asm.Amounts.Totals != want {
		t.Fatalf("totals = %+v, want %+v", asm.Amounts.Totals, want)
	}
	equalIDs(t, asm.BonusIDs, "kinerja")
	equalIDs(t, asm.ReimbursementIDs, "r1", "r2")
	if asm.SalaryID == nil || *asm.SalaryID != "s-budi" {
		t.Fatalf("salary id = %v", asm.SalaryID)
	}

	employee := in.Employee
	employee.EmployeeCode = strPtr("CTN-0002")
	payload := buildPayslipPayload(payslipHeader{
		DocNumber: "PAY/2026/09/0002", PayDate: DefaultPayslipPayDate(2026, 9, 25), Year: 2026, Month: 9,
		Employee: employee, JobTitle: asm.JobTitle, StatusKerja: asm.StatusKerja, Company: in.Company,
	}, asm.Amounts, "")

	checks := map[string]string{
		"total_gaji":               "17.650.000",
		"total_potongan":           "995.000",
		"total_reimbursement":      "535.000",
		"total_diterima":           "17.190.000",
		"total_diterima_terbilang": "tujuh belas juta seratus sembilan puluh ribu rupiah",
		"karyawan_norek":           "******0002",
		"karyawan_status_kerja":    "PKWTT / Permanent",
		"karyawan_jabatan":         "Backend Engineer",
		"karyawan_id":              "CTN-0002",
		"karyawan_email":           "staff.ops@kantor.local",
		"tanggal_bayar":            "25 September 2026",
		"periode":                  "September 2026",
		"catatan_slip":             "",
	}
	for key, want := range checks {
		if got := payload[key]; got != want {
			t.Errorf("payload[%s] = %v, want %q", key, got, want)
		}
	}
	gaji := payload["gaji"].([]map[string]string)
	wantNames := []string{
		"Gaji Pokok / Base Salary",
		"Tunjangan Internet / Internet Allowance",
		"Tunjangan Makan / Meal Allowance",
		"Tunjangan Transport / Transport Allowance",
		"Bonus",
	}
	if len(gaji) != len(wantNames) {
		t.Fatalf("gaji rows = %d (%v)", len(gaji), gaji)
	}
	for index, name := range wantNames {
		if gaji[index]["nama"] != name {
			t.Errorf("gaji[%d] = %q, want %q", index, gaji[index]["nama"], name)
		}
		if strings.HasPrefix(gaji[index]["jumlah"], "-") {
			t.Errorf("gaji row %d is negative: %v", index, gaji[index])
		}
	}
	potongan := payload["potongan"].([]map[string]string)
	if len(potongan) != 3 || potongan[0]["nama"] != "BPJS Kesehatan / Health Insurance" || potongan[2]["jumlah"] != "500.000" {
		t.Fatalf("potongan = %v", potongan)
	}

	// The payload renders against the real template.
	tpl, err := docgen.Load(docgen.TemplateSlip)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := docgen.Render(tpl, payload); err != nil {
		t.Fatalf("render: %v", err)
	}
}

// D3: without deductions the potongan table gets one placeholder row.
func TestAssemblePayslipNoDeductionsRow(t *testing.T) {
	employee := demoEmployee()
	employee.Position = "Internship"
	salary := &model.SalaryRecord{ID: "s", BaseSalary: 3_500_000, Allowances: map[string]int64{"Transport": 500_000, "Makan": 600_000}, Deductions: map[string]int64{}}
	asm := assemblePayslip(payslipAssemblyInput{Year: 2026, Month: 9, Now: wib(2026, 10, 1, 0), Employee: employee, Salary: salary, Company: authrepo.CompanyProfileRecord{DocCode: "CTN"}})
	if asm.Amounts.Totals.TotalGaji != 4_600_000 || asm.Amounts.Totals.TotalPotongan != 0 || asm.Amounts.Totals.TotalDiterima != 4_600_000 {
		t.Fatalf("totals = %+v", asm.Amounts.Totals)
	}
	payload := docgen.Payload{}
	applyAmountsToPayload(payload, asm.Amounts, "")
	potongan := payload["potongan"].([]map[string]string)
	if len(potongan) != 1 || potongan[0]["nama"] != "Tidak ada potongan / No deductions" || potongan[0]["jumlah"] != "0" || potongan[0]["keterangan"] != "" {
		t.Fatalf("potongan = %v", potongan)
	}
	if asm.StatusKerja != "Magang / Internship" {
		t.Fatalf("status kerja = %q", asm.StatusKerja)
	}
}

// Manual lines: positive ones are earnings, negative ones deductions shown
// as positive amounts; never a negative row in the gaji table.
func TestManualLinesSplitIntoEarningsAndDeductions(t *testing.T) {
	amounts := model.PayslipAmounts{
		Earnings:   []model.PayslipLine{{Kind: model.PayslipLineBase, Label: "Gaji Pokok / Base Salary", Amount: 8_000_000}},
		Deductions: []model.PayslipLine{{Kind: model.PayslipLineDeduction, Label: "PPh 21 / Income Tax", Amount: 150_000}},
		ManualLines: []model.PayslipManualLine{
			{Label: "Penyesuaian pro-rata", Amount: -2_000_000, Keterangan: "15 hari"},
			{Label: "Lembur", Amount: 250_000},
		},
		Reimbursements: []model.PayslipReimbursementLine{{ID: "r", TransactionDate: "2026-09-03", Title: "Grab", Category: "Transportasi", Amount: 100_000}},
	}
	amounts.Totals = computePayslipTotals(amounts)
	want := model.PayslipTotals{TotalGaji: 8_250_000, TotalPotongan: 2_150_000, TotalReimbursement: 100_000, TotalDiterima: 6_200_000}
	if amounts.Totals != want {
		t.Fatalf("totals = %+v, want %+v", amounts.Totals, want)
	}
	payload := docgen.Payload{}
	applyAmountsToPayload(payload, amounts, "Catatan\nsatu baris")
	potongan := payload["potongan"].([]map[string]string)
	if len(potongan) != 2 || potongan[1]["nama"] != "Penyesuaian pro-rata" || potongan[1]["jumlah"] != "2.000.000" {
		t.Fatalf("potongan = %v", potongan)
	}
	if payload["catatan_slip"] != "Catatan satu baris" {
		t.Fatalf("catatan = %q", payload["catatan_slip"])
	}
	if payload["total_diterima_terbilang"] != "enam juta dua ratus ribu rupiah" {
		t.Fatalf("terbilang = %q", payload["total_diterima_terbilang"])
	}
	reimbursement := payload["reimbursement"].([]map[string]string)
	if reimbursement[0]["tanggal"] != "03-09-2026" {
		t.Fatalf("reimbursement date = %q", reimbursement[0]["tanggal"])
	}
}

func TestDefaultPayslipPayDate(t *testing.T) {
	cases := []struct {
		year, month, payday int
		want                string
	}{
		{2026, 10, 25, "2026-10-23"}, // Sunday -> Friday
		{2026, 9, 25, "2026-09-25"},  // Friday
		{2026, 4, 25, "2026-04-24"},  // Saturday -> Friday
		{2026, 2, 31, "2026-02-27"},  // clamped to 28 Feb (Saturday) -> Friday
		{2026, 11, 30, "2026-11-30"}, // Monday
		{2026, 5, 0, "2026-05-25"},   // unset -> default 25 (Monday)
	}
	for _, tc := range cases {
		if got := DefaultPayslipPayDate(tc.year, tc.month, tc.payday).Format("2006-01-02"); got != tc.want {
			t.Errorf("DefaultPayslipPayDate(%d, %d, %d) = %s, want %s", tc.year, tc.month, tc.payday, got, tc.want)
		}
	}
}

func TestPayslipNumbering(t *testing.T) {
	if got := FormatPayslipNumber(2026, 9, 2, 0); got != "PAY/2026/09/0002" {
		t.Fatalf("number = %s", got)
	}
	if got := FormatPayslipNumber(2026, 10, 21, 1); got != "PAY/2026/10/0021-R1" {
		t.Fatalf("number = %s", got)
	}
	if got := payslipBaseNumber("PAY/2026/10/0021-R12"); got != "PAY/2026/10/0021" {
		t.Fatalf("base = %s", got)
	}
	if got := payslipBaseNumber("PAY/2026/10/0021"); got != "PAY/2026/10/0021" {
		t.Fatalf("base = %s", got)
	}
}

func warningCodes(warnings []model.PayslipWarning) map[string]model.PayslipWarning {
	result := map[string]model.PayslipWarning{}
	for _, item := range warnings {
		result[item.Code] = item
	}
	return result
}

func TestAssemblePayslipWarnings(t *testing.T) {
	t.Run("no salary blocks", func(t *testing.T) {
		asm := assemblePayslip(payslipAssemblyInput{Year: 2026, Month: 9, Now: wib(2026, 10, 1, 0), Employee: demoEmployee(), Company: authrepo.CompanyProfileRecord{DocCode: "CTN"}})
		codes := warningCodes(asm.Warnings)
		if !asm.Blocked || !codes[PayslipWarnNoSalary].Blocking {
			t.Fatalf("expected blocking no_salary, got %+v", asm.Warnings)
		}
		if !strings.Contains(strings.ToLower(codes[PayslipWarnNoSalary].Message), "tanpa data gaji") {
			t.Fatalf("message = %q", codes[PayslipWarnNoSalary].Message)
		}
	})

	t.Run("data checks", func(t *testing.T) {
		employee := demoEmployee()
		employee.JobTitle = nil
		employee.Position = "Belum Ditentukan"
		employee.BankAccountNumber = nil
		employee.DateJoined = time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
		employee.CreatedAt = wib(2026, 9, 16, 8)
		salary := &model.SalaryRecord{ID: "s", BaseSalary: 1, Allowances: map[string]int64{}, Deductions: map[string]int64{}}
		asm := assemblePayslip(payslipAssemblyInput{
			Year: 2026, Month: 9, Now: wib(2026, 10, 1, 0), Employee: employee, Salary: salary,
			Bonuses: []model.BonusRecord{bonus("p", 2_000_000, 9, 2026, "pending", nil)},
			Reimbursements: []hrisrepo.PayslipReimbursement{
				reimb("a1", 95_000, time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC), "approved", nil),
			},
			Company: authrepo.CompanyProfileRecord{},
		})
		if asm.Blocked {
			t.Fatalf("non-blocking warnings must not block: %+v", asm.Warnings)
		}
		codes := warningCodes(asm.Warnings)
		for _, code := range []string{
			PayslipWarnBaseSalaryLow, PayslipWarnJoinedInMonth, PayslipWarnBonusPending, PayslipWarnReimbursementUnpd,
			PayslipWarnJobTitleMissing, PayslipWarnStatusKerjaMissing, PayslipWarnDocCodeMissing, PayslipWarnBankMissing,
			PayslipWarnJoinedEqualsCreate,
		} {
			if _, ok := codes[code]; !ok {
				t.Errorf("missing warning %s in %+v", code, asm.Warnings)
			}
		}
		if got := codes[PayslipWarnBaseSalaryLow].Message; !strings.Contains(got, "Rp1 ") {
			t.Errorf("base salary message = %q", got)
		}
		if asm.JobTitle != "-" || asm.StatusKerja != "-" {
			t.Errorf("jabatan/status = %q/%q", asm.JobTitle, asm.StatusKerja)
		}
	})

	t.Run("contract source wins", func(t *testing.T) {
		employee := demoEmployee()
		employee.JobTitle = nil
		asm := assemblePayslip(payslipAssemblyInput{
			Year: 2026, Month: 9, Now: wib(2026, 10, 1, 0), Employee: employee, Salary: budiSalary(),
			Contract: &PayslipContractInfo{ContractID: "c1", JobTitle: "AI Engineer", StatusKerja: "PKWT"},
			Company:  authrepo.CompanyProfileRecord{DocCode: "CTN"},
		})
		if asm.JobTitle != "AI Engineer" || asm.StatusKerja != "PKWT" || asm.ContractID == nil || *asm.ContractID != "c1" {
			t.Fatalf("contract info not applied: %+v", asm)
		}
		if _, ok := warningCodes(asm.Warnings)[PayslipWarnJobTitleMissing]; ok {
			t.Fatal("job title warning despite contract job title")
		}
	})

	t.Run("many rows", func(t *testing.T) {
		// 5 earnings + 4 deductions + 4 reimbursements: 13 rows spill onto a
		// second page (measured), although no single table is long.
		salary := budiSalary()
		salary.Deductions["Koperasi"] = 50_000
		reimbursements := []hrisrepo.PayslipReimbursement{}
		for index := 0; index < 4; index++ {
			reimbursements = append(reimbursements, reimb(string(rune('a'+index)), 10_000, time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 9, 25, 10))))
		}
		in := payslipAssemblyInput{Year: 2026, Month: 9, Now: wib(2026, 10, 1, 0), Employee: demoEmployee(), Salary: salary,
			Bonuses:        []model.BonusRecord{bonus("b", 1, 9, 2026, "approved", timePtr(wib(2026, 9, 2, 0)))},
			Reimbursements: reimbursements, Company: authrepo.CompanyProfileRecord{DocCode: "CTN"}}
		asm := assemblePayslip(in)
		if _, ok := warningCodes(asm.Warnings)[PayslipWarnManyRows]; !ok {
			t.Fatalf("5/4/4 must warn: %+v", asm.Warnings)
		}

		// 5/3/4 fits (12 rows).
		delete(salary.Deductions, "Koperasi")
		if _, ok := warningCodes(assemblePayslip(in).Warnings)[PayslipWarnManyRows]; ok {
			t.Fatal("5/3/4 must not warn")
		}
		// ... but not with a catatan.
		in.Note = "Penyesuaian pro-rata"
		if _, ok := warningCodes(assemblePayslip(in).Warnings)[PayslipWarnManyRows]; !ok {
			t.Fatal("5/3/4 with a catatan must warn")
		}
	})
}

// The row budget counts a cell that wraps as two rows.
func TestPayslipPageUnits(t *testing.T) {
	amounts := model.PayslipAmounts{
		Earnings: []model.PayslipLine{
			{Label: "Gaji Pokok / Base Salary", Amount: 1},
			{Label: "Bonus", Keterangan: "Bonus kinerja kuartal tiga 2026", Amount: 1}, // 30 runes: wraps
		},
		Reimbursements: []model.PayslipReimbursementLine{{Title: "Taksi", Category: "Transportasi", Amount: 1}},
	}
	// 2 earnings (one wrapped = 3) + the "Tidak ada potongan" row + 1.
	if got := payslipPageUnits(amounts, ""); got != 5 {
		t.Fatalf("units = %d, want 5", got)
	}
	if got := payslipPageUnits(amounts, strings.Repeat("x", 100)); got != 7 {
		t.Fatalf("units with a long note = %d, want 7", got)
	}
}

// D3: a negative salary component never prints a negative gaji row: a
// negative allowance is a deduction and a negative deduction an earning,
// with the same totals.
func TestSalaryLinesNegativeComponents(t *testing.T) {
	salary := model.SalaryRecord{
		ID:         "s",
		BaseSalary: 10_000_000,
		Allowances: map[string]int64{"Transport": 500_000, "Koreksi absen": -500_000},
		Deductions: map[string]int64{"PPh 21": 300_000, "BPJS Kesehatan": -100_000},
	}
	earnings, deductions := salaryLines(salary)
	for _, line := range append(append([]model.PayslipLine{}, earnings...), deductions...) {
		if line.Amount <= 0 {
			t.Fatalf("non-positive row %+v", line)
		}
	}
	names := []string{}
	for _, line := range earnings {
		names = append(names, line.Label)
	}
	if strings.Join(names, "|") != "Gaji Pokok / Base Salary|Tunjangan Transport / Transport Allowance|Pengembalian BPJS Kesehatan / Health Insurance Refund" {
		t.Fatalf("earnings = %v", names)
	}
	if len(deductions) != 2 || deductions[0].Label != "Potongan Koreksi absen / Koreksi absen Deduction" || deductions[0].Amount != 500_000 {
		t.Fatalf("deductions = %+v", deductions)
	}
	totals := computePayslipTotals(model.PayslipAmounts{Earnings: earnings, Deductions: deductions})
	if totals.TotalDiterima != 10_000_000+500_000-500_000-300_000+100_000 {
		t.Fatalf("totals = %+v", totals)
	}
}

// The pay date range starts at the weekday on or before the 1st, so the
// default (a weekend payday moved back into the previous month) is valid.
func TestPayslipPayDateBounds(t *testing.T) {
	def := DefaultPayslipPayDate(2026, 11, 1) // 1 Nov 2026 is a Sunday
	if got := def.Format("2006-01-02"); got != "2026-10-30" {
		t.Fatalf("default = %s", got)
	}
	first, last := PayslipPayDateBounds(2026, 11)
	if first.Format("2006-01-02") != "2026-10-30" || last.Format("2006-01-02") != "2026-12-31" {
		t.Fatalf("bounds = %s .. %s", first.Format("2006-01-02"), last.Format("2006-01-02"))
	}
	s := &PayslipsService{}
	raw := "2026-10-30"
	if got, err := s.parsePayDate(2026, 11, &raw, 1); err != nil || !got.Equal(def) {
		t.Fatalf("default rejected: %v %v", got, err)
	}
	early := "2026-10-29"
	if _, err := s.parsePayDate(2026, 11, &early, 1); err == nil {
		t.Fatal("a date before the bound must be rejected")
	}
	// A weekday 1st keeps the 1st as the lower bound.
	if first, _ := PayslipPayDateBounds(2026, 10); first.Format("2006-01-02") != "2026-10-01" {
		t.Fatalf("october bound = %s", first.Format("2006-01-02"))
	}
}

// Void & reissue keeps exactly the voided slip's items: an item paid after
// the original was generated stays off (it belongs to the next slip), a
// rejected bonus stays on with a warning, a deleted reimbursement keeps its
// old line, and the generation time is the original's.
func TestAssembleReissueKeepsItems(t *testing.T) {
	original := wib(2026, 9, 29, 9)
	previous := model.PayslipAmounts{
		Earnings: []model.PayslipLine{{Kind: model.PayslipLineBonus, Label: "Bonus", Amount: 3_000_000, SourceID: "kinerja"}},
		Reimbursements: []model.PayslipReimbursementLine{
			{ID: "r1", TransactionDate: "2026-09-03", Title: "Grab", Category: "Transportasi", Amount: 185_000},
			{ID: "gone", TransactionDate: "2026-09-04", Title: "Parkir", Category: "Transportasi", Amount: 20_000},
		},
	}
	in := payslipAssemblyInput{
		Year: 2026, Month: 9, Now: original, Employee: demoEmployee(), Salary: budiSalary(),
		Bonuses: []model.BonusRecord{
			bonus("kinerja", 3_000_000, 9, 2026, "approved", timePtr(wib(2026, 9, 28, 10))),
			bonus("late", 1_000_000, 9, 2026, "approved", timePtr(wib(2026, 10, 5, 10))),
		},
		Company: authrepo.CompanyProfileRecord{DocCode: "CTN"},
		Reissue: &payslipReissue{
			BonusIDs:         []string{"kinerja"},
			ReimbursementIDs: []string{"r1", "gone"},
			Reimbursements: []hrisrepo.PayslipReimbursement{
				reimb("r1", 185_000, time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), "paid", timePtr(wib(2026, 9, 25, 10))),
			},
			Previous: previous,
		},
	}
	asm := assemblePayslip(in)
	equalIDs(t, asm.BonusIDs, "kinerja")
	equalIDs(t, asm.ReimbursementIDs, "r1", "gone")
	if asm.Amounts.Totals.TotalReimbursement != 205_000 || !asm.GeneratedAt.Equal(original) {
		t.Fatalf("totals %+v generated %v", asm.Amounts.Totals, asm.GeneratedAt)
	}
	if _, ok := warningCodes(asm.Warnings)[PayslipWarnReissueItemChanged]; !ok {
		t.Fatalf("a deleted item must warn: %+v", asm.Warnings)
	}

	in.Bonuses[0].ApprovalStatus = "rejected"
	in.Reissue.ReimbursementIDs = []string{"r1"}
	asm = assemblePayslip(in)
	equalIDs(t, asm.BonusIDs, "kinerja")
	if _, ok := warningCodes(asm.Warnings)[PayslipWarnReissueItemChanged]; !ok {
		t.Fatalf("a rejected bonus must warn: %+v", asm.Warnings)
	}
}

func TestPayslipRowNames(t *testing.T) {
	cases := map[string]string{
		"Transport":         "Tunjangan Transport / Transport Allowance",
		"Tunjangan Jabatan": "Tunjangan Jabatan / Position Allowance",
		"Kopi":              "Tunjangan Kopi / Kopi Allowance",
	}
	for label, want := range cases {
		if got := allowanceRowName(label); got != want {
			t.Errorf("allowanceRowName(%q) = %q, want %q", label, got, want)
		}
	}
	if got := deductionRowName("PPh 21"); got != "PPh 21 / Income Tax" {
		t.Errorf("deduction = %q", got)
	}
	if got := deductionRowName("Koperasi"); got != "Potongan Koperasi / Koperasi Deduction" {
		t.Errorf("deduction = %q", got)
	}
}

func TestNormalizePayslipNote(t *testing.T) {
	if note, ok := normalizePayslipNote("  baris\nkedua  "); !ok || note != "baris kedua" {
		t.Fatalf("note = %q ok=%v", note, ok)
	}
	if _, ok := normalizePayslipNote(strings.Repeat("a", 121)); ok {
		t.Fatal("121 characters must be rejected")
	}
	if _, ok := normalizePayslipNote(strings.Repeat("é", 120)); !ok {
		t.Fatal("120 runes must be accepted")
	}
}

func TestMaskEmail(t *testing.T) {
	cases := map[string]string{
		"staff.ops@kantor.local": "st***@kantor.local",
		"a@b.co":                 "a***@b.co",
		"broken":                 "***",
	}
	for in, want := range cases {
		if got := MaskEmail(in); got != want {
			t.Errorf("MaskEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmployeeBankMasking(t *testing.T) {
	employee := model.Employee{BankAccountNumber: strPtr("9990000000002")}
	masked := MaskEmployeeBankAccount(employee)
	if *masked.BankAccountNumber != "******0002" {
		t.Fatalf("masked = %q", *masked.BankAccountNumber)
	}
	if *employee.BankAccountNumber != "9990000000002" {
		t.Fatal("masking must not modify the input")
	}
	if MaskEmployeeBankAccount(model.Employee{}).BankAccountNumber != nil {
		t.Fatal("nil stays nil")
	}
	if !IsMaskedBankAccount(strPtr("******0002")) || IsMaskedBankAccount(strPtr("1234567890")) || IsMaskedBankAccount(nil) {
		t.Fatal("IsMaskedBankAccount mismatch")
	}
}
