# hanzoai/doctype — the DocType metadata model

`github.com/hanzoai/doctype` is the **value layer** of the Hanzo Framework
engine: Frappe's DocType/DocField/DocPerm model rebuilt in Go, with **no
Frappe/Python runtime** and **no I/O of any kind**.

Everything here is computable from a DocType definition and an input value
alone. Anything that must READ or WRITE tenant data lives one layer up in
`github.com/hanzoai/framework`.

## Why it is its own module

CMS content-types, ERPNext DocTypes, CRM records and Helpdesk tickets are all
just DocTypes. A tool that only needs to READ or VALIDATE a schema — a code
generator, a `@hanzo/ui` renderer, a migration linter, an app lane declaring its
content model — depends on this module and **links no database**. That is the
whole reason the split exists.

## Files

| File | Responsibility |
|------|----------------|
| `doctype.go` | `DocType`/`DocField`/`DocPerm`, the closed fieldtype set, limits, schema `Validate()` + `Normalize()` |
| `coerce.go`  | field-value coercion for every fieldtype → the canonical stored value, or a `*ValidationError` |
| `naming.go`  | the autoname engine: hash / `field:x` / prompt / series patterns, and `ResolveName` |
| `secret.go`  | argon2id hashing for Password fields (`HashPassword`/`VerifyPassword`/`IsHashed`) |
| `perm.go`    | the permission calculus: `Grants(dt, roles, right)`, role and right constants |
| `module.go`  | the app-lane fixture registry (`RegisterModule` / `MarkAlwaysOn` / `AlwaysOn`) |

## The split line

The rule is exactly **"does answering this question require the database?"**

- `doctype.Coerce(field, value)` — pure. Is `"2026-13-45"` a valid Date? No DB.
- `Store.validateDoc` (in `framework`) — impure. Does this Link point at a
  record that exists **in this org**? Needs the DB.

Both halves used to live in one `validate.go`; the seam is that question.

Two consequences worth knowing:

- `Grants` deliberately knows nothing about managers or SuperAdmins. An engine
  that has already decided the caller is a manager never asks. Keeping the
  override out of the calculus is what makes the calculus auditable.
- `Normalize()` seeds a `System Manager` grant onto a permission-less DocType,
  so **there is no "empty perms means open to all" path anywhere**. `Grants`
  fails closed for a role-less member regardless.

## Fieldtypes

`Data` `Text` `SmallText` `LongText` `RichText` `Int` `Float` `Currency`
`Check` `Date` `Datetime` `Select` `Link` `Table` `Attach` `JSON` `Password`.

The set is CLOSED: a DocField with any other fieldtype is rejected at define
time, because an unknown type has no validation and is therefore a hole.

`Password` is a **write-only credential**: hashed with argon2id on write, and
the engine returns the fixed marker `__set__` on read — never the plaintext,
never the hash. A DocType that needs a *retrievable* secret must use KMS, not a
Password field.

## Naming

- `""` / `"hash"` → random 128-bit hex id (the default)
- `"field:fieldname"` → the value of that field
- `"prompt"` → the client supplies the name
- anything else → a series pattern, e.g. `INV-.YYYY.-.#####` → `INV-2026-00001`

`ResolveName` handles every mode EXCEPT series, which is the one rule needing a
durable per-org counter — the engine allocates that inside the create
transaction.

## Compatibility

Go module rules apply: this stays at `v0.x`/`v1.x` forever. Never `v2`.
