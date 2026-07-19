package native

import (
	"math"
	"sort"
	"strings"

	eng "github.com/aql-lang/aql/eng/go"
)

// Static exhaustiveness checking for `case` (design/case-exhaustiveness.0.md).
//
// A default-less `case` whose clause matches do not provably cover the
// scrutinee's static type is a check ERROR (case_not_exhaustive): the
// uncovered runtime value would silently produce nothing. The trailing
// default is not required when the type-literal / value clauses cover every
// alternative of the scrutinee's type (a declared union's disjuncts, an
// enum's members, Boolean's true/false, a plain type, or a concrete value).
//
// Coverage is computed in the SOUND direction only — a clause is credited
// with covering an alternative only when every runtime inhabitant of the
// alternative provably matches it. Three coverage channels compose:
//
//   - value/type matches, via the same UnifyR relation caseClauses
//     applies, restricted by the NEWTYPE BOUNDARY (caseTypeCovers): a
//     base-type clause does not cover a user-minted newtype alternative;
//   - [is T] predicates, via the runtime is-relation (the plain lattice
//     walk — [is Integer] is the explicit family check);
//   - comparison predicates ([gt 3], [lte 3], [eq 0]) and DepScalar
//     refinement matches, via interval-union analysis over numeric
//     domains (ℤ-adjacency for the Integer family, the ℝ rule
//     otherwise) — a total pair like [gt 3]/[lte 3] proves Integer
//     covered.
//
// Unrecognized predicates and paren expressions stay opaque (no credit),
// so "cannot prove exhaustive" findings can be conservative. A gradual
// dynamic(T) scrutinee — including a code-body scrutinee, which computes
// its value — has no static type to prove coverage against, so it
// REQUIRES a trailing default or an Any clause.
//
// The same residual computation yields the two advisory duals (info,
// non-gating): a trailing default made unreachable because the clauses
// already cover every alternative (case_redundant_default), and a clause
// subsumed by the clauses before it (case_unreachable_clause).

// caseAltExpandDepth bounds the recursive expansion of union-typed
// alternatives so a (pathological) self-referential union cannot loop.
const caseAltExpandDepth = 8

// caseAlternatives decomposes the scrutinee's static shape into the
// alternatives a clause list must cover. ok=false means the scrutinee is
// not statically checkable — a bare type node used as data, a structural
// marker — and the exhaustiveness check is skipped (never a finding).
// Dynamic scrutinees never reach here: checkCaseExhaustiveness handles
// them first (they REQUIRE a default or an Any clause).
func caseAlternatives(r *Registry, v Value) ([]Value, bool) {
	if v.Carrier {
		var out []Value
		if !expandCaseAlt(r, v, caseAltExpandDepth, &out) { //covergate:allow expansion fails only on the provably-unreachable alternative shapes guarded inside expandCaseAlt (dynamic/depth/unrecognized)
			return nil, false
		}
		return out, true
	}
	if IsConcrete(v) || IsNoneShape(v) {
		// A concrete scrutinee is its own single alternative: coverage is
		// then value-precise (`case 2 [1 'a' 2 'b']` provably matches), and
		// only a provably-unmatched value demands more clauses or a default.
		return []Value{normalizeCaseNone(v)}, true
	}
	return nil, false
}

// expandCaseAlt appends the leaf alternatives of one scrutinee/union
// alternative to out. It returns false when the alternative is not
// statically representable (a gradual carrier, an exhausted depth bound,
// an unrecognized shape) — the caller then abandons the whole check
// rather than reason from a wrong model.
func expandCaseAlt(r *Registry, a Value, depth int, out *[]Value) bool {
	if a.Dynamic { //covergate:allow the root scrutinee is pre-checked by caseAlternatives, and disjunct alternatives are never Dynamic — JoinCarriers' gradual contagion makes a join with a dynamic arm dynamic as a WHOLE, and type-body alternatives (SimplifyDisjunctAlts / UnionCarrierForType) are literals or concretes
		return false
	}
	if IsDisjunct(a) {
		if depth <= 0 { //covergate:allow tor flattens nested disjunct VALUES (FlattenDisjunctAlts), so payload-carrying disjuncts never nest beyond the single root level and the recursion cannot exhaust the bound
			return false
		}
		info, err := AsDisjunct(a)
		if err != nil { //covergate:allow IsDisjunct already proved the DisjunctInfo payload
			return false
		}
		for _, alt := range info.Alternatives {
			if !expandCaseAlt(r, alt, depth-1, out) { //covergate:allow alternatives are SimplifyDisjunctAlts output (bare nodes or concrete values), for which the recursion cannot fail — the guarded failure modes (dynamic/depth/unrecognized) are unreachable from kernel-built alternative sets
				return false
			}
		}
		return true
	}
	if a.Carrier {
		// A non-disjunct carrier (an if/else join arm, a typed-list carrier)
		// contributes its Parent type as the alternative.
		if a.Parent == nil { //covergate:allow every kernel carrier constructor (NewCarrier and friends) mints from a non-nil *Type
			return false
		}
		return expandCaseAlt(r, NewTypeLiteral(a.Parent), depth, out)
	}
	if IsBareTypeNode(a) {
		t := CanonicalType(r, &a)
		if t.Equal(TBoolean) {
			// Boolean is an opaque scalar leaf in the lattice, not a
			// true|false disjunction — decompose it here so
			// `case b [true .. false ..]` proves exhaustive.
			*out = append(*out, NewBoolean(true), NewBoolean(false))
			return true
		}
		if dv, ok := eng.UnionCarrierForType(t); ok { //covergate:allow defensive arm — kernel paths deliver union-typed domains as DISTRIBUTING disjunct carriers (ParamInputCarrier / UnionCarrierForType at param+return sites), never as a bare union NODE; the arm keeps a future carrier shape sound instead of silently uncovered
			// A named union/enum type: its members are the alternatives.
			if depth <= 0 { //covergate:allow a union node recurses exactly once into its (flattened) distributing disjunct, so the bound cannot exhaust here
				return false
			}
			return expandCaseAlt(r, dv, depth-1, out) //covergate:allow same defensive arm — see the guard above
		}
		*out = append(*out, a)
		return true
	}
	if IsConcrete(a) {
		*out = append(*out, normalizeCaseNone(a))
		return true
	}
	return false //covergate:allow alternatives are carriers, bare type nodes, or concrete values — the arms above are exhaustive for every kernel-built alternative shape
}

// normalizeCaseNone folds every none-shaped value onto the canonical None
// node so the `none` word in match position and the `none` alternative of
// a union compare identically.
func normalizeCaseNone(v Value) Value {
	if IsNoneShape(v) {
		return NewTypeLiteral(TNone)
	}
	return v
}

// caseIval is the numeric set a comparison predicate or a DepScalar
// refinement covers: the values v with lo ≤/< v ≤/< hi. An absent end is
// unbounded; a point predicate ([eq c]) sets both ends to c inclusive.
type caseIval struct {
	lo, hi         float64
	hasLo, hasHi   bool
	loIncl, hiIncl bool
}

// caseClauseMatch is one resolved clause match of the static pass.
type caseClauseMatch struct {
	resolved Value
	// opaque marks a match whose runtime behaviour cannot be modelled
	// statically — an unrecognized code-body predicate or a paren
	// expression. Opaque matches contribute no coverage but may match
	// anything (they are wildcards for reachability).
	opaque bool
	// pred marks a code-body predicate that WAS statically recognized
	// (a comparison or an [is T]); resolved then still holds the raw
	// list, so the value-coverage path must not consume it.
	pred bool
	// ival is the numeric set this match covers — parsed from a
	// comparison predicate ([gt 3]), from an [is Big] over a DepScalar
	// refinement, or carried by a DepScalar type used directly as the
	// match. Interval-union coverage of a numeric alternative is
	// computed across all ival-bearing clauses.
	ival *caseIval
	// isT is the target of a recognized [is T] predicate: it covers an
	// alternative when every value of the alternative satisfies `is T`.
	isT *Type
	pos SrcPos
}

// resolveCaseMatch mirrors caseClauses' match-position resolution: a bare
// word resolves through the def table (user types, def'd values) and then
// ResolveWordValue (true/false/type names, else an Atom of the name). A
// code-body list is a predicate: comparison predicates ([gt 3]) and
// [is T] are statically recognized and contribute coverage; anything
// else — like a paren expression, which evaluates at runtime — is opaque.
func resolveCaseMatch(r *Registry, m Value) caseClauseMatch {
	cm := caseClauseMatch{resolved: m, pos: m.Pos()}
	if isCodeBody(m) {
		if iv, ok := parseCaseCmpPred(m); ok {
			cm.pred = true
			cm.ival = &iv
			return cm
		}
		if tgt, ok := parseCaseIsPred(r, m); ok {
			cm.pred = true
			if IsBareTypeNode(tgt) {
				cm.isT = CanonicalType(r, &tgt)
				return cm
			}
			if iv, ok := depScalarIval(tgt); ok {
				cm.ival = &iv
				return cm
			}
			cm.opaque = true //covergate:allow unreachable — parseCaseIsPred only returns bare type nodes and DepScalar bodies with a representable interval
			cm.pred = false
			return cm
		}
		cm.opaque = true
		return cm
	}
	if IsParenExpr(m) {
		cm.opaque = true
		return cm
	}
	if IsWord(m) {
		w, err := AsWord(m)
		if err != nil { //covergate:allow IsWord already proved the WordInfo payload
			cm.opaque = true
			return cm
		}
		if bound, ok := r.ResolveTypedName(w.Name); ok {
			cm.resolved = bound
		} else {
			// An unbound word resolves like the runtime does: true/false,
			// type names, else an Atom of the name (ResolveWordValue) —
			// which then simply covers nothing it shouldn't.
			cm.resolved = ResolveWordValue(m)
		}
	}
	cm.resolved = normalizeCaseNone(cm.resolved)
	// A DepScalar refinement type used directly as the match (`Big` for
	// `def Big (Integer gt 10)`) keeps its value-level unify semantics
	// AND contributes its bounds to interval-union coverage.
	if cm.resolved.IsDepScalar() {
		if iv, ok := depScalarIval(cm.resolved); ok {
			cm.ival = &iv
		}
	}
	return cm
}

// parseCaseCmpPred recognizes the comparison-predicate shape
// `[<op> <numeric literal>]` (op ∈ gt/gte/lt/lte/eq) and returns the
// numeric set it covers. Anything else — other ops, non-literal bounds,
// longer bodies — is not recognized and stays opaque.
func parseCaseCmpPred(m Value) (caseIval, bool) {
	lst, _ := AsList(m)
	if lst.IsNil() || lst.Len() != 2 {
		return caseIval{}, false
	}
	opv, arg := lst.Get(0), lst.Get(1)
	if !IsWord(opv) {
		return caseIval{}, false
	}
	w, err := AsWord(opv)
	if err != nil { //covergate:allow IsWord already proved the WordInfo payload
		return caseIval{}, false
	}
	bound, ok := caseNumericLiteral(arg)
	if !ok {
		return caseIval{}, false
	}
	switch w.Name {
	case "gt":
		return caseIval{lo: bound, hasLo: true}, true
	case "gte":
		return caseIval{lo: bound, hasLo: true, loIncl: true}, true
	case "lt":
		return caseIval{hi: bound, hasHi: true}, true
	case "lte":
		return caseIval{hi: bound, hasHi: true, hiIncl: true}, true
	case "eq":
		return caseIval{lo: bound, hi: bound, hasLo: true, hasHi: true, loIncl: true, hiIncl: true}, true
	}
	return caseIval{}, false
}

// parseCaseIsPred recognizes `[is <TypeName>]` and returns the resolved
// target — a bare type node, or a DepScalar body for a predicate
// refinement name.
func parseCaseIsPred(r *Registry, m Value) (Value, bool) {
	lst, _ := AsList(m)
	if lst.IsNil() || lst.Len() != 2 {
		return Value{}, false
	}
	opv, arg := lst.Get(0), lst.Get(1)
	if !IsWord(opv) {
		return Value{}, false
	}
	w, err := AsWord(opv)
	if err != nil { //covergate:allow IsWord already proved the WordInfo payload
		return Value{}, false
	}
	if w.Name != "is" {
		return Value{}, false
	}
	tgt := arg
	if IsWord(tgt) {
		tw, err := AsWord(tgt)
		if err != nil { //covergate:allow IsWord already proved the WordInfo payload
			return Value{}, false
		}
		if bound, ok := r.ResolveTypedName(tw.Name); ok {
			tgt = bound
		} else {
			tgt = ResolveWordValue(tgt)
		}
	}
	if IsBareTypeNode(tgt) && !IsNoneShape(tgt) {
		return tgt, true
	}
	if tgt.IsDepScalar() {
		if _, ok := depScalarIval(tgt); ok {
			return tgt, true
		}
	}
	return Value{}, false
}

// caseNumericLiteral reads a concrete Integer or Float literal as the
// float64 bound of a comparison predicate.
func caseNumericLiteral(v Value) (float64, bool) {
	if !IsConcrete(v) {
		return 0, false
	}
	if v.Parent.ConformsTo(TInteger) {
		if n, err := AsInteger(v); err == nil {
			return float64(n), true
		}
		return 0, false //covergate:allow a concrete Integer always reads as an int64
	}
	if v.Parent.ConformsTo(TFloat) {
		if f, err := AsFloat(v); err == nil {
			return f, true
		}
		return 0, false //covergate:allow a concrete Float always reads as a float64
	}
	return 0, false
}

// depScalarIval converts a DepScalar refinement body (`Integer gt 10`)
// into the numeric set it admits. ok=false when a bound is not a plain
// numeric literal or the base family is not numeric.
func depScalarIval(v Value) (caseIval, bool) {
	if !v.Parent.ConformsTo(TNumber) {
		return caseIval{}, false
	}
	info, err := v.AsDepScalar()
	if err != nil { //covergate:allow IsDepScalar already proved the DepScalarInfo payload
		return caseIval{}, false
	}
	var iv caseIval
	if info.Lo != nil {
		b, ok := caseNumericLiteral(info.Lo.Value)
		if !ok { //covergate:allow a numeric DepScalar bound is always a plain Integer/Float literal — an arbitrary-precision literal cannot be written (integer_overflow at parse)
			return caseIval{}, false
		}
		iv.lo, iv.hasLo, iv.loIncl = b, true, info.Lo.Inclusive
	}
	if info.Hi != nil {
		b, ok := caseNumericLiteral(info.Hi.Value)
		if !ok { //covergate:allow a numeric DepScalar bound is always a plain Integer/Float literal — an arbitrary-precision literal cannot be written (integer_overflow at parse)
			return caseIval{}, false
		}
		iv.hi, iv.hasHi, iv.hiIncl = b, true, info.Hi.Inclusive
	}
	return iv, true
}

// caseTypeCovers is the type-level coverage rule: every value of the
// alternative type at is admitted by a match denoting t. Lattice
// containment, restricted by the NEWTYPE BOUNDARY: a user-minted
// alternative (a `refine` newtype, a predicate refinement, a class) is
// covered only within its minted lineage — the base type does not cover
// the newtype, so a Pos-typed scrutinee demands a Pos clause (or an
// [is …] predicate, or a default), not an Integer clause. `Any` is
// exempt: it is the written catch-all, semantically a default.
func caseTypeCovers(at, t *Type) bool {
	if t.Equal(TAny) {
		return true
	}
	if !at.ConformsTo(t) {
		return false
	}
	if at.Equal(t) {
		return true
	}
	if at.Origin == eng.OriginUserDef && t.Origin != eng.OriginUserDef {
		return false
	}
	return true
}

// caseIsCovers reports whether an [is T] predicate covers alternative
// alt: every value of the alternative satisfies `is T`. The is-relation
// walks the plain lattice (a Pos value IS an Integer), so no newtype
// boundary applies here — [is Integer] is the explicit family check.
func caseIsCovers(r *Registry, t *Type, alt Value) bool {
	if IsBareTypeNode(alt) {
		at := CanonicalType(r, &alt)
		return at.ConformsTo(t)
	}
	if IsConcrete(alt) {
		return alt.Is(t)
	}
	return false //covergate:allow alternatives are bare nodes or concrete values (expandCaseAlt)
}

// caseMatchCovers reports whether every runtime inhabitant of alternative
// alt provably unifies with match m — the sound "m fully covers alt"
// relation. m and alt are resolved, none-normalized values.
func caseMatchCovers(r *Registry, m, alt Value, depth int) bool {
	if depth <= 0 { //covergate:allow tor flattens nested disjuncts, so the recursion (one level per disjunct payload or union node) cannot exhaust the bound
		return false
	}
	if IsDisjunct(m) {
		// A disjunction value in match position unifies when SOME
		// alternative unifies (unifyDisjunct), so its coverage is the
		// union of its alternatives' coverage.
		info, err := AsDisjunct(m)
		if err != nil { //covergate:allow IsDisjunct already proved the DisjunctInfo payload
			return false
		}
		for _, m2 := range info.Alternatives {
			if caseMatchCovers(r, normalizeCaseNone(m2), alt, depth-1) {
				return true
			}
		}
		return false
	}
	if IsBareTypeNode(m) {
		t := CanonicalType(r, &m)
		if dv, ok := eng.UnionCarrierForType(t); ok { //covergate:allow defensive arm — ResolveTypedName yields a union's BODY (a payload-carrying disjunct value, the IsDisjunct branch above), never its minted node; the arm keeps a future node-resolving path sound
			// A named union/enum type as a match covers what its members cover.
			return caseMatchCovers(r, dv, alt, depth-1)
		}
		if IsBareTypeNode(alt) {
			return caseTypeCovers(CanonicalType(r, &alt), t)
		}
		if IsConcrete(alt) {
			// Membership of the concrete alternative — runs the real
			// Behavior.Match (predicate refinements decide exactly).
			return alt.Is(t)
		}
		return false //covergate:allow alternatives and resolved matches are bare nodes or concrete values (expandCaseAlt / resolveCaseMatch), so no third shape reaches here
	}
	if IsConcrete(m) && IsConcrete(alt) {
		// Value-level coverage — the runtime's own unify relation, so
		// numeric-leaf exactness (2.0 vs 2) is decided identically.
		_, ok := UnifyR(m, alt, r)
		return ok
	}
	return false
}

// caseIvalsCover reports whether the union of the clause intervals
// covers every value of the numeric alternative type at. Integer-family
// alternatives merge with ℤ-adjacency ((-∞,3] ∪ [4,∞) covers ℤ); every
// other numeric family uses the ℝ rule, which is also the conservative
// answer for Number as a whole (ℝ coverage implies every numeric leaf).
func caseIvalsCover(ivals []caseIval, at *Type) bool {
	if len(ivals) == 0 || !at.ConformsTo(TNumber) {
		return false
	}
	if at.ConformsTo(TInteger) {
		return caseIvalsCoverInts(ivals)
	}
	return caseIvalsCoverReals(ivals)
}

// caseIntBounds normalizes one interval to closed INTEGER ends: the
// integers admitted are [lo, hi] (ok=false for an empty integer set).
func caseIntBounds(iv caseIval) (lo, hi float64, hasLo, hasHi, ok bool) {
	lo, hi, hasLo, hasHi = iv.lo, iv.hi, iv.hasLo, iv.hasHi
	if hasLo {
		// v > 3 → v ≥ 4; v > 3.5 → v ≥ 4; v ≥ 3.5 → v ≥ 4; v ≥ 3 → v ≥ 3.
		if iv.loIncl {
			lo = math.Ceil(lo)
		} else {
			lo = math.Floor(lo) + 1
		}
	}
	if hasHi {
		if iv.hiIncl {
			hi = math.Floor(hi)
		} else {
			hi = math.Ceil(hi) - 1
		}
	}
	if hasLo && hasHi && lo > hi {
		return 0, 0, false, false, false
	}
	return lo, hi, hasLo, hasHi, true
}

// caseIvalsCoverInts merges ℤ-normalized intervals and reports whether
// they span every integer.
func caseIvalsCoverInts(ivals []caseIval) bool {
	type span struct {
		lo, hi       float64
		hasLo, hasHi bool
	}
	spans := make([]span, 0, len(ivals))
	for _, iv := range ivals {
		lo, hi, hasLo, hasHi, ok := caseIntBounds(iv)
		if ok {
			spans = append(spans, span{lo, hi, hasLo, hasHi})
		}
	}
	if len(spans) == 0 {
		return false
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].hasLo != spans[j].hasLo {
			return !spans[i].hasLo
		}
		return spans[i].lo < spans[j].lo
	})
	cur := spans[0]
	if cur.hasLo {
		return false // no interval reaches -∞
	}
	for _, s := range spans[1:] {
		if !cur.hasHi {
			return true
		}
		if s.hasLo && s.lo > cur.hi+1 {
			return false // an integer gap
		}
		if !s.hasHi {
			cur.hasHi = false
		} else if s.hi > cur.hi {
			cur.hi = s.hi
		}
	}
	return !cur.hasHi
}

// caseIvalsCoverReals merges intervals with open/closed ends and reports
// whether they span all of ℝ.
func caseIvalsCoverReals(ivals []caseIval) bool {
	spans := make([]caseIval, len(ivals))
	copy(spans, ivals)
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].hasLo != spans[j].hasLo {
			return !spans[i].hasLo
		}
		if spans[i].lo != spans[j].lo {
			return spans[i].lo < spans[j].lo
		}
		return spans[i].loIncl && !spans[j].loIncl
	})
	cur := spans[0]
	if cur.hasLo {
		return false
	}
	for _, s := range spans[1:] {
		if !cur.hasHi {
			return true
		}
		if s.hasLo {
			if s.lo > cur.hi {
				return false
			}
			if s.lo == cur.hi && !s.loIncl && !cur.hiIncl {
				return false // both ends open at the same point: it is missed
			}
		}
		if !s.hasHi {
			cur.hasHi = false
		} else if s.hi > cur.hi || (s.hi == cur.hi && s.hiIncl && !cur.hiIncl) {
			cur.hi, cur.hiIncl = s.hi, s.hiIncl
		}
	}
	return !cur.hasHi
}

// caseIvalContainsPoint reports whether one interval admits the value x —
// the coverage question for a CONCRETE numeric alternative.
func caseIvalContainsPoint(iv caseIval, x float64) bool {
	if iv.hasLo && (x < iv.lo || (x == iv.lo && !iv.loIncl)) {
		return false
	}
	if iv.hasHi && (x > iv.hi || (x == iv.hi && !iv.hiIncl)) {
		return false
	}
	return true
}

// addCaseDiagnostic appends a case coverage finding, deduplicated by
// code+position (detail excluded): caseReturnsFn runs once per analysed
// call shape, and a later shape re-analyses the same site with a NARROWED
// scrutinee (`f 5` re-runs the body with x:Integer) whose detail differs —
// the first finding, from the generalized declared-domain analysis, is the
// authoritative one. Deliberately NOT CheckAddUniqueDiagnostic: these
// findings are static judgements, not guaranteed-runtime-error mirrors, so
// they must not carry the RuntimeMirror exemption from the compile
// pipeline's error refusal.
func addCaseDiagnostic(r *Registry, code, detail string, pos SrcPos) {
	for _, d := range r.Check.Diagnostics {
		if d.Code == code && d.Row == pos.Row && d.Col == pos.Col &&
			!d.CaughtAtRuntime {
			return
		}
	}
	r.Check.AddDiagnostic(CheckDiagnostic{
		Code:   code,
		Detail: detail,
		Word:   "case",
		Row:    pos.Row,
		Col:    pos.Col,
	})
}

// renderCaseAlts renders a list of alternatives for a diagnostic detail.
func renderCaseAlts(alts []Value) string {
	parts := make([]string, 0, len(alts))
	for _, a := range alts {
		parts = append(parts, a.String())
	}
	return strings.Join(parts, ", ")
}

// checkCaseExhaustiveness is the shared static coverage pass over a case's
// raw clause elements, run by caseReturnsFn BEFORE the plain-check /
// compile-desugar paths diverge so both report identical findings.
//
//   - no default and some alternative uncovered → case_not_exhaustive (error)
//   - default present and every alternative covered → case_redundant_default (info)
//   - a clause that can never be the first to match → case_unreachable_clause (info)
func checkCaseExhaustiveness(r *Registry, v Value, clauses Value, elems []Value) {
	// A CONCRETE scrutinee inside a fn body is a per-call-shape artefact:
	// AnalyseFnBody re-runs the body with the actual call argument bound
	// (`f 9` re-analyses with x=9 even when the param is declared :Any),
	// so a value-level finding there describes one call, not the code.
	// Concrete scrutinees are checked at the top level only; declared
	// type domains (carriers) are checked everywhere — under-coverage is
	// caught by the construction-time generalized analysis, and a
	// narrowed call-shape domain can only shrink the uncovered set.
	if !v.Carrier && r.Check.FnBodyDepth > 0 {
		return
	}
	hasDefault := len(elems)%2 == 1

	matches := make([]caseClauseMatch, 0, len(elems)/2)
	for i := 0; i+1 < len(elems); i += 2 {
		matches = append(matches, resolveCaseMatch(r, elems[i]))
	}

	// Unreachable-clause advisory — clause-vs-clause subsumption ONLY
	// (an earlier clause fully covers everything this match admits, so it
	// can never be the first to match). Deliberately independent of the
	// scrutinee's alternatives: a per-call-shape re-analysis narrows the
	// scrutinee (`f 5` re-runs the body with x:Integer), and a
	// domain-based rule would flag clauses that are live for the declared
	// union — this rule depends only on the clause list, so it is stable
	// across call shapes.
	for mi, cm := range matches {
		for mj := 0; mj < mi; mj++ {
			if caseClauseSubsumes(r, matches[mj], cm) {
				addCaseDiagnostic(r, "case_unreachable_clause",
					"case: clause "+cm.resolved.String()+" can never match — an earlier clause already covers every value it admits",
					caseFindingPos(cm.pos, clauses))
				break
			}
		}
	}

	// A DYNAMIC scrutinee has no static type to prove coverage against,
	// so the clause list must carry its own catch-all: a trailing
	// default, or an Any clause (the written equivalent).
	if v.Dynamic {
		if hasDefault || caseHasAnyClause(r, matches) {
			return
		}
		addCaseDiagnostic(r, "case_not_exhaustive",
			"case: the scrutinee is dynamic — no static type can prove the clauses cover it; add a trailing default (or an Any clause)",
			caseFindingPos(v.Pos(), clauses))
		return
	}

	alts, ok := caseAlternatives(r, v)
	if !ok || len(alts) == 0 {
		return
	}

	covered := make([]bool, len(alts))
	var ivals []caseIval
	for _, cm := range matches {
		if cm.ival != nil {
			ivals = append(ivals, *cm.ival)
		}
		if cm.opaque {
			continue
		}
		for ai, alt := range alts {
			if covered[ai] {
				continue
			}
			switch {
			case cm.isT != nil:
				covered[ai] = caseIsCovers(r, cm.isT, alt)
			case cm.pred:
				// A recognized comparison predicate covers only through
				// the interval UNION, computed after this loop.
			default:
				covered[ai] = caseMatchCovers(r, cm.resolved, alt, caseAltExpandDepth)
			}
		}
	}
	// Interval-union pass: the comparison predicates and DepScalar
	// matches jointly cover a numeric alternative when their intervals
	// span its whole domain ([gt 3] and [lte 3] together cover Integer),
	// or admit the concrete alternative's exact value.
	if len(ivals) > 0 {
		for ai, alt := range alts {
			if covered[ai] {
				continue
			}
			if IsBareTypeNode(alt) {
				covered[ai] = caseIvalsCover(ivals, CanonicalType(r, &alt))
				continue
			}
			if x, ok := caseNumericLiteral(alt); ok {
				for _, iv := range ivals {
					if caseIvalContainsPoint(iv, x) {
						covered[ai] = true
						break
					}
				}
			}
		}
	}

	var uncovered []Value
	for ai, alt := range alts {
		if !covered[ai] {
			uncovered = append(uncovered, alt)
		}
	}

	pos := caseFindingPos(v.Pos(), clauses)
	if !hasDefault && len(uncovered) > 0 {
		addCaseDiagnostic(r, "case_not_exhaustive",
			"case: clauses do not cover the scrutinee type — uncovered: "+
				renderCaseAlts(uncovered)+
				"; add matching clauses or a trailing default",
			pos)
		return
	}
	// The redundant-default advisory fires only over a DECLARED union
	// domain (a named-union param annotation, DisjunctInfo.Declared) —
	// the one alternative set that is stable and author-intent-backed.
	// A per-call-shape re-analysis narrows the scrutinee (`f true` binds
	// a Boolean param to the concrete true), under which any default
	// looks redundant even though other call shapes need it.
	if hasDefault && len(uncovered) == 0 && len(matches) > 0 && caseDeclaredDomain(v) {
		addCaseDiagnostic(r, "case_redundant_default",
			"case: the clauses already cover every alternative of the scrutinee's declared union — the trailing default is unreachable",
			pos)
	}
}

// checkCaseCodeBodyScrutinee applies the dynamic-scrutinee rule to the
// `case [body] [clauses]` shape: a code-body scrutinee dispatches on a
// computed value the checker does not model (it degrades to dynamic), so
// the clause list must carry its own catch-all — a trailing default or
// an Any clause.
func checkCaseCodeBodyScrutinee(r *Registry, v Value, clauses Value) {
	if !isCodeBody(clauses) { //covergate:allow caseReturnsFn's stack-form swap moves a non-code-body clause arg OUT of the code-body-scrutinee branch, so clauses is always a code body here
		return
	}
	lst, _ := AsList(clauses)
	if lst.IsNil() { //covergate:allow AsList of a concrete code-body list is never nil (an empty list is non-nil with zero elements)
		return
	}
	elems := lst.Slice()
	if len(elems)%2 == 1 {
		return
	}
	matches := make([]caseClauseMatch, 0, len(elems)/2)
	for i := 0; i+1 < len(elems); i += 2 {
		matches = append(matches, resolveCaseMatch(r, elems[i]))
	}
	if caseHasAnyClause(r, matches) {
		return
	}
	addCaseDiagnostic(r, "case_not_exhaustive",
		"case: the scrutinee is dynamic — no static type can prove the clauses cover it; add a trailing default (or an Any clause)",
		caseFindingPos(v.Pos(), clauses))
}

// caseHasAnyClause reports whether some clause match is the Any type
// literal — a written catch-all equivalent to a trailing default.
func caseHasAnyClause(r *Registry, matches []caseClauseMatch) bool {
	for _, cm := range matches {
		if cm.opaque || cm.pred {
			continue
		}
		m := cm.resolved
		if IsBareTypeNode(m) && CanonicalType(r, &m).Equal(TAny) {
			return true
		}
	}
	return false
}

// caseClauseSubsumes reports whether the earlier clause prev fully
// covers everything the later clause cur could match — the
// unreachable-clause relation. Only provable pairings count: value/type
// vs value/type via caseMatchCovers, interval vs interval by
// containment, [is T] vs [is T] by lattice containment.
func caseClauseSubsumes(r *Registry, prev, cur caseClauseMatch) bool {
	if prev.opaque || cur.opaque {
		return false
	}
	if prev.ival != nil && cur.ival != nil {
		return caseIvalContains(*prev.ival, *cur.ival)
	}
	if prev.isT != nil && cur.isT != nil {
		return cur.isT.ConformsTo(prev.isT)
	}
	if prev.pred || cur.pred || prev.ival != nil || cur.ival != nil {
		return false
	}
	return caseMatchCovers(r, prev.resolved, cur.resolved, caseAltExpandDepth)
}

// caseIvalContains reports a ⊇ b for two numeric sets.
func caseIvalContains(a, b caseIval) bool {
	if a.hasLo {
		if !b.hasLo {
			return false
		}
		if b.lo < a.lo || (b.lo == a.lo && b.loIncl && !a.loIncl) {
			return false
		}
	}
	if a.hasHi {
		if !b.hasHi {
			return false
		}
		if b.hi > a.hi || (b.hi == a.hi && b.hiIncl && !a.hiIncl) {
			return false
		}
	}
	return true
}

// caseFindingPos attributes a finding: the given position when the value
// carries one, else the clause list's — carriers and synthesized values are
// Pos-less, while the clause list is parsed source and always attributed.
func caseFindingPos(pos SrcPos, clauses Value) SrcPos {
	if pos.Row == 0 {
		return clauses.Pos()
	}
	return pos
}

// caseDeclaredDomain reports whether the scrutinee carries a DECLARED
// union domain — a check-mode disjunct carrier minted from a named-union
// fn-param annotation (ParamInputCarrier sets DisjunctInfo.Declared).
func caseDeclaredDomain(v Value) bool {
	if !v.Carrier || !IsDisjunct(v) {
		return false
	}
	info, err := AsDisjunct(v)
	if err != nil { //covergate:allow IsDisjunct already proved the DisjunctInfo payload
		return false
	}
	return info.Declared
}
