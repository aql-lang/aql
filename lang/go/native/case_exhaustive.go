package native

import (
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
// alternative provably unifies with the match (the same UnifyR relation
// caseClauses applies). Statically-opaque matches — code-body predicates
// (`[gt 3]`) and paren expressions — never contribute coverage, so
// "cannot prove exhaustive" findings can be conservative; a
// gradual dynamic(T) scrutinee skips the check entirely.
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
// not statically checkable — a gradual dynamic(T) carrier, a bare type
// node used as data, a structural marker — and the exhaustiveness check
// is skipped (never a finding).
func caseAlternatives(r *Registry, v Value) ([]Value, bool) {
	if v.Dynamic {
		return nil, false
	}
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

// caseClauseMatch is one resolved clause match of the static pass.
type caseClauseMatch struct {
	resolved Value
	// opaque marks a match whose runtime behaviour cannot be modelled
	// statically — a code-body predicate or a paren expression. Opaque
	// matches contribute no coverage but may match anything (they are
	// wildcards for reachability).
	opaque bool
	pos    SrcPos
}

// resolveCaseMatch mirrors caseClauses' match-position resolution: a bare
// word resolves through the def table (user types, def'd values) and then
// ResolveWordValue (true/false/type names, else an Atom of the name); a
// code-body list is a predicate and a paren expression evaluates at
// runtime — both opaque here.
func resolveCaseMatch(r *Registry, m Value) caseClauseMatch {
	cm := caseClauseMatch{resolved: m, pos: m.Pos()}
	if isCodeBody(m) || IsParenExpr(m) {
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
	return cm
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
			// Lattice containment: every value of the alt type is
			// construction-compatible with any of its ancestors.
			at := CanonicalType(r, &alt)
			return at.ConformsTo(t)
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
	alts, ok := caseAlternatives(r, v)
	if !ok || len(alts) == 0 {
		return
	}
	hasDefault := len(elems)%2 == 1

	matches := make([]caseClauseMatch, 0, len(elems)/2)
	for i := 0; i+1 < len(elems); i += 2 {
		matches = append(matches, resolveCaseMatch(r, elems[i]))
	}

	covered := make([]bool, len(alts))
	for mi, cm := range matches {
		// Unreachable-clause advisory — clause-vs-clause subsumption ONLY
		// (an earlier clause fully covers everything this match admits, so
		// it can never be the first to match). Deliberately independent of
		// the scrutinee's alternatives: a per-call-shape re-analysis
		// narrows the scrutinee (`f 5` re-runs the body with x:Integer),
		// and a domain-based rule would flag clauses that are live for the
		// declared union — this rule depends only on the clause list, so
		// it is stable across call shapes.
		if !cm.opaque {
			for mj := 0; mj < mi; mj++ {
				prev := matches[mj]
				if prev.opaque {
					continue
				}
				if caseMatchCovers(r, prev.resolved, cm.resolved, caseAltExpandDepth) {
					addCaseDiagnostic(r, "case_unreachable_clause",
						"case: clause "+cm.resolved.String()+" can never match — an earlier clause already covers every value it admits",
						caseFindingPos(cm.pos, clauses))
					break
				}
			}
		}
		if cm.opaque {
			continue
		}
		for ai, alt := range alts {
			if !covered[ai] && caseMatchCovers(r, cm.resolved, alt, caseAltExpandDepth) {
				covered[ai] = true
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
