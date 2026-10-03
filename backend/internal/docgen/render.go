// Package docgen renders the embedded DOCX document templates (PKWT, NDA,
// payslip) and converts them to PDF with LibreOffice.
//
// Rendering is single pass: every template part is tokenized once at load
// time into a tree of literal XML, variables and sections. Rendering walks
// that tree and writes values through encoding/xml.EscapeText without ever
// rescanning them, so a value that contains "{{" is written literally and can
// neither be substituted nor break the document.
//
// Placeholder syntax (a subset of Mustache, as used by the templates):
//
//	{{name}}            scalar, must be a string in the payload
//	{{#list}}..{{/list}} row loop (open and close tag in the same table row)
//	                     or inline section (same paragraph); a bool shows or
//	                     hides the section, a list repeats it per item
//	{{^flag}}..{{/flag}} inverted section (shown for false or an empty list)
//
// Every placeholder must sit inside a single w:t run. Only word/document.xml
// and word/header*.xml / word/footer*.xml are rendered; every other zip entry
// is copied byte for byte.
package docgen

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"image/png"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Payload holds the values for one render. Scalars are strings; sections take
// a bool, a list of maps ([]map[string]string, []map[string]any or []any of
// maps, as decoded from JSON) or a map.
type Payload map[string]any

// ErrRender wraps every error caused by the payload (missing or mistyped
// values) so callers can tell them apart from I/O failures.
var ErrRender = errors.New("docgen: render failed")

var renderedPartRe = regexp.MustCompile(`^word/(document|header[0-9]*|footer[0-9]*)\.xml$`)

type nodeKind int

const (
	nodeLiteral nodeKind = iota
	nodeVar
	nodeSection
	nodeLogoCX
	nodeLogoCY
)

type node struct {
	kind     nodeKind
	literal  []byte
	name     string
	inverted bool
	children []node
}

type compiledPart struct {
	nodes []node
}

type logoSlot struct {
	media  string // zip path of the image to replace
	cx, cy int64  // original slot size in EMU
}

// Template is a compiled, immutable DOCX template. It is safe for concurrent
// use.
type Template struct {
	ID TemplateID
	// Version is the hex sha256 of the embedded template file, stored with
	// every generated document so it can be re-rendered identically.
	Version string

	raw   []byte
	parts map[string]*compiledPart
	logo  *logoSlot
}

// HasLogoSlot reports whether the template carries a tenant logo slot.
func (t *Template) HasLogoSlot() bool { return t.logo != nil }

// Option adjusts a single render.
type Option func(*renderOptions)

type renderOptions struct {
	logo    []byte
	logoSet bool
}

// WithLogo sets the tenant logo for templates with a logo slot. img must be
// the output of ProcessLogo (the stored tenant logo): it is embedded as is,
// only its PNG header is read to aspect-fit the slot. Processing happens
// once at upload, so a render never repeats the trim and the slip shows
// exactly the logo Admin previews. nil leaves the slot empty, which is also
// the default. Templates without a slot ignore it.
func WithLogo(img []byte) Option {
	return func(o *renderOptions) {
		o.logo = img
		o.logoSet = true
	}
}

// Render renders tpl with payload and returns the DOCX bytes.
func Render(tpl *Template, payload Payload, opts ...Option) ([]byte, error) {
	if tpl == nil {
		return nil, errors.New("docgen: nil template")
	}
	var ro renderOptions
	for _, opt := range opts {
		opt(&ro)
	}

	st := &renderState{}
	var logoPNG []byte
	if tpl.logo != nil {
		var err error
		logoPNG, st.logoCX, st.logoCY, err = tpl.placeLogo(ro.logo)
		if err != nil {
			return nil, err
		}
	}

	zr, err := zip.NewReader(bytes.NewReader(tpl.raw), int64(len(tpl.raw)))
	if err != nil {
		return nil, fmt.Errorf("docgen: open template %s: %w", tpl.ID, err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		part, rendered := tpl.parts[f.Name]
		switch {
		case rendered:
			var buf bytes.Buffer
			buf.Grow(int(f.UncompressedSize64) + 4096)
			if err := st.render(&buf, part.nodes, []any{map[string]any(payload)}); err != nil {
				return nil, err
			}
			if err := writeEntry(zw, f, buf.Bytes()); err != nil {
				return nil, err
			}
		case tpl.logo != nil && f.Name == tpl.logo.media:
			if err := writeEntry(zw, f, logoPNG); err != nil {
				return nil, err
			}
		default:
			if err := copyRaw(zw, f); err != nil {
				return nil, err
			}
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("docgen: finish docx: %w", err)
	}
	return out.Bytes(), nil
}

func writeEntry(zw *zip.Writer, f *zip.File, data []byte) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Deflate, Modified: f.Modified})
	if err != nil {
		return fmt.Errorf("docgen: write %s: %w", f.Name, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("docgen: write %s: %w", f.Name, err)
	}
	return nil
}

func copyRaw(zw *zip.Writer, f *zip.File) error {
	raw, err := f.OpenRaw()
	if err != nil {
		return fmt.Errorf("docgen: read %s: %w", f.Name, err)
	}
	hdr := f.FileHeader
	w, err := zw.CreateRaw(&hdr)
	if err != nil {
		return fmt.Errorf("docgen: copy %s: %w", f.Name, err)
	}
	if _, err := io.Copy(w, raw); err != nil {
		return fmt.Errorf("docgen: copy %s: %w", f.Name, err)
	}
	return nil
}

// placeLogo returns the PNG to embed and the aspect-fitted extent (EMU) for
// the template's logo slot. img is a ProcessLogo result; anything that is
// not a PNG within the processed bounds is rejected with ErrInvalidLogo.
func (t *Template) placeLogo(img []byte) ([]byte, int64, int64, error) {
	if len(img) == 0 {
		return BlankPNG(), t.logo.cx, t.logo.cy, nil
	}
	if !bytes.HasPrefix(img, pngMagic) {
		return nil, 0, 0, fmt.Errorf("%w: logo belum diproses (bukan PNG)", ErrInvalidLogo)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(img))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("%w: %v", ErrInvalidLogo, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > MaxLogoWidth || cfg.Height > MaxLogoHeight {
		return nil, 0, 0, fmt.Errorf("%w: logo %dx%d px belum diproses", ErrInvalidLogo, cfg.Width, cfg.Height)
	}
	cx, cy := fitExtent(t.logo.cx, t.logo.cy, int64(cfg.Width), int64(cfg.Height))
	return img, cx, cy, nil
}

type renderState struct {
	logoCX, logoCY int64
}

func (st *renderState) render(w *bytes.Buffer, nodes []node, ctx []any) error {
	for i := range nodes {
		n := &nodes[i]
		switch n.kind {
		case nodeLiteral:
			w.Write(n.literal)
		case nodeLogoCX:
			w.WriteString(strconv.FormatInt(st.logoCX, 10))
		case nodeLogoCY:
			w.WriteString(strconv.FormatInt(st.logoCY, 10))
		case nodeVar:
			v, ok := lookup(ctx, n.name)
			if !ok {
				return fmt.Errorf("%w: missing value for {{%s}}", ErrRender, n.name)
			}
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("%w: {{%s}} must be a string, got %T", ErrRender, n.name, v)
			}
			if err := xml.EscapeText(w, []byte(SingleLine(s))); err != nil {
				return err
			}
		case nodeSection:
			if err := st.renderSection(w, n, ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func (st *renderState) renderSection(w *bytes.Buffer, n *node, ctx []any) error {
	v, ok := lookup(ctx, n.name)
	if !ok {
		return fmt.Errorf("%w: missing value for section {{#%s}}", ErrRender, n.name)
	}
	var items []any
	truthy := false
	switch val := v.(type) {
	case nil:
		// A present key with a null value (a nil Go slice after a JSON
		// round trip) is an empty list, not a missing value.
	case bool:
		truthy = val
	case string:
		truthy = val != ""
	case map[string]any, map[string]string:
		truthy = true
		items = []any{val}
	case []any:
		for _, it := range val {
			switch it.(type) {
			case map[string]any, map[string]string:
			default:
				return fmt.Errorf("%w: items of {{#%s}} must be objects, got %T", ErrRender, n.name, it)
			}
		}
		items = val
		truthy = len(val) > 0
	case []map[string]any:
		for _, it := range val {
			items = append(items, it)
		}
		truthy = len(val) > 0
	case []map[string]string:
		for _, it := range val {
			items = append(items, it)
		}
		truthy = len(val) > 0
	default:
		return fmt.Errorf("%w: section {{#%s}} needs a bool or a list, got %T", ErrRender, n.name, v)
	}

	if n.inverted {
		if truthy {
			return nil
		}
		return st.render(w, n.children, ctx)
	}
	if !truthy {
		return nil
	}
	if items == nil {
		return st.render(w, n.children, ctx)
	}
	for _, it := range items {
		inner := make([]any, len(ctx)+1)
		copy(inner, ctx)
		inner[len(ctx)] = it
		if err := st.render(w, n.children, inner); err != nil {
			return err
		}
	}
	return nil
}

// lookup resolves name from the innermost context outwards (Mustache rules).
func lookup(ctx []any, name string) (any, bool) {
	for i := len(ctx) - 1; i >= 0; i-- {
		switch m := ctx[i].(type) {
		case map[string]any:
			if v, ok := m[name]; ok {
				return v, true
			}
		case map[string]string:
			if v, ok := m[name]; ok {
				return v, true
			}
		}
	}
	return nil, false
}

// ---------------------------------------------------------------------------
// Compilation

// compile validates a DOCX template and turns its rendered parts into node
// trees. logoMedia, when non-empty, is the zip path of the image that forms
// the tenant logo slot.
func compile(id TemplateID, raw []byte, logoMedia string) (*Template, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("docgen: %s is not a zip: %w", id, err)
	}
	t := &Template{ID: id, Version: sha256Hex(raw), raw: raw, parts: map[string]*compiledPart{}}

	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}

	var slot *logoSlot
	var slotRID string
	if logoMedia != "" {
		if _, ok := files[logoMedia]; !ok {
			return nil, fmt.Errorf("docgen: %s: logo media %s not found", id, logoMedia)
		}
		rels, err := readEntry(files["word/_rels/document.xml.rels"])
		if err != nil {
			return nil, fmt.Errorf("docgen: %s: %w", id, err)
		}
		slotRID, err = relIDForTarget(rels, logoMedia)
		if err != nil {
			return nil, fmt.Errorf("docgen: %s: %w", id, err)
		}
		slot = &logoSlot{media: logoMedia}
	}

	for _, f := range zr.File {
		isXML := strings.HasSuffix(f.Name, ".xml") || strings.HasSuffix(f.Name, ".rels")
		if !isXML {
			continue
		}
		data, err := readEntry(f)
		if err != nil {
			return nil, fmt.Errorf("docgen: %s: %w", id, err)
		}
		if !renderedPartRe.MatchString(f.Name) {
			// Placeholders are only honoured in the rendered parts; anywhere
			// else (.rels, styles, comments, footnotes, docProps) they would
			// leak into the output unrendered.
			if hasBraces(data) {
				return nil, fmt.Errorf("docgen: %s: placeholder in %s, which is never rendered", id, f.Name)
			}
			continue
		}
		scan, err := scanPart(f.Name, data)
		if err != nil {
			return nil, fmt.Errorf("docgen: %s: %w", id, err)
		}
		if slot != nil && f.Name == "word/document.xml" {
			toks, cx, cy, err := findLogoSlot(data, slotRID)
			if err != nil {
				return nil, fmt.Errorf("docgen: %s: %w", id, err)
			}
			slot.cx, slot.cy = cx, cy
			scan.tokens = append(scan.tokens, toks...)
			sort.Slice(scan.tokens, func(a, b int) bool { return scan.tokens[a].start < scan.tokens[b].start })
		}
		nodes, err := buildTree(f.Name, data, scan)
		if err != nil {
			return nil, fmt.Errorf("docgen: %s: %w", id, err)
		}
		t.parts[f.Name] = &compiledPart{nodes: nodes}
	}
	if _, ok := t.parts["word/document.xml"]; !ok {
		return nil, fmt.Errorf("docgen: %s: word/document.xml missing", id)
	}
	if slot != nil {
		if slot.cx <= 0 || slot.cy <= 0 {
			return nil, fmt.Errorf("docgen: %s: logo slot not found in word/document.xml", id)
		}
		t.logo = slot
	}
	return t, nil
}

func readEntry(f *zip.File) ([]byte, error) {
	if f == nil {
		return nil, errors.New("zip entry missing")
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", f.Name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, 64<<20))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", f.Name, err)
	}
	return data, nil
}

type section struct {
	open, close int // token indices
	us, ue      int // unit byte range that is repeated/removed
}

type entry struct {
	start, end int
	tok        int // token index for var/slot entries, -1 for sections
	sec        int // section index, -1 for tokens
}

// buildTree matches sections, determines the unit each section repeats (its
// table row, or the text between its tags when both sit in one paragraph)
// and builds the node tree. Every template token ends up in exactly one node;
// anything else is an error, which is the leftover-tag check.
func buildTree(part string, data []byte, scan *partScan) ([]node, error) {
	toks := scan.tokens
	var sections []section
	var stack []int
	for i, tk := range toks {
		switch tk.kind {
		case tokOpen, tokInvert:
			stack = append(stack, i)
		case tokClose:
			if len(stack) == 0 {
				return nil, fmt.Errorf("%s: {{/%s}} without an opening tag", part, tk.name)
			}
			o := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if toks[o].name != tk.name {
				return nil, fmt.Errorf("%s: {{/%s}} closes {{#%s}}", part, tk.name, toks[o].name)
			}
			sec := section{open: o, close: i}
			ot := toks[o]
			switch {
			case ot.para >= 0 && ot.para == tk.para:
				// The text between the tags is the unit. It is only
				// balanced markup when both tags sit at the same depth
				// (e.g. not one inside a w:hyperlink and the other not).
				if ot.text < 0 || tk.text < 0 || !scan.sameNesting(ot.text, tk.text) {
					return nil, fmt.Errorf("%s: inline section {{#%s}} opens and closes at different element depths (hyperlink, smart tag, tracked change?)", part, tk.name)
				}
				sec.us, sec.ue = ot.start, tk.end
			case ot.row >= 0 && ot.row == tk.row:
				row := scan.elements[ot.row]
				sec.us, sec.ue = row.start, row.end
			default:
				return nil, fmt.Errorf("%s: section {{#%s}} must open and close in one paragraph or one table row", part, tk.name)
			}
			sections = append(sections, sec)
		}
	}
	if len(stack) > 0 {
		return nil, fmt.Errorf("%s: {{#%s}} is never closed", part, toks[stack[len(stack)-1]].name)
	}

	entries := make([]entry, 0, len(toks))
	for i, tk := range toks {
		if tk.kind == tokOpen || tk.kind == tokInvert || tk.kind == tokClose {
			continue
		}
		entries = append(entries, entry{start: tk.start, end: tk.end, tok: i, sec: -1})
	}
	for i, s := range sections {
		entries = append(entries, entry{start: s.us, end: s.ue, tok: -1, sec: i})
	}
	sort.SliceStable(entries, func(a, b int) bool {
		if entries[a].start != entries[b].start {
			return entries[a].start < entries[b].start
		}
		return entries[a].end > entries[b].end
	})

	b := &treeBuilder{part: part, data: data, toks: toks, sections: sections}
	nodes, err := b.build(0, len(data), entries)
	if err != nil {
		return nil, err
	}
	if b.used != len(toks) {
		return nil, fmt.Errorf("%s: %d placeholder(s) could not be placed", part, len(toks)-b.used)
	}
	return nodes, nil
}

type treeBuilder struct {
	part     string
	data     []byte
	toks     []token
	sections []section
	used     int
}

// build turns [from, to) into nodes. entries must be sorted and lie inside
// [from, to).
func (b *treeBuilder) build(from, to int, entries []entry) ([]node, error) {
	var nodes []node
	pos := from
	for i := 0; i < len(entries); {
		e := entries[i]
		if e.start < pos || e.end > to {
			return nil, b.overlapErr(e)
		}
		if e.start > pos {
			nodes = append(nodes, node{kind: nodeLiteral, literal: b.data[pos:e.start]})
		}
		if e.sec < 0 {
			tk := b.toks[e.tok]
			n := node{name: tk.name}
			switch tk.kind {
			case tokLogoCX:
				n.kind = nodeLogoCX
			case tokLogoCY:
				n.kind = nodeLogoCY
			default:
				n.kind = nodeVar
			}
			nodes = append(nodes, n)
			b.used++
			pos = e.end
			i++
			continue
		}

		// Collect the entries nested in this section's unit.
		j := i + 1
		for j < len(entries) && entries[j].start < e.end {
			if entries[j].end > e.end {
				return nil, b.overlapErr(entries[j])
			}
			j++
		}
		nested := entries[i+1 : j]
		sec := b.sections[e.sec]
		o, c := b.toks[sec.open], b.toks[sec.close]
		intervals := [3][2]int{{sec.us, o.start}, {o.end, c.start}, {c.end, sec.ue}}
		var children []node
		k := 0
		for _, iv := range intervals {
			start := k
			for k < len(nested) && nested[k].start >= iv[0] && nested[k].end <= iv[1] {
				k++
			}
			sub, err := b.build(iv[0], iv[1], nested[start:k])
			if err != nil {
				return nil, err
			}
			children = append(children, sub...)
		}
		if k != len(nested) {
			return nil, b.overlapErr(nested[k])
		}
		b.used += 2
		nodes = append(nodes, node{
			kind:     nodeSection,
			name:     o.name,
			inverted: o.kind == tokInvert,
			children: children,
		})
		pos = e.end
		i = j
	}
	if pos < to {
		nodes = append(nodes, node{kind: nodeLiteral, literal: b.data[pos:to]})
	}
	return nodes, nil
}

func (b *treeBuilder) overlapErr(e entry) error {
	if e.sec >= 0 {
		return fmt.Errorf("%s: section {{#%s}} overlaps another section", b.part, b.toks[b.sections[e.sec].open].name)
	}
	return fmt.Errorf("%s: placeholder {{%s}} overlaps a section boundary", b.part, b.toks[e.tok].name)
}

// ---------------------------------------------------------------------------
// Logo slot discovery

var relRe = regexp.MustCompile(`<Relationship\s[^>]*>`)
var attrRe = regexp.MustCompile(`([A-Za-z:]+)="([^"]*)"`)

func relIDForTarget(rels []byte, media string) (string, error) {
	want := strings.TrimPrefix(media, "word/")
	var found string
	for _, rel := range relRe.FindAll(rels, -1) {
		attrs := map[string]string{}
		for _, m := range attrRe.FindAllSubmatch(rel, -1) {
			attrs[string(m[1])] = string(m[2])
		}
		if attrs["Target"] == want {
			if found != "" {
				return "", fmt.Errorf("logo media %s referenced twice", media)
			}
			found = attrs["Id"]
		}
	}
	if found == "" {
		return "", fmt.Errorf("no relationship targets %s", media)
	}
	return found, nil
}

// findLogoSlot locates the w:drawing that embeds relID and returns tokens for
// the cx/cy attribute values of its wp:extent and a:ext, plus the original
// slot size.
func findLogoSlot(doc []byte, relID string) ([]token, int64, int64, error) {
	needle := []byte(`r:embed="` + relID + `"`)
	if n := bytes.Count(doc, needle); n != 1 {
		return nil, 0, 0, fmt.Errorf("expected one drawing embedding %s, found %d", relID, n)
	}
	at := bytes.Index(doc, needle)
	ds := bytes.LastIndex(doc[:at], []byte("<w:drawing>"))
	de := bytes.Index(doc[at:], []byte("</w:drawing>"))
	if ds < 0 || de < 0 {
		return nil, 0, 0, errors.New("logo drawing element not found")
	}
	de += at

	var toks []token
	var cx, cy int64
	for _, tagName := range []string{"<wp:extent ", "<a:ext "} {
		rel := bytes.Index(doc[ds:de], []byte(tagName))
		if rel < 0 {
			return nil, 0, 0, fmt.Errorf("logo drawing has no %s", tagName)
		}
		if bytes.Count(doc[ds:de], []byte(tagName)) != 1 {
			return nil, 0, 0, fmt.Errorf("logo drawing has more than one %s", tagName)
		}
		tagStart := ds + rel
		tagEnd := tagStart + bytes.IndexByte(doc[tagStart:], '>')
		for _, attr := range []struct {
			name string
			kind tokenKind
		}{{"cx", tokLogoCX}, {"cy", tokLogoCY}} {
			key := []byte(" " + attr.name + `="`)
			k := bytes.Index(doc[tagStart:tagEnd], key)
			if k < 0 {
				return nil, 0, 0, fmt.Errorf("%s has no %s", tagName, attr.name)
			}
			vs := tagStart + k + len(key)
			ve := vs + bytes.IndexByte(doc[vs:tagEnd], '"')
			val, err := strconv.ParseInt(string(doc[vs:ve]), 10, 64)
			if err != nil || val <= 0 {
				return nil, 0, 0, fmt.Errorf("%s %s is not a positive integer", tagName, attr.name)
			}
			if attr.kind == tokLogoCX {
				if cx != 0 && cx != val {
					return nil, 0, 0, errors.New("wp:extent and a:ext disagree on cx")
				}
				cx = val
			} else {
				if cy != 0 && cy != val {
					return nil, 0, 0, errors.New("wp:extent and a:ext disagree on cy")
				}
				cy = val
			}
			toks = append(toks, token{kind: attr.kind, name: attr.name, start: vs, end: ve, para: -1, row: -1, text: -1})
		}
	}
	return toks, cx, cy, nil
}
