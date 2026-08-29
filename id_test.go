package doctype

import (
	"encoding/json"
	"testing"
)

// TestParseIsTheInverseOfString is the property the whole address rests on: a
// rendered ID reads back as itself. It holds because neither half may carry the
// separator, so there is never a second place the split could have gone.
func TestParseIsTheInverseOfString(t *testing.T) {
	for _, id := range []ID{
		{Module: "kb", Name: "page"},
		{Module: "help", Name: "ticket"},
		{Module: "erp", Name: "stock-ledger"},
		{Module: "Projects", Name: "Sales Invoice"},
		{Module: "cms", Name: "Page"},
	} {
		got, err := ParseID(id.String())
		if err != nil {
			t.Fatalf("ParseID(%q): %v", id, err)
		}
		if got != id {
			t.Fatalf("round trip %q → %#v", id, got)
		}
	}
}

func TestAddressForm(t *testing.T) {
	if got := (ID{Module: "kb", Name: "page"}).String(); got != "kb.page" {
		t.Fatalf("address = %q, want kb.page", got)
	}
}

func TestParseRejectsMalformed(t *testing.T) {
	for _, s := range []string{
		"page",          // no module: the ambiguity the pair exists to remove
		"",              //
		".page",         // empty module
		"kb.",           // empty name
		"kb.page.extra", // a name may not carry the separator
		"a b.page",      // space in the module
		"kb.pa/ge",      // path character
		"summary",       // a static route segment is not an address, and cannot be
	} {
		if id, err := ParseID(s); err == nil {
			t.Fatalf("ParseID(%q) accepted → %#v", s, id)
		}
	}
}

// TestNoAddressCollidesWithAStaticSegment is why the reserved-name list is gone.
// Every document address carries a dot and no static segment does, so a DocType
// can no longer be named into a route that already exists.
func TestNoAddressCollidesWithAStaticSegment(t *testing.T) {
	for _, seg := range []string{"doctypes", "modules", "summary", "roles", "health"} {
		if _, err := ParseID(seg); err == nil {
			t.Fatalf("static segment %q parsed as an address", seg)
		}
		// And a DocType may legally be NAMED that, because its address is not.
		dt := DocType{Name: seg, Module: "kb", Fields: []DocField{{Fieldname: "a", Fieldtype: FieldData}}}
		if err := dt.Validate(); err != nil {
			t.Fatalf("DocType named %q rejected: %v", seg, err)
		}
		if got := dt.ID().String(); got == seg {
			t.Fatalf("address %q collides with a static segment", got)
		}
	}
}

// TestIDCrossesTheWireAsItsAddress: an ID is one string everywhere, never an
// object in JSON and a pair in Go.
func TestIDCrossesTheWireAsItsAddress(t *testing.T) {
	b, err := json.Marshal(ID{Module: "kb", Name: "page"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"kb.page"` {
		t.Fatalf("marshalled to %s, want \"kb.page\"", b)
	}
	var got ID
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if (got != ID{Module: "kb", Name: "page"}) {
		t.Fatalf("unmarshalled to %#v", got)
	}
	if err := json.Unmarshal([]byte(`"page"`), &got); err == nil {
		t.Fatal("an unqualified address unmarshalled")
	}
}

func TestZero(t *testing.T) {
	if !(ID{}).Zero() {
		t.Fatal("the zero ID does not report itself zero")
	}
	if (ID{Module: "kb"}).Zero() {
		t.Fatal("a half-filled ID reports itself zero")
	}
}
