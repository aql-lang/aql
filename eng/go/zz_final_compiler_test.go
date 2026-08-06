package eng

import (
	"testing"

	core "github.com/boru-lang/boru/core/go"
)

func TestParenBoundedTrailingApply(t *testing.T) {
	// `(5 (cmk1))` — the closure is the LAST value in the paren; the
	// check pass records a paren-bounded dyn-apply event.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{
			core.NewOpenParen(),
			core.NewInteger(5),
			core.NewOpenParen(), core.NewWord("cmk1"), core.NewCloseParen(),
			core.NewCloseParen(),
		}
	}, "105")
}

func TestParenBoundedApplyFeedsCall(t *testing.T) {
	// The paren-bounded apply result feeds a following native call —
	// the exact reorder hazard the event-collapse solves.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cdub"),
			core.NewOpenParen(),
			core.NewInteger(5),
			core.NewOpenParen(), core.NewWord("cmk1"), core.NewCloseParen(),
			core.NewCloseParen(),
		}
	}, "210")
}

func TestParenLeadingDynamicRefuses(t *testing.T) {
	// `((cany) 3)` — a leading DYNAMIC value before args inside a paren
	// is the unsound reorder shape: check refuses; interpreter runs.
	runTolerant(t, registerDynWords, func() []core.Value {
		return []core.Value{
			core.NewOpenParen(),
			core.NewOpenParen(), core.NewWord("cany"), core.NewCloseParen(),
			core.NewInteger(3),
			core.NewCloseParen(),
		}
	}, "7 | 3")
}

// --- user-fn poly re-match -------------------------------------------------------------

func TestCompiledDisjunctArgDispatch(t *testing.T) {
	// The same join consumed by cdub, end to end.
	runTolerant(t, registerBranchWords, func() []core.Value {
		return []core.Value{
			core.NewWord("cdub"),
			core.NewOpenParen(),
			core.NewWord("cif"),
			core.NewOpenParen(), core.NewWord("cgt"), core.NewInteger(1), core.NewInteger(2), core.NewCloseParen(),
			core.NewInteger(1),
			core.NewString("s"),
			core.NewCloseParen(),
		}
	}, "'ss'")
}

// --- dispatch error paths ----------------------------------------------------------------
