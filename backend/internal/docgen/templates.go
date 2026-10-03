package docgen

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"sync"
)

// The templates are the owner-approved legal pack, embedded read-only. They
// are the only templates the renderer ever loads: there is no upload path.
//
//go:embed templates/*.docx
var templateFS embed.FS

// TemplateID is the file name of an embedded template.
type TemplateID string

const (
	TemplatePKWT TemplateID = "01_PKWT_Perjanjian_Kerja_Waktu_Tertentu.docx"
	TemplateNDA  TemplateID = "02_NDA_dan_Pengalihan_HKI_Karyawan.docx"
	TemplateSlip TemplateID = "08_Slip_Gaji.docx"
)

// slipLogoMedia is the image behind the logo slot of the payslip. The
// committed file is a neutral grey placeholder; renders replace it with the
// tenant logo or a transparent pixel.
const slipLogoMedia = "word/media/01aa02cbf00ceaa627c732d7d61d8a20f5c4ffd7.png"

var templateLogoMedia = map[TemplateID]string{
	TemplateSlip: slipLogoMedia,
}

// TemplateIDs lists every embedded template.
func TemplateIDs() []TemplateID {
	return []TemplateID{TemplatePKWT, TemplateNDA, TemplateSlip}
}

var loadTemplates = sync.OnceValues(func() (map[TemplateID]*Template, error) {
	out := make(map[TemplateID]*Template, 3)
	for _, id := range TemplateIDs() {
		raw, err := templateFS.ReadFile("templates/" + string(id))
		if err != nil {
			return nil, fmt.Errorf("docgen: read embedded %s: %w", id, err)
		}
		tpl, err := compile(id, raw, templateLogoMedia[id])
		if err != nil {
			return nil, err
		}
		out[id] = tpl
	}
	return out, nil
})

// Load returns the compiled embedded template. Templates are compiled once
// per process; an invalid embedded template fails every call (and the
// validation tests), never silently.
func Load(id TemplateID) (*Template, error) {
	all, err := loadTemplates()
	if err != nil {
		return nil, err
	}
	tpl, ok := all[id]
	if !ok {
		return nil, fmt.Errorf("docgen: unknown template %q", id)
	}
	return tpl, nil
}

// Validate checks a DOCX template the same way embedded templates are
// checked at load time: placeholders only inside a single w:t run of the
// rendered parts, balanced sections that open and close in one paragraph or
// table row, and no placeholder in field codes, attributes, .rels or other
// parts.
func Validate(docx []byte) error {
	_, err := compile("validate", docx, "")
	return err
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
