// Package doctext extracts embedded text from local documents using deterministic
// parsers only. It does not perform OCR and never calls AI.
package doctext

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"patentmine/internal/pdf"
)

// ExtractText returns embedded text from path. Image-only scans return an empty
// string with no error when the container is readable but has no text layer.
func ExtractText(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".text", ".md", ".csv", ".tsv":
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	case ".pdf":
		return pdf.ExtractText(path)
	case ".docx":
		return extractDocxText(path)
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		return extractXlsxText(path)
	case ".xls":
		return "", fmt.Errorf("doctext: legacy .xls binary workbooks require a BIFF parser; convert to .xlsx or add an .xls parser")
	default:
		return "", fmt.Errorf("doctext: unsupported document format %q", filepath.Ext(path))
	}
}

// NeedsOCR reports whether a successfully parsed document has no embedded text
// and is a kind of file that commonly stores scanned pages or images. OCR itself
// is not implemented here.
func NeedsOCR(path, text string) bool {
	if strings.TrimSpace(text) != "" {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf", ".docx", ".png", ".jpg", ".jpeg", ".webp", ".tif", ".tiff":
		return true
	default:
		return false
	}
}

func extractDocxText(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = zr.Close() }()

	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer func() { _ = rc.Close() }()
			return parseDocxXML(rc)
		}
	}
	return "", fmt.Errorf("doctext: word/document.xml not found in docx")
}

func parseDocxXML(r io.Reader) (string, error) {
	dec := xml.NewDecoder(r)
	var b strings.Builder
	var inText bool
	for {
		t, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		switch se := t.(type) {
		case xml.StartElement:
			switch se.Name.Local {
			case "t":
				inText = true
			case "p", "br", "cr":
				b.WriteByte('\n')
			}
		case xml.EndElement:
			if se.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				b.Write(se)
			}
		}
	}
	return strings.TrimSpace(b.String()), nil
}

func extractXlsxText(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = zr.Close() }()

	shared, err := readSharedStrings(&zr.Reader)
	if err != nil {
		return "", err
	}
	sheets := worksheetFiles(&zr.Reader)
	var parts []string
	for _, f := range sheets {
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		text, parseErr := parseWorksheetXML(rc, shared)
		_ = rc.Close()
		if parseErr != nil {
			return "", parseErr
		}
		if strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n")), nil
}

func worksheetFiles(zr *zip.Reader) []*zip.File {
	var files []*zip.File
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "xl/worksheets/") && strings.HasSuffix(f.Name, ".xml") {
			files = append(files, f)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files
}

func readSharedStrings(zr *zip.Reader) ([]string, error) {
	for _, f := range zr.File {
		if f.Name != "xl/sharedStrings.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer func() { _ = rc.Close() }()
		return parseSharedStringsXML(rc)
	}
	return nil, nil
}

func parseSharedStringsXML(r io.Reader) ([]string, error) {
	dec := xml.NewDecoder(r)
	var stringsOut []string
	var b strings.Builder
	var inSI, inText bool
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "si":
				inSI = true
				b.Reset()
			case "t":
				if inSI {
					inText = true
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "si":
				stringsOut = append(stringsOut, b.String())
				inSI = false
			}
		case xml.CharData:
			if inText {
				b.Write(t)
			}
		}
	}
	return stringsOut, nil
}

func parseWorksheetXML(r io.Reader, shared []string) (string, error) {
	dec := xml.NewDecoder(r)
	var rows []string
	var cells []string
	var cellType, cellText string
	var inCell, inValue, inText bool
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "row":
				cells = nil
			case "c":
				inCell = true
				cellType = attr(t, "t")
				cellText = ""
			case "v":
				if inCell {
					inValue = true
				}
			case "t":
				if inCell {
					inText = true
				}
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "v":
				inValue = false
			case "t":
				inText = false
			case "c":
				if v := resolveCellText(cellType, cellText, shared); v != "" {
					cells = append(cells, v)
				}
				inCell = false
			case "row":
				if len(cells) > 0 {
					rows = append(rows, strings.Join(cells, "\t"))
				}
			}
		case xml.CharData:
			if inValue || inText {
				cellText += string(t)
			}
		}
	}
	return strings.Join(rows, "\n"), nil
}

func attr(se xml.StartElement, name string) string {
	for _, a := range se.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func resolveCellText(cellType, raw string, shared []string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	switch cellType {
	case "s":
		i, err := strconv.Atoi(raw)
		if err == nil && i >= 0 && i < len(shared) {
			return strings.TrimSpace(shared[i])
		}
	case "b":
		if raw == "1" {
			return "TRUE"
		}
		if raw == "0" {
			return "FALSE"
		}
	}
	return raw
}
