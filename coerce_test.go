package doctype

import (
	"strings"
	"testing"
)

func f(ft string, opts ...string) DocField {
	d := DocField{Fieldname: "x", Fieldtype: ft}
	if len(opts) > 0 {
		d.Options = opts[0]
	}
	return d
}

// TestCoerce_Accepts pins the canonical stored value for every fieldtype — the
// shape the store persists and every reader downstream relies on.
func TestCoerce_Accepts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field DocField
		in    any
		want  any
	}{
		{"data trims", f(FieldData), "  hi  ", "hi"},
		{"text", f(FieldText), "body", "body"},
		{"richtext opaque", f(FieldRichText), `{"root":{}}`, `{"root":{}}`},
		{"attach", f(FieldAttach), "s3://x", "s3://x"},
		{"int from number", f(FieldInt), float64(7), int64(7)},
		{"int from string", f(FieldInt), " 7 ", int64(7)},
		{"float", f(FieldFloat), float64(1.5), 1.5},
		{"currency from string", f(FieldCurrency), "2.25", 2.25},
		{"check true", f(FieldCheck), true, 1},
		{"check false", f(FieldCheck), false, 0},
		{"check numeric", f(FieldCheck), float64(3), 1},
		{"check string yes", f(FieldCheck), "yes", 1},
		{"check string off", f(FieldCheck), "off", 0},
		{"date", f(FieldDate), "2026-07-27", "2026-07-27"},
		{"datetime canonical", f(FieldDatetime), "2026-07-27 10:30:00", "2026-07-27 10:30:00"},
		{"datetime rfc3339", f(FieldDatetime), "2026-07-27T10:30:00Z", "2026-07-27 10:30:00"},
		{"select", f(FieldSelect, "Draft\nPaid"), "Paid", "Paid"},
		{"link trims", f(FieldLink, "Customer"), " ACME ", "ACME"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Coerce(tc.field, tc.in)
			if err != nil {
				t.Fatalf("Coerce(%v) errored: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("Coerce(%v) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

func TestCoerce_Rejects(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field DocField
		in    any
	}{
		{"data not text", f(FieldData), float64(1)},
		{"int fractional", f(FieldInt), 1.5},
		{"int garbage", f(FieldInt), "abc"},
		{"int wrong type", f(FieldInt), true},
		{"float garbage", f(FieldFloat), "abc"},
		{"check garbage", f(FieldCheck), "maybe"},
		{"date garbage", f(FieldDate), "27/07/2026"},
		{"datetime garbage", f(FieldDatetime), "not-a-time"},
		{"select not an option", f(FieldSelect, "Draft\nPaid"), "Void"},
		{"unknown fieldtype", f("Blob"), "x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Coerce(tc.field, tc.in)
			if err == nil {
				t.Fatalf("Coerce(%v) accepted %#v, want rejection", tc.in, got)
			}
			if !IsValidationError(err) {
				t.Fatalf("rejection is not a ValidationError: %T", err)
			}
		})
	}
}

// TestCoerce_ClipsOversizeText: a scalar is bounded so an unbounded blob can't
// amplify the shared store file.
func TestCoerce_ClipsOversizeText(t *testing.T) {
	got, err := Coerce(f(FieldLong), strings.Repeat("a", MaxFieldBytes+500))
	if err != nil {
		t.Fatalf("oversize text errored: %v", err)
	}
	if len(got.(string)) != MaxFieldBytes {
		t.Fatalf("clipped to %d, want %d", len(got.(string)), MaxFieldBytes)
	}
}

// TestCoerce_JSONAndTablePassThrough: JSON is stored as-is; Table shape is
// validated by the engine against the child DocType, not here.
func TestCoerce_PassThrough(t *testing.T) {
	obj := map[string]any{"a": 1}
	if got, err := Coerce(f(FieldJSON), obj); err != nil || got == nil {
		t.Fatalf("JSON passthrough = %v, %v", got, err)
	}
	arr := []any{map[string]any{"x": 1}}
	if got, err := Coerce(f(FieldTable, "Item"), arr); err != nil || got == nil {
		t.Fatalf("Table passthrough = %v, %v", got, err)
	}
}

// TestPasswordValue is the write-only-credential contract: a NEW plaintext is
// hashed; absent / empty / the redaction marker / an already-hashed value all
// mean PRESERVE the prior value and must never be written as a plaintext.
func TestPasswordValue(t *testing.T) {
	hashed, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	for _, tc := range []struct {
		name    string
		raw     any
		present bool
		want    string
		ok      bool
	}{
		{"new plaintext", "hunter2", true, "hunter2", true},
		{"trimmed", "  hunter2  ", true, "hunter2", true},
		{"absent", nil, false, "", false},
		{"empty", "", true, "", false},
		{"blank", "   ", true, "", false},
		{"redaction marker", RedactedMarker, true, "", false},
		{"already hashed", hashed, true, "", false},
		{"not a string", float64(1), true, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := PasswordValue(tc.raw, tc.present)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("PasswordValue = (%q,%v), want (%q,%v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestIsEmptyInput(t *testing.T) {
	for _, tc := range []struct {
		raw     any
		present bool
		want    bool
	}{
		{nil, false, true},
		{"", true, true},
		{"  ", true, true},
		{"x", true, false},
		{float64(0), true, false}, // a zero NUMBER is a value, not absence
		{false, true, false},      // an unchecked Check is a value
	} {
		if got := IsEmptyInput(tc.raw, tc.present); got != tc.want {
			t.Fatalf("IsEmptyInput(%#v,%v) = %v, want %v", tc.raw, tc.present, got, tc.want)
		}
	}
}
