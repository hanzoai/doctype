// Package doctype is the Hanzo DocType metadata model: the pure VALUE layer of
// the Hanzo Framework engine, faithful to Frappe's DocType/DocField/DocPerm and
// rebuilt in Go with no Frappe/Python runtime.
//
// It is deliberately free of I/O. Everything here is computable from a DocType
// definition and an input value alone: the schema and its well-formedness rules,
// field-value coercion, the autoname engine, argon2id Password hashing, the
// permission calculus, and the app-lane fixture registry. Anything that must
// READ or WRITE tenant data — the store, Link/Unique verification, hooks,
// leases, role resolution — lives one layer up in github.com/hanzoai/framework.
//
// That line is what makes this package reusable: CMS content-types, ERPNext
// DocTypes, CRM and Helpdesk records are all just DocTypes, and a tool that only
// needs to READ or VALIDATE a schema (a code generator, a UI renderer, a
// migration linter) depends on this module alone and links no database.
//
//   - A DocType is a metadata definition: {name, module, fields[], permissions[],
//     isSingle, isSubmittable, autoname, titleField}. It is data.
//   - A DocField mirrors Frappe's DocField. The fieldtype set is closed and
//     validated at define time (Data, Int, Float, Currency, Check, Date,
//     Datetime, Text, SmallText, LongText, RichText, Select, Link, Table,
//     Attach, JSON, Password).
//   - A document is a schemaless record validated against its DocType at write.
//     docstatus is the Frappe lifecycle: 0=draft, 1=submitted, 2=cancelled.
package doctype

import (
	"fmt"
	"regexp"
	"strings"
)

// Fieldtype constants mirror Frappe's DocField.fieldtype. This is the closed set
// the engine validates; a DocField with any other fieldtype is rejected at
// define time (fail closed — an unknown type has no validation and is a hole).
const (
	FieldData     = "Data"
	FieldInt      = "Int"
	FieldFloat    = "Float"
	FieldCurrency = "Currency"
	FieldCheck    = "Check"
	FieldDate     = "Date"
	FieldDatetime = "Datetime"
	FieldText     = "Text"
	FieldSmall    = "SmallText"
	FieldLong     = "LongText"
	FieldRichText = "RichText" // WYSIWYG body; value is a Lexical EditorState JSON string
	FieldSelect   = "Select"
	FieldLink     = "Link"
	FieldTable    = "Table"
	FieldAttach   = "Attach"
	FieldJSON     = "JSON"
	FieldPassword = "Password"
)

// validFieldtypes is the membership set for the constants above.
var validFieldtypes = map[string]bool{
	FieldData: true, FieldInt: true, FieldFloat: true, FieldCurrency: true,
	FieldCheck: true, FieldDate: true, FieldDatetime: true, FieldText: true,
	FieldSmall: true, FieldLong: true, FieldRichText: true, FieldSelect: true, FieldLink: true,
	FieldTable: true, FieldAttach: true, FieldJSON: true, FieldPassword: true,
}

// needsOptions is the set of fieldtypes whose `options` names a target: Select
// (newline-separated choices), Link (target DocType), Table (child DocType).
var needsOptions = map[string]bool{FieldSelect: true, FieldLink: true, FieldTable: true}

// reservedDocTypeNames are names a DocType may NOT take because they collide with
// a static route segment under /v1/framework/. Rejecting them at define time is
// why the static routes can be registered before the generic /:doctype routes
// without ambiguity.
var reservedDocTypeNames = map[string]bool{
	"doctypes": true, "roles": true, "health": true, "summary": true, "modules": true,
	"changes": true, "stream": true, "presence": true,
}

// Limits. MaxFieldBytes bounds a single scalar text value; MaxDocBytes bounds a whole
// document body so an unbounded blob can't amplify the shared SQLite file;
// MaxFields / MaxChildRows bound a schema and a child table.
const (
	MaxFieldBytes     = 100_000 // 100 KB per scalar (LongText/JSON); Data fields are far smaller
	MaxNameLen        = 255
	MaxDocTypeNameLen = 140
	MaxDocBytes       = 4 << 20 // 4 MB per document body
	MaxFields         = 512
	MaxChildRows      = 5_000
	DefaultLimit      = 100
	MaxLimit          = 1000
)

// fieldnameRe is the safe identifier a fieldname must match. Fieldnames are used
// verbatim in SQLite json paths ('$.'||fieldname is BOUND, not interpolated, but
// this is defense-in-depth) and as JSON keys, so they are strict snake tokens.
var fieldnameRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// docTypeNameRe is the safe identifier a DocType or document name must match. It
// permits the Frappe-ish label set (letters, digits, space, dash, dot, underscore)
// so "Sales Invoice" or "INV-2026-0001" are legal, while excluding path/quote/
// control characters. Names are always BOUND parameters in SQL — never
// interpolated — so this is a well-formedness gate, not the injection defense.
var docTypeNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]*$`)

// DocField is one field in a DocType, faithful to Frappe's DocField. JSON tags
// are the wire contract the sibling app lanes (CMS/ERP/CRM) and the generic
// @hanzo/ui DocType renderer build to.
type DocField struct {
	Fieldname string `json:"fieldname"`
	Fieldtype string `json:"fieldtype"`
	Label     string `json:"label,omitempty"`
	Reqd      bool   `json:"reqd,omitempty"`
	// Options is fieldtype-dependent: Select → newline-separated choices; Link →
	// target DocType name; Table → child DocType name. Empty otherwise.
	Options string `json:"options,omitempty"`
	// Default is applied when a create omits the field (before validation).
	Default  string `json:"default,omitempty"`
	Unique   bool   `json:"unique,omitempty"`
	ReadOnly bool   `json:"readOnly,omitempty"`
	Hidden   bool   `json:"hidden,omitempty"`
	// InListView flags the field for the generic list UI (metadata only).
	InListView bool `json:"inListView,omitempty"`
	// FetchFrom auto-populates this field from a linked document, in the form
	// "link_fieldname.source_fieldname": on save the engine loads the doc named by
	// the Link field `link_fieldname` and copies its `source_fieldname` here.
	FetchFrom string `json:"fetchFrom,omitempty"`
}

// DocPerm is a role's rights on a DocType, faithful to Frappe's DocPerm. The
// engine enforces these per-org against the caller's resolved roles.
type DocPerm struct {
	Role   string `json:"role"`
	Read   bool   `json:"read,omitempty"`
	Write  bool   `json:"write,omitempty"`
	Create bool   `json:"create,omitempty"`
	Delete bool   `json:"delete,omitempty"`
	Submit bool   `json:"submit,omitempty"`
	Cancel bool   `json:"cancel,omitempty"`
}

// DocType is a metadata definition. It is per-org data: the same DocType `name`
// may exist independently in many orgs with different fields, and one org's
// definition is invisible to another.
type DocType struct {
	Name          string `json:"name"`
	Module        string `json:"module,omitempty"`
	IsSingle      bool   `json:"isSingle,omitempty"`
	IsSubmittable bool   `json:"isSubmittable,omitempty"`
	// Autoname is the naming rule (see naming.go): "" or "hash" → random id;
	// "field:fieldname" → value of that field; "prompt" → client supplies name;
	// any other value is a series pattern, e.g. "INV-.YYYY.-.#####".
	Autoname   string     `json:"autoname,omitempty"`
	TitleField string     `json:"titleField,omitempty"`
	Fields     []DocField `json:"fields"`
	Perms      []DocPerm  `json:"permissions,omitempty"`
	CreatedAt  int64      `json:"createdAt,omitempty"`
	UpdatedAt  int64      `json:"updatedAt,omitempty"`
}

// field returns the named DocField and whether it exists.
func (d *DocType) Field(name string) (DocField, bool) {
	for _, f := range d.Fields {
		if f.Fieldname == name {
			return f, true
		}
	}
	return DocField{}, false
}

// SelectChoices splits a Select field's newline-separated options into the
// allowed values (Frappe semantics: a leading blank line means "" is allowed).
func (f DocField) SelectChoices() []string {
	raw := strings.Split(f.Options, "\n")
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		out = append(out, strings.TrimRight(s, "\r"))
	}
	return out
}

// Validate checks a DocType definition is well-formed. A malformed schema is
// rejected at define time (400) so the document validator can trust it later —
// validation is done once, at the boundary, not re-litigated per document.
func (d *DocType) Validate() error {
	name := strings.TrimSpace(d.Name)
	if name == "" {
		return fmt.Errorf("doctype name is required")
	}
	if len(name) > MaxDocTypeNameLen {
		return fmt.Errorf("doctype name too long (max %d)", MaxDocTypeNameLen)
	}
	if !docTypeNameRe.MatchString(name) {
		return fmt.Errorf("doctype name %q has invalid characters", name)
	}
	if reservedDocTypeNames[strings.ToLower(name)] {
		return fmt.Errorf("doctype name %q is reserved", name)
	}
	if len(d.Fields) == 0 {
		return fmt.Errorf("doctype must declare at least one field")
	}
	if len(d.Fields) > MaxFields {
		return fmt.Errorf("too many fields (max %d)", MaxFields)
	}
	seen := make(map[string]bool, len(d.Fields))
	for i, f := range d.Fields {
		if !fieldnameRe.MatchString(f.Fieldname) {
			return fmt.Errorf("field %d: fieldname %q must match %s", i, f.Fieldname, fieldnameRe)
		}
		if seen[f.Fieldname] {
			return fmt.Errorf("duplicate fieldname %q", f.Fieldname)
		}
		seen[f.Fieldname] = true
		if !validFieldtypes[f.Fieldtype] {
			return fmt.Errorf("field %q: unknown fieldtype %q", f.Fieldname, f.Fieldtype)
		}
		if needsOptions[f.Fieldtype] && strings.TrimSpace(f.Options) == "" {
			return fmt.Errorf("field %q: fieldtype %s requires options", f.Fieldname, f.Fieldtype)
		}
		if f.FetchFrom != "" {
			src, _, ok := strings.Cut(f.FetchFrom, ".")
			if !ok || strings.TrimSpace(src) == "" {
				return fmt.Errorf("field %q: fetchFrom must be \"link_field.source_field\"", f.Fieldname)
			}
		}
	}
	// FetchFrom + autoname field: references must point at declared fields.
	if src := autonameField(d.Autoname); src != "" {
		if _, ok := d.Field(src); !ok {
			return fmt.Errorf("autoname field:%s references unknown field", src)
		}
	}
	if d.TitleField != "" {
		if _, ok := d.Field(d.TitleField); !ok {
			return fmt.Errorf("titleField %q is not a declared field", d.TitleField)
		}
	}
	for _, f := range d.Fields {
		if f.FetchFrom == "" {
			continue
		}
		link, _, _ := strings.Cut(f.FetchFrom, ".")
		lf, ok := d.Field(link)
		if !ok || lf.Fieldtype != FieldLink {
			return fmt.Errorf("field %q: fetchFrom source %q must be a Link field", f.Fieldname, link)
		}
	}
	for _, p := range d.Perms {
		if strings.TrimSpace(p.Role) == "" {
			return fmt.Errorf("permission with empty role")
		}
	}
	return nil
}

// Normalize trims the DocType's identifying strings in place and defaults empty
// slices so the stored/returned shape is canonical.
//
// SECURE BY DEFAULT: when no permissions are declared, seed the System Manager
// grant so a doctype is NEVER silently permless. A permless doctype is thus
// governed by the org owner (a System Manager) and closed to everyone else — the
// generic UI and audit always see an explicit permission set, and the enforcement
// (permission.can) is default-deny for a role-less member.
func (d *DocType) Normalize() {
	d.Name = strings.TrimSpace(d.Name)
	d.Module = strings.TrimSpace(d.Module)
	d.Autoname = strings.TrimSpace(d.Autoname)
	if d.Fields == nil {
		d.Fields = []DocField{}
	}
	if len(d.Perms) == 0 {
		d.Perms = []DocPerm{{
			Role: RoleSystemManager,
			Read: true, Write: true, Create: true, Delete: true, Submit: true, Cancel: true,
		}}
	}
}
