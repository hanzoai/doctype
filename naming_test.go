package doctype

import (
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)

func TestNamingMode(t *testing.T) {
	for _, tc := range []struct {
		autoname             string
		hash, prompt, series bool
		field                string
	}{
		{"", true, false, false, ""},
		{"hash", true, false, false, ""},
		{"HASH", true, false, false, ""},
		{"prompt", false, true, false, ""},
		{"field:code", false, false, false, "code"},
		{"INV-.YYYY.-.#####", false, false, true, ""},
	} {
		t.Run(tc.autoname, func(t *testing.T) {
			if got := isHashNaming(tc.autoname); got != tc.hash {
				t.Errorf("isHashNaming = %v, want %v", got, tc.hash)
			}
			if got := isPromptNaming(tc.autoname); got != tc.prompt {
				t.Errorf("isPromptNaming = %v, want %v", got, tc.prompt)
			}
			if got := IsSeries(tc.autoname); got != tc.series {
				t.Errorf("IsSeries = %v, want %v", got, tc.series)
			}
			if got := autonameField(tc.autoname); got != tc.field {
				t.Errorf("autonameField = %q, want %q", got, tc.field)
			}
		})
	}
}

// TestExpandSeries covers Frappe's make_autoname token grammar: dots delimit,
// YYYY/YY/MM/DD expand, a run of '#' marks the zero-padded counter, and a
// pattern with no '#' gets a default 5-digit counter appended.
func TestExpandSeries(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		n       int64
		want    string
	}{
		{"INV-.YYYY.-.#####", 1, "INV-2026-00001"},
		{"INV-.YYYY.-.#####", 42, "INV-2026-00042"},
		{".YY..MM..DD.-.###", 7, "260727-007"},
		{"MYDOC-", 1, "MYDOC-00001"},
		{"TICKET-.####.-X", 12, "TICKET-0012-X"},
	} {
		t.Run(tc.pattern, func(t *testing.T) {
			if got := ExpandSeries(tc.pattern, testNow).Format(tc.n); got != tc.want {
				t.Fatalf("Format(%d) = %q, want %q", tc.n, got, tc.want)
			}
		})
	}
}

// TestExpandSeries_KeyIsInvariantAcrossCounter: the counter key must depend only
// on the expanded prefix/suffix, so every document in the same period shares one
// counter (and a different period starts its own).
func TestExpandSeries_Key(t *testing.T) {
	a := ExpandSeries("INV-.YYYY.-.#####", testNow)
	b := ExpandSeries("INV-.YYYY.-.#####", testNow.AddDate(0, 1, 0))
	if a.Key != b.Key {
		t.Fatalf("same year must share a counter key: %q vs %q", a.Key, b.Key)
	}
	c := ExpandSeries("INV-.YYYY.-.#####", testNow.AddDate(1, 0, 0))
	if a.Key == c.Key {
		t.Fatal("a new year must start a new counter key")
	}
}

func TestResolveName_Hash(t *testing.T) {
	dt := &DocType{Autoname: "hash"}
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		n, err := ResolveName(dt, nil, "")
		if err != nil {
			t.Fatalf("hash naming: %v", err)
		}
		if len(n) != 32 {
			t.Fatalf("hash name %q is not 128-bit hex", n)
		}
		if seen[n] {
			t.Fatalf("hash name collision on %q", n)
		}
		seen[n] = true
	}
}

func TestResolveName_Field(t *testing.T) {
	dt := &DocType{Autoname: "field:code"}
	got, err := ResolveName(dt, map[string]any{"code": " INV-1 "}, "")
	if err != nil || got != "INV-1" {
		t.Fatalf("field naming = %q, %v", got, err)
	}
	if _, err := ResolveName(dt, map[string]any{"code": "  "}, ""); err == nil {
		t.Fatal("empty autoname field must be rejected")
	}
	if _, err := ResolveName(dt, map[string]any{"code": "bad/name"}, ""); err == nil {
		t.Fatal("invalid characters in an autoname field must be rejected")
	}
	if _, err := ResolveName(dt, map[string]any{"code": strings.Repeat("x", MaxNameLen+1)}, ""); err == nil {
		t.Fatal("over-long autoname field must be rejected")
	}
}

func TestResolveName_Prompt(t *testing.T) {
	dt := &DocType{Autoname: "prompt"}
	if got, err := ResolveName(dt, nil, " Sales Invoice "); err != nil || got != "Sales Invoice" {
		t.Fatalf("prompt naming = %q, %v", got, err)
	}
	if _, err := ResolveName(dt, nil, ""); err == nil {
		t.Fatal("prompt naming with no name must be rejected")
	}
	if _, err := ResolveName(dt, nil, "bad/name"); err == nil {
		t.Fatal("prompt naming must reject invalid characters")
	}
}

// TestResolveName_ErrorsAreValidationErrors: a naming refusal is a 400-class
// schema violation, not a store failure — the engine maps it that way.
func TestResolveName_ErrorsAreValidationErrors(t *testing.T) {
	_, err := ResolveName(&DocType{Autoname: "prompt"}, nil, "")
	if !IsValidationError(err) {
		t.Fatalf("naming refusal is not a ValidationError: %T", err)
	}
}
