package docgen

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata/golden from the current renderer")

var loopFields = map[TemplateID]map[string]int{
	TemplatePKWT: {"benefit": 1},
	TemplateNDA:  {"karya_terdahulu": 1},
	TemplateSlip: {"gaji": 1, "potongan": 1, "reimbursement": 1},
}

func listLen(t *testing.T, p Payload, key string) int {
	t.Helper()
	switch v := p[key].(type) {
	case []any:
		return len(v)
	case []map[string]string:
		return len(v)
	case []map[string]any:
		return len(v)
	}
	t.Fatalf("payload %s is %T, not a list", key, p[key])
	return 0
}

// TestRenderKamusGolden renders every template with the sanitized kamus
// example payload and compares the extracted paragraph text with the golden
// files (go test ./internal/docgen -run Golden -update rewrites them).
func TestRenderKamusGolden(t *testing.T) {
	for _, id := range TemplateIDs() {
		t.Run(string(id), func(t *testing.T) {
			payload := kamusPayload(t, id)
			out := mustRender(t, id, payload)
			tplParts := zipEntries(t, mustLoad(t, id).raw)
			parts := zipEntries(t, out)

			var text []string
			for _, name := range []string{"word/header1.xml", "word/document.xml", "word/footer1.xml"} {
				data, ok := parts[name]
				if !ok {
					t.Fatalf("%s missing from output", name)
				}
				assertWellFormed(t, name, data)
				if bytes.Contains(data, []byte("{{")) || bytes.Contains(data, []byte("}}")) {
					t.Fatalf("%s still contains placeholder braces", name)
				}
				text = append(text, "## "+name)
				text = append(text, paragraphText(t, data)...)
			}

			// Loop counts: each loop row is replaced by one row per item.
			doc := parts["word/document.xml"]
			wantRows := countRows(tplParts["word/document.xml"])
			for key, rowsPerItem := range loopFields[id] {
				wantRows += rowsPerItem * (listLen(t, payload, key) - 1)
			}
			if got := countRows(doc); got != wantRows {
				t.Fatalf("rendered %d table rows, want %d", got, wantRows)
			}

			golden := filepath.Join("testdata", "golden", strings.TrimSuffix(string(id), ".docx")+".txt")
			got := strings.Join(text, "\n") + "\n"
			if *updateGolden {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden (run with -update to create): %v", err)
			}
			if got != string(want) {
				t.Fatalf("rendered text differs from %s (run with -update after checking):\n%s", golden, firstDiff(got, string(want)))
			}
		})
	}
}

func firstDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) || i < len(bl); i++ {
		var x, y string
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if x != y {
			return "line " + strconv.Itoa(i+1) + ":\n got: " + x + "\nwant: " + y
		}
	}
	return ""
}

func TestRenderSlipTotalsAreConsistent(t *testing.T) {
	p := kamusPayload(t, TemplateSlip)
	sum := func(key string) int64 {
		var total int64
		for _, it := range p[key].([]any) {
			total += parseThousands(t, it.(map[string]any)["jumlah"].(string))
		}
		return total
	}
	gaji, potongan, reimb := sum("gaji"), sum("potongan"), sum("reimbursement")
	check := func(key string, want int64) {
		if got := p[key].(string); got != Thousands(want) {
			t.Errorf("%s = %s, want %s", key, got, Thousands(want))
		}
	}
	check("total_gaji", gaji)
	check("total_potongan", potongan)
	check("total_reimbursement", reimb)
	check("total_diterima", gaji-potongan+reimb)
	if got, want := p["total_diterima_terbilang"].(string), TerbilangRupiah(gaji-potongan+reimb); got != want {
		t.Errorf("total_diterima_terbilang = %q, want %q", got, want)
	}
	if listLen(t, p, "gaji") != 5 || listLen(t, p, "potongan") != 4 || listLen(t, p, "reimbursement") != 3 {
		t.Errorf("kamus slip must carry 5 earnings, 4 deductions and 3 reimbursements")
	}
}

func parseThousands(t *testing.T, s string) int64 {
	t.Helper()
	var n int64
	for _, r := range s {
		if r == '.' {
			continue
		}
		if r < '0' || r > '9' {
			t.Fatalf("not a formatted amount: %q", s)
		}
		n = n*10 + int64(r-'0')
	}
	return n
}

func TestRenderEmptyLists(t *testing.T) {
	cases := map[TemplateID][]string{
		TemplatePKWT: {"benefit"},
		TemplateNDA:  {"karya_terdahulu"},
		TemplateSlip: {"gaji", "potongan", "reimbursement"},
	}
	for id, keys := range cases {
		payload := kamusPayload(t, id)
		for _, k := range keys {
			payload[k] = []any{}
		}
		out := mustRender(t, id, payload)
		parts := zipEntries(t, out)
		doc := parts["word/document.xml"]
		assertWellFormed(t, string(id), doc)
		if want := countRows(zipEntries(t, mustLoad(t, id).raw)["word/document.xml"]) - len(keys); countRows(doc) != want {
			t.Errorf("%s: %d rows with empty lists, want %d", id, countRows(doc), want)
		}
		if bytes.Contains(doc, []byte("{{")) {
			t.Errorf("%s: placeholder left with empty lists", id)
		}
	}

	// A nil Go slice (no deductions) renders as an empty loop, directly and
	// after the JSON round trip of the stored snapshot, where it becomes null.
	payload := kamusPayload(t, TemplateSlip)
	payload["potongan"] = []map[string]string(nil)
	direct := zipEntries(t, mustRender(t, TemplateSlip, payload))["word/document.xml"]
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var snap map[string]any
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	if v, ok := snap["potongan"]; !ok || v != nil {
		t.Fatalf("snapshot potongan = %#v, want null", v)
	}
	viaJSON := zipEntries(t, mustRender(t, TemplateSlip, Payload(snap)))["word/document.xml"]
	assertWellFormed(t, "slip with null potongan", viaJSON)
	if !bytes.Equal(direct, viaJSON) {
		t.Error("nil slice and JSON null render differently")
	}
	payload["potongan"] = []any{}
	empty := zipEntries(t, mustRender(t, TemplateSlip, payload))["word/document.xml"]
	if !bytes.Equal(empty, viaJSON) {
		t.Error("JSON null and an empty list render differently")
	}

	// An absent key is still an error.
	delete(snap, "potongan")
	if _, err := Render(mustLoad(t, TemplateSlip), Payload(snap)); !errors.Is(err, ErrRender) {
		t.Errorf("absent section key: %v", err)
	}
}

func TestRenderGoTypedLists(t *testing.T) {
	payload := kamusPayload(t, TemplateSlip)
	payload["potongan"] = []map[string]string{
		{"nama": "Potongan A", "keterangan": "", "jumlah": "1"},
		{"nama": "Potongan B", "keterangan": "", "jumlah": "2"},
	}
	payload["gaji"] = []map[string]any{{"nama": "Gaji X", "keterangan": "", "jumlah": "9"}}
	doc := zipEntries(t, mustRender(t, TemplateSlip, payload))["word/document.xml"]
	for _, s := range []string{"Potongan A", "Potongan B", "Gaji X"} {
		if bytes.Count(doc, []byte(s)) != 1 {
			t.Errorf("%q rendered %d times", s, bytes.Count(doc, []byte(s)))
		}
	}
}

// TestRenderEscaping: values are XML-escaped, made single-line, and never
// rescanned, so "{{karyawan_nik}}" inside a value stays literal.
func TestRenderEscaping(t *testing.T) {
	base := kamusPayload(t, TemplatePKWT)
	nik := base["karyawan_nik"].(string)
	baseDoc := zipEntries(t, mustRender(t, TemplatePKWT, base))["word/document.xml"]
	nikCount := bytes.Count(baseDoc, []byte(nik))

	p := kamusPayload(t, TemplatePKWT)
	p["karyawan_nama"] = `Budi & <Sons> "QA"`
	p["karyawan_alamat"] = "Jl. Satu\vBaris\r\nDua\tTiga"
	p["karyawan_email"] = "{{karyawan_nik}}"
	p["benefit"] = []any{map[string]any{"nama": "</w:t></w:r><w:r><w:t>x", "nilai": "{{#benefit}}", "keterangan": "]]>"}}

	parts := zipEntries(t, mustRender(t, TemplatePKWT, p))
	doc := parts["word/document.xml"]
	assertWellFormed(t, "document.xml", doc)
	assertWellFormed(t, "header1.xml", parts["word/header1.xml"])

	if !bytes.Contains(doc, []byte(`Budi &amp; &lt;Sons&gt; &#34;QA&#34;`)) {
		t.Error("karyawan_nama not escaped as expected")
	}
	if bytes.ContainsRune(doc, '\v') {
		t.Error("U+000B leaked into the document")
	}
	if !bytes.Contains(doc, []byte("Jl. Satu Baris Dua Tiga")) {
		t.Error("control characters were not turned into single spaces")
	}
	if got := bytes.Count(doc, []byte("{{karyawan_nik}}")); got != 2 {
		t.Errorf("literal {{karyawan_nik}} rendered %d times, want 2 (once per email placeholder)", got)
	}
	if got := bytes.Count(doc, []byte(nik)); got != nikCount {
		t.Errorf("NIK appears %d times, want %d: a value was substituted", got, nikCount)
	}
	if !bytes.Contains(doc, []byte("&lt;/w:t&gt;&lt;/w:r&gt;")) {
		t.Error("markup inside a loop value was not escaped")
	}

	text := strings.Join(paragraphText(t, doc), "\n")
	if !strings.Contains(text, `Budi & <Sons> "QA"`) {
		t.Error("decoded text does not contain the original name")
	}
}

func TestRenderErrors(t *testing.T) {
	missing := kamusPayload(t, TemplatePKWT)
	delete(missing, "karyawan_nik")
	if _, err := Render(mustLoad(t, TemplatePKWT), missing); !errors.Is(err, ErrRender) || !strings.Contains(err.Error(), "karyawan_nik") {
		t.Errorf("missing scalar: %v", err)
	}

	wrongType := kamusPayload(t, TemplatePKWT)
	wrongType["gaji_pokok"] = 8000000
	if _, err := Render(mustLoad(t, TemplatePKWT), wrongType); !errors.Is(err, ErrRender) {
		t.Errorf("non-string scalar: %v", err)
	}

	noList := kamusPayload(t, TemplateSlip)
	delete(noList, "potongan")
	if _, err := Render(mustLoad(t, TemplateSlip), noList); !errors.Is(err, ErrRender) || !strings.Contains(err.Error(), "potongan") {
		t.Errorf("missing list: %v", err)
	}

	badItem := kamusPayload(t, TemplateSlip)
	badItem["potongan"] = []any{"not an object"}
	if _, err := Render(mustLoad(t, TemplateSlip), badItem); !errors.Is(err, ErrRender) {
		t.Errorf("bad list item: %v", err)
	}

	if _, err := Render(nil, Payload{}); err == nil {
		t.Error("nil template accepted")
	}
}

func TestRenderInlineSections(t *testing.T) {
	raw := docxFrom(t, map[string]string{"word/document.xml": testDoc(
		para(`A{{#flag}}[on {{x}}]{{/flag}}{{^flag}}[off]{{/flag}}B`) +
			para(`{{#items}}({{v}}){{/items}}`))})
	tpl, err := compile("inline", raw, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		payload Payload
		want    []string
	}{
		{Payload{"flag": true, "x": "1", "items": []any{map[string]any{"v": "a"}, map[string]any{"v": "b"}}}, []string{"A[on 1]B", "(a)(b)"}},
		{Payload{"flag": false, "x": "1", "items": []any{}}, []string{"A[off]B", ""}},
	} {
		out, err := Render(tpl, tc.payload)
		if err != nil {
			t.Fatal(err)
		}
		got := paragraphText(t, zipEntries(t, out)["word/document.xml"])
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}

// TestRenderCopiesOtherEntriesVerbatim: only document/header/footer parts
// (and the logo media) change; every other entry is copied byte for byte.
func TestRenderCopiesOtherEntriesVerbatim(t *testing.T) {
	tpl := mustLoad(t, TemplateSlip)
	out := mustRender(t, TemplateSlip, kamusPayload(t, TemplateSlip))
	raws := func(b []byte) map[string][]byte {
		zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			t.Fatal(err)
		}
		m := map[string][]byte{}
		for _, f := range zr.File {
			rc, err := f.OpenRaw()
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(rc)
			m[f.Name] = data
		}
		return m
	}
	in, got := raws(tpl.raw), raws(out)
	if len(in) != len(got) {
		t.Fatalf("entry count %d, want %d", len(got), len(in))
	}
	for name, data := range in {
		if renderedPartRe.MatchString(name) || name == slipLogoMedia {
			continue
		}
		if !bytes.Equal(data, got[name]) {
			t.Errorf("%s was not copied verbatim", name)
		}
	}
}

func syntheticLogo(t *testing.T, w, h int, inner image.Rectangle) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := inner.Min.Y; y < inner.Max.Y; y++ {
		for x := inner.Min.X; x < inner.Max.X; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 0x41, G: 0x00, B: 0x98, A: 0xFF})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func processedLogo(t *testing.T, raw []byte) []byte {
	t.Helper()
	out, err := ProcessLogo(raw)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRenderSlipLogo(t *testing.T) {
	tpl := mustLoad(t, TemplateSlip)
	slotCX, slotCY := tpl.logo.cx, tpl.logo.cy
	if slotCX != 1809750 || slotCY != 390525 {
		t.Fatalf("slot = %dx%d EMU, want 1809750x390525", slotCX, slotCY)
	}

	extent := func(doc []byte) (string, string) {
		i := bytes.Index(doc, []byte("<wp:extent "))
		j := bytes.Index(doc, []byte("<a:ext "))
		return string(doc[i : i+bytes.IndexByte(doc[i:], '>')]), string(doc[j : j+bytes.IndexByte(doc[j:], '>')])
	}

	// No logo: transparent pixel, slot keeps its size.
	parts := zipEntries(t, mustRender(t, TemplateSlip, kamusPayload(t, TemplateSlip)))
	if !bytes.Equal(parts[slipLogoMedia], BlankPNG()) {
		t.Error("render without logo must embed the blank PNG")
	}
	we, ae := extent(parts["word/document.xml"])
	if !strings.Contains(we, `cx="1809750" cy="390525"`) || !strings.Contains(ae, `cx="1809750" cy="390525"`) {
		t.Errorf("empty slot extents changed: %s / %s", we, ae)
	}

	// Tall logo (content 200x200 inside transparent padding): height-bound.
	// Renders take the stored, already processed logo.
	logo := processedLogo(t, syntheticLogo(t, 400, 300, image.Rect(100, 50, 300, 250)))
	parts = zipEntries(t, mustRender(t, TemplateSlip, kamusPayload(t, TemplateSlip), WithLogo(logo)))
	assertWellFormed(t, "document.xml", parts["word/document.xml"])
	cfg, err := png.DecodeConfig(bytes.NewReader(parts[slipLogoMedia]))
	if err != nil {
		t.Fatalf("embedded logo: %v", err)
	}
	if cfg.Width != 200 || cfg.Height != 200 {
		t.Errorf("embedded logo %dx%d, want trimmed 200x200", cfg.Width, cfg.Height)
	}
	if !bytes.Equal(parts[slipLogoMedia], logo) {
		t.Error("the processed logo must be embedded byte for byte")
	}
	we, ae = extent(parts["word/document.xml"])
	want := `cx="390525" cy="390525"`
	if !strings.Contains(we, want) || !strings.Contains(ae, want) {
		t.Errorf("square logo extents: %s / %s, want %s", we, ae, want)
	}

	// Wide logo (10:1): width-bound.
	logo = processedLogo(t, syntheticLogo(t, 1000, 100, image.Rect(0, 0, 1000, 100)))
	parts = zipEntries(t, mustRender(t, TemplateSlip, kamusPayload(t, TemplateSlip), WithLogo(logo)))
	we, _ = extent(parts["word/document.xml"])
	if !strings.Contains(we, `cx="1809750" cy="180975"`) {
		t.Errorf("wide logo extent: %s", we)
	}

	// The logo option is ignored by templates without a slot.
	if _, err := Render(mustLoad(t, TemplatePKWT), kamusPayload(t, TemplatePKWT), WithLogo(logo)); err != nil {
		t.Errorf("PKWT with logo option: %v", err)
	}

	// Invalid logo bytes fail the render.
	if _, err := Render(tpl, kamusPayload(t, TemplateSlip), WithLogo([]byte("GIF89a"))); !errors.Is(err, ErrInvalidLogo) {
		t.Errorf("invalid logo: %v", err)
	}
}

func TestFitExtent(t *testing.T) {
	const W, H = 1809750, 390525
	cases := []struct{ w, h int64 }{
		{1200, 256},  // slightly wider than the slot
		{4634, 1000}, // about the slot ratio
		{100, 100},   // square: height-bound
		{2920, 1000}, // untrimmed master logo ratio
		{4900, 1000}, // trimmed master logo ratio
	}
	for _, c := range cases {
		cx, cy := fitExtent(W, H, c.w, c.h)
		if cx > W || cy > H || cx <= 0 || cy <= 0 {
			t.Errorf("%dx%d: %dx%d does not fit the slot", c.w, c.h, cx, cy)
		}
		if cx != W && cy != H {
			t.Errorf("%dx%d: %dx%d touches neither slot edge", c.w, c.h, cx, cy)
		}
		// Ratio preserved within one EMU of rounding.
		diff := c.w*cy - c.h*cx
		if diff < 0 {
			diff = -diff
		}
		if diff > max(c.w, c.h) {
			t.Errorf("%dx%d: ratio not preserved: %dx%d", c.w, c.h, cx, cy)
		}
	}
	if cx, cy := fitExtent(W, H, 1000, 1000000); cx != 391 || cy != H {
		t.Errorf("extremely tall: %dx%d", cx, cy)
	}
	if cx, cy := fitExtent(W, H, 1000000, 1); cx != W || cy != 2 {
		t.Errorf("extremely wide: %dx%d", cx, cy)
	}
	if cx, cy := fitExtent(W, H, 1200, 256); cx != W || cy != 386080 {
		t.Errorf("1200x256: %dx%d, want %dx386080", cx, cy, W)
	}
}
