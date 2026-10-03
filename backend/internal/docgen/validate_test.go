package docgen

import (
	"bytes"
	"image/png"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestEmbeddedTemplatesValidate compiles every embedded template, which runs
// the full validator: placeholders only in one w:t run of document/header/
// footer parts, balanced sections that open and close in one paragraph or
// table row, and nothing in field codes, attributes, .rels or other parts.
func TestEmbeddedTemplatesValidate(t *testing.T) {
	for _, id := range TemplateIDs() {
		t.Run(string(id), func(t *testing.T) {
			raw, err := os.ReadFile("templates/" + string(id))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if err := Validate(raw); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			tpl := mustLoad(t, id)
			if tpl.Version != sha256Hex(raw) || len(tpl.Version) != 64 {
				t.Fatalf("Version = %q, want sha256 of the file", tpl.Version)
			}
			if got, want := tpl.HasLogoSlot(), id == TemplateSlip; got != want {
				t.Fatalf("HasLogoSlot = %v, want %v", got, want)
			}
		})
	}
}

// TestEmbeddedTemplatesSections pins the loop sections each template exposes.
func TestEmbeddedTemplatesSections(t *testing.T) {
	want := map[TemplateID][]string{
		TemplatePKWT: {"benefit"},
		TemplateNDA:  {"karya_terdahulu"},
		TemplateSlip: {"gaji", "potongan", "reimbursement"},
	}
	for id, sections := range want {
		tpl := mustLoad(t, id)
		var got []string
		var walk func([]node)
		walk = func(nodes []node) {
			for _, n := range nodes {
				if n.kind == nodeSection {
					got = append(got, n.name)
					walk(n.children)
				}
			}
		}
		walk(tpl.parts["word/document.xml"].nodes)
		if strings.Join(got, ",") != strings.Join(sections, ",") {
			t.Errorf("%s sections = %v, want %v", id, got, sections)
		}
	}
}

// TestEmbeddedTemplatesSanitized: no real company identifiers and no original
// logo bytes in the committed templates (decision D2).
func TestEmbeddedTemplatesSanitized(t *testing.T) {
	for _, id := range TemplateIDs() {
		raw, err := os.ReadFile("templates/" + string(id))
		if err != nil {
			t.Fatal(err)
		}
		for name, data := range zipEntries(t, raw) {
			if containsBannedIdentifier(data) {
				t.Errorf("%s/%s contains a banned identifier", id, name)
			}
		}
	}
	kamusRaw, err := os.ReadFile("testdata/kamus_variabel.json")
	if err != nil {
		t.Fatal(err)
	}
	if containsBannedIdentifier(kamusRaw) {
		t.Error("testdata/kamus_variabel.json contains a banned identifier")
	}
	// The replacement names used by the sanitized data are not flagged.
	if containsBannedIdentifier([]byte("PT Contoh Teknologi Nusantara, Rudi Hartono")) {
		t.Error("sanitized names are flagged")
	}

	raw, err := os.ReadFile("templates/" + string(TemplateSlip))
	if err != nil {
		t.Fatal(err)
	}
	media := zipEntries(t, raw)[slipLogoMedia]
	img, err := png.Decode(bytes.NewReader(media))
	if err != nil {
		t.Fatalf("decode placeholder logo: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 1200 || b.Dy() != 256 {
		t.Fatalf("placeholder logo is %dx%d, want 1200x256", b.Dx(), b.Dy())
	}
	for y := b.Min.Y; y < b.Max.Y; y += 7 {
		for x := b.Min.X; x < b.Max.X; x += 11 {
			r, g, bl, a := img.At(x, y).RGBA()
			if r>>8 != 0xE5 || g>>8 != 0xE7 || bl>>8 != 0xEB || a>>8 != 0xFF {
				t.Fatalf("placeholder pixel (%d,%d) is not #E5E7EB", x, y)
			}
		}
	}
}

func TestValidateAcceptsMinimalTemplate(t *testing.T) {
	doc := docxFrom(t, map[string]string{
		"word/document.xml": testDoc(
			para(`Hello {{name}}`) +
				`<w:tbl><w:tr><w:tc>` + para(`{{#items}}{{a}}`) + `</w:tc><w:tc>` + para(`{{b}}{{/items}}`) + `</w:tc></w:tr></w:tbl>` +
				para(`{{#flag}}yes{{/flag}}{{^flag}}no{{/flag}}`)),
		"word/header1.xml":             `<w:hdr xmlns:w="w">` + para(`{{name}}`) + `</w:hdr>`,
		"word/_rels/document.xml.rels": `<Relationships/>`,
	})
	if err := Validate(doc); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	row := func(a, b string) string {
		return `<w:tbl><w:tr><w:tc>` + para(a) + `</w:tc><w:tc>` + para(b) + `</w:tc></w:tr></w:tbl>`
	}
	cases := map[string]struct {
		parts map[string]string
		want  string
	}{
		"token split across runs": {
			parts: map[string]string{"word/document.xml": testDoc(
				`<w:p><w:r><w:t>{{karyawan</w:t></w:r><w:r><w:t>_nama}}</w:t></w:r></w:p>`)},
			want: "split",
		},
		"token split by closing braces": {
			parts: map[string]string{"word/document.xml": testDoc(
				`<w:p><w:r><w:t>{{karyawan_nama}</w:t></w:r><w:r><w:t>}</w:t></w:r></w:p>`)},
			want: "split",
		},
		"stray closing braces": {
			parts: map[string]string{"word/document.xml": testDoc(para(`nama}}`))},
			want:  "stray",
		},
		"malformed token": {
			parts: map[string]string{"word/document.xml": testDoc(para(`{{ nama }}`))},
			want:  "malformed",
		},
		"unclosed loop": {
			parts: map[string]string{"word/document.xml": testDoc(row(`{{#gaji}}{{nama}}`, `{{jumlah}}`))},
			want:  "never closed",
		},
		"close without open": {
			parts: map[string]string{"word/document.xml": testDoc(row(`{{nama}}`, `{{jumlah}}{{/gaji}}`))},
			want:  "without an opening",
		},
		"mismatched close": {
			parts: map[string]string{"word/document.xml": testDoc(row(`{{#gaji}}{{nama}}`, `{{jumlah}}{{/potongan}}`))},
			want:  "closes",
		},
		"loop across paragraphs outside a row": {
			parts: map[string]string{"word/document.xml": testDoc(para(`{{#gaji}}{{nama}}`) + para(`{{/gaji}}`))},
			want:  "one paragraph or one table row",
		},
		"loop across rows": {
			parts: map[string]string{"word/document.xml": testDoc(
				`<w:tbl><w:tr><w:tc>` + para(`{{#gaji}}`) + `</w:tc></w:tr><w:tr><w:tc>` + para(`{{/gaji}}`) + `</w:tc></w:tr></w:tbl>`)},
			want: "one paragraph or one table row",
		},
		"two row loops in one row": {
			parts: map[string]string{"word/document.xml": testDoc(
				`<w:tbl><w:tr><w:tc>` + para(`{{#a}}x`) + para(`{{/a}}`) + `</w:tc><w:tc>` + para(`{{#b}}y`) + para(`{{/b}}`) + `</w:tc></w:tr></w:tbl>`)},
			want: "overlaps",
		},
		"placeholder in instrText": {
			parts: map[string]string{"word/document.xml": testDoc(
				`<w:p><w:r><w:fldChar w:fldCharType="begin"/><w:instrText xml:space="preserve">MERGEFIELD {{nama}}</w:instrText><w:fldChar w:fldCharType="end"/></w:r></w:p>`)},
			want: "field code",
		},
		"placeholder in fldSimple attribute": {
			parts: map[string]string{"word/document.xml": testDoc(
				`<w:p><w:fldSimple w:instr="MERGEFIELD {{nama}}"><w:r><w:t>x</w:t></w:r></w:fldSimple></w:p>`)},
			want: "attribute",
		},
		"placeholder in fldSimple result": {
			parts: map[string]string{"word/document.xml": testDoc(
				`<w:p><w:fldSimple w:instr="PAGE"><w:r><w:t>{{nama}}</w:t></w:r></w:fldSimple></w:p>`)},
			want: "w:fldSimple",
		},
		"placeholder in attribute": {
			parts: map[string]string{"word/document.xml": testDoc(
				`<w:p><w:hyperlink w:tooltip="{{nama}}"><w:r><w:t>x</w:t></w:r></w:hyperlink></w:p>`)},
			want: "attribute",
		},
		"placeholder outside w:t": {
			parts: map[string]string{"word/document.xml": testDoc(`<w:p><w:r><w:delText>{{nama}}</w:delText></w:r></w:p>`)},
			want:  "outside w:t",
		},
		"placeholder in rels": {
			parts: map[string]string{
				"word/document.xml":            testDoc(para(`ok`)),
				"word/_rels/document.xml.rels": `<Relationships><Relationship Id="rId1" Target="https://x/{{nama}}" TargetMode="External"/></Relationships>`,
			},
			want: "never rendered",
		},
		"placeholder in comments part": {
			parts: map[string]string{
				"word/document.xml": testDoc(para(`ok`)),
				"word/comments.xml": `<w:comments xmlns:w="w">` + para(`{{nama}}`) + `</w:comments>`,
			},
			want: "never rendered",
		},
		"malformed xml": {
			parts: map[string]string{"word/document.xml": testDoc(`<w:p><w:r><w:t>x</w:r></w:p>`)},
			want:  "unexpected",
		},
		"inline section at different element depths": {
			parts: map[string]string{"word/document.xml": testDoc(
				`<w:p><w:hyperlink r:id="rId1"><w:r><w:t>A{{#items}}x</w:t></w:r></w:hyperlink><w:r><w:t>{{v}}{{/items}}B</w:t></w:r></w:p>`)},
			want: "different element depths",
		},
		"braces split over three runs": {
			parts: map[string]string{"word/document.xml": testDoc(
				`<w:p><w:r><w:t>Nama: {</w:t></w:r><w:r><w:t>{karyawan_nama}</w:t></w:r><w:r><w:t>}</w:t></w:r></w:p>`)},
			want: "split placeholder",
		},
		"braces as character references": {
			parts: map[string]string{"word/document.xml": testDoc(
				para(`NIK: &#123;&#123;karyawan_nik&#125;&#125;`))},
			want: "character references",
		},
		"hex character references next to a real token": {
			parts: map[string]string{"word/document.xml": testDoc(
				para(`{{nama}} &#x7b;&#x7B;nik&#x7d;&#x7D;`))},
			want: "character references",
		},
		"no document part": {
			parts: map[string]string{"word/header1.xml": `<w:hdr xmlns:w="w"/>`},
			want:  "document.xml missing",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := Validate(docxFrom(t, tc.parts))
			if err == nil {
				t.Fatalf("expected an error containing %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

// TestInlineSectionAcrossRuns: an inline section whose tags sit in two runs
// at the same depth is valid and stays well-formed for 0, 1 and 2 items.
func TestInlineSectionAcrossRuns(t *testing.T) {
	doc := docxFrom(t, map[string]string{"word/document.xml": testDoc(
		`<w:p><w:r><w:t>A{{#items}}x</w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>{{v}}{{/items}}B</w:t></w:r></w:p>`)})
	tpl, err := compile("inline.docx", doc, "")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for n, want := range []string{"AB", "Ax1B", "Ax1x2B"} {
		items := []any{}
		for i := 1; i <= n; i++ {
			items = append(items, map[string]any{"v": strconv.Itoa(i)})
		}
		out, err := Render(tpl, Payload{"items": items})
		if err != nil {
			t.Fatalf("render %d items: %v", n, err)
		}
		xmlDoc := zipEntries(t, out)["word/document.xml"]
		assertWellFormed(t, "document.xml", xmlDoc)
		if got := paragraphText(t, xmlDoc); len(got) != 1 || got[0] != want {
			t.Errorf("%d items: text %q, want %q", n, got, want)
		}
	}
}

func TestValidateRejectsNonZip(t *testing.T) {
	if err := Validate([]byte("not a zip")); err == nil {
		t.Fatal("expected error")
	}
}
