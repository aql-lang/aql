package native

import (
	"strings"
	"testing"
)

// NUR048 (SHARP-EDGES G9): a match-position word bound to a FUNCTION
// value heads an OPEN-CALL default arm — the residue runs isolated with
// the case value pushed first, exactly like a matched code-body arm —
// instead of being torn into bogus (match, block) pairs whose last
// token leaked out as the silent result.

const nur048Prelude = `def g fn [[s:Map e:Map] [Map] [s]] def st {a:1} def ev {k:9} `

func TestCaseOpenCallDefaultRuntime(t *testing.T) {
	r := seam5Reg(t)
	// The open-call default collects its own WRITTEN args: g st ev → st.
	// The arm runs like a matched block — value pushed first, kept when
	// unconsumed — so the residual is [v, st].
	out, err := seam5Run(r, nur048Prelude+`case "z" ["q" ["quit"] g st ev]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].String() != "'z'" || out[1].String() != "{a:1}" {
		t.Fatalf("open-call default = %v, want ['z' {a:1}]", out)
	}
	// Negative twin (the pre-fix bug): the result must NOT be the arm's
	// last token (ev).
	if len(out) == 1 && out[0].String() == "{k:9}" {
		t.Fatal("open-call default leaked its last token (pre-NUR048 behaviour)")
	}
	// A matching pair still wins over the open-call default.
	out, err = seam5Run(r, nur048Prelude+`case "q" ["q" ["quit"] g st ev]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[1].String() != "'quit'" {
		t.Fatalf("matched pair should win, got %v", out)
	}
}

func TestCaseSingleFnWordDefaultDispatches(t *testing.T) {
	r := seam5Reg(t)
	// A single trailing fn word is an open-call head too: it dispatches
	// against the case value in isolation (before the fix it leaked to
	// the continuation as an inert word).
	out, err := seam5Run(r, `def h fn [[s:String] [String] [s add "!"]] case "z" ["q" ["quit"] h]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].String() != "'z!'" {
		t.Fatalf("fn-word default should dispatch against the value, got %v", out)
	}
	// Negative twin: a trailing word bound to a NON-fn value keeps the
	// plain default-clause reading (the value as-is).
	out, err = seam5Run(r, `def d "misc" case "z" ["q" ["quit"] d]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].String() != "'misc'" {
		t.Fatalf("non-fn word default should stay a plain value, got %v", out)
	}
}

func TestCaseOpenCallDefaultCheckMode(t *testing.T) {
	r := seam5Reg(t)
	// Check mode mirrors the runtime walk: the open-call arm joins as a
	// body (caseBranchJoin), and the arm counts as the catch-all for
	// exhaustiveness, so a dynamic scrutinee reports no findings.
	src := nur048Prelude +
		`def f fn [[x:Any] [Any] [case x ["q" ["quit"] g st ev]]] f "z"`
	if _, err := seam5Check(r, src); err != nil {
		t.Fatal(err)
	}
	for _, d := range r.Check.Diagnostics {
		if strings.Contains(d.Code, "case_not_exhaustive") {
			t.Fatalf("open-call default must count as the catch-all, got %+v", d)
		}
	}
}

func TestIsCaseOpenCallHeadGuards(t *testing.T) {
	r := seam5Reg(t)
	// Non-word elements are never heads.
	if isCaseOpenCallHead(r, NewInteger(1)) {
		t.Error("integer flagged as open-call head")
	}
	// An unbound word is not a head (keeps its atom-match reading).
	if isCaseOpenCallHead(r, NewWord("zed")) {
		t.Error("unbound word flagged as open-call head")
	}
	// A word bound to a non-fn value is not a head.
	if _, err := seam5Run(r, `def d "misc"`); err != nil {
		t.Fatal(err)
	}
	if isCaseOpenCallHead(r, NewWord("d")) {
		t.Error("non-fn binding flagged as open-call head")
	}
	// A def'd fn IS a head.
	if _, err := seam5Run(r, `def h fn [[s:String] [String] [s]]`); err != nil {
		t.Fatal(err)
	}
	if !isCaseOpenCallHead(r, NewWord("h")) {
		t.Error("def'd fn not flagged as open-call head")
	}
	// A parked Function binding IS a head.
	if _, err := seam5Run(r, `def hr (h/v)`); err != nil {
		t.Fatal(err)
	}
	if !isCaseOpenCallHead(r, NewWord("hr")) {
		t.Error("parked Function binding not flagged as open-call head")
	}
}
