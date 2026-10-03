package hris

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/docgen"
	hrisdto "github.com/kana-consultant/kantor/backend/internal/dto/hris"
	"github.com/kana-consultant/kantor/backend/internal/model"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
	hrisrepo "github.com/kana-consultant/kantor/backend/internal/repository/hris"
)

// Pure helpers of the contracts phase: template payloads (01 PKWT, 02
// NDA/HKI), document numbers, the completeness check, the renewal chain
// (5-year PKWT limit) and the notice dates. They take every input
// explicitly, so they are unit-tested without a database.

const (
	// contractChainLimitMonths is the PKWT ceiling over a renewal chain
	// (PP 35/2021: at most 5 years in total).
	contractChainLimitMonths = 60
	// contractNoticeLead: the 'Batas pemberitahuan' badge shows from
	// notice_days + 14 days before end_date.
	contractNoticeLead = 14

	contractDefaultWorkDays  = "Senin–Jumat / Monday–Friday"
	contractDefaultWorkHours = "09.00–18.00 WIB"
	contractDefaultWeekly    = 40
	contractDefaultNotice    = 30
	contractDefaultIncident  = 24
	contractDefaultSolicit   = 12
	contractDefaultSecrecy   = 3
)

// contractPeriodKey is the EMPLOYMENT sequence key: the month of the
// document date.
func contractPeriodKey(documentDate time.Time) string {
	return documentDate.Format("2006-01")
}

// FormatContractNumbers renders the PKWT and NDA numbers of one shared
// sequence value: "021/PKWT/CTN/X/2026" and "021/NDA-HKI/CTN/X/2026". The
// company code segment is left out when the profile has none.
func FormatContractNumbers(seq int, docCode string, documentDate time.Time) (pkwt string, nda string) {
	code := strings.ToUpper(strings.TrimSpace(docCode))
	tail := fmt.Sprintf("%s/%d", docgen.RomanMonth(documentDate.Month()), documentDate.Year())
	if code != "" {
		tail = code + "/" + tail
	}
	return fmt.Sprintf("%03d/PKWT/%s", seq, tail), fmt.Sprintf("%03d/NDA-HKI/%s", seq, tail)
}

// calendarDate drops the clock and zone of t (dates are stored as DATE and
// compared as UTC midnights).
func calendarDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// jakartaToday is today's calendar date in Asia/Jakarta.
func jakartaToday(now time.Time) time.Time {
	return calendarDate(now.In(payslipLocation))
}

// contractSpan is the length of [start, end] (both inclusive) as whole
// months plus remaining days: 1 Oct 2026 - 30 Sep 2027 is 12 months 0 days.
func contractSpan(start time.Time, end time.Time) (months int, days int) {
	start, end = calendarDate(start), calendarDate(end)
	exclusive := end.AddDate(0, 0, 1)
	if !exclusive.After(start) {
		return 0, 0
	}
	months = (exclusive.Year()-start.Year())*12 + int(exclusive.Month()-start.Month())
	for months > 0 && start.AddDate(0, months, 0).After(exclusive) {
		months--
	}
	anchor := start.AddDate(0, months, 0)
	days = int(exclusive.Sub(anchor).Hours() / 24)
	return months, days
}

// contractMonths is the span as fractional months (days / 30).
func contractMonths(start time.Time, end time.Time) float64 {
	months, days := contractSpan(start, end)
	return float64(months) + float64(days)/30
}

// durationMonthsText is durasi_bulan: whole months, rounded when the span is
// not a whole number of months (the preflight warns about it).
func durationMonthsText(start time.Time, end time.Time) string {
	return strconv.Itoa(int(math.Round(contractMonths(start, end))))
}

// renewalDates are the dates of a renewal: it starts the day after end and
// lasts as long as the original (whole months stay whole months).
func renewalDates(start time.Time, end time.Time) (time.Time, time.Time) {
	start, end = calendarDate(start), calendarDate(end)
	nextStart := end.AddDate(0, 0, 1)
	months, days := contractSpan(start, end)
	if days == 0 && months > 0 {
		return nextStart, nextStart.AddDate(0, months, -1)
	}
	return nextStart, nextStart.Add(end.Sub(start))
}

// contractEffectiveEnd is when a chain link stopped counting: its end date,
// or an earlier ended_at for a contract ended early.
func contractEffectiveEnd(link hrisrepo.ContractChainLink) *time.Time {
	end := link.EndDate
	if link.Status == model.ContractStatusEnded && link.EndedAt != nil && (end == nil || link.EndedAt.Before(*end)) {
		end = link.EndedAt
	}
	return end
}

// contractChainGapDays: a PKWT that starts at most this many days after the
// previous PKWT of the employee ended continues it for the 5-year check, even
// without a Perpanjang link (e.g. record-only history entered through 'Buat
// Kontrak').
const contractChainGapDays = 30

// contractChain is the chain of id among links (all contracts of one
// employee), newest first: each step follows previous_contract_id, or, for a
// PKWT without that link, the employee's latest PKWT (not cancelled) whose
// effective end lies within contractChainGapDays before its start.
func contractChain(id string, links []hrisrepo.ContractChainLink) []hrisrepo.ContractChainLink {
	byID := make(map[string]hrisrepo.ContractChainLink, len(links))
	for _, link := range links {
		byID[link.ID] = link
	}
	current, ok := byID[id]
	if !ok {
		return nil
	}
	chain := []hrisrepo.ContractChainLink{current}
	used := map[string]bool{id: true}
	for len(chain) < 50 {
		var previous *hrisrepo.ContractChainLink
		if current.PreviousContractID != nil {
			if link, ok := byID[*current.PreviousContractID]; ok && !used[link.ID] {
				previous = &link
			}
		} else if current.ContractType == model.ContractTypePKWT {
			previous = contiguousPKWT(current, links, used)
		}
		if previous == nil {
			break
		}
		chain = append(chain, *previous)
		used[previous.ID] = true
		current = *previous
	}
	return chain
}

// contiguousPKWT is the PKWT that ended last within contractChainGapDays
// before current starts (nil when there is none).
func contiguousPKWT(current hrisrepo.ContractChainLink, links []hrisrepo.ContractChainLink, used map[string]bool) *hrisrepo.ContractChainLink {
	start := calendarDate(current.StartDate)
	earliest := start.AddDate(0, 0, -(contractChainGapDays + 1))
	var best *hrisrepo.ContractChainLink
	var bestEnd time.Time
	for _, link := range links {
		if used[link.ID] || link.ContractType != model.ContractTypePKWT || link.Status == model.ContractStatusCancelled {
			continue
		}
		effective := contractEffectiveEnd(link)
		if effective == nil {
			continue
		}
		end := calendarDate(*effective)
		if !end.Before(start) || end.Before(earliest) {
			continue
		}
		if best == nil || end.After(bestEnd) || (end.Equal(bestEnd) && link.StartDate.After(best.StartDate)) {
			candidate := link
			best, bestEnd = &candidate, end
		}
	}
	return best
}

// chainPKWTMonths sums the PKWT months of a renewal chain (cancelled links
// and other types do not count; record-only PKWT entries do).
func chainPKWTMonths(chain []hrisrepo.ContractChainLink) float64 {
	total := 0.0
	for _, link := range chain {
		if link.ContractType != model.ContractTypePKWT || link.Status == model.ContractStatusCancelled {
			continue
		}
		end := contractEffectiveEnd(link)
		if end == nil {
			continue
		}
		total += contractMonths(link.StartDate, *end)
	}
	return math.Round(total*10) / 10
}

// contractNotice returns the notice deadline (end_date - notice_days), and
// whether the 'Batas pemberitahuan' badge (from notice_days + 14 days before
// the end) and the 'Kedaluwarsa' badge apply today. Ended and cancelled
// contracts get neither badge.
func contractNotice(contract model.EmploymentContract, today time.Time) (deadline *time.Time, alert bool, expired bool) {
	if contract.EndDate == nil {
		return nil, false, false
	}
	end := calendarDate(*contract.EndDate)
	value := end.AddDate(0, 0, -contract.NoticeDays)
	deadline = &value
	if contract.Status == model.ContractStatusEnded || contract.Status == model.ContractStatusCancelled {
		return deadline, false, false
	}
	today = calendarDate(today)
	expired = today.After(end)
	alertFrom := end.AddDate(0, 0, -(contract.NoticeDays + contractNoticeLead))
	alert = !expired && !today.Before(alertFrom)
	return deadline, alert, expired
}

// contractStatusKerja is what an active contract prints on a payslip
// (karyawan_status_kerja).
func contractStatusKerja(contractType string) string {
	switch contractType {
	case model.ContractTypePKWT:
		return "PKWT"
	case model.ContractTypePKWTT:
		return "PKWTT"
	case model.ContractTypeMagang:
		return "Magang"
	}
	return contractType
}

// contractWorkMode renders mode_kerja: the preset plus HR's detail, e.g.
// "hybrid (3 hari WFO, 2 hari WFH)". The template prints the same text in
// the English column (it has no _en twin).
func contractWorkMode(mode string, detail *string) string {
	label := map[string]string{
		model.ContractWorkModeWFO:    "WFO",
		model.ContractWorkModeHybrid: "hybrid",
		model.ContractWorkModeRemote: "remote",
	}[mode]
	if label == "" {
		label = mode
	}
	if text := docgen.SingleLine(optionalText(detail)); text != "" {
		return fmt.Sprintf("%s (%s)", label, text)
	}
	return label
}

func genderLabel(gender string) string {
	switch gender {
	case model.GenderFemale:
		return "Perempuan / Female"
	case model.GenderMale:
		return "Laki-laki / Male"
	}
	return ""
}

// contractPayloadInput is everything both document payloads are built from.
type contractPayloadInput struct {
	Contract     model.EmploymentContract
	Employee     hrisrepo.ContractEmployee
	Identity     model.EmployeeIdentity
	Company      authrepo.CompanyProfileRecord
	Compensation model.ContractCompensation
	DocNumber    string
	NDADocNumber string
	DocumentDate time.Time
	DocumentCity string
}

// contractText makes a template value: single line, "-" when empty.
func contractText(value string) string {
	return dashIfEmpty(docgen.SingleLine(value))
}

// buildContractPayloads maps every variable of 01 PKWT and 02 NDA/HKI. The
// full NIK and the full account number appear only in the PKWT: the NDA
// prints the masked NIK (it has no account number).
func buildContractPayloads(in contractPayloadInput) (pkwt docgen.Payload, nda docgen.Payload) {
	c := in.Contract
	birthDate := ""
	if parsed, err := time.Parse("2006-01-02", strings.TrimSpace(in.Identity.BirthDate)); err == nil {
		birthDate = docgen.FormatDMY(parsed)
	}
	department := optionalText(c.Department)
	if department == "" {
		department = optionalText(in.Employee.Department)
	}

	common := docgen.Payload{
		"perusahaan_nama":                  contractText(in.Company.LegalName),
		"perusahaan_alamat":                contractText(in.Company.Address),
		"perusahaan_jenis_usaha":           contractText(in.Company.BusinessType),
		"perusahaan_penandatangan_nama":    contractText(in.Company.SignerName),
		"perusahaan_penandatangan_jabatan": contractText(in.Company.SignerTitle),
		"dokumen_kota":                     contractText(in.DocumentCity),
		"dokumen_tanggal":                  docgen.FormatDateID(in.DocumentDate),
		"dokumen_tanggal_en":               docgen.FormatDateEN(in.DocumentDate),
		"karyawan_nama":                    contractText(in.Employee.FullName),
		"karyawan_tempat_lahir":            contractText(in.Identity.BirthPlace),
		"karyawan_tanggal_lahir":           contractText(birthDate),
		"karyawan_jenis_kelamin":           contractText(genderLabel(in.Identity.Gender)),
		"karyawan_alamat":                  contractText(in.Identity.KTPAddress),
		"karyawan_telepon":                 contractText(optionalText(in.Employee.Phone)),
		"karyawan_email":                   contractText(in.Employee.Email),
		"karyawan_jabatan":                 contractText(c.JobTitle),
	}
	clone := func() docgen.Payload {
		out := make(docgen.Payload, len(common)+32)
		for key, value := range common {
			out[key] = value
		}
		return out
	}

	pkwt = clone()
	pkwt["dokumen_nomor"] = contractText(in.DocNumber)
	pkwt["nomor_nda_hki"] = contractText(in.NDADocNumber)
	pkwt["karyawan_nik"] = contractText(in.Identity.NIK)
	pkwt["karyawan_departemen"] = contractText(department)
	pkwt["karyawan_atasan"] = contractText(optionalText(c.SupervisorName))
	pkwt["karyawan_bank"] = contractText(optionalText(in.Employee.BankName))
	pkwt["karyawan_norek"] = contractText(optionalText(in.Employee.BankAccountNumber))
	pkwt["karyawan_nama_rekening"] = contractText(in.Identity.BankAccountName)
	pkwt["dasar_pkwt"] = contractText(optionalText(c.PKWTBasis))
	pkwt["lokasi_kerja"] = contractText(c.WorkLocation)
	pkwt["mode_kerja"] = contractText(contractWorkMode(c.WorkMode, c.WorkModeDetail))
	pkwt["uraian_tugas"] = contractText(c.JobDescription)
	pkwt["tanggal_mulai"] = docgen.FormatDateID(c.StartDate)
	pkwt["tanggal_mulai_en"] = docgen.FormatDateEN(c.StartDate)
	if c.EndDate != nil {
		pkwt["durasi_bulan"] = durationMonthsText(c.StartDate, *c.EndDate)
		pkwt["tanggal_berakhir"] = docgen.FormatDateID(*c.EndDate)
		pkwt["tanggal_berakhir_en"] = docgen.FormatDateEN(*c.EndDate)
	} else {
		pkwt["durasi_bulan"], pkwt["tanggal_berakhir"], pkwt["tanggal_berakhir_en"] = "-", "-", "-"
	}
	pkwt["hari_pemberitahuan"] = strconv.Itoa(c.NoticeDays)
	pkwt["jam_kerja_mingguan"] = strconv.Itoa(c.WeeklyHours)
	pkwt["hari_kerja"] = contractText(c.WorkDays)
	pkwt["jam_kerja"] = contractText(c.WorkHours)
	pkwt["gaji_pokok"] = docgen.Thousands(in.Compensation.BaseSalary)
	pkwt["gaji_pokok_terbilang"] = docgen.TerbilangRupiah(in.Compensation.BaseSalary)
	pkwt["tunjangan_tetap"] = docgen.Thousands(in.Compensation.FixedAllowance)
	pkwt["tanggal_gajian"] = strconv.Itoa(in.Company.PaydayDay)
	pkwt["cuti_tahunan_hari"] = strconv.Itoa(in.Company.AnnualLeaveDays)
	benefits := make([]map[string]string, 0, len(c.Benefits))
	for _, item := range c.Benefits {
		benefits = append(benefits, map[string]string{
			"nama":       contractText(item.Name),
			"nilai":      contractText(item.Value),
			"keterangan": contractText(item.Notes),
		})
	}
	if len(benefits) == 0 {
		benefits = append(benefits, map[string]string{"nama": "Tidak ada benefit tambahan / No additional benefits", "nilai": "-", "keterangan": "-"})
	}
	pkwt["benefit"] = benefits

	nda = clone()
	nda["dokumen_nomor"] = contractText(in.NDADocNumber)
	nda["nomor_kontrak_kerja"] = contractText(in.DocNumber)
	nda["karyawan_nik"] = contractText(MaskNIK(in.Identity.NIK))
	nda["jam_lapor_insiden"] = strconv.Itoa(c.IncidentReportHours)
	nda["masa_non_solicit_bulan"] = strconv.Itoa(c.NonSolicitMonths)
	nda["masa_kerahasiaan_tahun"] = strconv.Itoa(c.ConfidentialityYears)
	works := make([]map[string]string, 0, len(c.PriorWorks))
	for _, item := range c.PriorWorks {
		works = append(works, map[string]string{
			"judul":     contractText(item.Title),
			"deskripsi": contractText(item.Description),
			"tahun":     contractText(item.Year),
		})
	}
	if len(works) == 0 {
		works = append(works, map[string]string{"judul": "Tidak ada / None", "deskripsi": "-", "tahun": "-"})
	}
	nda["karya_terdahulu"] = works
	return pkwt, nda
}

// contractCompleteness is what the preflight and generate check.
type contractCompleteness struct {
	Contract        model.EmploymentContract
	Employee        hrisrepo.ContractEmployee
	Identity        model.EmployeeIdentity
	Company         authrepo.CompanyProfileRecord
	HasCompensation bool
}

// contractMissingFields lists what a PKWT still needs before its documents
// can be generated, grouped by where HR fixes it.
func contractMissingFields(in contractCompleteness) []hrisdto.ContractMissingField {
	missing := []hrisdto.ContractMissingField{}
	add := func(scope string, field string, label string, value string) {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, hrisdto.ContractMissingField{Scope: scope, Field: field, Label: label})
		}
	}
	company := in.Company
	add("company", "legal_name", "Nama PT", company.LegalName)
	add("company", "address", "Alamat perusahaan", company.Address)
	add("company", "business_type", "Bidang usaha", company.BusinessType)
	add("company", "signer_name", "Nama penandatangan", company.SignerName)
	add("company", "signer_title", "Jabatan penandatangan", company.SignerTitle)
	add("company", "doc_code", "Kode dokumen", company.DocCode)
	if strings.TrimSpace(optionalText(in.Contract.DocumentCity)) == "" {
		add("company", "city", "Kota perusahaan (kota penandatanganan)", company.City)
	}

	identity := in.Identity
	add("identity", "nik", "NIK", identity.NIK)
	add("identity", "birth_place", "Tempat lahir", identity.BirthPlace)
	add("identity", "birth_date", "Tanggal lahir", identity.BirthDate)
	add("identity", "gender", "Jenis kelamin", identity.Gender)
	add("identity", "bank_account_name", "Nama pemilik rekening", identity.BankAccountName)
	add("identity", "ktp_address", "Alamat sesuai KTP", identity.KTPAddress)

	employee := in.Employee
	add("employee", "email", "Email karyawan", employee.Email)
	add("employee", "phone", "Nomor telepon", optionalText(employee.Phone))
	add("employee", "bank_name", "Nama bank", optionalText(employee.BankName))
	add("employee", "bank_account_number", "Nomor rekening", optionalText(employee.BankAccountNumber))

	c := in.Contract
	add("contract", "job_title", "Jabatan", c.JobTitle)
	department := optionalText(c.Department)
	if department == "" {
		department = optionalText(employee.Department)
	}
	add("contract", "department", "Departemen", department)
	add("contract", "supervisor_name", "Atasan langsung", optionalText(c.SupervisorName))
	add("contract", "work_location", "Lokasi kerja", c.WorkLocation)
	add("contract", "job_description", "Uraian tugas", c.JobDescription)
	add("contract", "pkwt_basis", "Dasar PKWT", optionalText(c.PKWTBasis))
	add("contract", "work_days", "Hari kerja", c.WorkDays)
	add("contract", "work_hours", "Jam kerja", c.WorkHours)
	if c.EndDate == nil {
		add("contract", "end_date", "Tanggal berakhir", "")
	}
	if !in.HasCompensation {
		add("contract", "compensation", "Kompensasi (gaji pokok)", "")
	}
	return missing
}

// contractWarnings are the preflight findings that do not block: a PKWT for
// an employee still on probation (a PKWT allows no probation period), a
// renewal chain above five years, and a duration that is not a whole number
// of months (durasi_bulan is rounded).
func contractWarnings(contract model.EmploymentContract, employeeStatus string, chainMonths float64) []hrisdto.ContractWarning {
	warnings := []hrisdto.ContractWarning{}
	if contract.ContractType == model.ContractTypePKWT && employeeStatus == "probation" {
		warnings = append(warnings, hrisdto.ContractWarning{
			Code:    "employee_probation",
			Message: "Status karyawan masih Probation, padahal PKWT tidak boleh mensyaratkan masa percobaan. Ubah status karyawan menjadi Active.",
			Action:  "set_employee_active",
		})
	}
	if chainMonths > contractChainLimitMonths {
		warnings = append(warnings, hrisdto.ContractWarning{
			Code:    "pkwt_chain_over_5_years",
			Message: fmt.Sprintf("Total PKWT berturut-turut %s bulan melebihi batas 5 tahun (60 bulan). Pertimbangkan PKWTT.", strconv.FormatFloat(chainMonths, 'f', -1, 64)),
		})
	}
	if contract.ContractType == model.ContractTypePKWT && contract.EndDate != nil {
		if _, days := contractSpan(contract.StartDate, *contract.EndDate); days != 0 {
			warnings = append(warnings, hrisdto.ContractWarning{
				Code:    "duration_not_whole_months",
				Message: fmt.Sprintf("Jangka waktu bukan kelipatan bulan penuh; dokumen menulis %s bulan.", durationMonthsText(contract.StartDate, *contract.EndDate)),
			})
		}
	}
	return warnings
}

// publicMailDomains are webmail providers anyone can register on: a company
// whose HR contact uses one has no company domain to restrict CC to.
var publicMailDomains = map[string]bool{
	"gmail.com": true, "googlemail.com": true, "yahoo.com": true, "yahoo.co.id": true, "ymail.com": true,
	"rocketmail.com": true, "outlook.com": true, "outlook.co.id": true, "hotmail.com": true, "hotmail.co.id": true,
	"live.com": true, "msn.com": true, "icloud.com": true, "me.com": true, "mac.com": true, "aol.com": true,
	"proton.me": true, "protonmail.com": true, "pm.me": true, "gmx.com": true, "gmx.net": true, "mail.com": true,
	"zoho.com": true, "zohomail.com": true, "yandex.com": true, "yandex.ru": true, "tutanota.com": true,
	"tuta.io": true, "fastmail.com": true, "hey.com": true, "qq.com": true, "163.com": true, "126.com": true,
	"rediffmail.com": true, "mail.ru": true, "inbox.com": true,
}

// isPublicMailDomain reports a webmail provider domain (including the
// country variants yahoo.*, hotmail.*, outlook.*, live.*).
func isPublicMailDomain(domain string) bool {
	if publicMailDomains[domain] {
		return true
	}
	for _, provider := range []string{"yahoo.", "hotmail.", "outlook.", "live.", "windowslive."} {
		if strings.HasPrefix(domain, provider) {
			return true
		}
	}
	return false
}

// contractCcDomain is the domain CC addresses must belong to: the domain of
// the company HR contact e-mail. "" (no CC allowed) when none is set or it
// is a public webmail domain, which would let anyone be CC'd.
func contractCcDomain(hrContactEmail string) string {
	_, domain, ok := strings.Cut(strings.ToLower(strings.TrimSpace(hrContactEmail)), "@")
	if !ok {
		return ""
	}
	domain = strings.TrimSpace(domain)
	if domain == "" || isPublicMailDomain(domain) {
		return ""
	}
	return domain
}

// normalizeContractCc lower-cases and de-duplicates the CC list and checks
// every address is on domain and is not the recipient itself.
func normalizeContractCc(input []string, domain string, recipient string) ([]string, error) {
	cc := []string{}
	seen := map[string]bool{}
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	for _, raw := range input {
		address := strings.ToLower(strings.TrimSpace(raw))
		if address == "" {
			continue
		}
		if seen[address] || address == recipient {
			continue
		}
		local, addrDomain, ok := strings.Cut(address, "@")
		if !ok || local == "" || validatePersonalEmail(address) != nil {
			return nil, ErrContractCcInvalid
		}
		if domain == "" || addrDomain != domain {
			return nil, ErrContractCcDomain
		}
		seen[address] = true
		cc = append(cc, address)
	}
	if len(cc) > 5 {
		return nil, ErrContractCcInvalid
	}
	return cc, nil
}
