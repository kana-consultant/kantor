package hris

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kana-consultant/kantor/backend/internal/docgen"
	"github.com/kana-consultant/kantor/backend/internal/model"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
)

// Payslip assembly: pure functions that turn the loaded data (salary row in
// force, bonuses, reimbursements, the previous sent slip, manual lines) into
// the slip rows, totals, payload and warnings. PayslipsService loads the data
// and stores the result; everything here is deterministic and unit-tested.

const (
	// payslipMinBaseSalary: a base salary below Rp100.000 is almost always a
	// typo (e.g. 'Gaji pokok Rp1') and is flagged.
	payslipMinBaseSalary int64 = 100_000
	// payslipNoteMaxRunes keeps catatan_slip on ONE short line: 08 only just
	// fits one A4 page with an empty note.
	payslipNoteMaxRunes = 120
	// payslipManualLinesMax caps HR adjustments per slip.
	payslipManualLinesMax = 5
	// payslipRowBudget is how many table rows (earnings + deductions +
	// reimbursements, a wrapped row counting twice) 08_Slip_Gaji fits on
	// one A4 page with an empty catatan: 5+4+3 and 5+3+4 fit, 5+4+4 spills
	// onto a second page. A catatan takes one row (two when it wraps). This
	// is the early hint; the page count of the rendered PDF is authoritative
	// (PayslipWarnPDFPages).
	payslipRowBudget = 12
	// Above these widths a cell wraps to a second line (9 pt in the
	// template's columns: nama 4600, keterangan 2638, judul 3700 and
	// kategori 2038 twips).
	payslipNameWrapRunes       = 44
	payslipKeteranganWrapRunes = 24
	payslipTitleWrapRunes      = 36
	payslipCategoryWrapRunes   = 18
	payslipNoteWrapRunes       = 90
	// payslipKeteranganMaxRunes keeps a row description to one short line
	// (it may still wrap once in the keterangan column).
	payslipKeteranganMaxRunes = 60
	// payslipCarryOverGrace covers the read race of a sent slip L: an item
	// stamped (approved_at / paid_at) just before L.generated_at whose
	// transaction committed after L's data was read is not lost. It only
	// widens the floor for items L's own selection would have taken had it
	// seen them (a bonus of L's period; a reimbursement paid within L's
	// period), never for an older-period bonus approved before L: that one
	// L excluded on purpose (D9 go-live floor), and widening for it would
	// pay e.g. a historical THR approved minutes before the first slip.
	payslipCarryOverGrace = 5 * time.Minute
)

// Warning codes (model.PayslipWarning.Code).
const (
	PayslipWarnNoSalary           = "no_salary"
	PayslipWarnBaseSalaryLow      = "base_salary_low"
	PayslipWarnJoinedInMonth      = "joined_in_month"
	PayslipWarnBonusPending       = "bonus_pending"
	PayslipWarnReimbursementUnpd  = "reimbursement_unpaid"
	PayslipWarnJobTitleMissing    = "job_title_missing"
	PayslipWarnStatusKerjaMissing = "status_kerja_missing"
	PayslipWarnDocCodeMissing     = "doc_code_missing"
	PayslipWarnBankMissing        = "bank_missing"
	PayslipWarnEmailMissing       = "email_missing"
	PayslipWarnJoinedEqualsCreate = "joined_equals_created"
	PayslipWarnManyRows           = "many_rows"
	PayslipWarnTotalNotPositive   = "total_not_positive"
	// PayslipWarnPDFPages is set by the render worker when the PDF is not
	// exactly one page (it replaces the many_rows hint once a PDF exists).
	PayslipWarnPDFPages = "pdf_pages"
	// PayslipWarnReissueItemChanged: a void & reissue keeps the items of the
	// voided slip, but one of them changed status or was deleted since.
	PayslipWarnReissueItemChanged = "reissue_item_changed"
)

// payslipLocation is the payroll calendar (WIB). time/tzdata is embedded in
// the server binary; the fixed offset is only a fallback (Indonesia has no
// DST).
var payslipLocation = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Jakarta"); err == nil {
		return loc
	}
	return time.FixedZone("WIB", 7*60*60)
}()

// PayslipContractInfo is what an active employment contract contributes to a
// payslip. The contracts phase implements PayslipContractSource; until then
// the job title comes from the HR profile and the status from the
// employment type.
type PayslipContractInfo struct {
	ContractID  string
	JobTitle    string
	StatusKerja string
}

type payslipAnchor = hrisrepo.PayslipAnchor

// payslipAssemblyInput is everything one slip is built from.
type payslipAssemblyInput struct {
	Year           int
	Month          int
	Now            time.Time
	Employee       hrisrepo.PayslipEmployee
	Salary         *model.SalaryRecord
	Bonuses        []model.BonusRecord
	Reimbursements []hrisrepo.PayslipReimbursement
	// Anchor is the employee's latest sent slip before the period (D9).
	Anchor           *payslipAnchor
	ConsumedBonus    map[string]bool
	ConsumedReimburs map[string]bool
	ManualLines      []model.PayslipManualLine
	// Note is the catatan of the slip (it takes room on the page).
	Note     string
	Contract *PayslipContractInfo
	Company  authrepo.CompanyProfileRecord
	// Reissue pins a void & reissue to the items of the slip it replaces
	// (nil: the D9 carry-over selection).
	Reissue *payslipReissue
}

// payslipReissue is what a void & reissue keeps from the voided slip: its
// exact bonus and reimbursement ids (D9: the same items, never items paid or
// approved since, which belong to the next slip). The records are reloaded,
// so a corrected amount is picked up; a deleted record keeps its line from
// the voided snapshot.
type payslipReissue struct {
	BonusIDs         []string
	ReimbursementIDs []string
	// Reimbursements are the records of ReimbursementIDs, any status.
	Reimbursements []hrisrepo.PayslipReimbursement
	Previous       model.PayslipAmounts
}

type payslipAssembly struct {
	Amounts          model.PayslipAmounts
	BonusIDs         []string
	ReimbursementIDs []string
	Warnings         []model.PayslipWarning
	Blocked          bool
	JobTitle         string
	StatusKerja      string
	ContractID       *string
	SalaryID         *string
	// GeneratedAt is the assembly's clock (input Now): stored as the slip's
	// generated_at, the next slip's carry-over floor.
	GeneratedAt time.Time
}

func periodIndex(year int, month int) int {
	return year*12 + month
}

// payslipPeriodBounds returns the first instant of the period and its last
// calendar day, both in the payroll timezone.
func payslipPeriodBounds(year int, month int) (time.Time, time.Time) {
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, payslipLocation)
	lastDay := start.AddDate(0, 1, -1)
	return start, lastDay
}

// selectCarryOverBonuses applies decision D9 to bonuses. A bonus goes on the
// slip of period M when it is approved, not on any sent (non-void) slip, its
// period is not after M, and
//   - its period is M, or
//   - the employee has a sent slip L before M and the bonus period is after
//     L's period (it was missed by L), or
//   - L exists and the bonus was approved after L was generated (a late
//     approval of an older period): strictly approved_at > L.generated_at.
//
// The read-race grace (payslipCarryOverGrace) applies only to a bonus of L's
// own period, which L always takes when it sees it approved; an older-period
// bonus approved before L.generated_at was left off L by the go-live floor
// and stays off.
//
// Without L only period == M counts: the first slip ever generated must not
// sweep up every historical bonus (go-live floor). consumed holds the ids on
// sent slips and on drafts of earlier periods. A void & reissue does not use
// this selection: it keeps the voided slip's items (payslipReissue).
func selectCarryOverBonuses(year int, month int, anchor *payslipAnchor, consumed map[string]bool, bonuses []model.BonusRecord) []model.BonusRecord {
	target := periodIndex(year, month)
	selected := make([]model.BonusRecord, 0)
	for _, bonus := range bonuses {
		if bonus.ApprovalStatus != "approved" || consumed[strings.ToLower(bonus.ID)] {
			continue
		}
		period := periodIndex(bonus.PeriodYear, bonus.PeriodMonth)
		if period > target {
			continue
		}
		include := period == target
		if !include && anchor != nil {
			anchorPeriod := periodIndex(anchor.PeriodYear, anchor.PeriodMonth)
			switch {
			case period > anchorPeriod:
				include = true
			case bonus.ApprovedAt == nil:
			case bonus.ApprovedAt.After(anchor.GeneratedAt):
				include = true
			case period == anchorPeriod && bonus.ApprovedAt.After(anchor.GeneratedAt.Add(-payslipCarryOverGrace)):
				include = true
			}
		}
		if include {
			selected = append(selected, bonus)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		pi, pj := periodIndex(selected[i].PeriodYear, selected[i].PeriodMonth), periodIndex(selected[j].PeriodYear, selected[j].PeriodMonth)
		if pi != pj {
			return pi < pj
		}
		return selected[i].CreatedAt.Before(selected[j].CreatedAt)
	})
	return selected
}

// selectCarryOverReimbursements applies decision D9 to reimbursements. A
// reimbursement goes on the slip of period M when it is paid, not on any
// sent (non-void) slip, paid at or before now, and paid at or after the
// floor: the first day of M (00:00 WIB) when the employee has no earlier
// sent slip, otherwise the generation time of that slip L (less
// payslipCarryOverGrace, but never before the first day of L's period, so
// the grace cannot reach items below L's own floor). generated_at is the same clock as the ceiling
// (now) of L's own selection, so items paid after a slip was generated roll
// to the next slip instead of falling between windows; a reimbursement
// transacted last month but paid in M is included (it is the payment date
// that counts).
func selectCarryOverReimbursements(year int, month int, now time.Time, anchor *payslipAnchor, consumed map[string]bool, items []hrisrepo.PayslipReimbursement) []hrisrepo.PayslipReimbursement {
	floor, _ := payslipPeriodBounds(year, month)
	if anchor != nil {
		floor = anchor.GeneratedAt.Add(-payslipCarryOverGrace)
		if anchorStart, _ := payslipPeriodBounds(anchor.PeriodYear, anchor.PeriodMonth); floor.Before(anchorStart) {
			floor = anchorStart
		}
	}
	selected := make([]hrisrepo.PayslipReimbursement, 0)
	for _, item := range items {
		if item.Status != "paid" || item.PaidAt == nil || consumed[strings.ToLower(item.ID)] {
			continue
		}
		if item.PaidAt.After(now) || item.PaidAt.Before(floor) {
			continue
		}
		selected = append(selected, item)
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if !selected[i].TransactionDate.Equal(selected[j].TransactionDate) {
			return selected[i].TransactionDate.Before(selected[j].TransactionDate)
		}
		return selected[i].ID < selected[j].ID
	})
	return selected
}

// payslipComponentEnglish translates common salary component labels for the
// bilingual row names; unknown labels are repeated as is.
var payslipComponentEnglish = map[string]string{
	"jabatan":              "Position",
	"transport":            "Transport",
	"transportasi":         "Transport",
	"makan":                "Meal",
	"uang makan":           "Meal",
	"internet":             "Internet",
	"pulsa":                "Phone Credit",
	"komunikasi":           "Communication",
	"kesehatan":            "Health",
	"tetap":                "Fixed",
	"keluarga":             "Family",
	"perumahan":            "Housing",
	"lembur":               "Overtime",
	"kehadiran":            "Attendance",
	"bpjs kesehatan":       "Health Insurance",
	"bpjs ketenagakerjaan": "Employment Insurance",
	"pph 21":               "Income Tax",
	"pph21":                "Income Tax",
	"pinjaman":             "Loan",
	"kasbon":               "Cash Advance",
}

func englishComponent(label string) (string, bool) {
	english, ok := payslipComponentEnglish[strings.ToLower(strings.TrimSpace(label))]
	return english, ok
}

// allowanceRowName: "Tunjangan Transport / Transport Allowance". A label that
// already starts with "Tunjangan" is not doubled.
func allowanceRowName(label string) string {
	label = docgen.SingleLine(label)
	core := label
	if len(core) > len("tunjangan ") && strings.EqualFold(core[:len("tunjangan ")], "tunjangan ") {
		core = strings.TrimSpace(core[len("tunjangan "):])
	}
	english, ok := englishComponent(core)
	if !ok {
		english = core
	}
	return fmt.Sprintf("Tunjangan %s / %s Allowance", core, english)
}

// deductionRowName: "BPJS Kesehatan / Health Insurance" for known labels,
// otherwise "Potongan <label> / <label> Deduction".
func deductionRowName(label string) string {
	label = docgen.SingleLine(label)
	if english, ok := englishComponent(label); ok {
		return fmt.Sprintf("%s / %s", label, english)
	}
	core := label
	if len(core) > len("potongan ") && strings.EqualFold(core[:len("potongan ")], "potongan ") {
		core = strings.TrimSpace(core[len("potongan "):])
	}
	return fmt.Sprintf("Potongan %s / %s Deduction", core, core)
}

func sortedComponentKeys(values map[string]int64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := strings.ToLower(keys[i]), strings.ToLower(keys[j])
		if a != b {
			return a < b
		}
		return keys[i] < keys[j]
	})
	return keys
}

func truncateRunes(value string, max int) string {
	value = docgen.SingleLine(value)
	if utf8.RuneCountInString(value) <= max {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:max-1])) + "…"
}

// refundRowName: "Pengembalian BPJS Kesehatan / Health Insurance Refund".
func refundRowName(label string) string {
	label = docgen.SingleLine(label)
	english, ok := englishComponent(label)
	if !ok {
		english = label
	}
	return fmt.Sprintf("Pengembalian %s / %s Refund", label, english)
}

// salaryLines turns the salary row into earnings (base + allowances) and
// deductions. Components of zero are left out. The gaji table never has a
// negative row (D3): a negative allowance (e.g. a correction) is printed as
// a deduction and a negative deduction as an earning (a refund), both with
// a positive amount, so the totals stay the same.
func salaryLines(salary model.SalaryRecord) (earnings []model.PayslipLine, deductions []model.PayslipLine) {
	earnings = append(earnings, model.PayslipLine{
		Kind:     model.PayslipLineBase,
		Label:    "Gaji Pokok / Base Salary",
		Amount:   salary.BaseSalary,
		SourceID: salary.ID,
	})
	deductions = []model.PayslipLine{}
	refunds := []model.PayslipLine{}
	for _, key := range sortedComponentKeys(salary.Allowances) {
		amount := salary.Allowances[key]
		switch {
		case amount > 0:
			earnings = append(earnings, model.PayslipLine{Kind: model.PayslipLineAllowance, Label: allowanceRowName(key), Amount: amount})
		case amount < 0:
			deductions = append(deductions, model.PayslipLine{Kind: model.PayslipLineDeduction, Label: deductionRowName(key), Amount: -amount})
		}
	}
	for _, key := range sortedComponentKeys(salary.Deductions) {
		amount := salary.Deductions[key]
		switch {
		case amount > 0:
			deductions = append(deductions, model.PayslipLine{Kind: model.PayslipLineDeduction, Label: deductionRowName(key), Amount: amount})
		case amount < 0:
			refunds = append(refunds, model.PayslipLine{Kind: model.PayslipLineAllowance, Label: refundRowName(key), Amount: -amount})
		}
	}
	return append(earnings, refunds...), deductions
}

func bonusLine(bonus model.BonusRecord, year int, month int) model.PayslipLine {
	keterangan := docgen.SingleLine(bonus.Reason)
	if bonus.PeriodYear != year || bonus.PeriodMonth != month {
		period := docgen.PeriodID(time.Date(bonus.PeriodYear, time.Month(bonus.PeriodMonth), 1, 0, 0, 0, 0, time.UTC))
		if keterangan == "" {
			keterangan = "Periode " + period
		} else {
			keterangan = keterangan + " (" + period + ")"
		}
	}
	return model.PayslipLine{
		Kind:       model.PayslipLineBonus,
		Label:      "Bonus",
		Keterangan: truncateRunes(keterangan, payslipKeteranganMaxRunes),
		Amount:     bonus.Amount,
		SourceID:   bonus.ID,
	}
}

// payslipRows returns the rows printed in the gaji and potongan tables:
// computed rows followed by the manual lines (positive ones as earnings,
// negative ones as deductions with a positive amount).
func payslipRows(amounts model.PayslipAmounts) (earnings []model.PayslipLine, deductions []model.PayslipLine) {
	earnings = append([]model.PayslipLine{}, amounts.Earnings...)
	deductions = append([]model.PayslipLine{}, amounts.Deductions...)
	for _, line := range amounts.ManualLines {
		row := model.PayslipLine{Kind: model.PayslipLineManual, Label: docgen.SingleLine(line.Label), Keterangan: docgen.SingleLine(line.Keterangan)}
		switch {
		case line.Amount > 0:
			row.Amount = line.Amount
			earnings = append(earnings, row)
		case line.Amount < 0:
			row.Amount = -line.Amount
			deductions = append(deductions, row)
		}
	}
	return earnings, deductions
}

// computePayslipTotals: total_diterima = total_gaji - total_potongan +
// total_reimbursement (decision D3).
func computePayslipTotals(amounts model.PayslipAmounts) model.PayslipTotals {
	earnings, deductions := payslipRows(amounts)
	var totals model.PayslipTotals
	for _, line := range earnings {
		totals.TotalGaji += line.Amount
	}
	for _, line := range deductions {
		totals.TotalPotongan += line.Amount
	}
	for _, item := range amounts.Reimbursements {
		totals.TotalReimbursement += item.Amount
	}
	totals.TotalDiterima = totals.TotalGaji - totals.TotalPotongan + totals.TotalReimbursement
	return totals
}

// payslipStatusKerja maps the employment type (employees.position) when no
// active contract says otherwise. ok is false when the type is not set.
func payslipStatusKerja(position string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(position))
	switch normalized {
	case "full time":
		return "PKWTT / Permanent", true
	case "internship":
		return "Magang / Internship", true
	case "", "belum ditentukan", "-":
		return "-", false
	}
	return docgen.SingleLine(position), true
}

// defaultPayslipRecipient is the address a slip goes to by default: the
// linked user's login e-mail, else employees.email.
func defaultPayslipRecipient(employee hrisrepo.PayslipEmployee) string {
	if employee.UserID != nil && employee.LoginEmail != nil && strings.TrimSpace(*employee.LoginEmail) != "" {
		return strings.ToLower(strings.TrimSpace(*employee.LoginEmail))
	}
	return strings.ToLower(strings.TrimSpace(employee.Email))
}

func optionalText(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func warning(code string, blocking bool, format string, args ...any) model.PayslipWarning {
	return model.PayslipWarning{Code: code, Blocking: blocking, Message: fmt.Sprintf(format, args...)}
}

// rowUnits is how many printed lines a row takes: two when a cell wraps.
func rowUnits(widths ...int) int {
	for index := 0; index+1 < len(widths); index += 2 {
		if widths[index] > widths[index+1] {
			return 2
		}
	}
	return 1
}

// payslipPageUnits estimates the table rows of a slip (see payslipRowBudget).
func payslipPageUnits(amounts model.PayslipAmounts, note string) int {
	earnings, deductions := payslipRows(amounts)
	units := 0
	for _, line := range earnings {
		units += rowUnits(utf8.RuneCountInString(line.Label), payslipNameWrapRunes, utf8.RuneCountInString(line.Keterangan), payslipKeteranganWrapRunes)
	}
	if len(deductions) == 0 {
		units++ // the "Tidak ada potongan" row
	}
	for _, line := range deductions {
		units += rowUnits(utf8.RuneCountInString(line.Label), payslipNameWrapRunes, utf8.RuneCountInString(line.Keterangan), payslipKeteranganWrapRunes)
	}
	for _, item := range amounts.Reimbursements {
		units += rowUnits(utf8.RuneCountInString(item.Title), payslipTitleWrapRunes, utf8.RuneCountInString(item.Category), payslipCategoryWrapRunes)
	}
	if note = docgen.SingleLine(note); note != "" {
		units++
		if utf8.RuneCountInString(note) > payslipNoteWrapRunes {
			units++
		}
	}
	return units
}

// rowWarnings are the warnings that depend only on the amounts and the note
// (recomputed whenever HR edits them).
func rowWarnings(amounts model.PayslipAmounts, note string) []model.PayslipWarning {
	warnings := []model.PayslipWarning{}
	if units := payslipPageUnits(amounts, note); units > payslipRowBudget {
		earnings, deductions := payslipRows(amounts)
		warnings = append(warnings, warning(PayslipWarnManyRows, false,
			"Slip berisi %d baris gaji, %d potongan, %d reimbursement%s: kemungkinan lebih dari 1 halaman, periksa PDF",
			len(earnings), len(deductions), len(amounts.Reimbursements), noteSuffix(note)))
	}
	if amounts.Totals.TotalDiterima <= 0 {
		warnings = append(warnings, warning(PayslipWarnTotalNotPositive, false, "Total diterima Rp%s: periksa penyesuaian", docgen.Thousands(amounts.Totals.TotalDiterima)))
	}
	return warnings
}

func noteSuffix(note string) string {
	if docgen.SingleLine(note) == "" {
		return ""
	}
	return " dan catatan"
}

// replaceRowWarnings swaps the amount-dependent warnings of a stored list.
// The page-count warning of the previous PDF goes too: the edit queues a new
// render, which records it again if the new PDF is still too long.
func replaceRowWarnings(stored []model.PayslipWarning, amounts model.PayslipAmounts, note string) []model.PayslipWarning {
	result := make([]model.PayslipWarning, 0, len(stored)+2)
	for _, item := range stored {
		if item.Code == PayslipWarnManyRows || item.Code == PayslipWarnTotalNotPositive || item.Code == PayslipWarnPDFPages {
			continue
		}
		result = append(result, item)
	}
	return append(result, rowWarnings(amounts, note)...)
}

// selectReissueItems rebuilds the bonus and reimbursement rows of a void &
// reissue from exactly the voided slip's ids. changed reports an item that
// is no longer approved / paid, or was deleted (its old line is kept).
func selectReissueItems(in payslipAssemblyInput) (bonusLines []model.PayslipLine, bonusIDs []string, reimbursements []model.PayslipReimbursementLine, reimbursementIDs []string, changed bool) {
	reissue := in.Reissue
	bonusByID := map[string]model.BonusRecord{}
	for _, item := range in.Bonuses {
		bonusByID[strings.ToLower(item.ID)] = item
	}
	oldBonus := map[string]model.PayslipLine{}
	for _, line := range reissue.Previous.Earnings {
		if line.Kind == model.PayslipLineBonus && line.SourceID != "" {
			oldBonus[strings.ToLower(line.SourceID)] = line
		}
	}
	bonusIDs = []string{}
	for _, id := range reissue.BonusIDs {
		key := strings.ToLower(id)
		if record, ok := bonusByID[key]; ok {
			if record.ApprovalStatus != "approved" {
				changed = true
			}
			bonusLines = append(bonusLines, bonusLine(record, in.Year, in.Month))
		} else if line, ok := oldBonus[key]; ok {
			changed = true
			bonusLines = append(bonusLines, line)
		} else {
			changed = true
			continue
		}
		bonusIDs = append(bonusIDs, id)
	}

	reimbursementByID := map[string]hrisrepo.PayslipReimbursement{}
	for _, item := range reissue.Reimbursements {
		reimbursementByID[strings.ToLower(item.ID)] = item
	}
	oldReimbursement := map[string]model.PayslipReimbursementLine{}
	for _, line := range reissue.Previous.Reimbursements {
		oldReimbursement[strings.ToLower(line.ID)] = line
	}
	reimbursementIDs = []string{}
	for _, id := range reissue.ReimbursementIDs {
		key := strings.ToLower(id)
		if record, ok := reimbursementByID[key]; ok {
			if record.Status != "paid" {
				changed = true
			}
			reimbursements = append(reimbursements, reimbursementLine(record))
		} else if line, ok := oldReimbursement[key]; ok {
			changed = true
			reimbursements = append(reimbursements, line)
		} else {
			changed = true
			continue
		}
		reimbursementIDs = append(reimbursementIDs, id)
	}
	return bonusLines, bonusIDs, reimbursements, reimbursementIDs, changed
}

func reimbursementLine(item hrisrepo.PayslipReimbursement) model.PayslipReimbursementLine {
	return model.PayslipReimbursementLine{
		ID:              item.ID,
		TransactionDate: item.TransactionDate.Format("2006-01-02"),
		Title:           truncateRunes(item.Title, payslipKeteranganMaxRunes),
		Category:        truncateRunes(item.Category, 40),
		Amount:          item.Amount,
	}
}

// assemblePayslip builds the slip of one employee. With no salary row the
// result is Blocked: a slip never falls back to the policy default salary.
func assemblePayslip(in payslipAssemblyInput) payslipAssembly {
	result := payslipAssembly{Warnings: []model.PayslipWarning{}, GeneratedAt: in.Now}
	periodStart, periodEnd := payslipPeriodBounds(in.Year, in.Month)
	periodLabel := docgen.PeriodID(periodStart)
	add := func(w model.PayslipWarning) {
		result.Warnings = append(result.Warnings, w)
		if w.Blocking {
			result.Blocked = true
		}
	}

	amounts := model.PayslipAmounts{
		Earnings:       []model.PayslipLine{},
		Deductions:     []model.PayslipLine{},
		Reimbursements: []model.PayslipReimbursementLine{},
		ManualLines:    append([]model.PayslipManualLine{}, in.ManualLines...),
	}

	if in.Salary == nil {
		add(warning(PayslipWarnNoSalary, true, "Tanpa data gaji: tambahkan data gaji yang berlaku per %s", docgen.FormatDateID(periodEnd)))
	} else {
		salaryID := in.Salary.ID
		result.SalaryID = &salaryID
		amounts.Earnings, amounts.Deductions = salaryLines(*in.Salary)
		if in.Salary.BaseSalary < payslipMinBaseSalary {
			add(warning(PayslipWarnBaseSalaryLow, false, "Gaji pokok Rp%s (di bawah Rp100.000): periksa data gaji", docgen.Thousands(in.Salary.BaseSalary)))
		}
	}

	result.BonusIDs = []string{}
	result.ReimbursementIDs = []string{}
	if in.Reissue != nil {
		bonusLines, bonusIDs, reimbursements, reimbursementIDs, changed := selectReissueItems(in)
		amounts.Earnings = append(amounts.Earnings, bonusLines...)
		amounts.Reimbursements = append(amounts.Reimbursements, reimbursements...)
		result.BonusIDs, result.ReimbursementIDs = bonusIDs, reimbursementIDs
		if changed {
			add(warning(PayslipWarnReissueItemChanged, false, "Bonus atau reimbursement slip yang dibatalkan berubah status atau dihapus: periksa rincian sebelum mengirim"))
		}
	} else {
		for _, bonus := range selectCarryOverBonuses(in.Year, in.Month, in.Anchor, in.ConsumedBonus, in.Bonuses) {
			amounts.Earnings = append(amounts.Earnings, bonusLine(bonus, in.Year, in.Month))
			result.BonusIDs = append(result.BonusIDs, bonus.ID)
		}
		for _, item := range selectCarryOverReimbursements(in.Year, in.Month, in.Now, in.Anchor, in.ConsumedReimburs, in.Reimbursements) {
			amounts.Reimbursements = append(amounts.Reimbursements, reimbursementLine(item))
			result.ReimbursementIDs = append(result.ReimbursementIDs, item.ID)
		}
	}
	amounts.Totals = computePayslipTotals(amounts)
	result.Amounts = amounts

	employee := in.Employee
	joined := employee.DateJoined
	if joined.Year() == in.Year && int(joined.Month()) == in.Month {
		add(warning(PayslipWarnJoinedInMonth, false, "Bergabung %s: periksa pro-rata", docgen.FormatDateID(joined)))
	}

	pendingBonuses := 0
	for _, bonus := range in.Bonuses {
		if bonus.ApprovalStatus == "pending" && bonus.PeriodYear == in.Year && bonus.PeriodMonth == in.Month {
			pendingBonuses++
		}
	}
	if pendingBonuses > 0 {
		add(warning(PayslipWarnBonusPending, false, "Bonus %s masih pending (%d): belum masuk slip", periodLabel, pendingBonuses))
	}

	unpaidCount, unpaidTotal := 0, int64(0)
	periodEndDate := time.Date(periodEnd.Year(), periodEnd.Month(), periodEnd.Day(), 0, 0, 0, 0, time.UTC)
	for _, item := range in.Reimbursements {
		if item.Status != "approved" {
			continue
		}
		transaction := time.Date(item.TransactionDate.Year(), item.TransactionDate.Month(), item.TransactionDate.Day(), 0, 0, 0, 0, time.UTC)
		if transaction.After(periodEndDate) {
			continue
		}
		unpaidCount++
		unpaidTotal += item.Amount
	}
	if unpaidCount > 0 {
		add(warning(PayslipWarnReimbursementUnpd, false, "Reimbursement disetujui belum dibayar (%d, Rp%s): masuk slip setelah dibayar", unpaidCount, docgen.Thousands(unpaidTotal)))
	}

	// Jabatan and status kerja: an active contract first (contracts phase),
	// then the HR profile job title and the employment type mapping.
	jobTitle := optionalText(employee.JobTitle)
	statusKerja, statusKnown := payslipStatusKerja(employee.Position)
	if in.Contract != nil {
		contractID := in.Contract.ContractID
		if contractID != "" {
			result.ContractID = &contractID
		}
		if strings.TrimSpace(in.Contract.JobTitle) != "" {
			jobTitle = strings.TrimSpace(in.Contract.JobTitle)
		}
		if strings.TrimSpace(in.Contract.StatusKerja) != "" {
			statusKerja, statusKnown = strings.TrimSpace(in.Contract.StatusKerja), true
		}
	}
	if jobTitle == "" {
		jobTitle = "-"
		add(warning(PayslipWarnJobTitleMissing, false, "Jabatan belum diisi (profil karyawan)"))
	}
	if !statusKnown {
		add(warning(PayslipWarnStatusKerjaMissing, false, "Status kerja belum ditentukan: isi tipe kepegawaian"))
	}
	result.JobTitle = docgen.SingleLine(jobTitle)
	result.StatusKerja = statusKerja

	if strings.TrimSpace(in.Company.DocCode) == "" {
		add(warning(PayslipWarnDocCodeMissing, false, "Kode dokumen perusahaan belum diisi: kode karyawan dibuat tanpa awalan"))
	}
	if optionalText(employee.BankName) == "" || optionalText(employee.BankAccountNumber) == "" {
		add(warning(PayslipWarnBankMissing, false, "Data rekening bank belum lengkap"))
	}
	if defaultPayslipRecipient(employee) == "" {
		add(warning(PayslipWarnEmailMissing, false, "Email penerima belum ada"))
	}
	created := employee.CreatedAt.In(payslipLocation)
	if joined.Year() == created.Year() && joined.Month() == created.Month() && joined.Day() == created.Day() {
		add(warning(PayslipWarnJoinedEqualsCreate, false, "Tanggal bergabung sama dengan tanggal data dibuat: periksa tanggal bergabung"))
	}

	if !result.Blocked {
		for _, w := range rowWarnings(amounts, in.Note) {
			add(w)
		}
	}
	return result
}

// DefaultPayslipPayDate is payday_day of the period, clamped to the month
// end and moved to the previous weekday when it falls on a weekend (PKWT
// 4.2): 25 Oct 2026 (Sunday) -> Friday 23 Oct 2026. With payday_day 1 this
// can be the last Friday of the month before (see PayslipPayDateBounds).
func DefaultPayslipPayDate(year int, month int, paydayDay int) time.Time {
	if paydayDay < 1 {
		paydayDay = authrepo.DefaultCompanyPaydayDay
	}
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	if paydayDay > last {
		paydayDay = last
	}
	return docgen.PrevWorkday(time.Date(year, time.Month(month), paydayDay, 0, 0, 0, 0, time.UTC))
}

// PayslipPayDateBounds is the range a slip's pay date may take: from the
// weekday on or before the 1st of the period (so the default, moved back
// from a weekend 1st, is always accepted) to the end of the month after.
func PayslipPayDateBounds(year int, month int) (time.Time, time.Time) {
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	return docgen.PrevWorkday(first), first.AddDate(0, 2, -1)
}

// FormatPayslipNumber renders 'PAY/YYYY/MM/<employee number %04d>', with
// '-R<n>' for a reissue (revision n > 0).
func FormatPayslipNumber(year int, month int, employeeNumber int, revision int) string {
	number := fmt.Sprintf("PAY/%04d/%02d/%04d", year, month, employeeNumber)
	if revision > 0 {
		number += fmt.Sprintf("-R%d", revision)
	}
	return number
}

// payslipBaseNumber strips a '-R<n>' suffix.
func payslipBaseNumber(docNumber string) string {
	if index := strings.LastIndex(docNumber, "-R"); index > 0 {
		suffix := docNumber[index+2:]
		if suffix != "" && strings.Trim(suffix, "0123456789") == "" {
			return docNumber[:index]
		}
	}
	return docNumber
}

// payslipHeader is the employee/company part of the payload, fixed at
// generation time.
type payslipHeader struct {
	DocNumber   string
	PayDate     time.Time
	Year        int
	Month       int
	Employee    hrisrepo.PayslipEmployee
	JobTitle    string
	StatusKerja string
	Company     authrepo.CompanyProfileRecord
}

func dashIfEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

// buildPayslipPayload renders the 08_Slip_Gaji variables. Every value is a
// formatted, single-line string; norek is masked.
func buildPayslipPayload(header payslipHeader, amounts model.PayslipAmounts, note string) docgen.Payload {
	periodStart, _ := payslipPeriodBounds(header.Year, header.Month)
	employee := header.Employee
	norek := docgen.MaskAccount(optionalText(employee.BankAccountNumber))
	payload := docgen.Payload{
		"perusahaan_nama":            dashIfEmpty(header.Company.LegalName),
		"perusahaan_alamat":          dashIfEmpty(header.Company.Address),
		"dokumen_nomor":              header.DocNumber,
		"periode":                    docgen.PeriodID(periodStart),
		"tanggal_bayar":              docgen.FormatDateID(header.PayDate),
		"karyawan_nama":              docgen.SingleLine(employee.FullName),
		"karyawan_id":                dashIfEmpty(optionalText(employee.EmployeeCode)),
		"karyawan_jabatan":           dashIfEmpty(header.JobTitle),
		"karyawan_departemen":        dashIfEmpty(optionalText(employee.Department)),
		"karyawan_status_kerja":      dashIfEmpty(header.StatusKerja),
		"karyawan_tanggal_bergabung": docgen.FormatDateID(employee.DateJoined),
		"karyawan_email":             dashIfEmpty(defaultPayslipRecipient(employee)),
		"karyawan_telepon":           dashIfEmpty(optionalText(employee.Phone)),
		"karyawan_bank":              dashIfEmpty(optionalText(employee.BankName)),
		"karyawan_norek":             dashIfEmpty(norek),
		"kontak_hr":                  dashIfEmpty(header.Company.HRContactEmail),
	}
	applyAmountsToPayload(payload, amounts, note)
	return payload
}

// applyAmountsToPayload (re)writes the row loops, totals and the note.
// Without deductions the potongan table gets one "Tidak ada potongan" row.
func applyAmountsToPayload(payload docgen.Payload, amounts model.PayslipAmounts, note string) {
	earnings, deductions := payslipRows(amounts)
	gaji := make([]map[string]string, 0, len(earnings))
	for _, line := range earnings {
		gaji = append(gaji, map[string]string{"nama": line.Label, "keterangan": line.Keterangan, "jumlah": docgen.Thousands(line.Amount)})
	}
	potongan := make([]map[string]string, 0, len(deductions))
	for _, line := range deductions {
		potongan = append(potongan, map[string]string{"nama": line.Label, "keterangan": line.Keterangan, "jumlah": docgen.Thousands(line.Amount)})
	}
	if len(potongan) == 0 {
		potongan = append(potongan, map[string]string{"nama": "Tidak ada potongan / No deductions", "keterangan": "", "jumlah": "0"})
	}
	reimbursements := make([]map[string]string, 0, len(amounts.Reimbursements))
	for _, item := range amounts.Reimbursements {
		date := item.TransactionDate
		if parsed, err := time.Parse("2006-01-02", item.TransactionDate); err == nil {
			date = docgen.FormatDMY(parsed)
		}
		reimbursements = append(reimbursements, map[string]string{
			"tanggal":  date,
			"judul":    item.Title,
			"kategori": item.Category,
			"jumlah":   docgen.Thousands(item.Amount),
		})
	}
	totals := amounts.Totals
	payload["gaji"] = gaji
	payload["potongan"] = potongan
	payload["reimbursement"] = reimbursements
	payload["total_gaji"] = docgen.Thousands(totals.TotalGaji)
	payload["total_potongan"] = docgen.Thousands(totals.TotalPotongan)
	payload["total_reimbursement"] = docgen.Thousands(totals.TotalReimbursement)
	payload["total_diterima"] = docgen.Thousands(totals.TotalDiterima)
	payload["total_diterima_terbilang"] = docgen.TerbilangRupiah(totals.TotalDiterima)
	payload["catatan_slip"] = docgen.SingleLine(note)
}

// normalizePayslipNote makes the note one line; ok is false when it is too
// long for the slip.
func normalizePayslipNote(note string) (string, bool) {
	cleaned := docgen.SingleLine(note)
	return cleaned, utf8.RuneCountInString(cleaned) <= payslipNoteMaxRunes
}
