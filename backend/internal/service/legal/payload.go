package legal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/docgen"
	"github.com/kana-consultant/kantor/backend/internal/mail"
	"github.com/kana-consultant/kantor/backend/internal/model"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
)

const (
	maxTextLength     = 300
	maxTextareaLength = 4000
	maxListRows       = 60
	maxMoney          = int64(1_000_000_000_000_000)
	dateLayout        = "2006-01-02"

	// The jurisdiction printed for an Indonesian company.
	jurisdictionIndonesia = "Negara Republik Indonesia / the Republic of Indonesia"
)

var shortMonthsID = [12]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}
var weekdaysID = [7]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}

// shortDateID formats t as "31 Okt 2026" (table cells).
func shortDateID(t time.Time) string {
	return strconv.Itoa(t.Day()) + " " + shortMonthsID[t.Month()-1] + " " + strconv.Itoa(t.Year())
}

// ---------------------------------------------------------------------------
// Form values

// NormalizeFields parses saved form values against the spec: unknown keys
// are dropped, every value is converted to its canonical JSON type (strings,
// int64 amounts, booleans, lists of objects) and empty list rows are
// removed. Required fields are not checked here (a draft may be incomplete);
// see MissingFields. The returned errors are keyed by field ("termin.2.nilai"
// for a list cell).
func NormalizeFields(spec *Spec, raw json.RawMessage) (map[string]any, map[string]string) {
	values := map[string]any{}
	errs := map[string]string{}
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return values, errs
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var input map[string]any
	if err := decoder.Decode(&input); err != nil {
		errs["fields"] = "harus berupa objek JSON"
		return values, errs
	}
	for _, field := range spec.Fields {
		rawValue, present := input[field.Key]
		if !present || rawValue == nil {
			continue
		}
		if field.Type == FieldList {
			rows, ok := rawValue.([]any)
			if !ok {
				errs[field.Key] = "harus berupa daftar"
				continue
			}
			if len(rows) > maxListRows {
				errs[field.Key] = fmt.Sprintf("maksimal %d baris", maxListRows)
				continue
			}
			list := make([]any, 0, len(rows))
			for index, rawRow := range rows {
				row, ok := rawRow.(map[string]any)
				if !ok {
					errs[fmt.Sprintf("%s.%d", field.Key, index)] = "baris tidak valid"
					continue
				}
				out := map[string]any{}
				empty := true
				for _, column := range field.Columns {
					value, err := normalizeValue(column.Type, column.Options, nil, nil, row[column.Key])
					if err != "" {
						errs[fmt.Sprintf("%s.%d.%s", field.Key, index, column.Key)] = err
						continue
					}
					if value == nil {
						continue
					}
					out[column.Key] = value
					if !isBlank(value) {
						empty = false
					}
				}
				if !empty {
					// A filled row gets the column defaults for cells left
					// empty (an item's status defaults to "Diterima").
					for _, column := range field.Columns {
						if column.Default != nil && isBlank(out[column.Key]) {
							out[column.Key] = column.Default
						}
					}
					list = append(list, out)
				}
			}
			values[field.Key] = list
			continue
		}
		value, err := normalizeValue(field.Type, field.Options, field.Min, field.Max, rawValue)
		if err != "" {
			errs[field.Key] = err
			continue
		}
		if value != nil {
			values[field.Key] = value
		}
	}
	return values, errs
}

func isBlank(value any) bool {
	switch v := value.(type) {
	case string:
		return v == ""
	case nil:
		return true
	}
	return false
}

// normalizeValue converts one value; nil means "no value".
func normalizeValue(fieldType FieldType, options []Option, min *int64, max *int64, raw any) (any, string) {
	if raw == nil {
		return nil, ""
	}
	switch fieldType {
	case FieldText, FieldTextarea, FieldSelect, FieldEmail:
		s, ok := raw.(string)
		if !ok {
			if number, isNumber := raw.(json.Number); isNumber {
				s = number.String()
			} else {
				return nil, "harus berupa teks"
			}
		}
		s = strings.TrimSpace(s)
		limit := maxTextLength
		if fieldType == FieldTextarea {
			limit = maxTextareaLength
		}
		if len([]rune(s)) > limit {
			return nil, fmt.Sprintf("maksimal %d karakter", limit)
		}
		if s == "" {
			return "", ""
		}
		if fieldType == FieldSelect && len(options) > 0 && !hasOption(options, s) {
			return nil, "pilihan tidak valid"
		}
		if fieldType == FieldEmail {
			if err := mail.ValidateAddress(s); err != nil {
				return nil, "alamat email tidak valid"
			}
		}
		return s, ""
	case FieldDate:
		s, ok := raw.(string)
		if !ok {
			return nil, "harus berupa tanggal"
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return "", ""
		}
		if _, err := time.Parse(dateLayout, s); err != nil {
			return nil, "format tanggal harus YYYY-MM-DD"
		}
		return s, ""
	case FieldMoney, FieldNumber, FieldPercent:
		value, ok, blank := toInt(raw)
		if blank {
			return nil, ""
		}
		if !ok {
			return nil, "harus berupa angka bulat"
		}
		lo, hi := int64(0), maxMoney
		switch fieldType {
		case FieldPercent:
			hi = 100
		case FieldNumber:
			if min != nil {
				lo = *min
			}
			hi = 1_000_000
			if max != nil {
				hi = *max
			}
		}
		if value < lo || value > hi {
			return nil, fmt.Sprintf("harus antara %d dan %d", lo, hi)
		}
		return value, ""
	case FieldBool:
		b, ok := raw.(bool)
		if !ok {
			return nil, "harus berupa ya/tidak"
		}
		return b, ""
	}
	return nil, "jenis isian tidak dikenal"
}

func hasOption(options []Option, value string) bool {
	for _, option := range options {
		if option.Value == value {
			return true
		}
	}
	return false
}

// toInt accepts a JSON number or a string of digits (thousand separators
// '.' or ',' allowed). blank reports an empty string.
func toInt(raw any) (value int64, ok bool, blank bool) {
	switch v := raw.(type) {
	case json.Number:
		i, err := v.Int64()
		return i, err == nil, false
	case float64:
		if v != float64(int64(v)) {
			return 0, false, false
		}
		return int64(v), true, false
	case int64:
		return v, true, false
	case int:
		return int64(v), true, false
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return 0, false, true
		}
		s = strings.NewReplacer(".", "", ",", "", " ", "").Replace(s)
		i, err := strconv.ParseInt(s, 10, 64)
		return i, err == nil, false
	}
	return 0, false, false
}

// WithDefaults returns values with every missing field that has a default
// filled in (a new document starts from these).
func WithDefaults(spec *Spec, values map[string]any) map[string]any {
	out := make(map[string]any, len(spec.Fields))
	for key, value := range values {
		out[key] = value
	}
	for _, field := range spec.Fields {
		if _, ok := out[field.Key]; ok || field.Default == nil {
			continue
		}
		switch d := field.Default.(type) {
		case int:
			out[field.Key] = int64(d)
		case []map[string]any:
			rows := make([]any, 0, len(d))
			for _, row := range d {
				copied := map[string]any{}
				for k, v := range row {
					copied[k] = v
				}
				rows = append(rows, copied)
			}
			out[field.Key] = rows
		default:
			out[field.Key] = d
		}
	}
	return out
}

// visible reports whether a field is shown for these values (ShowWhen).
func visible(field Field, values map[string]any) bool {
	if field.ShowWhen == nil {
		return true
	}
	value, ok := values[field.ShowWhen.Field]
	if !ok {
		return false
	}
	return fmt.Sprint(value) == fmt.Sprint(field.ShowWhen.Equals)
}

// MissingFields lists the required fields (and required list cells) that
// have no value, keyed like NormalizeFields' errors. A hidden field
// (ShowWhen) is never required.
func MissingFields(spec *Spec, values map[string]any) map[string]string {
	missing := map[string]string{}
	for _, field := range spec.Fields {
		if !visible(field, values) {
			continue
		}
		value, ok := values[field.Key]
		if field.Type == FieldList {
			rows, _ := value.([]any)
			if field.Required && len(rows) == 0 {
				missing[field.Key] = "isi minimal satu baris"
			}
			for index, rawRow := range rows {
				row, _ := rawRow.(map[string]any)
				for _, column := range field.Columns {
					if column.Required && isBlank(row[column.Key]) {
						missing[fmt.Sprintf("%s.%d.%s", field.Key, index, column.Key)] = "wajib diisi"
					}
				}
			}
			continue
		}
		if field.Required && (!ok || isBlank(value)) {
			missing[field.Key] = "wajib diisi"
		}
	}
	return missing
}

// ---------------------------------------------------------------------------
// Template payload

// PartySecrets are the decrypted sensitive values of the party.
type PartySecrets struct {
	IDNumber    string
	TaxID       string
	BankAccount string
}

type payloadInput struct {
	spec         *Spec
	values       map[string]any
	company      authrepo.CompanyProfileRecord
	party        model.LegalParty
	secrets      PartySecrets
	docNumber    string
	documentDate time.Time
	documentCity string
	payload      docgen.Payload
}

// FormatNumber renders a document number: "007/FRL/CTN/X/2026" (the company
// code segment is left out when the profile has none).
func FormatNumber(code string, seq int, docCode string, documentDate time.Time) string {
	tail := fmt.Sprintf("%s/%d", docgen.RomanMonth(documentDate.Month()), documentDate.Year())
	if c := strings.ToUpper(strings.TrimSpace(docCode)); c != "" {
		tail = c + "/" + tail
	}
	return fmt.Sprintf("%03d/%s/%s", seq, code, tail)
}

// BuildPayload maps the form values, the company profile and the party onto
// the template variables of the document type.
func BuildPayload(spec *Spec, values map[string]any, company authrepo.CompanyProfileRecord, party model.LegalParty, secrets PartySecrets, docNumber string, documentDate time.Time, documentCity string) docgen.Payload {
	in := &payloadInput{
		spec: spec, values: WithDefaults(spec, values), company: company, party: party, secrets: secrets,
		docNumber: docNumber, documentDate: documentDate, documentCity: documentCity,
		payload: docgen.Payload{},
	}
	p := in.payload
	p["dokumen_nomor"] = docNumber
	p["dokumen_kota"] = docgen.SingleLine(documentCity)
	p["dokumen_tanggal"] = docgen.FormatDateID(documentDate)
	p["dokumen_tanggal_en"] = docgen.FormatDateEN(documentDate)

	for _, field := range spec.Fields {
		value := in.values[field.Key]
		switch field.Type {
		case FieldList:
			rows, _ := value.([]any)
			list := make([]any, 0, len(rows))
			for index, rawRow := range rows {
				row, _ := rawRow.(map[string]any)
				out := map[string]any{}
				for _, column := range field.Columns {
					out[column.Key] = formatCell(column.Type, row[column.Key])
				}
				if field.NoColumn {
					out["no"] = field.AutoNumber + strconv.Itoa(index+1)
				}
				list = append(list, out)
			}
			p[field.Key] = list
		case FieldBool:
			b, _ := value.(bool)
			p[field.Key] = b
		case FieldDate:
			s, _ := value.(string)
			if t, err := time.Parse(dateLayout, s); err == nil {
				p[field.Key] = docgen.FormatDateID(t)
				p[field.Key+"_en"] = docgen.FormatDateEN(t)
			} else {
				p[field.Key] = ""
				p[field.Key+"_en"] = ""
			}
		case FieldMoney:
			if amount, ok := value.(int64); ok {
				p[field.Key] = docgen.Thousands(amount)
			} else {
				p[field.Key] = ""
			}
		case FieldNumber, FieldPercent:
			if number, ok := value.(int64); ok {
				p[field.Key] = strconv.FormatInt(number, 10)
			} else {
				p[field.Key] = ""
			}
		default:
			s, _ := value.(string)
			p[field.Key] = docgen.SingleLine(s)
		}
	}

	// The company signs as perusahaan (freelance) or penyedia (SOW, SLA,
	// BAST); the subcontract maps both sides itself.
	for _, prefix := range []string{"perusahaan", "penyedia"} {
		p[prefix+"_nama"] = docgen.SingleLine(company.LegalName)
		p[prefix+"_alamat"] = docgen.SingleLine(company.Address)
		p[prefix+"_penandatangan_nama"] = docgen.SingleLine(company.SignerName)
		p[prefix+"_penandatangan_jabatan"] = docgen.SingleLine(company.SignerTitle)
	}
	p["perusahaan_jenis_usaha"] = docgen.SingleLine(company.BusinessType)

	// The party as client (SOW, SLA, BAST).
	p["klien_nama"] = docgen.SingleLine(party.Name)
	p["klien_alamat"] = docgen.SingleLine(party.Address)
	p["klien_penandatangan_nama"] = docgen.SingleLine(firstNonEmpty(party.RepresentativeName, party.Name))
	p["klien_penandatangan_jabatan"] = docgen.SingleLine(firstNonEmpty(party.RepresentativeTitle, "-"))

	spec.build(in)
	return p
}

func formatCell(columnType FieldType, value any) any {
	switch columnType {
	case FieldDate:
		s, _ := value.(string)
		if t, err := time.Parse(dateLayout, s); err == nil {
			return shortDateID(t)
		}
		return ""
	case FieldMoney:
		if amount, ok := value.(int64); ok {
			return docgen.Thousands(amount)
		}
		return ""
	case FieldPercent:
		if percent, ok := value.(int64); ok {
			return strconv.FormatInt(percent, 10) + "%"
		}
		return ""
	case FieldNumber:
		if number, ok := value.(int64); ok {
			return strconv.FormatInt(number, 10)
		}
		return ""
	}
	s, _ := value.(string)
	return docgen.SingleLine(s)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// listSum adds up a money column of a list value.
func listSum(value any, column string) int64 {
	rows, _ := value.([]any)
	var total int64
	for _, rawRow := range rows {
		row, _ := rawRow.(map[string]any)
		if amount, ok := row[column].(int64); ok {
			total += amount
		}
	}
	return total
}

func buildFreelance(in *payloadInput) {
	p, party, secrets := in.payload, in.party, in.secrets
	p["mitra_nama"] = docgen.SingleLine(party.Name)
	p["mitra_nik"] = docgen.SingleLine(secrets.IDNumber)
	p["mitra_npwp"] = docgen.SingleLine(secrets.TaxID)
	p["mitra_alamat"] = docgen.SingleLine(party.Address)
	p["mitra_telepon"] = docgen.SingleLine(party.Phone)
	p["mitra_email"] = docgen.SingleLine(party.Email)
	p["mitra_bank"] = docgen.SingleLine(party.BankName)
	p["mitra_norek"] = docgen.SingleLine(secrets.BankAccount)
	p["mitra_nama_rekening"] = docgen.SingleLine(firstNonEmpty(party.BankAccountHolder, party.Name))
	p["total_fee"] = docgen.Thousands(listSum(in.values["pekerjaan"], "fee"))
}

func buildSOW(in *payloadInput) {
	in.payload["nilai_total"] = docgen.Thousands(listSum(in.values["milestone"], "nilai"))
}

func buildBAST(in *payloadInput) {
	weekday := in.documentDate.Weekday()
	in.payload["hari"] = weekdaysID[weekday]
	in.payload["hari_en"] = weekday.String()
}

// amountInWords spells a contract value: rupiah in Indonesian words, other
// currencies with their name.
func amountInWords(amount int64, currency string) string {
	switch currency {
	case "USD":
		return docgen.Terbilang(amount) + " dolar Amerika Serikat"
	case "SGD":
		return docgen.Terbilang(amount) + " dolar Singapura"
	}
	return docgen.TerbilangRupiah(amount)
}

// buildSubcontract fills the kontraktor_* and subkontraktor_* variables from
// the company and the party, by the company's role, and the payee account:
// the party's account when the party is the subcontractor, the company
// account entered in the form otherwise.
func buildSubcontract(in *payloadInput) {
	p, values, company, party, secrets := in.payload, in.values, in.company, in.party, in.secrets
	companyIsMain := fmt.Sprint(values["peran_perusahaan"]) != "subkontraktor"
	email, _ := values["kontak_email_perusahaan"].(string)

	us := map[string]any{
		"nama":                  company.LegalName,
		"badan_usaha":           true,
		"alamat":                company.Address,
		"negara":                jurisdictionIndonesia,
		"no_identitas":          "",
		"npwp":                  "",
		"email":                 email,
		"penandatangan_nama":    company.SignerName,
		"penandatangan_jabatan": company.SignerTitle,
	}
	partyIsCompany := party.PartyType == model.LegalPartyCompany
	identity := secrets.IDNumber
	if !partyIsCompany && identity != "" && !strings.HasPrefix(strings.ToUpper(identity), "NIK") {
		identity = "NIK " + identity
	}
	roleLabel := "Subkontraktor / Subcontractor"
	if !companyIsMain {
		roleLabel = "Kontraktor Utama / Main Contractor"
	}
	them := map[string]any{
		"nama":         party.Name,
		"badan_usaha":  partyIsCompany,
		"alamat":       party.Address,
		"negara":       firstNonEmpty(party.Jurisdiction, jurisdictionIndonesia),
		"no_identitas": identity,
		"npwp":         secrets.TaxID,
		"email":        party.Email,
	}
	if partyIsCompany {
		them["penandatangan_nama"] = firstNonEmpty(party.RepresentativeName, party.Name)
		them["penandatangan_jabatan"] = firstNonEmpty(party.RepresentativeTitle, roleLabel)
	} else {
		them["penandatangan_nama"] = party.Name
		them["penandatangan_jabatan"] = roleLabel
	}

	main, sub := us, them
	if !companyIsMain {
		main, sub = them, us
	}
	for key, value := range main {
		if s, ok := value.(string); ok {
			value = docgen.SingleLine(s)
		}
		p["kontraktor_"+key] = value
	}
	for key, value := range sub {
		if s, ok := value.(string); ok {
			value = docgen.SingleLine(s)
		}
		p["subkontraktor_"+key] = value
	}

	if companyIsMain {
		p["subkontraktor_bank"] = docgen.SingleLine(party.BankName)
		p["subkontraktor_norek"] = docgen.SingleLine(secrets.BankAccount)
		p["subkontraktor_nama_rekening"] = docgen.SingleLine(firstNonEmpty(party.BankAccountHolder, party.Name))
		p["subkontraktor_swift"] = docgen.SingleLine(party.BankSwift)
	} else {
		text := func(key string) string {
			s, _ := values[key].(string)
			return docgen.SingleLine(s)
		}
		p["subkontraktor_bank"] = text("rekening_bank")
		p["subkontraktor_norek"] = text("rekening_nomor")
		p["subkontraktor_nama_rekening"] = text("rekening_nama")
		p["subkontraktor_swift"] = text("rekening_swift")
	}

	amount, _ := values["nilai_kontrak"].(int64)
	currency, _ := values["mata_uang"].(string)
	p["nilai_kontrak_terbilang"] = amountInWords(amount, currency)
}
