package doctype

// coerce.go is field-value coercion: the pure, DB-free half of document
// validation. Given a DocField and a raw JSON value it returns the canonical
// value to store, or a *ValidationError. Everything here is a function of the
// schema and the input alone.
//
// The half that needs to READ tenant data — Link existence, Unique, child
// tables, fetch-from — is Store.ValidateDoc in github.com/hanzoai/framework,
// which calls into this file for every scalar. Splitting on "does it touch the
// database" is what lets a renderer or linter validate field values with no
// store at all.

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ValidationError is a document-schema violation (400). It is distinct from the
// store sentinels (errBadRef → 422, errConflict → 409) so the HTTP layer answers
// the right code. Detect with errors.As.
type ValidationError struct{ msg string }

func (e *ValidationError) Error() string { return e.msg }

func Errorf(format string, args ...any) error {
	return &ValidationError{msg: fmt.Sprintf(format, args...)}
}

// Coerce converts a raw JSON value to the canonical stored value for a
// fieldtype, or returns a ValidationError. This is the pure (DB-free) core of
// field-type validation — the gate RED must not be able to bypass.
func Coerce(f DocField, raw any) (any, error) {
	switch f.Fieldtype {
	case FieldData, FieldText, FieldSmall, FieldLong, FieldRichText, FieldAttach:
		// RichText is an opaque Lexical EditorState JSON string — stored verbatim
		// (clipped to the scalar bound) like any text; its shape is a UI concern.
		s, err := asString(f, raw)
		if err != nil {
			return nil, err
		}
		return clip(s), nil

	case FieldSelect:
		s, err := asString(f, raw)
		if err != nil {
			return nil, err
		}
		s = strings.TrimSpace(s)
		for _, choice := range f.SelectChoices() {
			if s == strings.TrimSpace(choice) {
				return s, nil
			}
		}
		return nil, Errorf("field %q: %q is not an allowed option", f.Fieldname, s)

	case FieldLink:
		s, err := asString(f, raw)
		if err != nil {
			return nil, err
		}
		return strings.TrimSpace(s), nil

	case FieldInt:
		return asInt64(f, raw)

	case FieldFloat, FieldCurrency:
		return asFloat64(f, raw)

	case FieldCheck:
		return asCheck(f, raw)

	case FieldDate:
		return asDate(f, raw, "2006-01-02", "date (YYYY-MM-DD)")

	case FieldDatetime:
		return asDatetime(f, raw)

	case FieldJSON:
		// Stored as-is; it is already valid JSON (it was unmarshaled from the body).
		return raw, nil

	case FieldTable:
		return raw, nil // shape-validated by validateChildTable

	default:
		return nil, Errorf("field %q: unsupported fieldtype %q", f.Fieldname, f.Fieldtype)
	}
}

// ---- typed coercions ----

func asString(f DocField, raw any) (string, error) {
	s, ok := raw.(string)
	if !ok {
		return "", Errorf("field %q expects text", f.Fieldname)
	}
	return s, nil
}

func asInt64(f DocField, raw any) (int64, error) {
	switch v := raw.(type) {
	case float64:
		if v != float64(int64(v)) {
			return 0, Errorf("field %q expects an integer", f.Fieldname)
		}
		return int64(v), nil
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0, Errorf("field %q expects an integer", f.Fieldname)
		}
		return n, nil
	default:
		return 0, Errorf("field %q expects an integer", f.Fieldname)
	}
}

func asFloat64(f DocField, raw any) (float64, error) {
	switch v := raw.(type) {
	case float64:
		return v, nil
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0, Errorf("field %q expects a number", f.Fieldname)
		}
		return n, nil
	default:
		return 0, Errorf("field %q expects a number", f.Fieldname)
	}
}

// asCheck normalizes to 0/1. A Check is stored as an integer so json_extract
// filters compare cleanly.
func asCheck(f DocField, raw any) (int, error) {
	switch v := raw.(type) {
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case float64:
		if v == 0 {
			return 0, nil
		}
		return 1, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return 1, nil
		case "0", "false", "no", "off", "":
			return 0, nil
		}
	}
	return 0, Errorf("field %q expects a boolean/check", f.Fieldname)
}

func asDate(f DocField, raw any, layout, label string) (string, error) {
	s, err := asString(f, raw)
	if err != nil {
		return "", err
	}
	s = strings.TrimSpace(s)
	if _, err := time.Parse(layout, s); err != nil {
		return "", Errorf("field %q expects a %s", f.Fieldname, label)
	}
	return s, nil
}

// asDatetime accepts "YYYY-MM-DD HH:MM:SS" or RFC3339 and canonicalizes to the
// former.
func asDatetime(f DocField, raw any) (string, error) {
	s, err := asString(f, raw)
	if err != nil {
		return "", err
	}
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02 15:04:05"), nil
		}
	}
	return "", Errorf("field %q expects a datetime (YYYY-MM-DD HH:MM:SS)", f.Fieldname)
}

// PasswordValue reports the plaintext to hash for a Password field, and false
// when the input should PRESERVE the prior value (absent, empty, the redacted
// marker, or an already-hashed value — none of which is a new plaintext).
func PasswordValue(raw any, present bool) (string, bool) {
	if !present {
		return "", false
	}
	s, ok := raw.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" || s == RedactedMarker || IsHashed(s) {
		return "", false
	}
	return s, true
}

// IsEmptyInput reports whether a raw input is absent or an empty string.
func IsEmptyInput(raw any, present bool) bool {
	if !present {
		return true
	}
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return false
}

// clip trims and bounds a text value to MaxFieldBytes.
func clip(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > MaxFieldBytes {
		return s[:MaxFieldBytes]
	}
	return s
}

// IsValidationError reports whether err is a document-schema violation — the
// 400-class error Coerce and Store.ValidateDoc return. An in-process caller uses
// it to answer Bad Request instead of a 500, without string-matching.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
