# DocType

The metadata model behind the Hanzo Framework, in pure Go: Frappe's
DocType / DocField / DocPerm rebuilt with no Frappe or Python runtime, and no I/O of any
kind.

Everything here is computable from a DocType definition and an input value alone —
schema, coercion, naming, the permission calculus. Anything that has to read or write
tenant data lives one layer up, in
[`hanzoai/framework`](https://github.com/hanzoai/framework).

## Install

```bash
go get github.com/hanzoai/doctype
```

## Why it is its own module

CMS content types, ERP DocTypes, CRM records and helpdesk tickets are all just DocTypes. A
tool that only needs to read or validate a schema — a code generator, a UI renderer, a
migration linter, an app declaring its content model — depends on this module and links no
database. That is the whole reason for the split.

## Docs

[`LLM.md`](LLM.md) has the file-by-file responsibilities and the rules that apply when
changing them. The Go doc comments on each type are the reference.

## License

See [LICENSE](LICENSE).
