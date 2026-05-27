package uspto

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// StructToTOML marshals any Go struct (in practice a *USPTOGrantXML) to
// structured TOML text suitable for the raw XML viewer popup. It performs a
// JSON round-trip to get a map, then pretty-prints with MapToTOML while
// dropping the enormous description/claims body sections so the result stays
// readable.
func StructToTOML(val any) (string, error) {
	bytes, err := json.Marshal(val)
	if err != nil {
		return "", err
	}

	var m map[string]any
	if err := json.Unmarshal(bytes, &m); err != nil {
		return "", err
	}

	// Filter out extremely long body elements to keep metadata extremely readable
	delete(m, "description")
	delete(m, "claims")

	return MapToTOML(m, ""), nil
}

// MapToTOML recursively formats maps (from the JSON roundtrip) into clean TOML.
func MapToTOML(m map[string]any, indent string) string {
	var b strings.Builder
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// First print primitive attributes
	for _, k := range keys {
		v := m[k]
		if v == nil {
			continue
		}
		switch val := v.(type) {
		case map[string]any:
			// printed later
		case []any:
			isTableArray := false
			if len(val) > 0 {
				if _, ok := val[0].(map[string]any); ok {
					isTableArray = true
				}
			}
			if isTableArray {
				// printed later
			} else {
				b.WriteString(fmt.Sprintf("%s%s = %s\n", indent, k, formatTOMLValue(val)))
			}
		default:
			b.WriteString(fmt.Sprintf("%s%s = %s\n", indent, k, formatTOMLValue(val)))
		}
	}

	// Next print sub-tables and table arrays
	for _, k := range keys {
		v := m[k]
		if v == nil {
			continue
		}
		switch val := v.(type) {
		case map[string]any:
			if len(val) == 0 {
				continue
			}
			b.WriteString(fmt.Sprintf("\n%s[%s]\n", indent, k))
			b.WriteString(MapToTOML(val, indent+"  "))
		case []any:
			isTableArray := false
			if len(val) > 0 {
				if _, ok := val[0].(map[string]any); ok {
					isTableArray = true
				}
			}
			if isTableArray {
				for _, item := range val {
					if itemMap, ok := item.(map[string]any); ok {
						b.WriteString(fmt.Sprintf("\n%s[[%s]]\n", indent, k))
						b.WriteString(MapToTOML(itemMap, indent+"  "))
					}
				}
			}
		}
	}
	return b.String()
}

func formatTOMLValue(v any) string {
	if v == nil {
		return "nil"
	}
	switch val := v.(type) {
	case string:
		escaped := strings.ReplaceAll(val, "\"", "\\\"")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		return fmt.Sprintf("\"%s\"", escaped)
	case bool:
		return fmt.Sprintf("%t", val)
	case float64:
		return fmt.Sprintf("%g", val)
	case []any:
		var parts []string
		for _, item := range val {
			parts = append(parts, formatTOMLValue(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprintf("%v", val)
	}
}
