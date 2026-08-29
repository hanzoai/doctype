package doctype

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// id.go is the DocType's identity: the pair (module, name), and the one string
// that renders it.
//
// A DocType's name is unique WITHIN ITS MODULE and nowhere else. "page" is the
// knowledge lane's wiki page or the CMS's marketing page depending on which
// module is asking, and nothing but the pair says which. Keying anything on the
// name alone therefore has no answer — which is why lanes used to write the
// module into the name ("kb-page") and store it twice.
//
// The rendering is module.name, and it is TOTAL: neither half may contain a dot
// (moduleRe, docTypeNameRe), so Parse is String's exact inverse and a round trip
// cannot lose or invent a boundary. That one dot is also what keeps a document
// address out of the static routes' namespace — "doctypes", "modules" and
// "summary" carry none, so no address can ever be mistaken for one.

// ID addresses one DocType.
type ID struct {
	Module string
	Name   string
}

// String renders the pair as module.name — the address. It is the form a URL
// segment, a Link/Table target and a stored document's doctype key all carry,
// and the only way to write one.
func (id ID) String() string { return id.Module + "." + id.Name }

// Zero reports that the ID names nothing. The zero ID is what an unresolved
// caller is handed, and every lookup refuses it.
func (id ID) Zero() bool { return id.Module == "" && id.Name == "" }

// Validate checks both halves are well-formed identifiers. A malformed address
// is rejected at the boundary, so nothing downstream re-litigates it.
func (id ID) Validate() error {
	if !moduleRe.MatchString(id.Module) {
		return fmt.Errorf("module %q has invalid characters", id.Module)
	}
	if len(id.Name) > MaxDocTypeNameLen {
		return fmt.Errorf("doctype name too long (max %d)", MaxDocTypeNameLen)
	}
	if !docTypeNameRe.MatchString(id.Name) {
		return fmt.Errorf("doctype name %q has invalid characters", id.Name)
	}
	return nil
}

// ParseID reads an address. The separator is the ONLY dot: a module name carries
// none and a doctype name carries none, so a second dot is a malformed address
// rather than a name to be split differently.
func ParseID(s string) (ID, error) {
	module, name, ok := strings.Cut(strings.TrimSpace(s), ".")
	if !ok {
		return ID{}, fmt.Errorf("doctype address %q is not module.name", s)
	}
	id := ID{Module: strings.TrimSpace(module), Name: strings.TrimSpace(name)}
	if err := id.Validate(); err != nil {
		return ID{}, err
	}
	return id, nil
}

// ID is this DocType's address.
func (d *DocType) ID() ID { return ID{Module: d.Module, Name: d.Name} }

// MarshalJSON writes the address, so an ID crosses the wire as the same string a
// URL carries rather than as an object nothing else spells that way.
func (id ID) MarshalJSON() ([]byte, error) { return json.Marshal(id.String()) }

// UnmarshalJSON reads an address.
func (id *ID) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	got, err := ParseID(s)
	if err != nil {
		return err
	}
	*id = got
	return nil
}

// moduleRe is the identifier a module name must match. A module namespaces every
// DocType it declares and is the first half of every address, so it carries no
// dot and no space — unlike a doctype name, which may carry a space ("Sales
// Invoice") because it is the second half and the split has already happened.
var moduleRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
