package compiler

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

// RecordDynBind's name gate, regime-split (the parity oracle's founding
// fix): a `_`/`$`-prefixed name historically records NOTHING — under
// keep-installs the check pass's install is the kept binding, so a name
// nobody dyn-reads needs no op — but under the TWIN REGIME a root def of
// such a name needs its Push-mode OpBindGlobal partner or the rollback
// silently loses the binding cross-request (ApplyBindTwin's carrier-class
// skip consumes the twin; nothing installs). The gate therefore admits a
// ROOT `_`-name only when the regime is armed, and keeps the historical
// skip everywhere else: default regime, capitalised names, and fn-body
// recording (FnBodyDepth), so default bytecode stays byte-identical.
func TestRecordDynBindNameGateRegimeSplit(t *testing.T) {
	carrier := core.NewCarrier(core.TList)
	events := func(es *EmitState) int { return len(es.frames[0]) }

	// Default regime: `_` stays skipped.
	es := NewEmitState()
	es.RecordDynBind("_", carrier, core.SrcPos{Row: 1, Col: 5})
	if events(es) != 0 {
		t.Fatal("a `_` root def must not record under the default regime (byte-identical bytecode)")
	}

	t.Setenv("BORU_TWIN_REGIME", "1")

	r, err := core.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Regime, root: `_` records its dyn-bind event (the partner's seat).
	es = NewEmitState()
	es.BindRegistry(r)
	es.RecordDynBind("_", carrier, core.SrcPos{Row: 1, Col: 5})
	if events(es) != 1 {
		t.Fatal("a `_` root def must record under the regime — the Push-mode partner's seat")
	}

	// Regime, capitalised: still skipped (a type install replays through
	// its own twin arms; no partner is needed or wanted).
	es = NewEmitState()
	es.RecordDynBind("Foo", carrier, core.SrcPos{Row: 1, Col: 5})
	if events(es) != 0 {
		t.Fatal("a capitalised name must stay skipped under the regime")
	}

	// Regime, fn body: the historical skip stands (fn-body defs are
	// frame-locals; the ledger never notes them, so no twin needs a
	// partner there).
	es = NewEmitState()
	es.BindRegistry(r)
	es.reg.Check.FnBodyDepth = 1
	defer func() { es.reg.Check.FnBodyDepth = 0 }()
	es.RecordDynBind("_", carrier, core.SrcPos{Row: 1, Col: 5})
	if events(es) != 0 {
		t.Fatal("a `_` fn-body def must stay skipped under the regime")
	}
}
