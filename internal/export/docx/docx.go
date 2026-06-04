// Package docx writes WordprocessingML (.docx) documents using only the Go
// standard library — a .docx is a ZIP of XML parts, so archive/zip plus a few
// templated parts is enough. Keeping it dependency-free preserves the project's
// pure-Go, single-static-binary build (the same reason the IDS exporter fills a
// PDF form rather than pulling in a renderer).
//
// The builder is intentionally small: paragraphs made of styled runs, a handful
// of paragraph styles (title, two heading levels, body), and an optional first
// line indent. That is all the drafting renderer needs, and it is all that has
// to be correct for Word and LibreOffice to open the file cleanly.
package docx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
)

// Style is a named paragraph style mapped to an entry in styles.xml.
type Style string

const (
	// StyleNormal is body text.
	StyleNormal Style = "Normal"
	// StyleTitle is the document title, centered and large.
	StyleTitle Style = "Title"
	// StyleHeading1 is a top-level section heading.
	StyleHeading1 Style = "Heading1"
	// StyleHeading2 is a sub-heading.
	StyleHeading2 Style = "Heading2"
)

// Run is a contiguous span of text sharing one set of character properties.
// Underline and Strike carry the claim-amendment markup (additions underlined,
// deletions struck through) computed deterministically by the renderer.
type Run struct {
	Text      string
	Bold      bool
	Italic    bool
	Underline bool
	Strike    bool
}

type paragraph struct {
	style       Style
	runs        []Run
	firstIndent bool // first-line indent, used for spec/claim body paragraphs
}

// Document accumulates paragraphs and renders them to .docx bytes. The zero
// value is ready to use; call the Add* helpers then Bytes.
type Document struct {
	paragraphs []paragraph
}

// New returns an empty Document.
func New() *Document { return &Document{} }

// Title adds a centered title paragraph.
func (d *Document) Title(text string) *Document {
	return d.add(paragraph{style: StyleTitle, runs: []Run{{Text: text, Bold: true}}})
}

// Heading1 adds a top-level heading paragraph.
func (d *Document) Heading1(text string) *Document {
	return d.add(paragraph{style: StyleHeading1, runs: []Run{{Text: text, Bold: true}}})
}

// Heading2 adds a sub-heading paragraph.
func (d *Document) Heading2(text string) *Document {
	return d.add(paragraph{style: StyleHeading2, runs: []Run{{Text: text, Bold: true}}})
}

// Para adds a plain body paragraph.
func (d *Document) Para(text string) *Document {
	return d.add(paragraph{style: StyleNormal, runs: []Run{{Text: text}}})
}

// IndentedPara adds a body paragraph with a first-line indent — the usual shape
// of a specification or claim paragraph.
func (d *Document) IndentedPara(text string) *Document {
	return d.add(paragraph{style: StyleNormal, runs: []Run{{Text: text}}, firstIndent: true})
}

// RunsPara adds a body paragraph composed of explicit styled runs. Used by the
// claim renderer to mix struck-through and underlined spans in one paragraph.
func (d *Document) RunsPara(runs []Run, firstIndent bool) *Document {
	return d.add(paragraph{style: StyleNormal, runs: runs, firstIndent: firstIndent})
}

// Spacer adds an empty body paragraph for vertical separation.
func (d *Document) Spacer() *Document {
	return d.add(paragraph{style: StyleNormal, runs: []Run{{Text: ""}}})
}

func (d *Document) add(p paragraph) *Document {
	d.paragraphs = append(d.paragraphs, p)
	return d
}

// Bytes renders the accumulated paragraphs to a complete .docx archive.
func (d *Document) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	parts := []struct {
		name string
		body string
	}{
		{"[Content_Types].xml", contentTypesXML},
		{"_rels/.rels", relsXML},
		{"word/_rels/document.xml.rels", documentRelsXML},
		{"word/styles.xml", stylesXML},
		{"word/document.xml", d.documentXML()},
	}
	for _, p := range parts {
		w, err := zw.Create(p.name)
		if err != nil {
			return nil, fmt.Errorf("docx: create %s: %w", p.name, err)
		}
		if _, err := w.Write([]byte(p.body)); err != nil {
			return nil, fmt.Errorf("docx: write %s: %w", p.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("docx: finalize archive: %w", err)
	}
	return buf.Bytes(), nil
}

// documentXML renders the body part.
func (d *Document) documentXML() string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, p := range d.paragraphs {
		writeParagraph(&b, p)
	}
	// A trailing section properties element keeps Word from reporting a repair.
	b.WriteString(`<w:sectPr><w:pgSz w:w="12240" w:h="15840"/>` +
		`<w:pgMar w:top="1440" w:right="1440" w:bottom="1440" w:left="1440" w:header="720" w:footer="720" w:gutter="0"/>` +
		`</w:sectPr>`)
	b.WriteString(`</w:body></w:document>`)
	return b.String()
}

func writeParagraph(b *strings.Builder, p paragraph) {
	b.WriteString(`<w:p><w:pPr>`)
	if p.style != "" && p.style != StyleNormal {
		fmt.Fprintf(b, `<w:pStyle w:val="%s"/>`, p.style)
	}
	if p.firstIndent {
		b.WriteString(`<w:ind w:firstLine="720"/>`)
	}
	b.WriteString(`</w:pPr>`)
	for _, r := range p.runs {
		writeRun(b, r)
	}
	b.WriteString(`</w:p>`)
}

func writeRun(b *strings.Builder, r Run) {
	b.WriteString(`<w:r>`)
	if r.Bold || r.Italic || r.Underline || r.Strike {
		b.WriteString(`<w:rPr>`)
		if r.Bold {
			b.WriteString(`<w:b/>`)
		}
		if r.Italic {
			b.WriteString(`<w:i/>`)
		}
		if r.Underline {
			b.WriteString(`<w:u w:val="single"/>`)
		}
		if r.Strike {
			b.WriteString(`<w:strike/>`)
		}
		b.WriteString(`</w:rPr>`)
	}
	fmt.Fprintf(b, `<w:t xml:space="preserve">%s</w:t>`, escapeXML(r.Text))
	b.WriteString(`</w:r>`)
}

// escapeXML escapes the five XML special characters for text content.
func escapeXML(s string) string {
	rep := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return rep.Replace(s)
}

const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

const contentTypesXML = xmlHeader +
	`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
	`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
	`</Types>`

const relsXML = xmlHeader +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>` +
	`</Relationships>`

const documentRelsXML = xmlHeader +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
	`</Relationships>`

// stylesXML declares the document defaults (Times New Roman 12pt, the patent
// drafting convention) and the four paragraph styles the builder emits.
const stylesXML = xmlHeader +
	`<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
	`<w:docDefaults><w:rPrDefault><w:rPr>` +
	`<w:rFonts w:ascii="Times New Roman" w:hAnsi="Times New Roman" w:cs="Times New Roman"/>` +
	`<w:sz w:val="24"/><w:szCs w:val="24"/>` +
	`</w:rPr></w:rPrDefault></w:docDefaults>` +
	`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/>` +
	`<w:pPr><w:spacing w:after="120" w:line="360" w:lineRule="auto"/></w:pPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Title"><w:name w:val="Title"/>` +
	`<w:pPr><w:jc w:val="center"/><w:spacing w:after="240"/></w:pPr>` +
	`<w:rPr><w:b/><w:sz w:val="32"/><w:szCs w:val="32"/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/>` +
	`<w:pPr><w:spacing w:before="240" w:after="120"/><w:outlineLvl w:val="0"/></w:pPr>` +
	`<w:rPr><w:b/><w:sz w:val="26"/><w:szCs w:val="26"/></w:rPr></w:style>` +
	`<w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/>` +
	`<w:pPr><w:spacing w:before="160" w:after="80"/><w:outlineLvl w:val="1"/></w:pPr>` +
	`<w:rPr><w:b/><w:i/><w:sz w:val="24"/><w:szCs w:val="24"/></w:rPr></w:style>` +
	`</w:styles>`
