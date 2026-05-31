package ids

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"

	"patentmine/internal/domain"
)

// textValue is one filled text field in pdfcpu's form JSON.
type textValue struct {
	Pages []int  `json:"pages"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// boxValue is one filled checkbox in pdfcpu's form JSON.
type boxValue struct {
	Pages []int  `json:"pages"`
	Name  string `json:"name"`
	Value bool   `json:"value"`
}

// formData is the top-level JSON pdfcpu accepts for FillForm.
type formData struct {
	Forms []formPage `json:"forms"`
}

type formPage struct {
	Textfields []textValue `json:"textfield,omitempty"`
	Checkboxes []boxValue  `json:"checkbox,omitempty"`
}

func (f *formPage) addText(name, value string) {
	if name == "" {
		return
	}
	f.Textfields = append(f.Textfields, textValue{Pages: []int{1}, Name: name, Value: value})
}

func (f *formPage) addBox(name string, value bool) {
	if name == "" {
		return
	}
	f.Checkboxes = append(f.Checkboxes, boxValue{Pages: []int{1}, Name: name, Value: value})
}

// fillTemplate runs pdfcpu FillForm against the given template bytes with the
// supplied page-1 field values, returning the rendered PDF bytes.
func fillTemplate(template []byte, page formPage) ([]byte, error) {
	data, err := json.Marshal(formData{Forms: []formPage{page}})
	if err != nil {
		return nil, fmt.Errorf("ids: marshal form data: %w", err)
	}
	var out bytes.Buffer
	if err := api.FillForm(bytes.NewReader(template), bytes.NewReader(data), &out, nil); err != nil {
		return nil, fmt.Errorf("ids: fill form: %w", err)
	}
	return out.Bytes(), nil
}

// formatDate renders a time as MM-DD-YYYY per USPTO convention. Zero time
// returns the empty string so the cell stays blank.
func formatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(domain.USDateLayout)
}

// docCellUS builds the "Document Number-Kind Code" cell for a US patent row.
// The 08a form already has "US-" pre-printed for each row, so we emit only
// the serial and kind code.
func docCellUS(n any, kindOverride string) string {
	switch v := n.(type) {
	case Entry:
		k := v.Number.Kind
		if kindOverride != "" {
			k = kindOverride
		}
		if k == "" {
			return v.Number.Serial
		}
		return v.Number.Serial + "-" + k
	default:
		return ""
	}
}

// docCellForeign builds the foreign-row cell containing country-number-kind.
func docCellForeign(e Entry) string {
	parts := []string{e.Number.Country, e.Number.Serial}
	out := strings.Join(parts, "-")
	if e.Number.Kind != "" {
		out += "-" + e.Number.Kind
	}
	return out
}
