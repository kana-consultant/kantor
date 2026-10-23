package legal

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kana-consultant/kantor/backend/internal/docgen"
	"github.com/kana-consultant/kantor/backend/internal/model"
	authrepo "github.com/kana-consultant/kantor/backend/internal/repository/auth"
)

func testCompany() authrepo.CompanyProfileRecord {
	return authrepo.CompanyProfileRecord{
		LegalName: "PT Contoh Teknologi Nusantara", Address: "Jl. Contoh Raya No. 10, Jakarta",
		BusinessType: "pengembangan perangkat lunak", City: "Jakarta",
		SignerName: "Rudi Hartono", SignerTitle: "Direktur", HRContactEmail: "hr@contoh.co.id", DocCode: "CTN",
	}
}

// sampleValues fills every field of a spec with a plausible value.
func sampleValues(spec *Spec) map[string]any {
	values := map[string]any{}
	for _, field := range spec.Fields {
		switch field.Type {
		case FieldList:
			row := map[string]any{}
			for _, column := range field.Columns {
				row[column.Key] = sampleCell(column)
			}
			values[field.Key] = []any{row, row}
		default:
			values[field.Key] = sampleCell(Column{Key: field.Key, Type: field.Type, Options: field.Options})
		}
	}
	return values
}

func sampleCell(column Column) any {
	switch column.Type {
	case FieldDate:
		return "2026-10-31"
	case FieldMoney:
		return json.Number("15000000")
	case FieldNumber:
		return json.Number("5")
	case FieldPercent:
		return json.Number("40")
	case FieldBool:
		return true
	case FieldEmail:
		return "pic@contoh.co.id"
	case FieldSelect:
		return column.Options[len(column.Options)-1].Value
	}
	return "Isi " + column.Key
}

func documentText(t *testing.T, docx []byte) string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	t.Fatal("no word/document.xml")
	return ""
}

// TestBuildPayloadRendersEveryTemplate: every document type, for both party
// types and (subcontract) both roles, fills every variable its template
// uses, so the render never fails on a missing value.
func TestBuildPayloadRendersEveryTemplate(t *testing.T) {
	date := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	for _, spec := range Specs() {
		for _, partyType := range spec.PartyTypes {
			for _, role := range []string{"kontraktor_utama", "subkontraktor"} {
				if spec.Type != model.LegalDocSubcontract && role == "subkontraktor" {
					continue
				}
				name := spec.Type + "/" + partyType + "/" + role
				t.Run(name, func(t *testing.T) {
					raw, _ := json.Marshal(sampleValues(spec))
					values, errs := NormalizeFields(spec, raw)
					if len(errs) > 0 {
						t.Fatalf("normalize: %v", errs)
					}
					if spec.Type == model.LegalDocSubcontract {
						values["peran_perusahaan"] = role
					}
					if missing := MissingFields(spec, values); len(missing) > 0 {
						t.Fatalf("missing: %v", missing)
					}
					party := model.LegalParty{
						PartyType: partyType, Name: "Mitra Contoh", Address: "Jl. Mawar 1", Email: "mitra@example.com",
						RepresentativeName: "Budi", RepresentativeTitle: "Direktur", BankName: "BNI", BankAccountHolder: "Mitra Contoh",
					}
					secrets := PartySecrets{IDNumber: "3174020101950002", TaxID: "12.345.678.9-012.000", BankAccount: "1400001234567"}
					number := FormatNumber(spec.Code, 7, "CTN", date)
					payload := BuildPayload(spec, values, testCompany(), party, secrets, number, date, "Jakarta")
					tpl, err := docgen.Load(spec.Template)
					if err != nil {
						t.Fatal(err)
					}
					out, err := docgen.Render(tpl, payload)
					if err != nil {
						t.Fatalf("render: %v", err)
					}
					text := documentText(t, out)
					if strings.Contains(text, "{{") {
						t.Fatal("output still contains placeholders")
					}
					if !strings.Contains(text, number) {
						t.Fatalf("document number %q not printed", number)
					}
				})
			}
		}
	}
}

func TestSubcontractRoles(t *testing.T) {
	spec, _ := SpecFor(model.LegalDocSubcontract)
	date := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	party := model.LegalParty{PartyType: model.LegalPartyPerson, Name: "Dimas Nugroho", BankName: "BNI", Email: "dimas@example.com"}
	secrets := PartySecrets{IDNumber: "3175030303930003", BankAccount: "0987654321"}

	main := BuildPayload(spec, map[string]any{"peran_perusahaan": "kontraktor_utama", "nilai_kontrak": int64(45_000_000), "mata_uang": "IDR"}, testCompany(), party, secrets, "001/SUB/CTN/X/2026", date, "Jakarta")
	if main["kontraktor_nama"] != "PT Contoh Teknologi Nusantara" || main["subkontraktor_nama"] != "Dimas Nugroho" {
		t.Fatalf("company as main contractor: %v / %v", main["kontraktor_nama"], main["subkontraktor_nama"])
	}
	if main["subkontraktor_badan_usaha"] != false || main["subkontraktor_no_identitas"] != "NIK 3175030303930003" {
		t.Fatalf("person subcontractor identity: %v %v", main["subkontraktor_badan_usaha"], main["subkontraktor_no_identitas"])
	}
	if main["subkontraktor_norek"] != "0987654321" {
		t.Fatalf("payee should be the party: %v", main["subkontraktor_norek"])
	}
	if main["nilai_kontrak_terbilang"] != "empat puluh lima juta rupiah" {
		t.Fatalf("terbilang: %v", main["nilai_kontrak_terbilang"])
	}

	sub := BuildPayload(spec, map[string]any{"peran_perusahaan": "subkontraktor", "rekening_bank": "Mandiri", "rekening_nomor": "123"}, testCompany(), party, secrets, "002/SUB/CTN/X/2026", date, "Jakarta")
	if sub["kontraktor_nama"] != "Dimas Nugroho" || sub["subkontraktor_nama"] != "PT Contoh Teknologi Nusantara" {
		t.Fatalf("company as subcontractor: %v / %v", sub["kontraktor_nama"], sub["subkontraktor_nama"])
	}
	if sub["subkontraktor_norek"] != "123" || sub["subkontraktor_bank"] != "Mandiri" {
		t.Fatalf("payee should be the company account: %v", sub["subkontraktor_norek"])
	}
}

func TestNormalizeFields(t *testing.T) {
	spec, _ := SpecFor(model.LegalDocFreelance)
	raw := []byte(`{"nilai_fee": "15.000.000", "termin_bayar_hari": 500, "tanggal_mulai": "31-10-2026", "unknown": 1,
		"pekerjaan": [{"nama": "", "fee": ""}, {"nama": "Desain", "fee": 2000000, "tenggat": "2026-11-01"}]}`)
	values, errs := NormalizeFields(spec, raw)
	if values["nilai_fee"] != int64(15_000_000) {
		t.Fatalf("money with separators: %v", values["nilai_fee"])
	}
	if errs["termin_bayar_hari"] == "" || errs["tanggal_mulai"] == "" {
		t.Fatalf("expected range and date errors, got %v", errs)
	}
	if _, ok := values["unknown"]; ok {
		t.Fatal("unknown key kept")
	}
	rows, _ := values["pekerjaan"].([]any)
	if len(rows) != 1 {
		t.Fatalf("empty rows should be dropped, got %d", len(rows))
	}
	if missing := MissingFields(spec, values); missing["lingkup_jasa"] == "" {
		t.Fatalf("lingkup_jasa should be missing: %v", missing)
	}
}

func TestFormatNumber(t *testing.T) {
	date := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	if got := FormatNumber("FRL", 7, "ctn", date); got != "007/FRL/CTN/X/2026" {
		t.Fatalf("got %q", got)
	}
	if got := FormatNumber("BAST", 12, "", date); got != "012/BAST/X/2026" {
		t.Fatalf("got %q", got)
	}
}
