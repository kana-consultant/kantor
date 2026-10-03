package docgen

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
)

// This file holds the minimal XML scanner the renderer is built on. It does
// not build a DOM: it walks the part once, records the byte ranges of the
// elements the renderer cares about (w:p, w:tr, w:t) and extracts the
// placeholder tokens from w:t text. Any "{{" or "}}" found anywhere else
// (attributes, field codes, w:fldSimple, bare text) is rejected, as is a
// token split across runs or written with character references: the decoded
// text of every paragraph must hold exactly one "{{" and one "}}" per token
// found in it.

type element struct {
	name   string
	start  int // offset of the '<' of the start tag
	end    int // offset just past the end tag; -1 while open
	parent int // index of the parent element, -1 for the root
}

type tokenKind int

const (
	tokVar    tokenKind = iota // {{name}}
	tokOpen                    // {{#name}}
	tokInvert                  // {{^name}}
	tokClose                   // {{/name}}
	tokLogoCX                  // logo slot width (attribute value, internal)
	tokLogoCY                  // logo slot height (attribute value, internal)
)

type token struct {
	kind  tokenKind
	name  string
	start int // byte offset of the token in the part
	end   int
	para  int // innermost enclosing w:p element index, -1 if none
	row   int // innermost enclosing w:tr element index, -1 if none
	text  int // enclosing w:t element index, -1 for internal tokens
}

type partScan struct {
	elements []element
	tokens   []token
	// paraText is the entity-decoded w:t text per paragraph (key: w:p
	// element index, -1 for w:t outside any paragraph).
	paraText map[int]*strings.Builder
}

// placeholderRe matches one complete placeholder anchored at the start of
// the input. Names are restricted to [A-Za-z0-9_] so a token can never carry
// markup, entities or whitespace.
var placeholderRe = regexp.MustCompile(`^\{\{([#^/]?)([A-Za-z0-9_]+)\}\}`)

var (
	openBraces  = []byte("{{")
	closeBraces = []byte("}}")
)

func hasBraces(b []byte) bool {
	return bytes.Contains(b, openBraces) || bytes.Contains(b, closeBraces)
}

// scanPart scans one XML part. part is used in error messages only.
func scanPart(part string, data []byte) (*partScan, error) {
	s := &partScan{paraText: map[int]*strings.Builder{}}
	stack := make([]int, 0, 32)
	top := func() int {
		if len(stack) == 0 {
			return -1
		}
		return stack[len(stack)-1]
	}

	i := 0
	for i < len(data) {
		lt := bytes.IndexByte(data[i:], '<')
		textEnd := len(data)
		if lt >= 0 {
			textEnd = i + lt
		}
		if textEnd > i {
			if err := s.handleText(part, data, i, textEnd, top()); err != nil {
				return nil, err
			}
		}
		if lt < 0 {
			break
		}
		lt = textEnd

		switch {
		case bytes.HasPrefix(data[lt:], []byte("<?")):
			end := bytes.Index(data[lt:], []byte("?>"))
			if end < 0 {
				return nil, fmt.Errorf("%s: unterminated processing instruction at %d", part, lt)
			}
			if hasBraces(data[lt : lt+end]) {
				return nil, fmt.Errorf("%s: placeholder inside a processing instruction at %d", part, lt)
			}
			i = lt + end + 2
			continue
		case bytes.HasPrefix(data[lt:], []byte("<!--")):
			end := bytes.Index(data[lt:], []byte("-->"))
			if end < 0 {
				return nil, fmt.Errorf("%s: unterminated comment at %d", part, lt)
			}
			i = lt + end + 3
			continue
		case bytes.HasPrefix(data[lt:], []byte("<![CDATA[")):
			end := bytes.Index(data[lt:], []byte("]]>"))
			if end < 0 {
				return nil, fmt.Errorf("%s: unterminated CDATA at %d", part, lt)
			}
			if hasBraces(data[lt : lt+end]) {
				return nil, fmt.Errorf("%s: placeholder inside CDATA at %d", part, lt)
			}
			i = lt + end + 3
			continue
		case bytes.HasPrefix(data[lt:], []byte("<!")):
			end := bytes.IndexByte(data[lt:], '>')
			if end < 0 {
				return nil, fmt.Errorf("%s: unterminated declaration at %d", part, lt)
			}
			i = lt + end + 1
			continue
		}

		// Find the end of the tag, honouring quoted attribute values.
		j := lt + 1
		var quote byte
		for ; j < len(data); j++ {
			c := data[j]
			if quote != 0 {
				if c == quote {
					quote = 0
				}
				continue
			}
			if c == '"' || c == '\'' {
				quote = c
				continue
			}
			if c == '>' {
				break
			}
		}
		if j >= len(data) {
			return nil, fmt.Errorf("%s: unterminated tag at %d", part, lt)
		}
		tag := data[lt : j+1]
		if hasBraces(tag) {
			return nil, fmt.Errorf("%s: placeholder inside an attribute or tag at %d", part, lt)
		}
		closing := len(tag) > 2 && tag[1] == '/'
		selfClosing := !closing && len(tag) > 2 && tag[len(tag)-2] == '/'
		name := tagName(tag, closing)
		if name == "" {
			return nil, fmt.Errorf("%s: malformed tag at %d", part, lt)
		}

		if closing {
			t := top()
			if t < 0 || s.elements[t].name != name {
				return nil, fmt.Errorf("%s: unexpected </%s> at %d", part, name, lt)
			}
			s.elements[t].end = j + 1
			stack = stack[:len(stack)-1]
		} else {
			s.elements = append(s.elements, element{name: name, start: lt, end: -1, parent: top()})
			idx := len(s.elements) - 1
			if selfClosing {
				s.elements[idx].end = j + 1
			} else {
				stack = append(stack, idx)
			}
		}
		i = j + 1
	}
	if len(stack) > 0 {
		return nil, fmt.Errorf("%s: unclosed <%s>", part, s.elements[top()].name)
	}
	if err := s.checkParagraphText(part); err != nil {
		return nil, err
	}
	return s, nil
}

// checkParagraphText catches placeholders that the per-text-node scan cannot
// see: braces split over several runs ("{" "{name}" "}") or written as
// character references (&#123;&#123;name&#125;&#125;). Either would pass
// through unrendered, so every "{{" and "}}" in a paragraph's decoded text
// must belong to a token found in that paragraph.
func (s *partScan) checkParagraphText(part string) error {
	perPara := map[int]int{}
	for _, tk := range s.tokens {
		perPara[tk.para]++
	}
	paras := make([]int, 0, len(s.paraText))
	for p := range s.paraText {
		paras = append(paras, p)
	}
	sort.Ints(paras)
	for _, p := range paras {
		text := s.paraText[p].String()
		n := perPara[p]
		if strings.Count(text, "{{") != n || strings.Count(text, "}}") != n {
			at := -1
			if p >= 0 {
				at = s.elements[p].start
			}
			return fmt.Errorf("%s: split placeholder across runs or braces written as character references in the paragraph at %d: %q",
				part, at, snippet([]byte(text)))
		}
	}
	return nil
}

func tagName(tag []byte, closing bool) string {
	k := 1
	if closing {
		k = 2
	}
	end := k
	for end < len(tag) {
		c := tag[end]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '/' || c == '>' {
			break
		}
		end++
	}
	return string(tag[k:end])
}

// ancestor returns the innermost element named name that encloses element
// idx (idx itself included), or -1.
func (s *partScan) ancestor(idx int, name string) int {
	for idx >= 0 {
		if s.elements[idx].name == name {
			return idx
		}
		idx = s.elements[idx].parent
	}
	return -1
}

func (s *partScan) handleText(part string, data []byte, from, to, parent int) error {
	text := data[from:to]
	if parent >= 0 && s.elements[parent].name == "w:t" {
		p := s.ancestor(parent, "w:p")
		b := s.paraText[p]
		if b == nil {
			b = &strings.Builder{}
			s.paraText[p] = b
		}
		if bytes.IndexByte(text, '&') >= 0 {
			b.WriteString(html.UnescapeString(string(text)))
		} else {
			b.Write(text)
		}
	}
	if !hasBraces(text) {
		return nil
	}
	if parent < 0 {
		return fmt.Errorf("%s: placeholder outside any element at %d", part, from)
	}
	switch {
	case s.ancestor(parent, "w:instrText") >= 0:
		return fmt.Errorf("%s: placeholder inside a field code (w:instrText) at %d", part, from)
	case s.ancestor(parent, "w:fldSimple") >= 0:
		return fmt.Errorf("%s: placeholder inside a simple field (w:fldSimple) at %d", part, from)
	case s.elements[parent].name != "w:t":
		return fmt.Errorf("%s: placeholder outside w:t (in <%s>) at %d", part, s.elements[parent].name, from)
	}

	para := s.ancestor(parent, "w:p")
	row := s.ancestor(parent, "w:tr")
	k := from
	for k < to {
		o := bytes.Index(data[k:to], openBraces)
		c := bytes.Index(data[k:to], closeBraces)
		if o < 0 {
			if c >= 0 {
				return fmt.Errorf("%s: stray '}}' (split placeholder?) at %d", part, k+c)
			}
			break
		}
		if c >= 0 && c < o {
			return fmt.Errorf("%s: stray '}}' (split placeholder?) at %d", part, k+c)
		}
		o += k
		m := placeholderRe.FindSubmatch(data[o:to])
		if m == nil {
			return fmt.Errorf("%s: split or malformed placeholder at %d: %q", part, o, snippet(data[o:to]))
		}
		tok := token{name: string(m[2]), start: o, end: o + len(m[0]), para: para, row: row, text: parent}
		switch string(m[1]) {
		case "#":
			tok.kind = tokOpen
		case "^":
			tok.kind = tokInvert
		case "/":
			tok.kind = tokClose
		default:
			tok.kind = tokVar
		}
		s.tokens = append(s.tokens, tok)
		k = tok.end
	}
	return nil
}

// sameNesting reports whether text elements a and b sit at the same element
// path below their lowest common ancestor (same element names, level by
// level). Only then is the markup between a point inside a and a point
// inside b balanced, so the span can be removed or repeated and still leave
// well-formed XML.
func (s *partScan) sameNesting(a, b int) bool {
	pa, pb := s.path(a), s.path(b)
	i := 0
	for i < len(pa) && i < len(pb) && pa[i] == pb[i] {
		i++
	}
	ra, rb := pa[i:], pb[i:]
	if len(ra) != len(rb) {
		return false
	}
	for k := range ra {
		if s.elements[ra[k]].name != s.elements[rb[k]].name {
			return false
		}
	}
	return true
}

// path returns the element indices from the root down to idx.
func (s *partScan) path(idx int) []int {
	var p []int
	for ; idx >= 0; idx = s.elements[idx].parent {
		p = append(p, idx)
	}
	for l, r := 0, len(p)-1; l < r; l, r = l+1, r-1 {
		p[l], p[r] = p[r], p[l]
	}
	return p
}

func snippet(b []byte) string {
	if len(b) > 40 {
		b = b[:40]
	}
	return string(b)
}
