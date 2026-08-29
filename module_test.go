package doctype

import "testing"

func lane() []DocType {
	return []DocType{
		{Name: "Article", TitleField: "title", Fields: []DocField{
			{Fieldname: "title", Fieldtype: FieldData, Reqd: true},
		}},
		{Name: "Category", Fields: []DocField{{Fieldname: "label", Fieldtype: FieldData}}},
	}
}

func withLane(t *testing.T, module string, fx []DocType) {
	t.Helper()
	ResetModules()
	RegisterModule(module, fx)
	t.Cleanup(ResetModules)
}

func TestRegisterModule(t *testing.T) {
	withLane(t, "cms", lane())
	fx := Fixtures("cms")
	if len(fx) != 2 {
		t.Fatalf("Fixtures(cms) = %d fixtures, want 2", len(fx))
	}
	if got := RegisteredModules(); len(got) != 1 || got[0] != "cms" {
		t.Fatalf("RegisteredModules = %v", got)
	}
	if Fixtures("nope") != nil {
		t.Fatal("Fixtures of an unregistered module must be nil")
	}
}

func TestRegisterModule_IgnoresEmpty(t *testing.T) {
	ResetModules()
	t.Cleanup(ResetModules)
	RegisterModule("", lane())
	RegisterModule("cms", nil)
	if got := RegisteredModules(); len(got) != 0 {
		t.Fatalf("empty registrations were recorded: %v", got)
	}
}

// TestRegisterModule_ClonesInput: the caller's slice must not be a live handle
// into the process-global registry.
func TestRegisterModule_ClonesInput(t *testing.T) {
	fx := lane()
	withLane(t, "cms", fx)
	fx[0].Name = "MUTATED"
	if got := Fixtures("cms"); got[0].Name != "Article" {
		t.Fatalf("registry mutated through the caller's slice: %q", got[0].Name)
	}
}

func TestRegisteredModules_Sorted(t *testing.T) {
	ResetModules()
	t.Cleanup(ResetModules)
	for _, m := range []string{"zoo", "cms", "erp"} {
		RegisterModule(m, lane())
	}
	got := RegisteredModules()
	want := []string{"cms", "erp", "zoo"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("RegisteredModules = %v, want %v", got, want)
		}
	}
}

func TestAlwaysOn(t *testing.T) {
	withLane(t, "guide", lane())
	if _, ok := AlwaysOn(ID{Module: "guide", Name: "Article"}); ok {
		t.Fatal("a module resolves always-on before MarkAlwaysOn")
	}
	MarkAlwaysOn("guide")
	dt, ok := AlwaysOn(ID{Module: "guide", Name: "Article"})
	if !ok {
		t.Fatal("always-on fixture did not resolve")
	}
	// Resolved fixtures are module-stamped and normalized (perms seeded).
	if dt.Module != "guide" {
		t.Fatalf("fixture not stamped with its module: %q", dt.Module)
	}
	if len(dt.Perms) != 1 || dt.Perms[0].Role != RoleSystemManager {
		t.Fatalf("resolved fixture not normalized: %+v", dt.Perms)
	}
	if _, ok := AlwaysOn(ID{Module: "guide", Name: "Nope"}); ok {
		t.Fatal("resolved a fixture that was never registered")
	}
	if got := AlwaysOnModules(); len(got) != 1 || got[0] != "guide" {
		t.Fatalf("AlwaysOnModules = %v", got)
	}
}

// TestAlwaysOnAll_MatchesAlwaysOn: listing and resolution must agree, or a
// fresh org gets a DocType that resolves but never lists.
func TestAlwaysOnAll_MatchesAlwaysOn(t *testing.T) {
	withLane(t, "guide", lane())
	MarkAlwaysOn("guide")
	all := AlwaysOnAll()
	if len(all) != 2 {
		t.Fatalf("AlwaysOnAll = %d, want 2", len(all))
	}
	for _, dt := range all {
		if _, ok := AlwaysOn(dt.ID()); !ok {
			t.Fatalf("%q lists but does not resolve", dt.Name)
		}
	}
}

// TestAlwaysOn_ResolvedFixtureCannotCorruptRegistry is the cross-tenant
// invariant: an org mutating the DocType it resolved must not change what the
// next org resolves.
func TestAlwaysOn_ResolvedFixtureIsIndependent(t *testing.T) {
	withLane(t, "guide", lane())
	MarkAlwaysOn("guide")

	a, _ := AlwaysOn(ID{Module: "guide", Name: "Article"})
	a.Name = "HIJACKED"
	a.Fields[0].Fieldname = "hijacked"
	a.Perms[0].Role = "Hijacker"

	b, ok := AlwaysOn(ID{Module: "guide", Name: "Article"})
	if !ok || b.Name != "Article" {
		t.Fatalf("registry name corrupted: %q", b.Name)
	}
	if b.Fields[0].Fieldname != "title" {
		t.Fatalf("registry fields corrupted across orgs: %q", b.Fields[0].Fieldname)
	}
	if b.Perms[0].Role != RoleSystemManager {
		t.Fatalf("registry perms corrupted across orgs: %q", b.Perms[0].Role)
	}
}

func TestMarkAlwaysOn_IgnoresEmpty(t *testing.T) {
	ResetModules()
	t.Cleanup(ResetModules)
	MarkAlwaysOn("")
	if got := AlwaysOnModules(); len(got) != 0 {
		t.Fatalf("empty module marked always-on: %v", got)
	}
}

// TestAlwaysOn_SameNameDifferentModules: two lanes may both declare a "Page",
// and each resolves its OWN. Asking by name alone had to break that tie by
// module order, which handed one lane the other lane's schema.
func TestAlwaysOn_SameNameDifferentModules(t *testing.T) {
	ResetModules()
	t.Cleanup(ResetModules)
	RegisterModule("zoo", []DocType{{Name: "Page", Fields: []DocField{{Fieldname: "a", Fieldtype: FieldData}}}})
	RegisterModule("cms", []DocType{{Name: "Page", Fields: []DocField{{Fieldname: "b", Fieldtype: FieldData}}}})
	MarkAlwaysOn("zoo")
	MarkAlwaysOn("cms")
	for _, want := range []struct{ module, field string }{{"zoo", "a"}, {"cms", "b"}} {
		dt, ok := AlwaysOn(ID{Module: want.module, Name: "Page"})
		if !ok || dt.Module != want.module || dt.Fields[0].Fieldname != want.field {
			t.Fatalf("%s.Page resolved to module=%q field=%q ok=%v",
				want.module, dt.Module, dt.Fields[0].Fieldname, ok)
		}
	}
	// A module that declares no such fixture answers no, rather than borrowing
	// somebody else's.
	if _, ok := AlwaysOn(ID{Module: "erp", Name: "Page"}); ok {
		t.Fatal("erp.Page resolved from another module's fixtures")
	}
}
