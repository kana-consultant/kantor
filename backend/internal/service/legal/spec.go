// Package legal runs the Dokumen Legal workflow (Operasional): agreements and
// reports with parties outside the company, generated from templates 03-07
// of the legal pack. Each document type declares its form fields once (Spec);
// the API serves them to the web form, validates saved values against them
// and maps them onto the template variables.
package legal

import (
	"github.com/kana-consultant/kantor/backend/internal/docgen"
	"github.com/kana-consultant/kantor/backend/internal/model"
)

// FieldType is how a form value is entered, validated and printed.
type FieldType string

const (
	FieldText     FieldType = "text"     // one line
	FieldTextarea FieldType = "textarea" // a paragraph (printed on one line)
	FieldDate     FieldType = "date"     // YYYY-MM-DD; printed "1 Oktober 2026" (and <key>_en)
	FieldMoney    FieldType = "money"    // integer amount; printed "15.000.000"
	FieldNumber   FieldType = "number"   // integer within Min..Max
	FieldPercent  FieldType = "percent"  // integer 0-100; printed "40%" (list columns)
	FieldEmail    FieldType = "email"    // one address
	FieldSelect   FieldType = "select"   // one of Options; the value is what is printed
	FieldBool     FieldType = "bool"
	FieldList     FieldType = "list" // table rows of Columns
)

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Column is a column of a list field.
type Column struct {
	Key      string    `json:"key"`
	Label    string    `json:"label"`
	Type     FieldType `json:"type"`
	Required bool      `json:"required,omitempty"`
	Options  []Option  `json:"options,omitempty"`
	Default  any       `json:"default,omitempty"`
}

// Condition shows a field only when another field has a value.
type Condition struct {
	Field  string `json:"field"`
	Equals any    `json:"equals"`
}

// Field is one form value. Its Key is the template variable it fills.
type Field struct {
	Key         string     `json:"key"`
	Label       string     `json:"label"`
	Type        FieldType  `json:"type"`
	Section     string     `json:"section"`
	Required    bool       `json:"required,omitempty"`
	Default     any        `json:"default,omitempty"`
	Placeholder string     `json:"placeholder,omitempty"`
	Help        string     `json:"help,omitempty"`
	Options     []Option   `json:"options,omitempty"`
	Min         *int64     `json:"min,omitempty"`
	Max         *int64     `json:"max,omitempty"`
	Columns     []Column   `json:"columns,omitempty"`
	ShowWhen    *Condition `json:"show_when,omitempty"`
	// AutoNumber prefixes the "no" column of a list ("D" gives D1, D2...).
	AutoNumber string `json:"-"`
	// NoColumn: the template row has a "no" cell filled automatically.
	NoColumn bool `json:"-"`
}

// Spec describes one document type.
type Spec struct {
	Type        string   `json:"type"`
	Code        string   `json:"code"`
	Label       string   `json:"label"`
	ShortLabel  string   `json:"short_label"`
	Description string   `json:"description"`
	PartyLabel  string   `json:"party_label"`
	PartyTypes  []string `json:"party_types"`
	// RelatedTypes: documents this one may build on (a BAST on its SOW).
	RelatedTypes []string            `json:"related_types"`
	Fields       []Field             `json:"fields"`
	Template     docgen.TemplateID   `json:"-"`
	sequence     string              // document_sequences.doc_type
	build        func(*payloadInput) // type-specific variables
}

func n(v int64) *int64 { return &v }

var taxOptions = []Option{
	{Value: "belum termasuk PPN / exclusive of VAT", Label: "Belum termasuk PPN"},
	{Value: "sudah termasuk pajak yang berlaku / inclusive of applicable taxes", Label: "Sudah termasuk pajak"},
}

var currencyOptions = []Option{
	{Value: "IDR", Label: "IDR (Rupiah)"},
	{Value: "USD", Label: "USD (Dolar AS)"},
	{Value: "SGD", Label: "SGD (Dolar Singapura)"},
}

func itemList(key string, label string, section string, required bool) Field {
	return Field{
		Key: key, Label: label, Type: FieldList, Section: section, Required: required, NoColumn: true,
		Columns: []Column{{Key: "item", Label: "Uraian", Type: FieldText, Required: true}},
	}
}

func forumFields(section string) []Field {
	return []Field{
		{Key: "forum_sengketa", Label: "Forum sengketa (Indonesia)", Type: FieldText, Section: section, Required: true, Placeholder: "Pengadilan Negeri Jakarta Selatan", Help: "Pengadilan negeri atau arbitrase (mis. BANI) yang dipilih bila musyawarah gagal"},
		{Key: "forum_sengketa_en", Label: "Forum sengketa (Inggris)", Type: FieldText, Section: section, Required: true, Placeholder: "the South Jakarta District Court"},
	}
}

var specs = []*Spec{
	{
		Type: model.LegalDocFreelance, Code: "FRL", Template: docgen.TemplateFreelance, sequence: "LEGAL_FRL",
		Label: "Perjanjian Kemitraan Freelance", ShortLabel: "Freelance",
		Description: "Talent perorangan yang bukan karyawan: lingkup jasa, fee, penagihan, HKI, dan Lampiran A daftar pekerjaan.",
		PartyLabel:  "Mitra (freelancer)", PartyTypes: []string{model.LegalPartyPerson},
		build: buildFreelance,
		Fields: append([]Field{
			{Key: "lingkup_jasa", Label: "Ringkasan jasa", Type: FieldTextarea, Section: "Lingkup & jangka waktu", Required: true, Placeholder: "mis. pengembangan frontend (React) dan integrasi API untuk proyek klien"},
			{Key: "tanggal_mulai", Label: "Tanggal mulai", Type: FieldDate, Section: "Lingkup & jangka waktu", Required: true},
			{Key: "tanggal_berakhir", Label: "Tanggal berakhir", Type: FieldDate, Section: "Lingkup & jangka waktu", Required: true},
			{Key: "skema_fee", Label: "Skema fee", Type: FieldSelect, Section: "Fee & pembayaran", Required: true, Default: "per proyek / per project", Options: []Option{
				{Value: "per proyek / per project", Label: "Per proyek"},
				{Value: "per jam / per hour", Label: "Per jam"},
				{Value: "retainer bulanan / monthly retainer", Label: "Retainer bulanan"},
			}},
			{Key: "nilai_fee", Label: "Nilai fee (Rp)", Type: FieldMoney, Section: "Fee & pembayaran", Required: true},
			{Key: "satuan_fee", Label: "Satuan fee", Type: FieldText, Section: "Fee & pembayaran", Required: true, Default: "per milestone", Help: "Tercetak setelah nilai fee, mis. per milestone, per jam, per bulan"},
			{Key: "periode_tagihan", Label: "Waktu penagihan", Type: FieldText, Section: "Fee & pembayaran", Required: true, Default: "setiap milestone selesai / upon completion of each milestone"},
			{Key: "termin_bayar_hari", Label: "Jatuh tempo pembayaran (hari)", Type: FieldNumber, Section: "Fee & pembayaran", Required: true, Default: 14, Min: n(1), Max: n(120)},
			{Key: "hari_review", Label: "Masa pemeriksaan deliverable (hari kerja)", Type: FieldNumber, Section: "Ketentuan", Required: true, Default: 5, Min: n(1), Max: n(60)},
			{Key: "jumlah_revisi", Label: "Revisi tanpa biaya", Type: FieldNumber, Section: "Ketentuan", Required: true, Default: 2, Min: n(0), Max: n(20)},
			{Key: "masa_kerahasiaan_tahun", Label: "Kerahasiaan setelah berakhir (tahun)", Type: FieldNumber, Section: "Ketentuan", Required: true, Default: 3, Min: n(0), Max: n(30)},
			{Key: "masa_non_solicit_bulan", Label: "Larangan membujuk (bulan)", Type: FieldNumber, Section: "Ketentuan", Required: true, Default: 12, Min: n(0), Max: n(60)},
			{Key: "hari_pemberitahuan", Label: "Pemberitahuan pengakhiran (hari)", Type: FieldNumber, Section: "Ketentuan", Required: true, Default: 14, Min: n(0), Max: n(180)},
		}, append(forumFields("Ketentuan"),
			Field{Key: "pekerjaan", Label: "Lampiran A — daftar pekerjaan", Type: FieldList, Section: "Lampiran A", Required: true, NoColumn: true, Columns: []Column{
				{Key: "nama", Label: "Pekerjaan", Type: FieldText, Required: true},
				{Key: "deliverable", Label: "Deliverable", Type: FieldText},
				{Key: "tenggat", Label: "Tenggat", Type: FieldDate},
				{Key: "fee", Label: "Fee (Rp)", Type: FieldMoney},
			}},
		)...),
	},
	{
		Type: model.LegalDocSubcontract, Code: "SUB", Template: docgen.TemplateSubcontract, sequence: "LEGAL_SUB",
		Label: "Perjanjian Subkontrak", ShortLabel: "Subkontrak",
		Description: "Pekerjaan proyek klien yang disubkontrakkan. Perusahaan bisa menjadi Kontraktor Utama atau Subkontraktor.",
		PartyLabel:  "Pihak lain (subkontraktor atau kontraktor utama)", PartyTypes: []string{model.LegalPartyPerson, model.LegalPartyCompany},
		build: buildSubcontract,
		Fields: append([]Field{
			{Key: "peran_perusahaan", Label: "Peran perusahaan kita", Type: FieldSelect, Section: "Peran & proyek", Required: true, Default: "kontraktor_utama", Options: []Option{
				{Value: "kontraktor_utama", Label: "Kontraktor Utama (pihak lain sebagai Subkontraktor)"},
				{Value: "subkontraktor", Label: "Subkontraktor (pihak lain sebagai Kontraktor Utama)"},
			}},
			{Key: "klien_akhir_nama", Label: "Klien akhir", Type: FieldText, Section: "Peran & proyek", Required: true, Help: "Boleh diisi \"Klien yang dirahasiakan\""},
			{Key: "proyek_nama", Label: "Nama proyek", Type: FieldText, Section: "Peran & proyek", Required: true},
			{Key: "kontrak_induk_nomor", Label: "No. kontrak dengan klien akhir", Type: FieldText, Section: "Peran & proyek", Required: true},
			{Key: "sow_nomor", Label: "No. Lingkup Kerja (SOW) terkait", Type: FieldText, Section: "Peran & proyek", Required: true},
			{Key: "lingkup_pekerjaan", Label: "Ringkasan pekerjaan", Type: FieldTextarea, Section: "Pekerjaan & jangka waktu", Required: true},
			{Key: "tanggal_mulai", Label: "Tanggal mulai", Type: FieldDate, Section: "Pekerjaan & jangka waktu", Required: true},
			{Key: "tanggal_berakhir", Label: "Tanggal berakhir", Type: FieldDate, Section: "Pekerjaan & jangka waktu", Required: true},
			{Key: "subkontraktor_pic", Label: "PIC Subkontraktor", Type: FieldText, Section: "Pekerjaan & jangka waktu", Required: true},
			{Key: "kontak_email_perusahaan", Label: "Email perusahaan kita di perjanjian", Type: FieldEmail, Section: "Pekerjaan & jangka waktu", Required: true, Help: "Tercetak sebagai email perusahaan kita; bawaan: email kontak HR di Profil Perusahaan"},
			{Key: "mata_uang", Label: "Mata uang", Type: FieldSelect, Section: "Nilai & pembayaran", Required: true, Default: "IDR", Options: currencyOptions},
			{Key: "nilai_kontrak", Label: "Nilai kontrak", Type: FieldMoney, Section: "Nilai & pembayaran", Required: true},
			{Key: "ketentuan_pajak", Label: "Pajak", Type: FieldSelect, Section: "Nilai & pembayaran", Required: true, Default: taxOptions[1].Value, Options: taxOptions},
			{Key: "skema_pembayaran", Label: "Skema pembayaran", Type: FieldText, Section: "Nilai & pembayaran", Required: true, Default: "per milestone"},
			{Key: "termin", Label: "Termin pembayaran", Type: FieldList, Section: "Nilai & pembayaran", Required: true, NoColumn: true, Columns: []Column{
				{Key: "milestone", Label: "Milestone / deliverable", Type: FieldText, Required: true},
				{Key: "target_tanggal", Label: "Target", Type: FieldDate},
				{Key: "persen", Label: "%", Type: FieldPercent},
				{Key: "nilai", Label: "Nilai", Type: FieldMoney},
			}},
			{Key: "termin_bayar_hari", Label: "Jatuh tempo pembayaran (hari)", Type: FieldNumber, Section: "Nilai & pembayaran", Required: true, Default: 14, Min: n(1), Max: n(120)},
			{Key: "bayar_setelah_klien_bayar", Label: "Bayar setelah klien membayar (pay-when-paid)", Type: FieldBool, Section: "Nilai & pembayaran", Default: false},
			{Key: "batas_bayar_hari", Label: "Batas maksimal sejak BAST (hari)", Type: FieldNumber, Section: "Nilai & pembayaran", Default: 60, Min: n(1), Max: n(365), ShowWhen: &Condition{Field: "bayar_setelah_klien_bayar", Equals: true}},
			{Key: "rekening_bank", Label: "Bank perusahaan kita", Type: FieldText, Section: "Rekening perusahaan kita", ShowWhen: &Condition{Field: "peran_perusahaan", Equals: "subkontraktor"}, Help: "Diisi bila perusahaan kita Subkontraktor: pembayaran masuk ke rekening ini"},
			{Key: "rekening_nomor", Label: "Nomor rekening", Type: FieldText, Section: "Rekening perusahaan kita", ShowWhen: &Condition{Field: "peran_perusahaan", Equals: "subkontraktor"}},
			{Key: "rekening_nama", Label: "Atas nama", Type: FieldText, Section: "Rekening perusahaan kita", ShowWhen: &Condition{Field: "peran_perusahaan", Equals: "subkontraktor"}},
			{Key: "rekening_swift", Label: "Kode SWIFT (bank luar negeri saja)", Type: FieldText, Section: "Rekening perusahaan kita", ShowWhen: &Condition{Field: "peran_perusahaan", Equals: "subkontraktor"}},
			{Key: "hari_review", Label: "Masa pemeriksaan deliverable (hari kerja)", Type: FieldNumber, Section: "Ketentuan", Required: true, Default: 5, Min: n(1), Max: n(60)},
			{Key: "masa_garansi_hari", Label: "Masa perbaikan cacat (hari)", Type: FieldNumber, Section: "Ketentuan", Required: true, Default: 30, Min: n(0), Max: n(365)},
			{Key: "masa_kerahasiaan_tahun", Label: "Kerahasiaan setelah berakhir (tahun)", Type: FieldNumber, Section: "Ketentuan", Required: true, Default: 3, Min: n(0), Max: n(30)},
			{Key: "jam_lapor_insiden", Label: "Batas lapor insiden data (jam)", Type: FieldNumber, Section: "Ketentuan", Required: true, Default: 24, Min: n(1), Max: n(720)},
			{Key: "masa_non_solicit_bulan", Label: "Larangan membujuk (bulan)", Type: FieldNumber, Section: "Ketentuan", Required: true, Default: 12, Min: n(0), Max: n(60)},
			{Key: "hari_pemberitahuan", Label: "Pemberitahuan pengakhiran (hari)", Type: FieldNumber, Section: "Ketentuan", Required: true, Default: 14, Min: n(0), Max: n(180)},
			{Key: "hari_perbaikan", Label: "Masa perbaikan pelanggaran (hari)", Type: FieldNumber, Section: "Ketentuan", Required: true, Default: 14, Min: n(0), Max: n(180)},
			{Key: "hukum_yang_berlaku", Label: "Hukum yang berlaku (Indonesia)", Type: FieldText, Section: "Hukum & sengketa", Required: true, Default: "Negara Republik Indonesia"},
			{Key: "hukum_yang_berlaku_en", Label: "Hukum yang berlaku (Inggris)", Type: FieldText, Section: "Hukum & sengketa", Required: true, Default: "the Republic of Indonesia"},
		}, append(forumFields("Hukum & sengketa"),
			Field{Key: "bahasa_yang_berlaku", Label: "Bahasa yang berlaku (Indonesia)", Type: FieldText, Section: "Hukum & sengketa", Required: true, Default: "Bahasa Indonesia"},
			Field{Key: "bahasa_yang_berlaku_en", Label: "Bahasa yang berlaku (Inggris)", Type: FieldText, Section: "Hukum & sengketa", Required: true, Default: "Indonesian"},
		)...),
	},
	{
		Type: model.LegalDocSOW, Code: "SOW", Template: docgen.TemplateSOW, sequence: "LEGAL_SOW",
		Label: "Lingkup Kerja (SOW)", ShortLabel: "SOW",
		Description: "Lampiran per proyek: lingkup, deliverable, milestone dan pembayaran, kriteria UAT, tim.",
		PartyLabel:  "Klien", PartyTypes: []string{model.LegalPartyCompany, model.LegalPartyPerson},
		build: buildSOW,
		Fields: []Field{
			{Key: "proyek_nama", Label: "Nama proyek", Type: FieldText, Section: "Proyek", Required: true},
			{Key: "proyek_kode", Label: "Kode proyek", Type: FieldText, Section: "Proyek", Required: true, Placeholder: "mis. KLN-APP-01"},
			{Key: "perjanjian_induk_nomor", Label: "No. perjanjian induk (MSA)", Type: FieldText, Section: "Proyek", Required: true},
			{Key: "perjanjian_induk_tanggal", Label: "Tanggal perjanjian induk", Type: FieldDate, Section: "Proyek", Required: true},
			{Key: "tanggal_mulai", Label: "Tanggal mulai", Type: FieldDate, Section: "Proyek", Required: true},
			{Key: "tanggal_berakhir", Label: "Tanggal berakhir", Type: FieldDate, Section: "Proyek", Required: true},
			{Key: "mode_deployment", Label: "Mode deployment", Type: FieldText, Section: "Proyek", Required: true, Placeholder: "On-premise (server Klien), Cloud, atau Hybrid"},
			{Key: "proyek_latar_belakang", Label: "Latar belakang", Type: FieldTextarea, Section: "Tujuan", Required: true},
			{Key: "proyek_tujuan", Label: "Tujuan", Type: FieldTextarea, Section: "Tujuan", Required: true},
			{Key: "proyek_indikator_keberhasilan", Label: "Indikator keberhasilan", Type: FieldTextarea, Section: "Tujuan", Required: true},
			{Key: "penyedia_pic_nama", Label: "PIC perusahaan kita", Type: FieldText, Section: "PIC", Required: true},
			{Key: "penyedia_pic_email", Label: "Email PIC perusahaan kita", Type: FieldEmail, Section: "PIC", Required: true},
			{Key: "klien_pic_nama", Label: "PIC klien", Type: FieldText, Section: "PIC", Required: true},
			{Key: "klien_pic_email", Label: "Email PIC klien", Type: FieldEmail, Section: "PIC", Required: true},
			itemList("lingkup_dalam", "Termasuk lingkup (in scope)", "Lingkup", true),
			itemList("lingkup_luar", "Di luar lingkup (out of scope)", "Lingkup", false),
			{Key: "deliverable", Label: "Deliverable", Type: FieldList, Section: "Deliverable & milestone", Required: true, NoColumn: true, AutoNumber: "D", Columns: []Column{
				{Key: "nama", Label: "Deliverable", Type: FieldText, Required: true},
				{Key: "deskripsi", Label: "Deskripsi", Type: FieldText},
				{Key: "format", Label: "Format", Type: FieldText},
				{Key: "target_tanggal", Label: "Target", Type: FieldDate},
			}},
			{Key: "milestone", Label: "Milestone & pembayaran", Type: FieldList, Section: "Deliverable & milestone", Required: true, Columns: []Column{
				{Key: "nama", Label: "Milestone", Type: FieldText, Required: true},
				{Key: "deliverable_terkait", Label: "Deliverable terkait", Type: FieldText},
				{Key: "target_tanggal", Label: "Target", Type: FieldDate},
				{Key: "persen", Label: "%", Type: FieldPercent},
				{Key: "nilai", Label: "Nilai", Type: FieldMoney},
			}},
			{Key: "mata_uang", Label: "Mata uang", Type: FieldSelect, Section: "Deliverable & milestone", Required: true, Default: "IDR", Options: currencyOptions},
			{Key: "ketentuan_pajak", Label: "Pajak", Type: FieldSelect, Section: "Deliverable & milestone", Required: true, Default: taxOptions[0].Value, Options: taxOptions},
			{Key: "termin_bayar_hari", Label: "Jatuh tempo pembayaran (hari)", Type: FieldNumber, Section: "Deliverable & milestone", Required: true, Default: 30, Min: n(1), Max: n(120)},
			itemList("kriteria_penerimaan", "Kriteria penerimaan (UAT)", "Penerimaan", true),
			{Key: "hari_review", Label: "Masa pemeriksaan deliverable (hari kerja)", Type: FieldNumber, Section: "Penerimaan", Required: true, Default: 5, Min: n(1), Max: n(60)},
			itemList("asumsi", "Asumsi", "Asumsi & kewajiban klien", false),
			{Key: "kewajiban_klien", Label: "Kewajiban klien", Type: FieldList, Section: "Asumsi & kewajiban klien", NoColumn: true, Columns: []Column{
				{Key: "item", Label: "Kewajiban", Type: FieldText, Required: true},
				{Key: "tenggat", Label: "Tenggat", Type: FieldDate},
			}},
			{Key: "tim", Label: "Tim proyek", Type: FieldList, Section: "Tim & komunikasi", Required: true, Columns: []Column{
				{Key: "pihak", Label: "Pihak", Type: FieldSelect, Required: true, Default: "Penyedia", Options: []Option{{Value: "Penyedia", Label: "Penyedia"}, {Value: "Klien", Label: "Klien"}}},
				{Key: "peran", Label: "Peran", Type: FieldText, Required: true},
				{Key: "nama", Label: "Nama", Type: FieldText, Required: true},
				{Key: "tanggung_jawab", Label: "Tanggung jawab", Type: FieldText},
			}},
			{Key: "frekuensi_rapat", Label: "Frekuensi rapat status", Type: FieldText, Section: "Tim & komunikasi", Required: true, Default: "setiap minggu / weekly"},
			{Key: "kanal_laporan", Label: "Kanal laporan progres", Type: FieldText, Section: "Tim & komunikasi", Required: true, Default: "email"},
			{Key: "lingkungan_data", Label: "Lokasi pemrosesan data", Type: FieldTextarea, Section: "Tim & komunikasi", Required: true},
		},
	},
	{
		Type: model.LegalDocSLA, Code: "SLA", Template: docgen.TemplateSLA, sequence: "LEGAL_SLA",
		Label: "Perjanjian Tingkat Layanan (SLA)", ShortLabel: "SLA",
		Description: "Support dan maintenance: jam layanan, severity, uptime, service credit, eskalasi.",
		PartyLabel:  "Klien", PartyTypes: []string{model.LegalPartyCompany, model.LegalPartyPerson},
		build: func(*payloadInput) {},
		Fields: []Field{
			{Key: "layanan_nama", Label: "Nama layanan", Type: FieldText, Section: "Layanan", Required: true},
			{Key: "perjanjian_induk_nomor", Label: "No. perjanjian induk (MSA)", Type: FieldText, Section: "Layanan", Required: true},
			{Key: "perjanjian_induk_tanggal", Label: "Tanggal perjanjian induk", Type: FieldDate, Section: "Layanan", Required: true},
			{Key: "tanggal_mulai", Label: "Tanggal mulai", Type: FieldDate, Section: "Layanan", Required: true},
			{Key: "tanggal_berakhir", Label: "Tanggal berakhir", Type: FieldDate, Section: "Layanan", Required: true},
			{Key: "mode_deployment", Label: "Mode deployment", Type: FieldText, Section: "Layanan", Required: true, Placeholder: "On-premise (server Klien), Cloud, atau Hybrid"},
			{Key: "komponen", Label: "Komponen yang didukung", Type: FieldList, Section: "Layanan", Required: true, NoColumn: true, Columns: []Column{
				{Key: "nama", Label: "Komponen", Type: FieldText, Required: true},
				{Key: "lingkungan", Label: "Lingkungan", Type: FieldText, Default: "Produksi"},
				{Key: "catatan", Label: "Catatan", Type: FieldText},
			}},
			{Key: "mata_uang", Label: "Mata uang", Type: FieldSelect, Section: "Biaya", Required: true, Default: "IDR", Options: currencyOptions},
			{Key: "biaya_layanan", Label: "Biaya layanan", Type: FieldMoney, Section: "Biaya", Required: true},
			{Key: "periode_biaya", Label: "Periode biaya", Type: FieldSelect, Section: "Biaya", Required: true, Default: "bulan / month", Options: []Option{{Value: "bulan / month", Label: "Per bulan"}, {Value: "tahun / year", Label: "Per tahun"}}},
			{Key: "ketentuan_pajak", Label: "Pajak", Type: FieldSelect, Section: "Biaya", Required: true, Default: taxOptions[0].Value, Options: taxOptions},
			{Key: "jam_layanan", Label: "Jam dukungan", Type: FieldText, Section: "Dukungan", Required: true, Default: "Senin–Jumat 09.00–17.00 WIB (kecuali hari libur nasional)"},
			{Key: "kanal_dukungan", Label: "Kanal dukungan", Type: FieldText, Section: "Dukungan", Required: true, Placeholder: "Portal tiket, email support, WhatsApp"},
			{Key: "dukungan_luar_jam", Label: "Dukungan di luar jam", Type: FieldText, Section: "Dukungan", Required: true, Default: "Hanya Severity 1"},
			{Key: "severity", Label: "Tingkat severity", Type: FieldList, Section: "Dukungan", Required: true, Columns: []Column{
				{Key: "level", Label: "Level", Type: FieldText, Required: true},
				{Key: "definisi", Label: "Definisi", Type: FieldText, Required: true},
				{Key: "respons", Label: "Respons", Type: FieldText},
				{Key: "pemulihan", Label: "Pemulihan", Type: FieldText},
				{Key: "update", Label: "Update", Type: FieldText},
			}, Default: []map[string]any{
				{"level": "S1", "definisi": "Layanan tidak dapat digunakan sama sekali", "respons": "1 jam (24x7)", "pemulihan": "8 jam", "update": "Tiap 2 jam"},
				{"level": "S2", "definisi": "Fungsi utama terganggu, ada workaround", "respons": "4 jam kerja", "pemulihan": "2 hari kerja", "update": "Harian"},
				{"level": "S3", "definisi": "Gangguan minor / pertanyaan", "respons": "1 hari kerja", "pemulihan": "5 hari kerja", "update": "Saat selesai"},
			}},
			{Key: "target_uptime", Label: "Target ketersediaan (%)", Type: FieldText, Section: "Ketersediaan", Required: true, Default: "99,5", Help: "Angka saja, mis. 99,5"},
			{Key: "jendela_pemeliharaan", Label: "Jendela pemeliharaan", Type: FieldText, Section: "Ketersediaan", Required: true, Default: "Sabtu 22.00–02.00 WIB"},
			{Key: "hari_pemberitahuan_pemeliharaan", Label: "Pemberitahuan pemeliharaan (hari)", Type: FieldNumber, Section: "Ketersediaan", Required: true, Default: 3, Min: n(0), Max: n(60)},
			{Key: "kredit_layanan", Label: "Service credit", Type: FieldList, Section: "Ketersediaan", Required: true, Columns: []Column{
				{Key: "rentang", Label: "Ketersediaan", Type: FieldText, Required: true},
				{Key: "kredit", Label: "Kredit", Type: FieldText, Required: true},
			}, Default: []map[string]any{
				{"rentang": "99,0% – < 99,5%", "kredit": "5%"},
				{"rentang": "98,0% – < 99,0%", "kredit": "10%"},
				{"rentang": "< 98,0%", "kredit": "20%"},
			}},
			{Key: "maks_kredit_persen", Label: "Batas total kredit (%)", Type: FieldNumber, Section: "Ketersediaan", Required: true, Default: 20, Min: n(0), Max: n(100)},
			{Key: "hari_klaim", Label: "Batas klaim kredit (hari)", Type: FieldNumber, Section: "Ketersediaan", Required: true, Default: 15, Min: n(1), Max: n(90)},
			{Key: "eskalasi", Label: "Jalur eskalasi", Type: FieldList, Section: "Eskalasi & laporan", Required: true, Columns: []Column{
				{Key: "level", Label: "Level", Type: FieldText, Required: true},
				{Key: "nama", Label: "Nama / tim", Type: FieldText, Required: true},
				{Key: "jabatan", Label: "Jabatan", Type: FieldText},
				{Key: "kontak", Label: "Kontak", Type: FieldText},
			}},
			{Key: "tanggal_laporan", Label: "Tanggal laporan bulanan", Type: FieldNumber, Section: "Eskalasi & laporan", Required: true, Default: 10, Min: n(1), Max: n(28)},
		},
	},
	{
		Type: model.LegalDocBAST, Code: "BAST", Template: docgen.TemplateBAST, sequence: "LEGAL_BAST",
		Label: "Berita Acara Serah Terima (BAST)", ShortLabel: "BAST",
		Description: "Serah terima pekerjaan per termin, sekaligus dasar penagihan.",
		PartyLabel:  "Klien", PartyTypes: []string{model.LegalPartyCompany, model.LegalPartyPerson},
		RelatedTypes: []string{model.LegalDocSOW},
		build:        buildBAST,
		Fields: []Field{
			{Key: "kontrak_nomor", Label: "No. kontrak / SOW acuan", Type: FieldText, Section: "Acuan", Required: true},
			{Key: "kontrak_tanggal", Label: "Tanggal kontrak", Type: FieldDate, Section: "Acuan", Required: true},
			{Key: "proyek_nama", Label: "Nama proyek", Type: FieldText, Section: "Acuan", Required: true},
			{Key: "termin_ke", Label: "Termin ke", Type: FieldText, Section: "Termin", Required: true},
			{Key: "milestone_nama", Label: "Milestone", Type: FieldText, Section: "Termin", Required: true},
			{Key: "uat_referensi", Label: "Referensi dokumen UAT", Type: FieldText, Section: "Termin", Required: true, Default: "-"},
			{Key: "item_serah_terima", Label: "Item yang diserahkan", Type: FieldList, Section: "Serah terima", Required: true, NoColumn: true, Columns: []Column{
				{Key: "nama", Label: "Item", Type: FieldText, Required: true},
				{Key: "deskripsi", Label: "Deskripsi", Type: FieldText},
				{Key: "jumlah", Label: "Jumlah", Type: FieldText, Default: "1 paket"},
				{Key: "status", Label: "Status", Type: FieldSelect, Required: true, Default: "Diterima", Options: []Option{
					{Value: "Diterima", Label: "Diterima"}, {Value: "Diterima dengan catatan", Label: "Diterima dengan catatan"}, {Value: "Belum diterima", Label: "Belum diterima"},
				}},
			}},
			{Key: "status_penerimaan", Label: "Status penerimaan", Type: FieldSelect, Section: "Serah terima", Required: true, Default: "Diterima penuh / Fully accepted", Options: []Option{
				{Value: "Diterima penuh / Fully accepted", Label: "Diterima penuh"},
				{Value: "Diterima dengan catatan / Accepted with notes", Label: "Diterima dengan catatan"},
			}},
			{Key: "catatan", Label: "Catatan perbaikan", Type: FieldList, Section: "Serah terima", NoColumn: true, Columns: []Column{
				{Key: "isi", Label: "Catatan", Type: FieldText, Required: true},
				{Key: "tenggat", Label: "Tenggat", Type: FieldDate},
			}},
			{Key: "mata_uang", Label: "Mata uang", Type: FieldSelect, Section: "Tagihan", Required: true, Default: "IDR", Options: currencyOptions},
			{Key: "nilai_termin", Label: "Nilai tagihan termin", Type: FieldMoney, Section: "Tagihan", Required: true},
			{Key: "masa_garansi", Label: "Masa garansi", Type: FieldText, Section: "Tagihan", Required: true, Default: "3 bulan / 3 months"},
		},
	},
}

var specByType = func() map[string]*Spec {
	out := make(map[string]*Spec, len(specs))
	for _, spec := range specs {
		out[spec.Type] = spec
	}
	return out
}()

// Specs lists every document type in menu order.
func Specs() []*Spec { return specs }

// SpecFor returns the spec of a document type.
func SpecFor(docType string) (*Spec, bool) {
	spec, ok := specByType[docType]
	return spec, ok
}
