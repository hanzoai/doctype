package doctype

import "testing"

func permDT(perms ...DocPerm) *DocType {
	return &DocType{Name: "Sales Invoice", Perms: perms}
}

func TestGrants(t *testing.T) {
	dt := permDT(
		DocPerm{Role: "Accounts Manager", Read: true, Write: true, Create: true, Delete: true, Submit: true, Cancel: true},
		DocPerm{Role: "Accounts User", Read: true, Create: true},
	)
	mgr := map[string]bool{RoleAll: true, "Accounts Manager": true}
	usr := map[string]bool{RoleAll: true, "Accounts User": true}
	none := map[string]bool{RoleAll: true}

	for _, right := range []string{RightRead, RightWrite, RightCreate, RightDelete, RightSubmit, RightCancel} {
		if !Grants(dt, mgr, right) {
			t.Errorf("manager denied %q", right)
		}
		if Grants(dt, none, right) {
			t.Errorf("role-less member granted %q — default-deny broken", right)
		}
	}
	if !Grants(dt, usr, RightRead) || !Grants(dt, usr, RightCreate) {
		t.Error("user denied a right its role carries")
	}
	for _, right := range []string{RightWrite, RightDelete, RightSubmit, RightCancel} {
		if Grants(dt, usr, right) {
			t.Errorf("user granted %q, which its role does not carry", right)
		}
	}
}

// TestGrants_NoPermsIsClosed: the DENY default. There is no "empty perms means
// open" branch anywhere in the calculus.
func TestGrants_NoPermsIsClosed(t *testing.T) {
	dt := permDT()
	roles := map[string]bool{RoleAll: true, RoleSystemManager: true}
	for _, right := range []string{RightRead, RightWrite, RightCreate, RightDelete, RightSubmit, RightCancel} {
		if Grants(dt, roles, right) {
			t.Fatalf("permission-less doctype granted %q", right)
		}
	}
}

// TestGrants_UnionsAcrossRoles: holding two roles grants the union of their rights.
func TestGrants_UnionsAcrossRoles(t *testing.T) {
	dt := permDT(
		DocPerm{Role: "Reader", Read: true},
		DocPerm{Role: "Writer", Write: true},
	)
	both := map[string]bool{"Reader": true, "Writer": true}
	if !Grants(dt, both, RightRead) || !Grants(dt, both, RightWrite) {
		t.Fatal("union of two roles' rights not granted")
	}
	if Grants(dt, both, RightDelete) {
		t.Fatal("granted a right neither role carries")
	}
}

func TestGrants_UnknownRightAndNilDocType(t *testing.T) {
	dt := permDT(DocPerm{Role: "R", Read: true, Write: true, Create: true, Delete: true, Submit: true, Cancel: true})
	if Grants(dt, map[string]bool{"R": true}, "escalate") {
		t.Fatal("an unknown right must never be granted")
	}
	if Grants(nil, map[string]bool{"R": true}, RightRead) {
		t.Fatal("a nil DocType must never grant")
	}
}

// TestGrants_NoRolesNoAccess: an empty role set is denied regardless of perms.
func TestGrants_EmptyRoleSet(t *testing.T) {
	dt := permDT(DocPerm{Role: RoleAll, Read: true})
	if Grants(dt, nil, RightRead) {
		t.Fatal("a caller with no roles was granted read")
	}
}
