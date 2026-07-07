package native

import "testing"

// Coverage for the error/edge arms of the `unpack` destructuring word
// (native_unpack.go): bad selector/source types, the module-unpack
// string/atom guards, the record source branch, the check-mode lenient
// getter, and the per-name validation rejects.

func TestW9UnpackExportMapArms(t *testing.T) {
	r := w9Reg(t)

	// nil export map is a no-op (guard).
	unpackExportMap(r, nil)

	// Capitalised (type) and $-prefixed keys are skipped; only the plain
	// lowercase key binds.
	em := NewOrderedMap()
	em.Set("Foo", NewInteger(1)) // capitalised → skipped
	em.Set("$name", NewInteger(2))
	em.Set("bar", NewInteger(3)) // plain → bound
	unpackExportMap(r, em)

	if _, ok := r.Defs.Top("Foo"); ok {
		t.Fatalf("capitalised key Foo should not have been bound")
	}
	if v, ok := r.Defs.Top("bar"); !ok {
		t.Fatalf("plain key bar should have been bound")
	} else if got, _ := AsInteger(v); got != 3 {
		t.Fatalf("bar = %d, want 3", got)
	}
}

func TestW9UnpackModuleHandlerArms(t *testing.T) {
	r := w9Reg(t)

	// unpack 'mod': non-concrete string arg rejected.
	_, err := unpackModuleHandler([]Value{NewTypeLiteral(TString)}, nil, nil, r)
	w9wantErr(t, err, "module name must be a string")

	// unpack ExportName 'mod': first arg must be an atom.
	_, err = unpackModuleExportHandler([]Value{NewInteger(5), NewString("aql:x")}, nil, nil, r)
	w9wantErr(t, err, "export name must be a word/atom")

	// unpack ExportName 'mod': second arg must be a concrete string.
	_, err = unpackModuleExportHandler([]Value{NewAtom("Foo"), NewTypeLiteral(TString)}, nil, nil, r)
	w9wantErr(t, err, "module name must be a string")
}

func TestW9UnpackSourceArms(t *testing.T) {
	r := w9Reg(t)

	// Record source: AsMap fails but IsRecordType is true → field getter.
	fields := NewOrderedMap()
	fields.Set("a", NewInteger(1))
	rec := NewRecordType(fields)
	get, keys, proven, err := unpackSource(rec, r)
	if err != nil {
		t.Fatalf("record source: %v", err)
	}
	if len(keys) != 1 || keys[0] != "a" {
		t.Fatalf("record keys = %v, want [a]", keys)
	}
	if v, ok := get("a"); !ok {
		t.Fatalf("record getter missing key a")
	} else if n, _ := AsInteger(v); n != 1 {
		t.Fatalf("record a = %d, want 1", n)
	}
	if !proven {
		t.Fatalf("record source getter should be proven (concrete)")
	}

	// Concrete non-map/non-record source is rejected.
	_, _, _, err = unpackSource(NewInteger(5), r)
	w9wantErr(t, err, "source must be a map or record")

	// Non-concrete source out of check mode is rejected.
	_, _, _, err = unpackSource(NewTypeLiteral(TMap), r)
	w9wantErr(t, err, "not a concrete map")
}

func TestW9UnpackSourceCheckModeLenient(t *testing.T) {
	r := w9Reg(t)
	restore := r.Check.Begin()
	defer restore()

	// In check mode a non-concrete source yields an empty getter and no
	// error so static analysis can continue.
	get, keys, proven, err := unpackSource(NewTypeLiteral(TMap), r)
	if err != nil {
		t.Fatalf("check-mode source: unexpected error %v", err)
	}
	if keys != nil {
		t.Fatalf("check-mode source keys = %v, want nil", keys)
	}
	if _, ok := get("anything"); ok {
		t.Fatalf("check-mode empty getter should report missing")
	}
	if proven {
		t.Fatalf("check-mode stub getter must NOT be proven — its misses are not static evidence")
	}
}

func TestW9BindUnpackEntryRejects(t *testing.T) {
	r := w9Reg(t)
	get := func(string) (Value, bool) { return NewInteger(1), true }

	// Invalid word name (leading digit) is rejected by ValidateWordName.
	err := bindUnpackEntry(r, "1bad", "1bad", get, true, SrcPos{})
	w9wantErr(t, err, "must begin with")

	// Name clash with an existing (lowercase) type binding. A lowercase
	// type cannot arise through def/InstallType (which require a capital),
	// so force the DefTable state directly to exercise the guard.
	r.Defs.PushType("lc_type", TInteger, NewTypeLiteral(TInteger))
	err = bindUnpackEntry(r, "lc_type", "lc_type", get, true, SrcPos{})
	w9wantErr(t, err, "name clash")
}

func TestW9UnpackAllRenameArms(t *testing.T) {
	r := w9Reg(t)

	// unpack all <bad source>: unpackSource error propagates.
	_, err := unpackAllHandler([]Value{NewAtom("all"), NewInteger(5)}, nil, nil, r)
	w9wantErr(t, err, "source must be a map or record")

	// unpack all {Foo:1}: binding a capitalised source key is rejected.
	_, err = unpackAllHandler([]Value{NewAtom("all"), w9Map("Foo", NewInteger(1))}, nil, nil, r)
	w9wantErr(t, err, "capitalised")

	// unpack {a:x} <bad source>: unpackSource error propagates.
	rmap := w9Map("a", NewAtom("x"))
	_, err = unpackRenameHandler([]Value{rmap, NewInteger(5)}, nil, nil, r)
	w9wantErr(t, err, "source must be a map or record")
}

func TestW9UnpackPositive(t *testing.T) {
	r := w9Reg(t)
	out, err := w9Run(t, r, "def m {a:1 b:2}\nunpack [a] m\na")
	if err != nil {
		t.Fatalf("unpack positive: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("unpack positive: empty stack")
	}
	if n, _ := AsInteger(out[len(out)-1]); n != 1 {
		t.Fatalf("unpack positive: a = %d, want 1", n)
	}
}
