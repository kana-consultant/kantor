package docgen

import (
	"regexp"
	"strconv"
)

var (
	pdfPageObjectRe = regexp.MustCompile(`/Type\s*/Page\b`)
	pdfPageCountRe  = regexp.MustCompile(`/Type\s*/Pages\b[^>]*?/Count\s+(\d+)|/Count\s+(\d+)[^>]*?/Type\s*/Pages\b`)
)

// PDFPageCount returns the page count of a PDF written by LibreOffice: the
// /Count of the root page tree when present, else the number of page
// objects. 0 means it could not be determined (e.g. the page tree sits in a
// compressed object stream).
func PDFPageCount(pdf []byte) int {
	if match := pdfPageCountRe.FindSubmatch(pdf); match != nil {
		value := string(match[1])
		if value == "" {
			value = string(match[2])
		}
		if count, err := strconv.Atoi(value); err == nil && count > 0 {
			return count
		}
	}
	return len(pdfPageObjectRe.FindAll(pdf, -1))
}
