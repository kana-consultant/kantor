package docgen

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
)

// kamus mirrors testdata/kamus_variabel.json (sanitized copy of the template
// pack's variable dictionary, restricted to templates 01, 02 and 08).
type kamus struct {
	Templates map[string]struct {
		Variabel      map[string]json.RawMessage `json:"variabel"`
		ContohPayload map[string]any             `json:"contoh_payload"`
	} `json:"templates"`
}

func loadKamus(t *testing.T) kamus {
	t.Helper()
	raw, err := os.ReadFile("testdata/kamus_variabel.json")
	if err != nil {
		t.Fatalf("read kamus: %v", err)
	}
	var k kamus
	if err := json.Unmarshal(raw, &k); err != nil {
		t.Fatalf("parse kamus: %v", err)
	}
	return k
}

// kamusPayload returns a fresh copy of the example payload of a template.
func kamusPayload(t *testing.T, id TemplateID) Payload {
	t.Helper()
	k := loadKamus(t)
	tpl, ok := k.Templates[string(id)]
	if !ok {
		t.Fatalf("kamus has no template %s", id)
	}
	return Payload(tpl.ContohPayload)
}

func mustLoad(t *testing.T, id TemplateID) *Template {
	t.Helper()
	tpl, err := Load(id)
	if err != nil {
		t.Fatalf("Load(%s): %v", id, err)
	}
	return tpl
}

func mustRender(t *testing.T, id TemplateID, payload Payload, opts ...Option) []byte {
	t.Helper()
	out, err := Render(mustLoad(t, id), payload, opts...)
	if err != nil {
		t.Fatalf("Render(%s): %v", id, err)
	}
	return out
}

// zipEntries returns the uncompressed content of every entry.
func zipEntries(t *testing.T, docx []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		out[f.Name] = data
	}
	return out
}

// assertWellFormed fails unless data is well-formed XML.
func assertWellFormed(t *testing.T, name string, data []byte) {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		_, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatalf("%s is not well-formed XML: %v", name, err)
		}
	}
}

// paragraphText extracts the decoded text of every w:p, one line per
// paragraph, in document order (nested table paragraphs included).
func paragraphText(t *testing.T, data []byte) []string {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(data))
	var lines []string
	var cur strings.Builder
	inT := false
	inTabs := false
	depth := 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			switch el.Name.Local {
			case "p":
				depth++
				if depth == 1 {
					cur.Reset()
				}
			case "t":
				inT = true
			case "tabs":
				inTabs = true
			case "tab":
				if !inTabs {
					cur.WriteByte('\t')
				}
			}
		case xml.EndElement:
			switch el.Name.Local {
			case "p":
				depth--
				if depth == 0 {
					lines = append(lines, cur.String())
				}
			case "t":
				inT = false
			case "tabs":
				inTabs = false
			}
		case xml.CharData:
			if inT {
				cur.Write(el)
			}
		}
	}
	return lines
}

var trRe = regexp.MustCompile(`<w:tr[ >]`)

func countRows(data []byte) int { return len(trRe.FindAll(data, -1)) }

// docxFrom builds a minimal DOCX-like zip from parts, for validator tests.
func docxFrom(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range parts {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const testDocHead = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
	`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:body>`

const testDocTail = `</w:body></w:document>`

func testDoc(body string) string { return testDocHead + body + testDocTail }

func para(text string) string {
	return `<w:p><w:r><w:t xml:space="preserve">` + text + `</w:t></w:r></w:p>`
}

// bannedIdentifierDigests are the sha256 digests of the lower-cased
// real-world identifiers (client brand, company name, signer name) that must
// never appear in the committed templates or test data, mapped to the
// identifier's length in bytes. Only digests are kept, so the identifiers
// themselves are not in the repository either.
var bannedIdentifierDigests = map[string]int{
	"658532f754deac4270c84e522974e534e027786081edc2a58d9fe841f9e09596": 9, // client brand
	"d67fd8ee2949a0573107ef8f7c9758674816cc3f4554cc9aa509d60d9ce74dec": 9, // company name
	"8c3fb8f94678f1214084c690601d0afd12a45c3b6940b040344c389b6225e547": 4, // signer name
}

// containsBannedIdentifier reports whether text (any case) contains one of
// the banned identifiers, by hashing every substring of each banned length.
func containsBannedIdentifier(text []byte) bool {
	lower := bytes.ToLower(text)
	lengths := map[int]bool{}
	for _, n := range bannedIdentifierDigests {
		lengths[n] = true
	}
	for n := range lengths {
		for i := 0; i+n <= len(lower); i++ {
			sum := sha256.Sum256(lower[i : i+n])
			if _, ok := bannedIdentifierDigests[hex.EncodeToString(sum[:])]; ok {
				return true
			}
		}
	}
	return false
}
