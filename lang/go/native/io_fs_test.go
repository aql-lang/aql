package native

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aql-lang/aql/lang/go/capabilities"
)

// ioFSReg builds a registry with the io words under bare names and an
// in-memory FS, returning the registry and the MemFileOps for setup/assert.
func ioFSReg(t *testing.T) (*Registry, *capabilities.MemFileOps) {
	t.Helper()
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registerIOWords(r)
	mem := setupMemFS(t, r)
	return r, mem
}

func statField(t *testing.T, r *Registry, path string) ReadMap {
	t.Helper()
	res := runAQL(t, r, []Value{NewWord("stat"), NewString(path)})
	if len(res) != 1 {
		t.Fatalf("stat %q: expected 1 result, got %d", path, len(res))
	}
	m, err := AsMap(res[0])
	if err != nil || m == nil {
		t.Fatalf("stat %q: not a record: %v (%v)", path, res[0], err)
	}
	return m
}

func TestStatFileAndDir(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("d/a.txt", []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	m := statField(t, r, "d/a.txt")
	if name, _ := m.Get("name"); name.String() != "'a.txt'" {
		t.Errorf("name = %v", name)
	}
	if size, _ := m.Get("size"); size.String() != "5" {
		t.Errorf("size = %v", size)
	}
	typ, _ := m.Get("type")
	if a, _ := typ.AsConcreteAtom(); a != "file" {
		t.Errorf("type = %v, want file atom", typ)
	}
	if mode, _ := m.Get("mode"); mode.String() != "420" { // 0644
		t.Errorf("mode = %v, want 420", mode)
	}

	dm := statField(t, r, "d")
	dtyp, _ := dm.Get("type")
	if a, _ := dtyp.AsConcreteAtom(); a != "dir" {
		t.Errorf("dir type = %v, want dir atom", dtyp)
	}
}

func TestStatAbsentReturnsNone(t *testing.T) {
	r, _ := ioFSReg(t)
	res := runAQL(t, r, []Value{NewWord("stat"), NewString("ghost")})
	if len(res) != 1 || !res[0].Is(TNone) {
		t.Fatalf("stat of absent path: expected none, got %v", res)
	}
}

func TestStatSymlinkAndFollow(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("t.txt", []byte("yo"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := mem.Symlink("t.txt", "link"); err != nil {
		t.Fatal(err)
	}

	// Default follow=true dereferences: type is the target's (file).
	fm := statField(t, r, "link")
	ftyp, _ := fm.Get("type")
	if a, _ := ftyp.AsConcreteAtom(); a != "file" {
		t.Errorf("followed symlink type = %v, want file", ftyp)
	}

	// {follow:false} describes the link itself and exposes .target.
	res := runAQL(t, r, []Value{
		NewWord("stat"), NewString("link"),
		wrapMap(func(om *OrderedMap) { om.Set("follow", NewBoolean(false)) }),
	})
	lm, err := AsMap(res[0])
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	ltyp, _ := lm.Get("type")
	if a, _ := ltyp.AsConcreteAtom(); a != "symlink" {
		t.Errorf("lstat type = %v, want symlink", ltyp)
	}
	if tgt, _ := lm.Get("target"); tgt.String() != "'t.txt'" {
		t.Errorf("lstat target = %v", tgt)
	}
}

func TestStatResolveOption(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("a.txt", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	res := runAQL(t, r, []Value{
		NewWord("stat"), NewString("a.txt"),
		wrapMap(func(om *OrderedMap) { om.Set("resolve", NewBoolean(true)) }),
	})
	m, err := AsMap(res[0])
	if err != nil {
		t.Fatal(err)
	}
	// mem ResolvePath cleans a relative path to itself; the field is present.
	if p, ok := m.Get("path"); !ok || p.String() != "'a.txt'" {
		t.Errorf("resolved path = %v", p)
	}
}

func TestStatCycleErrors(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.Symlink("cyc", "cyc"); err != nil { // self-referential
		t.Fatal(err)
	}
	if err := runAQLError(t, r, []Value{NewWord("stat"), NewString("cyc")}); err == nil {
		t.Error("expected stat error on a symlink cycle")
	}
}

func TestStatPathonTarget(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("p.txt", []byte("z"), 0644); err != nil {
		t.Fatal(err)
	}
	res := runAQL(t, r, []Value{NewWord("stat"), NewPathon([]string{"p.txt"}, false)})
	if _, err := AsMap(res[0]); err != nil {
		t.Fatalf("stat of Pathon target: %v", err)
	}
}

func TestListNamesDetailRecursive(t *testing.T) {
	r, mem := ioFSReg(t)
	for _, p := range []string{"root/a.txt", "root/b.txt", "root/sub/c.txt"} {
		if err := mem.WriteFile(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Shallow names.
	res := runAQL(t, r, []Value{NewWord("list"), NewString("root")})
	lst, err := AsList(res[0])
	if err != nil {
		t.Fatal(err)
	}
	if lst.Len() != 3 { // a.txt, b.txt, sub
		t.Errorf("shallow list len = %d, want 3", lst.Len())
	}

	// Detail records.
	res = runAQL(t, r, []Value{
		NewWord("list"), NewString("root"),
		wrapMap(func(om *OrderedMap) { om.Set("detail", NewBoolean(true)) }),
	})
	dlst, _ := AsList(res[0])
	if _, err := AsMap(dlst.Get(0)); err != nil {
		t.Errorf("detail entry not a record: %v", err)
	}

	// Recursive walk.
	res = runAQL(t, r, []Value{
		NewWord("list"), NewString("root"),
		wrapMap(func(om *OrderedMap) { om.Set("recursive", NewBoolean(true)) }),
	})
	rlst, _ := AsList(res[0])
	if rlst.Len() != 4 { // a.txt, b.txt, sub, sub/c.txt
		t.Errorf("recursive list len = %d, want 4", rlst.Len())
	}
}

func TestListErrors(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("f.txt", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runAQLError(t, r, []Value{NewWord("list"), NewString("ghost")}); err == nil {
		t.Error("expected list error on nonexistent dir")
	}
	if err := runAQLError(t, r, []Value{NewWord("list"), NewString("f.txt")}); err == nil {
		t.Error("expected list error on a file")
	}
}

// ── pure helpers ─────────────────────────────────────────────────────────

func TestFileTypeName(t *testing.T) {
	cases := []struct {
		fi   capabilities.FileInfo
		want string
	}{
		{capabilities.FileInfo{Symlink: true}, "symlink"},
		{capabilities.FileInfo{IsDir: true}, "dir"},
		{capabilities.FileInfo{Mode: 0}, "file"},                 // regular
		{capabilities.FileInfo{Mode: os.ModeNamedPipe}, "other"}, // neither
		{capabilities.FileInfo{Mode: os.ModeDevice}, "other"},    // neither
	}
	for _, c := range cases {
		if got := fileTypeName(c.fi); got != c.want {
			t.Errorf("fileTypeName(%+v) = %q, want %q", c.fi, got, c.want)
		}
	}
}

func TestMtimeUnix(t *testing.T) {
	if got := mtimeUnix(time.Time{}); got != 0 {
		t.Errorf("mtimeUnix(zero) = %d, want 0", got)
	}
	ts := time.Unix(12345, 0)
	if got := mtimeUnix(ts); got != 12345 {
		t.Errorf("mtimeUnix(%v) = %d, want 12345", ts, got)
	}
}

func TestIsFileTypeAtom(t *testing.T) {
	ft := NewFileType()
	if !isFileTypeAtom(newFileTypeAtom("file", ft)) {
		t.Error("file atom should be a FileType member")
	}
	if isFileTypeAtom(NewAtom("nope")) {
		t.Error("unrelated atom is not a FileType member")
	}
	if isFileTypeAtom(NewString("file")) {
		t.Error("the string \"file\" is not a FileType member")
	}
}

func TestParseStatOptsNonMap(t *testing.T) {
	// A type-literal (non-concrete) opts value falls back to defaults.
	follow, resolve := parseStatOpts(NewTypeLiteral(TMap))
	if !follow || resolve {
		t.Errorf("parseStatOpts(type literal) = follow %v resolve %v, want true false", follow, resolve)
	}
	detail, recursive := parseListOpts(NewTypeLiteral(TMap))
	if detail || recursive {
		t.Errorf("parseListOpts(type literal) = detail %v recursive %v, want false false", detail, recursive)
	}
}

// wrapMap builds a concrete Map value from a setter, for opts arguments.
func wrapMap(set func(*OrderedMap)) Value {
	om := NewOrderedMap()
	set(om)
	return NewMap(om)
}

// TestParseOptsNonOrderedMap covers the AsMap-nil path: an Options-typed
// value is a concrete TMap whose AsMap returns nil, so the option readers
// fall back to defaults rather than dereferencing a nil map.
func TestParseOptsNonOrderedMap(t *testing.T) {
	opts := NewOptionsType(NewOrderedMap())
	if follow, resolve := parseStatOpts(opts); !follow || resolve {
		t.Errorf("parseStatOpts(options) = %v %v, want true false", follow, resolve)
	}
	if detail, recursive := parseListOpts(opts); detail || recursive {
		t.Errorf("parseListOpts(options) = %v %v, want false false", detail, recursive)
	}
}

// walkFailOps lists a single subdirectory at the root but fails to list that
// subdirectory, so a recursive walk hits the recursion-error propagation.
type walkFailOps struct{ *capabilities.MemFileOps }

func (w walkFailOps) ReadDir(path string) ([]capabilities.FileInfo, error) {
	if strings.Contains(path, "sub") {
		return nil, fmt.Errorf("readdir boom")
	}
	return []capabilities.FileInfo{{Name: "sub", IsDir: true}}, nil
}

func TestCollectEntriesRecursionError(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	SetHostFileOps(r, walkFailOps{capabilities.NewMem()})
	if _, err := collectEntries(r, "root", "", true); err == nil {
		t.Error("expected the recursive-walk error to propagate")
	}
}
