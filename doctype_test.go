package doctype

import (
	"strings"
	"testing"
)

// ok is the minimal well-formed DocType the schema tests mutate.
func ok() DocType {
	return DocType{Name: "Sales Invoice", Fields: []DocField{
		{Fieldname: "customer", Fieldtype: FieldData, Reqd: true},
	}}
}

func TestValidate_AcceptsWellFormed(t *testing.T) {
	dt := ok()
	if err := dt.Validate(); err != nil {
		t.Fatalf("well-formed doctype rejected: %v", err)
	}
}

// TestValidate_RejectsMalformed covers every gate Validate is responsible for.
// A malformed schema MUST be refused at define time — the document validator
// trusts the schema afterwards, so a hole here is a hole everywhere.
func TestValidate_RejectsMalformed(t *testing.T) {
	for _, tc := range []struct {
		name string
		want string
		mut  func(*DocType)
	}{
		{"empty name", "name is required", func(d *DocType) { d.Name = "  " }},
		{"long name", "too long", func(d *DocType) { d.Name = strings.Repeat("x", MaxDocTypeNameLen+1) }},
		{"bad chars", "invalid characters", func(d *DocType) { d.Name = "Sales/Invoice" }},
		{"reserved", "is reserved", func(d *DocType) { d.Name = "doctypes" }},
		{"reserved cased", "is reserved", func(d *DocType) { d.Name = "Roles" }},
		{"reserved stream", "is reserved", func(d *DocType) { d.Name = "stream" }},
		{"reserved changes", "is reserved", func(d *DocType) { d.Name = "Changes" }},
		{"reserved presence", "is reserved", func(d *DocType) { d.Name = "presence" }},
		{"no fields", "at least one field", func(d *DocType) { d.Fields = nil }},
		{"bad fieldname", "must match", func(d *DocType) { d.Fields[0].Fieldname = "Customer" }},
		{"unknown fieldtype", "unknown fieldtype", func(d *DocType) { d.Fields[0].Fieldtype = "Blob" }},
		{"select needs options", "requires options", func(d *DocType) { d.Fields[0].Fieldtype = FieldSelect }},
		{"link needs options", "requires options", func(d *DocType) { d.Fields[0].Fieldtype = FieldLink }},
		{"dup fieldname", "duplicate fieldname", func(d *DocType) {
			d.Fields = append(d.Fields, DocField{Fieldname: "customer", Fieldtype: FieldData})
		}},
		{"autoname unknown field", "references unknown field", func(d *DocType) { d.Autoname = "field:nope" }},
		{"titleField unknown", "not a declared field", func(d *DocType) { d.TitleField = "nope" }},
		{"fetchFrom malformed", "must be \"link_field.source_field\"", func(d *DocType) {
			d.Fields = append(d.Fields, DocField{Fieldname: "region", Fieldtype: FieldData, FetchFrom: "customer"})
		}},
		{"fetchFrom non-link source", "must be a Link field", func(d *DocType) {
			d.Fields = append(d.Fields, DocField{Fieldname: "region", Fieldtype: FieldData, FetchFrom: "customer.region"})
		}},
		{"empty role", "empty role", func(d *DocType) { d.Perms = []DocPerm{{Role: " "}} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dt := ok()
			tc.mut(&dt)
			err := dt.Validate()
			if err == nil {
				t.Fatalf("expected rejection, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestValidate_TooManyFields(t *testing.T) {
	dt := ok()
	dt.Fields = make([]DocField, MaxFields+1)
	for i := range dt.Fields {
		dt.Fields[i] = DocField{Fieldname: "f", Fieldtype: FieldData}
	}
	if err := dt.Validate(); err == nil || !strings.Contains(err.Error(), "too many fields") {
		t.Fatalf("expected too-many-fields rejection, got %v", err)
	}
}

// TestNormalize_SeedsManagerGrant is the secure-by-default invariant: a DocType
// declared with NO permissions must never end up open to all — it is seeded a
// System Manager grant, so Grants denies every role-less member.
func TestNormalize_SeedsManagerGrant(t *testing.T) {
	dt := ok()
	dt.Normalize()
	if len(dt.Perms) != 1 || dt.Perms[0].Role != RoleSystemManager {
		t.Fatalf("permission-less doctype not seeded with System Manager: %+v", dt.Perms)
	}
	p := dt.Perms[0]
	if !(p.Read && p.Write && p.Create && p.Delete && p.Submit && p.Cancel) {
		t.Fatalf("seeded manager grant is not full: %+v", p)
	}
	// A role-less member gets nothing from it.
	if Grants(&dt, map[string]bool{RoleAll: true}, RightRead) {
		t.Fatal("permission-less doctype is readable by a role-less member — default-open hole")
	}
}

func TestNormalize_TrimsAndDefaults(t *testing.T) {
	dt := DocType{Name: "  Invoice  ", Module: " Accounts ", Autoname: " hash "}
	dt.Normalize()
	if dt.Name != "Invoice" || dt.Module != "Accounts" || dt.Autoname != "hash" {
		t.Fatalf("not trimmed: %q %q %q", dt.Name, dt.Module, dt.Autoname)
	}
	if dt.Fields == nil {
		t.Fatal("Fields must default to an empty slice, not nil")
	}
}

// TestNormalize_KeepsDeclaredPerms: seeding applies ONLY when none were declared.
func TestNormalize_KeepsDeclaredPerms(t *testing.T) {
	dt := ok()
	dt.Perms = []DocPerm{{Role: "Accounts User", Read: true}}
	dt.Normalize()
	if len(dt.Perms) != 1 || dt.Perms[0].Role != "Accounts User" {
		t.Fatalf("declared perms clobbered: %+v", dt.Perms)
	}
}

func TestField(t *testing.T) {
	dt := ok()
	if f, found := dt.Field("customer"); !found || f.Fieldtype != FieldData {
		t.Fatalf("Field(customer) = %+v %v", f, found)
	}
	if _, found := dt.Field("nope"); found {
		t.Fatal("Field returned a field that was never declared")
	}
}

// TestSelectChoices_LeadingBlankAllowsEmpty mirrors Frappe: a leading blank line
// in Select options means "" is a legal value.
func TestSelectChoices(t *testing.T) {
	f := DocField{Fieldtype: FieldSelect, Options: "\nDraft\nPaid\r"}
	got := f.SelectChoices()
	want := []string{"", "Draft", "Paid"}
	if len(got) != len(want) {
		t.Fatalf("choices = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("choice %d = %q, want %q", i, got[i], want[i])
		}
	}
}
