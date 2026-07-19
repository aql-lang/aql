package capabilities

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// buildZip writes a deflate archive from files (name→content) and explicit
// directory entries, in deterministic order.
func buildZip(t *testing.T, files map[string]string, dirs ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, d := range dirs {
		if _, err := w.Create(strings.TrimSuffix(d, "/") + "/"); err != nil {
			t.Fatal(err)
		}
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fw, err := w.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(files[n])); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestZipReadStatListOpen(t *testing.T) {
	arc := buildZip(t, map[string]string{
		"top.txt":      "hello",
		"a/b.txt":      "nested",
		"a/c/deep.bin": "deep",
	}, "explicitdir/")
	z, err := NewZip(arc)
	if err != nil {
		t.Fatal(err)
	}

	// ReadFile returns a COPY of the member bytes.
	got, err := z.ReadFile("top.txt")
	if err != nil || string(got) != "hello" {
		t.Fatalf("ReadFile top.txt = %q, %v", got, err)
	}
	got[0] = 'X'
	if again, _ := z.ReadFile("top.txt"); string(again) != "hello" {
		t.Errorf("ReadFile did not copy: mutated to %q", again)
	}
	// A synthesised nested file and its synthesised parent dir both resolve.
	if b, err := z.ReadFile("a/b.txt"); err != nil || string(b) != "nested" {
		t.Errorf("nested read = %q, %v", b, err)
	}
	fi, err := z.Stat("a", true)
	if err != nil || !fi.IsDir || fi.Name != "a" || fi.UID != -1 {
		t.Errorf("Stat synthesised dir = %+v, %v", fi, err)
	}
	if fi, _ := z.Stat("top.txt", false); fi.Size != 5 || fi.IsDir {
		t.Errorf("Stat file = %+v", fi)
	}

	// ReadDir of the root lists direct children only, name-sorted.
	roots, err := z.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range roots {
		names = append(names, e.Name)
	}
	if strings.Join(names, ",") != "a,explicitdir,top.txt" {
		t.Errorf("root ReadDir = %v", names)
	}
	// ReadDir descends one level.
	sub, _ := z.ReadDir("a")
	if len(sub) != 2 || sub[0].Name != "b.txt" || sub[1].Name != "c" {
		t.Errorf("ReadDir a = %+v", sub)
	}

	// Open gives a read-only, seekable handle.
	h, err := z.Open("a/b.txt", OpenOpts{})
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 3)
	if n, _ := h.Read(buf); n != 3 || string(buf) != "nes" {
		t.Errorf("handle Read = %q", buf[:n])
	}
	if off, _ := h.Seek(0, io.SeekStart); off != 0 {
		t.Errorf("handle Seek = %d", off)
	}
	at := make([]byte, 2)
	if n, _ := h.ReadAt(at, 4); n != 2 || string(at) != "ed" {
		t.Errorf("handle ReadAt = %q", at[:n])
	}
	if h.Name() != "a/b.txt" {
		t.Errorf("handle Name = %q", h.Name())
	}
	if err := h.Sync(); err != nil {
		t.Errorf("handle Sync = %v", err)
	}
	if err := h.Close(); err != nil {
		t.Errorf("handle Close = %v", err)
	}
	// The read-only handle refuses every write.
	if _, err := h.Write([]byte("x")); !errors.Is(err, errReadOnlyFS) {
		t.Errorf("handle Write = %v", err)
	}
	if _, err := h.WriteAt([]byte("x"), 0); !errors.Is(err, errReadOnlyFS) {
		t.Errorf("handle WriteAt = %v", err)
	}
	if err := h.Truncate(0); !errors.Is(err, errReadOnlyFS) {
		t.Errorf("handle Truncate = %v", err)
	}

	// Statfs is the synthetic zip volume; ResolvePath canonicalises.
	fs, err := z.Statfs("top.txt")
	if err != nil || fs.Type != "zip" || fs.FreeBytes != 0 || fs.TotalBytes == 0 {
		t.Errorf("Statfs = %+v, %v", fs, err)
	}
	if rp, _ := z.ResolvePath("./a/../top.txt"); rp != "top.txt" {
		t.Errorf("ResolvePath = %q", rp)
	}
	// XattrList is empty; XattrGet reports no attribute (both read, not EROFS).
	if xs, err := z.XattrList("top.txt"); err != nil || len(xs) != 0 {
		t.Errorf("XattrList = %v, %v", xs, err)
	}
	if _, err := z.XattrGet("top.txt", "user.x"); err == nil || errors.Is(err, errReadOnlyFS) {
		t.Errorf("XattrGet = %v, want a not-found error", err)
	}
}

func TestZipNotFoundArms(t *testing.T) {
	z, err := NewZip(buildZip(t, map[string]string{"f.txt": "x"}))
	if err != nil {
		t.Fatal(err)
	}
	notExist := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: err = %v, want not-exist", name, err)
		}
	}
	_, e := z.ReadFile("ghost")
	notExist("ReadFile", e)
	_, e = z.Stat("ghost", true)
	notExist("Stat", e)
	_, e = z.ReadDir("ghost")
	notExist("ReadDir", e)
	_, e = z.Statfs("ghost")
	notExist("Statfs", e)
	_, e = z.Open("ghost", OpenOpts{})
	notExist("Open", e)
	_, e = z.XattrGet("ghost", "user.x")
	notExist("XattrGet", e)
	_, e = z.XattrList("ghost")
	notExist("XattrList", e)

	// Reading a directory as a file, and listing a file as a directory.
	if _, err := z.ReadFile("."); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("ReadFile dir = %v", err)
	}
	if _, err := z.ReadDir("f.txt"); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("ReadDir file = %v", err)
	}
	if _, err := z.Open(".", OpenOpts{}); err == nil || !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("Open dir = %v", err)
	}
}

func TestZipMutationsRefused(t *testing.T) {
	z, err := NewZip(buildZip(t, map[string]string{"f.txt": "x"}))
	if err != nil {
		t.Fatal(err)
	}
	ro := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, errReadOnlyFS) {
			t.Errorf("%s: err = %v, want read-only", name, err)
		}
	}
	ro("WriteFile", z.WriteFile("f.txt", []byte("y"), 0o644))
	ro("MkdirAll", z.MkdirAll("d", 0o755))
	ro("Remove", z.Remove("f.txt", false))
	ro("Rename", z.Rename("f.txt", "g.txt"))
	ro("Symlink", z.Symlink("f.txt", "l"))
	ro("Link", z.Link("f.txt", "h"))
	ro("Chmod", z.Chmod("f.txt", 0o600))
	ro("Chtimes", z.Chtimes("f.txt", time.Time{}, time.Time{}))
	ro("Truncate", z.Truncate("f.txt", 0))
	ro("Chown", z.Chown("f.txt", 1, 1, true))
	ro("XattrSet", z.XattrSet("f.txt", "user.x", []byte("v")))
	ro("XattrRemove", z.XattrRemove("f.txt", "user.x"))
	_, e := z.TempFile("", "p")
	ro("TempFile", e)
	_, e = z.TempDir("", "p")
	ro("TempDir", e)
	_, e = z.Open("f.txt", OpenOpts{Write: true})
	ro("Open{write}", e)
	_, e = z.Lock("f.txt", false, true)
	ro("Lock", e)
	_, e = z.Mmap("f.txt", 0, 0, false)
	ro("Mmap", e)
	_, _, e = z.Watch("f.txt", WatchOpts{})
	ro("Watch", e)
}

func TestZipInvalidArchive(t *testing.T) {
	if _, err := NewZip([]byte("not a zip at all")); err == nil {
		t.Error("expected an invalid archive to error")
	}

	// A corrupt local-header signature: the central directory (read by
	// NewReader) stays intact, but f.Open fails to validate the local header.
	arc := buildStoredZip(t, "f.txt", "hello world payload")
	badHeader := append([]byte(nil), arc...)
	badHeader[0] = 'X' // clobber the "PK\x03\x04" local signature
	if _, err := NewZip(badHeader); err == nil {
		t.Error("expected the corrupt local header to fail f.Open")
	}

	// Corrupt the STORED payload: NewReader and f.Open succeed, but reading
	// the member trips the CRC check.
	badData := append([]byte(nil), arc...)
	idx := bytes.Index(badData, []byte("payload"))
	if idx < 0 {
		t.Fatal("stored payload not found in archive bytes")
	}
	badData[idx] ^= 0xff
	if _, err := NewZip(badData); err == nil {
		t.Error("expected the corrupt payload to fail the CRC read")
	}
}

// TestZipExplicitRootEntry covers the `name == "."` skip: an archive member
// whose name cleans to the root is ignored in favour of the synthesised root.
func TestZipExplicitRootEntry(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	if _, err := w.Create("./"); err != nil {
		t.Fatal(err)
	}
	fw, err := w.Create("keep.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("k")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	z, err := NewZip(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := z.Stat(".", true); err != nil || !fi.IsDir {
		t.Errorf("root = %+v, %v", fi, err)
	}
	if b, err := z.ReadFile("keep.txt"); err != nil || string(b) != "k" {
		t.Errorf("keep.txt = %q, %v", b, err)
	}
}

// TestZipVsMemReadParity builds a tree in the mem backend, zips it, and
// asserts read/stat/list/open agree between the two backends.
func TestZipVsMemReadParity(t *testing.T) {
	tree := map[string]string{
		"readme.md":  "top-level",
		"src/a.go":   "package a",
		"src/b.go":   "package b",
		"docs/x.txt": "doc",
	}
	mem := NewMem()
	for name, content := range tree {
		if err := mem.WriteFile(name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	z, err := NewZip(buildZip(t, tree, "src/", "docs/"))
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range tree {
		zb, zerr := z.ReadFile(name)
		mb, merr := mem.ReadFile(name)
		if zerr != nil || merr != nil || !bytes.Equal(zb, mb) || string(zb) != content {
			t.Errorf("read %s: zip=%q(%v) mem=%q(%v)", name, zb, zerr, mb, merr)
		}
		zfi, _ := z.Stat(name, true)
		mfi, _ := mem.Stat(name, true)
		if zfi.Size != mfi.Size || zfi.IsDir != mfi.IsDir || zfi.Name != mfi.Name {
			t.Errorf("stat %s: zip=%+v mem=%+v", name, zfi, mfi)
		}
	}
	// Directory listings agree on names and dir-ness.
	zdir, _ := z.ReadDir("src")
	mdir, _ := mem.ReadDir("src")
	if len(zdir) != len(mdir) {
		t.Fatalf("ReadDir src: zip %d vs mem %d entries", len(zdir), len(mdir))
	}
	for i := range zdir {
		if zdir[i].Name != mdir[i].Name || zdir[i].IsDir != mdir[i].IsDir {
			t.Errorf("ReadDir src[%d]: zip=%+v mem=%+v", i, zdir[i], mdir[i])
		}
	}
}

// buildStoredZip writes a single STORED (uncompressed) member so the payload
// bytes appear literally in the archive, for the CRC-corruption arm.
func buildStoredZip(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	fw, err := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
