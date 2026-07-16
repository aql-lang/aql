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

func TestMapBoolOptNonMap(t *testing.T) {
	// A type-literal (non-concrete) opts value falls back to the default.
	if !mapBoolOpt(NewTypeLiteral(TMap), "follow", true) {
		t.Error("type-literal opts should yield the true default")
	}
	if mapBoolOpt(NewTypeLiteral(TMap), "detail", false) {
		t.Error("type-literal opts should yield the false default")
	}
}

// wrapMap builds a concrete Map value from a setter, for opts arguments.
func wrapMap(set func(*OrderedMap)) Value {
	om := NewOrderedMap()
	set(om)
	return NewMap(om)
}

// TestMapBoolOptNonOrderedMap covers the AsMap-nil path: an Options-typed
// value is a concrete TMap whose AsMap returns nil, so mapBoolOpt falls back
// to the default rather than dereferencing a nil map.
func TestMapBoolOptNonOrderedMap(t *testing.T) {
	opts := NewOptionsType(NewOrderedMap())
	if !mapBoolOpt(opts, "follow", true) {
		t.Error("options-typed opts should yield the true default")
	}
	if mapBoolOpt(opts, "recursive", false) {
		t.Error("options-typed opts should yield the false default")
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

// ── remove / move / copy ─────────────────────────────────────────────────

func TestRemoveWord(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("f.txt", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// remove a file → returns the path, file gone.
	res := runAQL(t, r, []Value{NewWord("remove"), NewString("f.txt")})
	if res[0].String() != "'f.txt'" {
		t.Errorf("remove returned %v", res[0])
	}
	if _, err := mem.Stat("f.txt", false); err == nil {
		t.Error("file not removed")
	}
	// removing an absent path errors without {force}.
	if err := runAQLError(t, r, []Value{NewWord("remove"), NewString("f.txt")}); err == nil {
		t.Error("expected error removing an absent path")
	}
	// {force:true} makes an absent removal a no-op.
	res = runAQL(t, r, []Value{
		NewWord("remove"), NewString("ghost"),
		wrapMap(func(om *OrderedMap) { om.Set("force", NewBoolean(true)) }),
	})
	if res[0].String() != "'ghost'" {
		t.Errorf("forced remove returned %v", res[0])
	}
	// removing a non-empty dir needs {recursive}.
	if err := mem.WriteFile("d/c.txt", []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runAQLError(t, r, []Value{NewWord("remove"), NewString("d")}); err == nil {
		t.Error("expected error removing a non-empty dir non-recursively")
	}
	runAQL(t, r, []Value{
		NewWord("remove"), NewString("d"),
		wrapMap(func(om *OrderedMap) { om.Set("recursive", NewBoolean(true)) }),
	})
	if _, err := mem.Stat("d/c.txt", false); err == nil {
		t.Error("tree not removed")
	}
}

func TestMoveWord(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("a.txt", []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	res := runAQL(t, r, []Value{NewWord("move"), NewString("a.txt"), NewString("b.txt")})
	if res[0].String() != "'b.txt'" {
		t.Errorf("move returned %v", res[0])
	}
	if _, err := mem.Stat("b.txt", false); err != nil {
		t.Error("move destination missing")
	}
	if _, err := mem.Stat("a.txt", false); err == nil {
		t.Error("move source remains")
	}
	if err := runAQLError(t, r, []Value{NewWord("move"), NewString("ghost"), NewString("z")}); err == nil {
		t.Error("expected error moving an absent path")
	}
	// A Pathon destination is returned as a Pathon.
	if err := mem.WriteFile("c.txt", []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}
	res = runAQL(t, r, []Value{NewWord("move"), NewString("c.txt"), NewPathon([]string{"e.txt"}, false)})
	if !IsPathon(res[0]) {
		t.Errorf("move to Pathon returned %v (want Pathon)", res[0])
	}
}

func TestCopyWord(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("src.txt", []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	// copy a file.
	res := runAQL(t, r, []Value{NewWord("copy"), NewString("src.txt"), NewString("dst.txt")})
	if res[0].String() != "'dst.txt'" {
		t.Errorf("copy returned %v", res[0])
	}
	if b, err := mem.ReadFile("dst.txt"); err != nil || string(b) != "hello" {
		t.Errorf("copy content = %q (%v)", b, err)
	}
	// copy a symlink recreates it.
	if err := mem.Symlink("src.txt", "link"); err != nil {
		t.Fatal(err)
	}
	runAQL(t, r, []Value{NewWord("copy"), NewString("link"), NewString("link2")})
	if li, err := mem.Stat("link2", false); err != nil || !li.Symlink || li.Target != "src.txt" {
		t.Errorf("copied symlink = %+v (%v)", li, err)
	}
	// copying a directory needs {recursive}.
	if err := mem.WriteFile("tree/a.txt", []byte("1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := mem.WriteFile("tree/sub/b.txt", []byte("2"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := runAQLError(t, r, []Value{NewWord("copy"), NewString("tree"), NewString("treecopy")}); err == nil {
		t.Error("expected error copying a dir without {recursive}")
	}
	runAQL(t, r, []Value{
		NewWord("copy"), NewString("tree"), NewString("treecopy"),
		wrapMap(func(om *OrderedMap) { om.Set("recursive", NewBoolean(true)) }),
	})
	if b, err := mem.ReadFile("treecopy/sub/b.txt"); err != nil || string(b) != "2" {
		t.Errorf("recursive copy child = %q (%v)", b, err)
	}
	// copying an absent source errors.
	if err := runAQLError(t, r, []Value{NewWord("copy"), NewString("ghost"), NewString("z")}); err == nil {
		t.Error("expected error copying an absent path")
	}
}

// failAtOps wraps a MemFileOps and fails a chosen operation when its path
// contains the configured substring, so doCopy's error branches are reachable.
type failAtOps struct {
	*capabilities.MemFileOps
	failReadFile, failWriteFile, failMkdir, failReadDir, failSymlink string
}

func (f *failAtOps) ReadFile(p string) ([]byte, error) {
	if f.failReadFile != "" && strings.Contains(p, f.failReadFile) {
		return nil, fmt.Errorf("readfile boom")
	}
	return f.MemFileOps.ReadFile(p)
}
func (f *failAtOps) WriteFile(p string, d []byte, m os.FileMode) error {
	if f.failWriteFile != "" && strings.Contains(p, f.failWriteFile) {
		return fmt.Errorf("writefile boom")
	}
	return f.MemFileOps.WriteFile(p, d, m)
}
func (f *failAtOps) MkdirAll(p string, m os.FileMode) error {
	if f.failMkdir != "" && strings.Contains(p, f.failMkdir) {
		return fmt.Errorf("mkdir boom")
	}
	return f.MemFileOps.MkdirAll(p, m)
}
func (f *failAtOps) ReadDir(p string) ([]capabilities.FileInfo, error) {
	if f.failReadDir != "" && strings.Contains(p, f.failReadDir) {
		return nil, fmt.Errorf("readdir boom")
	}
	return f.MemFileOps.ReadDir(p)
}
func (f *failAtOps) Symlink(target, link string) error {
	if f.failSymlink != "" && strings.Contains(link, f.failSymlink) {
		return fmt.Errorf("symlink boom")
	}
	return f.MemFileOps.Symlink(target, link)
}

func TestCopyErrorBranches(t *testing.T) {
	newReg := func(setup func(*capabilities.MemFileOps), f *failAtOps) *Registry {
		r, err := DefaultRegistry()
		if err != nil {
			t.Fatal(err)
		}
		mem := capabilities.NewMem()
		setup(mem)
		f.MemFileOps = mem
		SetHostFileOps(r, f)
		return r
	}

	// Stat error: copying an absent source.
	r := newReg(func(*capabilities.MemFileOps) {}, &failAtOps{})
	if err := doCopy(r, "ghost", "dst", false); err == nil {
		t.Error("expected Stat error copying absent source")
	}
	// ReadFile error on a file copy.
	r = newReg(func(m *capabilities.MemFileOps) { m.WriteFile("s.txt", []byte("x"), 0644) },
		&failAtOps{failReadFile: "s.txt"})
	if err := doCopy(r, "s.txt", "d.txt", false); err == nil {
		t.Error("expected ReadFile error")
	}
	// WriteFile error on a file copy.
	r = newReg(func(m *capabilities.MemFileOps) { m.WriteFile("s.txt", []byte("x"), 0644) },
		&failAtOps{failWriteFile: "d.txt"})
	if err := doCopy(r, "s.txt", "d.txt", false); err == nil {
		t.Error("expected WriteFile error")
	}
	// MkdirAll error in copyTree.
	r = newReg(func(m *capabilities.MemFileOps) { m.WriteFile("dir/a.txt", []byte("x"), 0644) },
		&failAtOps{failMkdir: "out"})
	if err := doCopy(r, "dir", "out", true); err == nil {
		t.Error("expected MkdirAll error")
	}
	// ReadDir error in copyTree.
	r = newReg(func(m *capabilities.MemFileOps) { m.WriteFile("dir/a.txt", []byte("x"), 0644) },
		&failAtOps{failReadDir: "dir"})
	if err := doCopy(r, "dir", "out", true); err == nil {
		t.Error("expected ReadDir error")
	}
	// Symlink error copying a symlink.
	r = newReg(func(m *capabilities.MemFileOps) { m.Symlink("t", "sl") },
		&failAtOps{failSymlink: "d"})
	if err := doCopy(r, "sl", "d", false); err == nil {
		t.Error("expected Symlink error")
	}
	// Recursion error: a child copy fails inside copyTree.
	r = newReg(func(m *capabilities.MemFileOps) { m.WriteFile("dir/a.txt", []byte("x"), 0644) },
		&failAtOps{failReadFile: "a.txt"})
	if err := doCopy(r, "dir", "out", true); err == nil {
		t.Error("expected child-copy error to propagate")
	}
}
