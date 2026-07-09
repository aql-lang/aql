package eng

import "testing"

// TestDefTableGenNil pins the nil-receiver read.
func TestDefTableGenNil(t *testing.T) {
	if g := (*DefTable)(nil).Gen("x"); g != 0 {
		t.Fatalf("nil DefTable Gen = %d, want 0", g)
	}
}

// TestDefTableGenBumps asserts every stack-mutating operation bumps the
// per-name generation counter (the dispatchCache invalidation key), and
// that an unrelated name's generation is untouched — the property that
// makes the cache survive a hot loop's param push/pop.
func TestDefTableGenBumps(t *testing.T) {
	check := func(name string, op func(dt *DefTable), wantBump bool) {
		t.Helper()
		dt := NewDefTable()
		dt.Push("a", NewInteger(1)) // seed an unrelated name
		genOther := dt.Gen("a")
		before := dt.Gen(name)
		op(dt)
		if got := dt.Gen(name); (got > before) != wantBump {
			t.Fatalf("%s: gen before=%d after=%d, wantBump=%v", name, before, got, wantBump)
		}
		if dt.Gen("a") != genOther {
			t.Fatalf("%s: unrelated name 'a' generation changed", name)
		}
	}

	check("p", func(dt *DefTable) { dt.Push("p", NewInteger(1)) }, true)
	check("t", func(dt *DefTable) { dt.PushType("t", nil, NewInteger(1)) }, true)
	check("p", func(dt *DefTable) { dt.Push("p", NewInteger(1)); dt.Pop("p") }, true)
	check("p", func(dt *DefTable) { dt.Push("p", NewInteger(1)); dt.Replace("p", NewInteger(2)) }, true)
	check("p", func(dt *DefTable) { dt.Push("p", NewInteger(1)); dt.Push("p", NewInteger(2)); dt.Truncate("p", 1) }, true)
	check("p", func(dt *DefTable) { dt.Push("p", NewInteger(1)); dt.Delete("p") }, true)
	check("p", func(dt *DefTable) { dt.Set("p", []Value{NewInteger(1)}) }, true)

	// No-op operations do not bump: Replace/Truncate/Delete on an absent
	// name.
	check("absent", func(dt *DefTable) { dt.Replace("absent", NewInteger(1)) }, false)
	check("absent", func(dt *DefTable) { dt.Truncate("absent", 3) }, false)
	check("absent", func(dt *DefTable) { dt.Delete("absent") }, false)

	// Truncate to a depth >= current is a no-op and must not bump: seed,
	// record the post-seed generation, then truncate above the depth.
	dt := NewDefTable()
	dt.Push("p", NewInteger(1))
	g := dt.Gen("p")
	dt.Truncate("p", 5)
	if dt.Gen("p") != g {
		t.Fatalf("Truncate above depth bumped gen: %d -> %d", g, dt.Gen("p"))
	}
}

// TestDefTableRestoreBumpsGen pins that a sandbox rollback (Restore) that
// deletes a name not present in the snapshot bumps that name's generation
// so a cached dispatch aggregate built before the sandbox is invalidated.
func TestDefTableRestoreBumpsGen(t *testing.T) {
	dt := NewDefTable()
	snap := dt.Snapshot() // empty
	dt.Push("added", NewInteger(1))
	before := dt.Gen("added")
	dt.Restore(snap) // deletes "added" (not in snap)
	if dt.Gen("added") <= before {
		t.Fatalf("Restore did not bump gen for deleted name: before=%d after=%d", before, dt.Gen("added"))
	}
	if _, ok := dt.Top("added"); ok {
		t.Fatal("Restore did not delete 'added'")
	}
}

// TestDispatchCacheHitAndInvalidate exercises the runtime-mode Lookup
// cache: repeated lookups of an unchanged name return the SAME cached
// aggregate (hit), and any binding change to the name invalidates it
// (miss → fresh aggregate). It also pins the negative-cache path (a
// name with no FnDefInfo binding).
func TestDispatchCacheHitAndInvalidate(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.upsertFnDef("w", Signature{Args: []*Type{TInteger}, Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return a, nil })})

	// Runtime mode (Check inactive): two lookups hit the same cached ptr.
	first := r.Lookup("w")
	if first == nil {
		t.Fatal("Lookup(w) = nil")
	}
	if second := r.Lookup("w"); second != first {
		t.Fatal("second Lookup did not hit the cache (different pointer)")
	}

	// A new overload bumps gen → the cache must miss and rebuild.
	r.upsertFnDef("w", Signature{Args: []*Type{TString}, Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return a, nil })})
	third := r.Lookup("w")
	if third == first {
		t.Fatal("cache served a stale aggregate after a binding change")
	}
	if len(third.Signatures) < 2 {
		t.Fatalf("rebuilt aggregate missing the new overload: %d sigs", len(third.Signatures))
	}

	// Negative cache: an unbound name resolves to nil and is cached.
	if r.Lookup("nope") != nil {
		t.Fatal("Lookup(nope) = non-nil")
	}
	if r.Lookup("nope") != nil {
		t.Fatal("Lookup(nope) second call = non-nil")
	}
}

// TestDispatchCacheCheckModeBypass pins that check mode never serves the
// cache: it always returns a fresh (uncached) aggregate, so the
// pointer-identity contracts of the refusal/emit passes hold.
func TestDispatchCacheCheckModeBypass(t *testing.T) {
	r, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.upsertFnDef("w", Signature{Args: []*Type{TInteger}, Impl: Go(func(a []Value, _ map[string]Value, _ []Value, _ *Registry) ([]Value, error) { return a, nil })})

	done := r.Check.Begin()
	defer done()
	a := r.Lookup("w")
	b := r.Lookup("w")
	if a == nil || b == nil {
		t.Fatal("check-mode Lookup returned nil")
	}
	if a == b {
		t.Fatal("check mode served a cached (identical) aggregate; must be fresh each call")
	}
}

// TestTypeMetaAccessorsNilValue pins that the lattice-int accessors return
// 0 for an ordinary runtime value (tmeta == nil) — the zero the fields
// returned when they were inline. Guards the nil branch of each accessor.
func TestTypeMetaAccessorsNilValue(t *testing.T) {
	v := NewInteger(5) // an ordinary value: no typeMeta
	if v.FixedID() != 0 || v.Rank() != 0 || v.Depth() != 0 || v.In() != 0 || v.Out() != 0 {
		t.Fatalf("nil-tmeta accessors not all 0: FixedID=%d Rank=%d Depth=%d In=%d Out=%d",
			v.FixedID(), v.Rank(), v.Depth(), v.In(), v.Out())
	}
	if v.Name() != "" {
		t.Fatalf("nil-tmeta Name = %q, want empty", v.Name())
	}
	// SetName lazily allocates the tmeta on a value that had none.
	v.SetName("x")
	if v.Name() != "x" {
		t.Fatalf("after SetName, Name = %q, want x", v.Name())
	}
	// Pos: nil-safe read returns the zero SrcPos; SetPos then attaches one.
	if (v.Pos() != SrcPos{}) {
		t.Fatalf("nil-pos Pos() = %+v, want zero", v.Pos())
	}
	v.SetPos(SrcPos{Row: 3, Col: 7})
	if v.Pos().Row != 3 || v.Pos().Col != 7 {
		t.Fatalf("after SetPos, Pos() = %+v, want Row3 Col7", v.Pos())
	}
}
