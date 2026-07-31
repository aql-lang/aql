package native

import (
	"errors"
	"strings"
	"testing"
)

// failMmapRegion fails Flush and Close, to reach the region error-forward
// arms of IO.flush and IO.close.
type failMmapRegion struct{}

func (failMmapRegion) Bytes() []byte { return nil }
func (failMmapRegion) Flush() error  { return errors.New("flush boom") }
func (failMmapRegion) Close() error  { return errors.New("close boom") }

// io_mmap_test.go — P5 word layer for memory-mapped files: IO.mmap, the
// read/write region overloads (with the munmap-safe copy-out), IO.flush
// and IO.close arms, and the read-only / out-of-bounds refusals.

// mmapVal maps a path through IO.mmap and returns the Mmap value.
func mmapVal(t *testing.T, r *Registry, path string, opts func(*OrderedMap)) Value {
	t.Helper()
	res := runBORU(t, r, []Value{NewWord("mmap"), pathV(path), wrapMap(opts)})
	if _, ok := asMmapInfo(res[0]); !ok {
		t.Fatalf("mmap returned %v, not a Mmap", res[0])
	}
	return res[0]
}

func TestMmapWord(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("m.bin", []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Read a region as text and as bytes; a window slices it.
	m := mmapVal(t, r, "m.bin", func(*OrderedMap) {})
	txt := runBORU(t, r, []Value{NewWord("read"), m})
	if s, _ := AsString(txt[0]); s != "hello" {
		t.Errorf("mmap read text = %q", s)
	}
	b := runBORU(t, r, []Value{NewWord("read"), m, wrapMap(func(om *OrderedMap) { om.Set("enc", NewString("bytes")) })})
	if bs, ok := AsBytesValue(b[0]); !ok || string(bs) != "hello" {
		t.Errorf("mmap read bytes = %v", b[0])
	}
	win := runBORU(t, r, []Value{NewWord("read"), m,
		wrapMap(func(om *OrderedMap) { om.Set("offset", NewInteger(1)); om.Set("length", NewInteger(3)) })})
	if s, _ := AsString(win[0]); s != "ell" {
		t.Errorf("mmap window = %q", s)
	}
	runBORU(t, r, []Value{NewWord("close"), m})

	// Writable: splice in place, flush, and the file changes.
	w := mmapVal(t, r, "m.bin", func(om *OrderedMap) { om.Set("writable", NewBoolean(true)) })
	runBORU(t, r, []Value{NewWord("write"), w, NewBytesValue([]byte("HE"))})
	runBORU(t, r, []Value{NewWord("flush"), w})
	runBORU(t, r, []Value{NewWord("close"), w})
	if got, _ := mem.ReadFile("m.bin"); string(got) != "HEllo" {
		t.Errorf("writable mmap = %q", got)
	}

	// A windowed mmap ({offset,length}) maps only the requested bytes.
	ww := mmapVal(t, r, "m.bin", func(om *OrderedMap) { om.Set("offset", NewInteger(1)); om.Set("length", NewInteger(3)) })
	wb := runBORU(t, r, []Value{NewWord("read"), ww})
	if s, _ := AsString(wb[0]); s != "Ell" {
		t.Errorf("windowed mmap region = %q", s)
	}
	runBORU(t, r, []Value{NewWord("close"), ww})

	// mmap of an absent path errors.
	if err := runBORUError(t, r, []Value{NewWord("mmap"), pathV("ghost")}); err == nil {
		t.Error("mmap of an absent path should error")
	}
}

func TestMmapWriteRefusals(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("m.bin", []byte("abcde"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A read-only region refuses a write.
	ro := mmapVal(t, r, "m.bin", func(*OrderedMap) {})
	if err := runBORUError(t, r, []Value{NewWord("write"), ro, NewBytesValue([]byte("x"))}); err == nil {
		t.Error("write to a read-only region should refuse")
	}
	// A write past the region cannot grow the mapping.
	w := mmapVal(t, r, "m.bin", func(om *OrderedMap) { om.Set("writable", NewBoolean(true)) })
	if err := runBORUError(t, r, []Value{NewWord("write"), w, NewBytesValue([]byte("toolong!!")),
		wrapMap(func(om *OrderedMap) { om.Set("offset", NewInteger(0)) })}); err == nil {
		t.Error("an over-long region write should refuse")
	}
	// A positioned write within bounds succeeds and threads the region.
	res := runBORU(t, r, []Value{NewWord("write"), w, NewBytesValue([]byte("Z")),
		wrapMap(func(om *OrderedMap) { om.Set("offset", NewInteger(4)) })})
	if _, ok := asMmapInfo(res[0]); !ok {
		t.Error("write should return the Mmap region")
	}
	runBORU(t, r, []Value{NewWord("close"), w})
}

// TestMmapUseAfterClose pins the PR-review fix: reading, writing, or
// flushing a Mmap region AFTER IO.close must refuse cleanly rather than
// touch the unmapped bytes (a use-after-free / SIGSEGV on the OS backend).
func TestMmapUseAfterClose(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("m.bin", []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := mmapVal(t, r, "m.bin", func(om *OrderedMap) { om.Set("writable", NewBoolean(true)) })
	runBORU(t, r, []Value{NewWord("close"), w})
	if err := runBORUError(t, r, []Value{NewWord("read"), w}); err == nil {
		t.Error("read after close must refuse (no use-after-free)")
	}
	if err := runBORUError(t, r, []Value{NewWord("write"), w, NewBytesValue([]byte("x"))}); err == nil {
		t.Error("write after close must refuse")
	}
	if err := runBORUError(t, r, []Value{NewWord("flush"), w}); err == nil {
		t.Error("flush after close must refuse")
	}
	// close is idempotent (the memoised close-error path).
	runBORU(t, r, []Value{NewWord("close"), w})
}

func TestMmapHandlerGuards(t *testing.T) {
	r, mem := ioFSReg(t)
	_ = mem.WriteFile("m.bin", []byte("data"), 0o644)
	// read/write/flush of a NON-Mmap error (reached directly — the sigs
	// would otherwise reject a non-region).
	nonRegion := NewInteger(9)
	if _, err := readMmapWord([]Value{nonRegion}, r); err == nil {
		t.Error("read of a non-Mmap should error")
	}
	if _, err := writeMmapWord([]Value{nonRegion, NewString("x")}, r); err == nil {
		t.Error("write of a non-Mmap should error")
	}
	if _, err := doFlushWord([]Value{nonRegion}, r); err == nil {
		t.Error("flush of a non-Mmap/File should error")
	}
	// A read with a bogus enc fails to decode; a read window out of range errors.
	m := mmapVal(t, r, "m.bin", func(*OrderedMap) {})
	if _, err := readMmapWord([]Value{m, wrapMap(func(om *OrderedMap) { om.Set("enc", NewString("bogus")) })}, r); err == nil {
		t.Error("read with a bogus enc should error")
	}
	if _, err := readMmapWord([]Value{m, wrapMap(func(om *OrderedMap) { om.Set("offset", NewInteger(100)) })}, r); err == nil {
		t.Error("out-of-range read window should error")
	}
	if _, err := readMmapWord([]Value{m, wrapMap(func(om *OrderedMap) { om.Set("length", NewInteger(999)) })}, r); err == nil {
		t.Error("over-long read window should error")
	}
	// A write of a non-string/bytes payload errors (handleWriteBytes).
	w := mmapVal(t, r, "m.bin", func(om *OrderedMap) { om.Set("writable", NewBoolean(true)) })
	if _, err := writeMmapWord([]Value{w, wrapMap(func(om *OrderedMap) { om.Set("a", NewInteger(1)) })}, r); err == nil {
		t.Error("write of a map payload should error")
	}
	// flush of a File handle still works (polymorphic flush).
	f := openFileVal(t, r, "m.bin", func(om *OrderedMap) { om.Set("mode", NewString("rw")) })
	runBORU(t, r, []Value{NewWord("flush"), f})
	runBORU(t, r, []Value{NewWord("close"), f})
	runBORU(t, r, []Value{NewWord("close"), m})
	runBORU(t, r, []Value{NewWord("close"), w})

	// A region whose Flush/Close fail surfaces the error from IO.flush and
	// IO.close (built directly).
	bad := &MmapInfo{ID: "M_x", Path: "p", Writable: true, region: failMmapRegion{}}
	badVal := NewValueRaw(TAny, ExtensionPayload{Body: bad})
	if _, err := doFlushWord([]Value{badVal}, r); err == nil {
		t.Error("a region Flush failure should surface")
	}
	if _, err := doCloseWord([]Value{badVal}, r); err == nil {
		t.Error("a region Close failure should surface")
	}
}

func TestMmapTypeFormat(t *testing.T) {
	r, _ := DefaultRegistry()
	mt := MintMmapType(r)
	if NewMmapType() == nil {
		t.Error("NewMmapType nil")
	}
	live := NewValueRaw(mt, ExtensionPayload{Body: &MmapInfo{ID: "M_1", Path: "p"}})
	if !strings.Contains(live.String(), "Mmap(M_1,p)") {
		t.Errorf("live mmap format = %q", live.String())
	}
	if (mmapFormatBehavior{}).Format(NewInteger(1)) != "Mmap(nil)" {
		t.Error("non-mmap format")
	}
	if !(mmapFormatBehavior{}).Match(NewTypeLiteral(mt), mt) || !(mmapFormatBehavior{}).Equal(live, live) {
		t.Error("Match/Equal delegate to default")
	}
}
