package eng

import (
	"testing"

	compiler "github.com/boru-lang/boru/compiler/go"
	core "github.com/boru-lang/boru/core/go"
)

func TestS6aTryRecordFallbackStackFormRebuild(t *testing.T) {
	r := newTestRegistry(t)
	armEmit(r)
	sig := registerIslandWord(t, r, "s6afb5", core.CompileIslandPure, 2, 1)
	args := []core.Value{core.NewInteger(1), core.NewInteger(2)}
	outs := []core.Value{core.NewDynamicCarrier(core.TAny)} // dynamic out: island is legitimate
	// barrier 1 < len(args) 2 with all-baked args on a pure word: the span
	// is rebuilt in stack form (deepest stack arg first, then the word,
	// then the forward args).
	if !compiler.TryRecordFallback(r, "s6afb5", sig, args, outs, core.SrcPos{}) {
		t.Fatal("all-baked pure island in stack form should record")
	}
}
