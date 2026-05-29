package engine

import "testing"

func TestGrantNumberFromXMLName(t *testing.T) {
	cases := map[string]string{
		"10269843_06915216.xml": "6915216", // real USPTO grant XML name
		"10269843_06915216":     "6915216", // no extension
		"12945822_08126680.xml": "8126680", // leading zero stripped
		"justanapp.xml":         "",        // no underscore
		"10269843_.xml":         "",        // empty grant token
		"10269843_00000000.xml": "",        // all zeros
		"10269843_abc123.xml":   "",        // non-digit token
	}
	for name, want := range cases {
		if got := grantNumberFromXMLName(name); got != want {
			t.Errorf("grantNumberFromXMLName(%q) = %q, want %q", name, got, want)
		}
	}
}
