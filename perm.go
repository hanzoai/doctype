package doctype

// perm.go is the permission CALCULUS: given a DocType's declared permission rows,
// a caller's effective role set, and a right, does the right hold? That question
// is a pure function of values, so it lives here with the schema.
//
// What is NOT here is role RESOLUTION — reading which roles a user holds in an
// org, the trust-on-first-use owner seed, and the SuperAdmin escape hatch. Those
// read tenant state and belong to the engine (github.com/hanzoai/framework),
// which calls Grants once it knows the caller's roles. One calculus, one
// resolver, no second copy of either.

const (
	// RoleSystemManager is the admin role: it manages DocTypes + role assignments
	// and is granted every document right. Mirrors Frappe's System Manager. It is
	// also the grant Normalize seeds onto a permission-less DocType, which is why
	// the constant lives with the schema rather than with the resolver.
	RoleSystemManager = "System Manager"
	// RoleAll is the implicit role every validated org member holds (Frappe "All").
	RoleAll = "All"
)

// The rights a DocPerm can carry. These are the values the engine gates each
// operation on; they are part of the schema contract, not the transport.
const (
	RightRead   = "read"
	RightWrite  = "write"
	RightCreate = "create"
	RightDelete = "delete"
	RightSubmit = "submit"
	RightCancel = "cancel"
)

// Grants reports whether any role in `roles` carries `right` on dt.
//
// SECURE BY DEFAULT: there is NO "empty perms means open to all" branch. A
// DocType with no matching grant is closed — the DENY default. (Normalize seeds
// a System Manager grant at define time so a stored DocType is never silently
// permission-less; this loop is the enforcement, and it fails closed for a
// role-less member regardless.)
//
// It deliberately knows nothing about managers or SuperAdmins: an engine that
// has decided the caller is a manager never asks this question. Keeping the
// override out of the calculus is what makes the calculus auditable.
func Grants(dt *DocType, roles map[string]bool, right string) bool {
	if dt == nil {
		return false
	}
	for _, p := range dt.Perms {
		if !roles[p.Role] {
			continue
		}
		if grants(p, right) {
			return true
		}
	}
	return false
}

func grants(p DocPerm, right string) bool {
	switch right {
	case RightRead:
		return p.Read
	case RightWrite:
		return p.Write
	case RightCreate:
		return p.Create
	case RightDelete:
		return p.Delete
	case RightSubmit:
		return p.Submit
	case RightCancel:
		return p.Cancel
	default:
		return false
	}
}
