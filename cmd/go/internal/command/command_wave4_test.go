// Wave-4 coverage for the command registry: New, Register (including
// the overwrite path), Lookup, and Commands ordering.
package command

import (
	"io"
	"testing"
)

type fakeCmd struct {
	name     string
	synopsis string
	code     int
}

func (f *fakeCmd) Name() string     { return f.name }
func (f *fakeCmd) Synopsis() string { return f.synopsis }
func (f *fakeCmd) Run([]string, io.Reader, io.Writer, io.Writer) int {
	return f.code
}

func TestW4RegistryRegisterLookupOrder(t *testing.T) {
	r := New()

	if _, ok := r.Lookup("a"); ok {
		t.Error("Lookup on empty registry found something")
	}
	if got := r.Commands(); len(got) != 0 {
		t.Errorf("Commands on empty registry = %d entries, want 0", len(got))
	}

	a := &fakeCmd{name: "a", synopsis: "first", code: 1}
	b := &fakeCmd{name: "b", synopsis: "second", code: 2}
	r.Register(a)
	r.Register(b)

	got, ok := r.Lookup("a")
	if !ok || got.Synopsis() != "first" {
		t.Errorf("Lookup(a) = %v, %v", got, ok)
	}
	if _, ok := r.Lookup("zz"); ok {
		t.Error("Lookup(zz) found an unregistered command")
	}

	cmds := r.Commands()
	if len(cmds) != 2 || cmds[0].Name() != "a" || cmds[1].Name() != "b" {
		t.Fatalf("Commands order wrong: %v", cmds)
	}

	// Re-registering a name overwrites in place without duplicating the
	// order entry.
	a2 := &fakeCmd{name: "a", synopsis: "replaced", code: 3}
	r.Register(a2)
	cmds = r.Commands()
	if len(cmds) != 2 {
		t.Fatalf("re-register duplicated order: %d entries", len(cmds))
	}
	if cmds[0].Synopsis() != "replaced" {
		t.Errorf("re-register did not overwrite: %q", cmds[0].Synopsis())
	}
	if code := cmds[0].Run(nil, nil, io.Discard, io.Discard); code != 3 {
		t.Errorf("replaced command Run = %d, want 3", code)
	}
}
