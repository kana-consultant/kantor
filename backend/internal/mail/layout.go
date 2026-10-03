package mail

import (
	"bytes"
	"html/template"
	"strings"
)

// DocumentEmail is the content of the small, neutral, bilingual layout used
// for every document email (payslip, contract, test). It deliberately has no
// slots for amounts: salary figures only ever travel inside the PDF.
type DocumentEmail struct {
	// RecipientName is used in the greeting; empty falls back to
	// "Bapak/Ibu" / "Sir/Madam".
	RecipientName string
	// ParagraphsID and ParagraphsEN are plain-text paragraphs; they are
	// HTML-escaped in the HTML part.
	ParagraphsID []string
	ParagraphsEN []string
	// Signature is the closing name (usually the configured sender name).
	Signature string
}

const (
	footerID = "Email ini dikirim otomatis oleh sistem KANTOR."
	footerEN = "This email was sent automatically by the KANTOR system."
)

var documentEmailHTML = template.Must(template.New("document-email").Parse(`<!DOCTYPE html>
<html lang="id">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body style="margin:0;padding:0;background:#f4f5f7;font-family:Arial,Helvetica,sans-serif;color:#172b4d;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f4f5f7;padding:24px 0;">
<tr><td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;background:#ffffff;border:1px solid #dfe1e6;border-radius:6px;">
<tr><td style="padding:24px 28px 8px 28px;font-size:14px;line-height:22px;">
<p style="margin:0 0 12px 0;">Yth. {{.NameID}},</p>
{{range .ParagraphsID}}<p style="margin:0 0 12px 0;">{{.}}</p>
{{end}}<p style="margin:0 0 4px 0;">Salam,</p>
<p style="margin:0 0 20px 0;"><strong>{{.Signature}}</strong></p>
</td></tr>
<tr><td style="padding:0 28px;"><hr style="border:none;border-top:1px solid #dfe1e6;margin:0;"></td></tr>
<tr><td lang="en" style="padding:20px 28px 8px 28px;font-size:14px;line-height:22px;color:#42526e;">
<p style="margin:0 0 12px 0;">Dear {{.NameEN}},</p>
{{range .ParagraphsEN}}<p style="margin:0 0 12px 0;">{{.}}</p>
{{end}}<p style="margin:0 0 4px 0;">Regards,</p>
<p style="margin:0 0 20px 0;"><strong>{{.Signature}}</strong></p>
</td></tr>
<tr><td style="padding:12px 28px 20px 28px;font-size:12px;line-height:18px;color:#6b778c;border-top:1px solid #dfe1e6;">
{{.FooterID}}<br>{{.FooterEN}}
</td></tr>
</table>
</td></tr>
</table>
</body>
</html>
`))

// RenderDocumentEmail returns the text/plain and text/html bodies.
func RenderDocumentEmail(content DocumentEmail) (text string, html string, err error) {
	nameID := strings.TrimSpace(content.RecipientName)
	nameEN := nameID
	if nameID == "" {
		nameID = "Bapak/Ibu"
		nameEN = "Sir/Madam"
	}
	signature := strings.TrimSpace(content.Signature)
	if signature == "" {
		signature = "KANTOR"
	}
	paragraphsID := cleanParagraphs(content.ParagraphsID)
	paragraphsEN := cleanParagraphs(content.ParagraphsEN)

	var textBuilder strings.Builder
	textBuilder.WriteString("Yth. " + nameID + ",\n\n")
	for _, paragraph := range paragraphsID {
		textBuilder.WriteString(paragraph + "\n\n")
	}
	textBuilder.WriteString("Salam,\n" + signature + "\n\n")
	textBuilder.WriteString("----------------------------------------\n\n")
	textBuilder.WriteString("Dear " + nameEN + ",\n\n")
	for _, paragraph := range paragraphsEN {
		textBuilder.WriteString(paragraph + "\n\n")
	}
	textBuilder.WriteString("Regards,\n" + signature + "\n\n")
	textBuilder.WriteString("--\n" + footerID + "\n" + footerEN + "\n")

	var htmlBuffer bytes.Buffer
	if err := documentEmailHTML.Execute(&htmlBuffer, map[string]any{
		"NameID":       nameID,
		"NameEN":       nameEN,
		"ParagraphsID": paragraphsID,
		"ParagraphsEN": paragraphsEN,
		"Signature":    signature,
		"FooterID":     footerID,
		"FooterEN":     footerEN,
	}); err != nil {
		return "", "", err
	}

	return textBuilder.String(), htmlBuffer.String(), nil
}

func cleanParagraphs(paragraphs []string) []string {
	cleaned := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		if trimmed := strings.TrimSpace(paragraph); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}
