package help

import (
	"strings"
	"testing"
)

// Precedence reporting used to branch on a single binary flag, so any word
// whose signatures DISAGREED about argument sourcing advertised the forward
// equivalence chain for all of them — including spellings it refuses.
// `apply` is the recorded case (NUR023): its `[Function]` row is stack-only,
// so `apply f/r 5` raises a signature error while the help promised
// `apply x y <=> y apply x <=> y x apply`. `dot` (NUR049's subject) has
// sixteen mixed-barrier rows with the same problem.
//
// These tests pin the classifier in all three shapes and BOTH directions:
// a word that really is uniform must keep its unqualified line (no churn
// for the common case), and a word that is not must never claim one.

func fwdSig(args ...string) SigInfo {
	return SigInfo{Args: args, Returns: []string{"Any"}, BarrierPos: len(args)}
}

func stackSig(args ...string) SigInfo {
	return SigInfo{Args: args, Returns: []string{"Any"}, BarrierPos: 0}
}

// mixedSig is the shape the binary flag could not express: `barrier` leading
// positions are forward-eligible, the rest come from the stack.
func mixedSig(barrier int, args ...string) SigInfo {
	return SigInfo{Args: args, Returns: []string{"Any"}, BarrierPos: barrier}
}

// sentinelSig is a module-export wrapper's shape: BarrierPos -1, the
// unnormalized "all forward-eligible" sentinel. Natives get this rewritten
// to TotalArgs() at registration, FnDef wrappers do not.
func sentinelSig(args ...string) SigInfo {
	return SigInfo{Args: args, Returns: []string{"Any"}, BarrierPos: -1}
}

func TestPrecedenceShapeUniform(t *testing.T) {
	cases := []struct {
		name string
		info FuncInfo
		want precedenceKind
	}{
		{
			name: "every signature all-forward",
			info: FuncInfo{Name: "w", ForwardArgs: true,
				Sigs: []SigInfo{fwdSig("Integer", "Integer"), fwdSig("String")}},
			want: precedenceForward,
		},
		{
			name: "every signature stack-only",
			info: FuncInfo{Name: "w", ForwardArgs: false,
				Sigs: []SigInfo{stackSig("Integer"), stackSig("Integer", "Integer")}},
			want: precedenceStack,
		},
		{
			name: "zero-arg word keeps the flag's reading (NUR023 territory)",
			info: FuncInfo{Name: "now", ForwardArgs: false,
				Sigs: []SigInfo{stackSig()}},
			want: precedenceStack,
		},
		{
			name: "zero-arg word declared forward",
			info: FuncInfo{Name: "stdin", ForwardArgs: true,
				Sigs: []SigInfo{fwdSig()}},
			want: precedenceForward,
		},
		{
			name: "no signatures at all falls back to the flag",
			info: FuncInfo{Name: "w", ForwardArgs: true},
			want: precedenceForward,
		},
		{
			// The regression TestFormatDynamicModuleHeader caught: an
			// unnormalized -1 must not read as stack-only, or EVERY module
			// export reports the wrong precedence.
			name: "module wrapper's -1 sentinel is forward, not stack",
			info: FuncInfo{Name: "ArrayUtil.indices", ForwardArgs: true,
				Sigs: []SigInfo{sentinelSig("List", "List")}},
			want: precedenceForward,
		},
		{
			name: "sentinel beside an explicit all-forward row still agrees",
			info: FuncInfo{Name: "w", ForwardArgs: true,
				Sigs: []SigInfo{sentinelSig("A", "B"), fwdSig("A")}},
			want: precedenceForward,
		},
	}
	for _, c := range cases {
		if got := precedenceShape(c.info); got != c.want {
			t.Errorf("%s: precedenceShape = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestPrecedenceShapeMixed is the regression this exists for: each of these
// words would have claimed an unqualified precedence before the fix.
func TestPrecedenceShapeMixed(t *testing.T) {
	cases := []struct {
		name string
		info FuncInfo
	}{
		{
			// `apply`: a forward Reach row plus a stack-only Function row.
			name: "forward row beside a stack-only row",
			info: FuncInfo{Name: "apply", ForwardArgs: true,
				Sigs: []SigInfo{fwdSig("Reach", "Any"), stackSig("Function")}},
		},
		{
			// `dot`: mixed-barrier rows (receiver from the stack) beside
			// all-forward Error rows.
			name: "mixed-barrier row beside an all-forward row",
			info: FuncInfo{Name: "dot", ForwardArgs: true,
				Sigs: []SigInfo{mixedSig(1, "String", "Map"), fwdSig("String", "Error")}},
		},
		{
			name: "a single mixed-barrier row is already not uniform",
			info: FuncInfo{Name: "w", ForwardArgs: true,
				Sigs: []SigInfo{mixedSig(1, "String", "Map")}},
		},
		{
			name: "mixed-barrier beside stack-only",
			info: FuncInfo{Name: "w", ForwardArgs: true,
				Sigs: []SigInfo{mixedSig(1, "A", "B"), stackSig("A")}},
		},
	}
	for _, c := range cases {
		if got := precedenceShape(c.info); got != precedenceMixed {
			t.Errorf("%s: precedenceShape = %v, want precedenceMixed", c.name, got)
		}
		// And the rendered output must not assert either unqualified line.
		out := FormatDynamic(c.info)
		if strings.Contains(out, "Precedence: forward —") {
			t.Errorf("%s: claims unqualified forward precedence:\n%s", c.name, out)
		}
		if strings.Contains(out, "Precedence: stack —") {
			t.Errorf("%s: claims unqualified stack precedence:\n%s", c.name, out)
		}
		if !strings.Contains(out, "Precedence: varies by signature") {
			t.Errorf("%s: missing the varies-by-signature header:\n%s", c.name, out)
		}
	}
}

// TestPrecedenceMixedRenderNamesTheSplit checks the mixed rendering actually
// tells a reader something actionable: how many signatures fall in each
// group, and one spelling that always dispatches.
func TestPrecedenceMixedRenderNamesTheSplit(t *testing.T) {
	info := FuncInfo{Name: "apply", ForwardArgs: true,
		Sigs: []SigInfo{fwdSig("Reach", "Any"), stackSig("Function")}}
	out := FormatDynamic(info)
	for _, want := range []string{
		"Precedence: varies by signature",
		"1 signature looks ahead for every argument.",
		"1 signature takes every argument from the stack.",
		"Full stack form satisfies every signature:  y x apply",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("mixed render missing %q in:\n%s", want, out)
		}
	}

	// Plural agreement, and the mixed-barrier group's own line.
	many := FuncInfo{Name: "dot", ForwardArgs: true,
		Sigs: []SigInfo{
			mixedSig(1, "String", "Map"), mixedSig(1, "Atom", "Map"),
			fwdSig("String", "Error"),
		}}
	out = FormatDynamic(many)
	if !strings.Contains(out, "2 signatures take a leading argument forward and the rest from the stack.") {
		t.Errorf("expected the mixed-barrier group line:\n%s", out)
	}
	if !strings.Contains(out, "1 signature looks ahead") {
		t.Errorf("expected singular agreement for the forward group:\n%s", out)
	}
}

// TestPrecedenceMixedClampsWideSigs pins the >3-arg clamp on the mixed path,
// matching the forward and stack renderers.
func TestPrecedenceMixedClampsWideSigs(t *testing.T) {
	info := FuncInfo{Name: "wide", ForwardArgs: true,
		Sigs: []SigInfo{
			mixedSig(1, "A", "B", "C", "D"),
			stackSig("A"),
		}}
	out := FormatDynamic(info)
	if !strings.Contains(out, "z y x wide") {
		t.Errorf("mixed render should clamp to three vars:\n%s", out)
	}
}

// TestSigClassifiersBoundaries pins the two predicates at their edges — an
// off-by-one either way silently reclassifies whole words.
func TestSigClassifiersBoundaries(t *testing.T) {
	two := []string{"A", "B"}
	cases := []struct {
		barrier          int
		forward, stackOn bool
	}{
		{barrier: -1, forward: true, stackOn: false}, // sentinel: all forward
		{barrier: -2, forward: true, stackOn: false}, // any negative reads the same
		{barrier: 0, forward: false, stackOn: true},  // all stack
		{barrier: 1, forward: false, stackOn: false}, // mixed
		{barrier: 2, forward: true, stackOn: false},  // all forward
		{barrier: 3, forward: true, stackOn: false},  // over-wide, still forward
	}
	for _, c := range cases {
		s := SigInfo{Args: two, BarrierPos: c.barrier}
		if got := sigIsAllForward(s); got != c.forward {
			t.Errorf("BarrierPos %d: sigIsAllForward = %v, want %v", c.barrier, got, c.forward)
		}
		if got := sigIsStackOnly(s); got != c.stackOn {
			t.Errorf("BarrierPos %d: sigIsStackOnly = %v, want %v", c.barrier, got, c.stackOn)
		}
	}
}

// TestPrecedenceMixedIgnoresZeroArgRows pins that a zero-arg row does not by
// itself make a word "mixed" — it has no arguments to source, so it agrees
// with everything. Without this, any word carrying a nullary overload would
// be reported as varying.
func TestPrecedenceMixedIgnoresZeroArgRows(t *testing.T) {
	info := FuncInfo{Name: "w", ForwardArgs: true,
		Sigs: []SigInfo{fwdSig(), fwdSig("A", "B")}}
	if got := precedenceShape(info); got != precedenceForward {
		t.Errorf("a nullary row beside forward rows must stay forward, got %v", got)
	}
	info = FuncInfo{Name: "w", ForwardArgs: false,
		Sigs: []SigInfo{stackSig(), stackSig("A")}}
	if got := precedenceShape(info); got != precedenceStack {
		t.Errorf("a nullary row beside stack rows must stay stack, got %v", got)
	}

	// The RENDERER skips them too, and must not count one into a group:
	// a nullary row sources no arguments, so reporting it under "look
	// ahead" or "from the stack" would inflate whichever group it landed
	// in. `dot` and `apply` have no nullary overload, but nothing stops
	// a word from having one alongside disagreeing rows.
	mixedWithNullary := FuncInfo{Name: "w", ForwardArgs: true,
		Sigs: []SigInfo{fwdSig(), fwdSig("A", "B"), stackSig("A")}}
	if got := precedenceShape(mixedWithNullary); got != precedenceMixed {
		t.Fatalf("fixture must be mixed, got %v", got)
	}
	out := FormatDynamic(mixedWithNullary)
	for _, want := range []string{
		"1 signature looks ahead for every argument.",
		"1 signature takes every argument from the stack.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the nullary row was counted into a group; missing %q in:\n%s", want, out)
		}
	}
}

// TestExamplePrefixesFollowTheBarrier pins the second half of the
// mixed-precedence fix (PR #325 review, Codex P2): the Precedence
// section stopped claiming a spelling the engine refuses, but the
// generated EXAMPLES were still built from the word-wide flag, so
// `describe or` taught `or true false` — an all-forward spelling its
// [Boolean Boolean] BarrierPos-1 signature raises signature_error on.
// The prefix (how many args sit before the word) must come from each
// signature's own barrier: at most BarrierPos args may go forward.
func TestExamplePrefixesFollowTheBarrier(t *testing.T) {
	fwd := FuncInfo{Name: "w", ForwardArgs: true}
	stack := FuncInfo{Name: "w", ForwardArgs: false}
	cases := []struct {
		name string
		info FuncInfo
		sig  SigInfo
		want []int
	}{
		{"all-forward keeps prefix 0", fwd, fwdSig("A", "B"), []int{0}},
		{"sentinel -1 reads all-forward", fwd, sentinelSig("A", "B"), []int{0}},
		{"stack-only word places every arg before", stack, stackSig("A", "B"), []int{2}},
		// The regression: an `or`-shaped sig must place its stack arg
		// before the word — prefix 1, never 0.
		{"mixed barrier 1 of 2", fwd, mixedSig(1, "A", "B"), []int{1}},
		{"mixed barrier 1 of 3", fwd, mixedSig(1, "A", "B", "C"), []int{2}},
		{"mixed barrier 2 of 3", fwd, mixedSig(2, "A", "B", "C"), []int{1}},
		// A stack-only SIG on a word whose other sigs are forward.
		{"stack-only sig on a forward word", fwd, stackSig("A"), []int{1}},
		{"zero args yields no examples", fwd, fwdSig(), nil},
	}
	for _, c := range cases {
		got := examplePrefixes(c.info, c.sig)
		if len(got) != len(c.want) {
			t.Errorf("%s: prefixes = %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: prefixes = %v, want %v", c.name, got, c.want)
				break
			}
		}
	}
}

// TestMixedExamplesRespectTheBarrier drives the whole render: a mixed
// word's generated examples must never spell more forward args than the
// matched signature's barrier admits.
func TestMixedExamplesRespectTheBarrier(t *testing.T) {
	// An `or`-shaped word: two Boolean args, barrier 1. The only sound
	// two-arg spelling puts one arg before the word.
	info := FuncInfo{Name: "orlike", ForwardArgs: true,
		Sigs: []SigInfo{mixedSig(1, "Boolean", "Boolean")}}
	out := FormatDynamic(info)
	if strings.Contains(out, "orlike true false") {
		t.Errorf("the all-forward spelling is back — the sig refuses it:\n%s", out)
	}
	if !strings.Contains(out, " orlike ") && !strings.Contains(out, "\n  orlike") {
		// The example must still exist, in the one-before form.
		t.Errorf("no example generated for the mixed word:\n%s", out)
	}
}
