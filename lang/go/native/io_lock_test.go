package native

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aql-lang/aql/lang/go/capabilities"
	"github.com/aql-lang/aql/lang/go/policy"
)

// io_lock_test.go — P5 word layer for advisory locks: IO.lock / IO.unlock,
// the none-on-contention path, the polymorphic close arm, and the
// permissioned / notinstalled / mount postures.

// failCloser is an io.Closer whose Close fails.
type failCloser struct{}

func (failCloser) Close() error { return errors.New("release boom") }

// lockCloserOps wraps a mem and returns a preset closer from Lock.
type lockCloserOps struct {
	*capabilities.MemFileOps
	closer io.Closer
}

func (o *lockCloserOps) Lock(string, bool, bool) (io.Closer, error) { return o.closer, nil }

func TestLockWord(t *testing.T) {
	r, mem := ioFSReg(t)
	if err := mem.WriteFile("f", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A lock is an IO.Lock; unlocking releases it.
	res := runAQL(t, r, []Value{NewWord("lock"), pathV("f")})
	if _, ok := asLockInfo(res[0]); !ok {
		t.Fatalf("lock returned %v, not a Lock", res[0])
	}
	runAQL(t, r, []Value{NewWord("unlock"), res[0]})

	// {shared} and {block} options parse.
	sh := runAQL(t, r, []Value{NewWord("lock"), pathV("f"),
		wrapMap(func(om *OrderedMap) { om.Set("shared", NewBoolean(true)) })})
	li, _ := asLockInfo(sh[0])
	if !li.Shared {
		t.Error("shared lock flag not set")
	}
	runAQL(t, r, []Value{NewWord("close"), sh[0]}) // polymorphic close over a Lock

	// Non-blocking contention returns none, not an error.
	held := runAQL(t, r, []Value{NewWord("lock"), pathV("f")})
	none := runAQL(t, r, []Value{NewWord("lock"), pathV("f"),
		wrapMap(func(om *OrderedMap) { om.Set("block", NewBoolean(false)) })})
	if !none[0].Is(TNone) {
		t.Errorf("contended non-block lock = %v, want none", none[0])
	}
	runAQL(t, r, []Value{NewWord("unlock"), held[0]})

	// Locking an absent path errors.
	if err := runAQLError(t, r, []Value{NewWord("lock"), pathV("ghost")}); err == nil {
		t.Error("lock of an absent path should error")
	}
}

func TestUnlockErrors(t *testing.T) {
	r, _ := ioFSReg(t)
	// unlock of a non-Lock errors (reached directly — the sig would reject).
	if _, err := doUnlockWord([]Value{NewInteger(1)}, r); err == nil {
		t.Error("unlock of a non-Lock should error")
	}
	// A release failure surfaces from unlock and from close.
	newReg := func() *Registry {
		rr, _ := DefaultRegistry()
		registerIOWords(rr)
		mem := capabilities.NewMem()
		_ = mem.WriteFile("f", []byte("x"), 0o644)
		SetHostFileOps(rr, &lockCloserOps{MemFileOps: mem, closer: failCloser{}})
		return rr
	}
	r2 := newReg()
	l := runAQL(t, r2, []Value{NewWord("lock"), pathV("f")})
	if err := runAQLError(t, r2, []Value{NewWord("unlock"), l[0]}); err == nil {
		t.Error("a release failure should surface from unlock")
	}
	r3 := newReg()
	l3 := runAQL(t, r3, []Value{NewWord("lock"), pathV("f")})
	if _, err := doCloseWord([]Value{l3[0]}, r3); err == nil {
		t.Error("a release failure should surface from close")
	}
	// Release is idempotent: the underlying Close fires exactly once.
	cc := &countCloser{}
	li := &LockInfo{closer: cc}
	_ = li.Release()
	_ = li.Release()
	if cc.n != 1 {
		t.Errorf("Close fired %d times, want 1", cc.n)
	}
}

// countCloser counts Close calls.
type countCloser struct{ n int }

func (c *countCloser) Close() error { c.n++; return nil }

func TestLockTypeFormat(t *testing.T) {
	r, _ := DefaultRegistry()
	lt := MintLockType(r)
	if NewLockType() == nil {
		t.Error("NewLockType nil")
	}
	live := NewValueRaw(lt, ExtensionPayload{Body: &LockInfo{ID: "L_1", Path: "p"}})
	if !strings.Contains(live.String(), "Lock(L_1,p)") {
		t.Errorf("live lock format = %q", live.String())
	}
	if (lockFormatBehavior{}).Format(NewInteger(1)) != "Lock(nil)" {
		t.Error("non-lock format")
	}
	if !(lockFormatBehavior{}).Match(NewTypeLiteral(lt), lt) || !(lockFormatBehavior{}).Equal(live, live) {
		t.Error("Match/Equal delegate to default")
	}
}

func TestNotInstalledLock(t *testing.T) {
	var ops capabilities.FileOps = notInstalledFileOps{}
	if _, err := ops.Lock("x", false, true); err == nil {
		t.Error("not-installed Lock should error")
	}
	if _, err := ops.Mmap("x", 0, 0, false); err == nil {
		t.Error("not-installed Mmap should error")
	}
}

func TestPermissionedLockMmap(t *testing.T) {
	r, err := DefaultRegistryWithPolicy(loadPolicy(t, "sandbox"))
	if err != nil {
		t.Fatal(err)
	}
	ops := HostFileOps(r)
	var d *policy.Denied
	if _, err := ops.Lock("/tmp/x", false, true); !errors.As(err, &d) {
		t.Errorf("lock gate: %v", err)
	}
	if _, err := ops.Mmap("/tmp/x", 0, 0, false); !errors.As(err, &d) {
		t.Errorf("read-mmap gate: %v", err)
	}
	if _, err := ops.Mmap("/tmp/x", 0, 0, true); !errors.As(err, &d) {
		t.Errorf("write-mmap gate: %v", err)
	}
	// Trusted passes through.
	mem := capabilities.NewMem()
	_ = mem.WriteFile("f", []byte("x"), 0o644)
	wrapped := NewPermissionedFileOps(mem, loadPolicy(t, "trusted"))
	l, lerr := wrapped.Lock("f", false, true)
	if lerr != nil {
		t.Errorf("trusted lock: %v", lerr)
	}
	_ = l.Close()
	m, merr := wrapped.Mmap("f", 0, 0, false)
	if merr != nil {
		t.Errorf("trusted mmap: %v", merr)
	} else {
		_ = m.Close()
	}
}

func TestMountLockMmapRefuse(t *testing.T) {
	ops := HostFileOps(mountFixture(t, `mount { read: (p:Pathon => ["seed"]) }`))
	if _, err := ops.Lock("x", false, true); err == nil {
		t.Error("mount Lock should refuse")
	}
	if _, err := ops.Mmap("x", 0, 0, false); err == nil {
		t.Error("mount Mmap should refuse")
	}
}
