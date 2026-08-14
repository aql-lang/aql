package check

import core "github.com/boru-lang/boru/core/go"

// Check-piece fn-analysis models extracted from core_helpers.go (Stage 2c
// of the four-piece split): the param->carrier builders the ReturnsFunc
// analysis binds, the residual-stack probes, the disjointness proof behind
// the return-conformance mirror, and the quote-param dispatch screen. All
// of it runs only under an active analysis pass.

// BuildFnBodyReturnsFn produces the check-mode ReturnsFn for one boru fn
// signature. In static-check mode the engine skips the runtime handler
// and calls this with carrier-typed args; it analyses the body via
// AnalyseFnBody (so body diagnostics propagate) and returns the carrier
// result — declared return types when present, else the analyser's
// residual top-of-stack carrier(s). It also performs record-shape
// checking for pattern params.
//
// Anonymous lambdas (afn / =>) declare Returns=[Any] as a placeholder;
// for them the declared-returns fast path is dropped so the analyser's
// inferred residual wins (this is what makes `([n:Integer] => [n add 1])`
// infer Integer rather than Any in check mode).
//
// Extracted verbatim from InstallFnDef so the same return-inference can
// be shared with the Function-value dispatch path.
// narrowArgsToParams returns args with each gradual (dynamic) arg whose bound
// is strictly BROADER than its declared param type narrowed to a dynamic
// carrier of that param type. The arg already passed the param match at the
// call site (a disjoint arg never reaches body analysis), so narrowing its
// gradual bound to the declared contract is sound and stays optimistic — it
// just ensures a body word sees the param's declared shape. Without it, a
// recursive call threading a `get`-result `dynamic(Any)` into a `ch:List` param
// makes the body's `each` match the map-each overload (→ Map) instead of
// list-each (→ List), so a downstream `all`/`any`/`[List]` consumer then fails
// no_signature against the spurious Map (the decision `eval-pred-all` cycle).
// A concrete arg, an arg already conforming to the param, or an Any/untyped
// param is left untouched (no precision lost).
// recordSchemaCarrier returns the body-analysis view of a map/gradual-Any arg
// bound to a RECORD-shape param (declared `c:SomeRecord` — Type=Map + a NewMap
// field→type schema pattern): a carrier CARRYING the record schema
// (RecordTypeInfo), so a body `c get "field"` recovers the field's declared type
// instead of degrading to Any. The arg already passed the param match at the
// call site and the runtime CALL_USER guard re-checks it, so committing to the
// schema for body analysis is sound; recovered field types are GRADUAL
// (getNodeReturns), discharged by a guard for a non-conforming value. Returns
// (zero,false) for Options-typed params (OptionsTypeInfo, not MapPayload),
// patternless params, and non-map args. Applied at BOTH body-analysis paths —
// narrowArgsToParams (result/summary) and the armed-compile genArgs — so the
// schema survives into the closure CAPTURE that the armed compile records.
func recordSchemaCarrier(p core.FnParam, a core.Value) (core.Value, bool) {
	if p.Pattern == nil || a.Parent == nil {
		return core.Value{}, false
	}
	if !(a.Parent.ConformsTo(core.TMap) || a.Parent.Equal(core.TAny)) {
		return core.Value{}, false
	}
	if pm, ok := p.Pattern.Data.(core.MapPayload); ok && pm.M != nil {
		// A fresh, distinct ID per carrier: the bytecode compiler keys a
		// param's frame-local slot by Value.ID (StartFnCompile →
		// RegisterLocal). A struct-literal carrier left with the zero ID would
		// make every record-typed param collapse onto the SAME slot (id ""),
		// so a fn reading >1 record param (`fn [[c:C d:D] …]`) miscompiles to a
		// single local → RunCompiledStrict VM error / wrong local. Mint a
		// unique node ID exactly as NewValueRaw does for ordinary values.
		return core.Value{ID: core.GenerateID(core.IDPrefixForType(core.TMap)), Parent: core.TMap, Carrier: true, Dynamic: true, Data: core.RecordTypeInfo{Fields: pm.M}}, true
	}
	return core.Value{}, false
}

// recordReturnCarrier is recordSchemaCarrier's RETURN-side twin (NUR068): a
// declared return naming a record type resolves to TMap plus a NewMap
// field→type schema pattern (ResolveSigType's Record rule — the nominal node
// is dropped so dispatch stays structural), and the carrier builders used to
// keep only the *Type, producing a bare Map carrier that NEITHER
// getNodeReturns recovery branch can read (no RecordTypeInfo payload, no
// schema-bearing Parent). Rebuild the schema-bearing carrier the make/param
// paths already produce — dynamic(Map) carrying RecordTypeInfo — so a field
// read through a constructor fn (`(mk) get "name"`) narrows exactly as
// `(make R {…}) get "name"` does. GRADUAL, never strict, for the same reason
// the param twin is: the runtime return check enforces the declared shape, so
// the schema claim only narrows reads; a strict shaped carrier could refuse
// dispatches today's bare Map carrier admits. Returns (zero,false) for a
// non-Map declared type, a patternless return, and non-record patterns
// (Options → OptionsTypeInfo, typed maps → ChildTypeInfo — both keep their
// existing carriers). The fresh ID follows the param twin's rule: result
// carriers are recorded per-value by the bytecode emitter (RecordUserCall
// provenance), so two record returns must not share one identity.
func recordReturnCarrier(t *core.Type, pat *core.Value) (core.Value, bool) {
	if t == nil || pat == nil || !t.Equal(core.TMap) {
		return core.Value{}, false
	}
	if pm, ok := pat.Data.(core.MapPayload); ok && pm.M != nil {
		return core.Value{ID: core.GenerateID(core.IDPrefixForType(core.TMap)), Parent: core.TMap, Carrier: true, Dynamic: true, Data: core.RecordTypeInfo{Fields: pm.M}}, true
	}
	return core.Value{}, false
}

// returnPatternAt reads a positional return pattern, tolerating a nil or
// short slice (ParseFnReturns allocates patterns only when some position
// has one) — the same leniency ReturnCheckInfo.ReturnPattern applies.
func returnPatternAt(pats []*core.Value, i int) *core.Value {
	if i < 0 || i >= len(pats) {
		return nil
	}
	return pats[i]
}

// typedContainerCarrier generalises a TYPED-container param ({:T} map / [:T] list)
// to a carrier that PRESERVES its declared ELEMENT type — the D2 read-precision
// foundation (design/TYPED-CONTAINER-ELEMENT-PRECISION.0.md, Part A). The element
// type rides on p.Pattern (a typed-map/list value carrying a ChildTypeInfo), NOT
// p.Type (which generalises to bare Node), so mirror recordSchemaCarrier: read the
// pattern and mint a FRESH ID so each typed-container param keys a DISTINCT
// frame-local slot (StartFnCompile → RegisterLocal; a zero ID collapses two
// params onto one slot). Without this, NewCarrier(a.Parent) collapses the child to
// Any and the body reads the container back as dynamic(Any). {:Any} (child Any)
// falls through so its reads stay dynamic(Any), unchanged. The CONTAINER carrier
// stays a plain (non-dynamic) typed carrier — the accessor makes the READ dynamic
// (Part B), keeping the container's own dispatch (`m size`) unchanged.
func typedContainerCarrier(p core.FnParam, a core.Value) (core.Value, bool) {
	if p.Pattern == nil || a.Parent == nil {
		return core.Value{}, false
	}
	pat := *p.Pattern
	ci, err := core.AsChildType(pat)
	if err != nil || ci.Child.Parent == nil || ci.Child.Parent.Equal(core.TAny) {
		return core.Value{}, false
	}
	if core.IsTypedMap(pat) && (a.Parent.ConformsTo(core.TMap) || a.Parent.Equal(core.TAny)) {
		v := core.NewTypedMap(ci.Child)
		v.ID = core.GenerateID(core.IDPrefixForType(core.TMap))
		v.Carrier = true
		v.Dynamic = true // a {:T} param admits a flex arg — in-place mutation must runtime-rematch, not preselect the immutable handler
		return v, true
	}
	if core.IsTypedList(pat) && (a.Parent.ConformsTo(core.TList) || a.Parent.Equal(core.TAny)) {
		v := core.NewCarrierTypedListValue(ci.Child)
		v.ID = core.GenerateID(core.IDPrefixForType(core.TList))
		v.Dynamic = true
		return v, true
	}
	return core.Value{}, false
}

// paramBodyCarrier builds a fn-body INPUT carrier for a param when no call-site
// arg is available: a pattern-aware typed-container carrier for a {:T}/[:T]
// param (so body reads narrow and disjoint uses are diagnosed), else the plain
// ParamInputCarrier(p.Type). The main genArgs path uses typedContainerCarrier
// with the actual arg; the poly-arm (user_poly.go) and construction-check
// (BuildFnBodyReturnsFn) body builders have only the param, so they route
// here — otherwise `m:{:Integer}`'s `p.Type` is bare Map and reads stay Any.
func ParamBodyCarrier(p core.FnParam) core.Value {
	if p.Pattern != nil {
		pat := *p.Pattern
		if ci, err := core.AsChildType(pat); err == nil && ci.Child.Parent != nil && !ci.Child.Parent.Equal(core.TAny) {
			if core.IsTypedMap(pat) {
				v := core.NewTypedMap(ci.Child)
				v.ID = core.GenerateID(core.IDPrefixForType(core.TMap))
				v.Carrier = true
				v.Dynamic = true
				return v
			}
			if core.IsTypedList(pat) {
				v := core.NewCarrierTypedListValue(ci.Child)
				v.ID = core.GenerateID(core.IDPrefixForType(core.TList))
				v.Dynamic = true
				return v
			}
		}
		// An INLINE union param — `x:(Integer tor String)`. ResolveSigType
		// hands the paren annotation's Disjunct back as the PATTERN with
		// Type=TAny (it has no minted lattice node to name), so without this
		// the body binds dynamic(Any) and every downstream analysis that
		// asks "what is this parameter?" is told "unknown" — a case over it
		// reports the scrutinee as dynamic despite the author having written
		// the union out in full.
		//
		// The named form `x:T` with `def T (Integer tor String)` binds the
		// DISTRIBUTING declared carrier (ParamInputCarrier's union arm). The
		// two spellings denote the same domain, so they get the same carrier;
		// Declared is set for the same reason it is there — the annotation
		// claims every alternative is a valid input, so a body dispatch that
		// fails for one is an error rather than an analysis-join warning.
		if core.IsDisjunct(pat) && (p.Type == nil || p.Type.Equal(core.TAny)) {
			if di, err := core.AsDisjunct(pat); err == nil && len(di.Alternatives) > 0 {
				dv := core.NewDisjunct(core.SimplifyDisjunctAlts(di.Alternatives))
				dv.Carrier = true
				if ndi, ok := dv.Data.(core.DisjunctInfo); ok {
					ndi.Declared = true
					dv.Data = ndi
				}
				return dv
			}
		}
	}
	return ParamInputCarrier(p.Type)
}

func narrowArgsToParams(args []core.Value, params []core.FnParam) []core.Value {
	var out []core.Value
	for i := range args {
		if i >= len(params) {
			break
		}
		a := args[i]
		pt := params[i].Type
		if rc, ok := recordSchemaCarrier(params[i], a); ok {
			if out == nil {
				out = append([]core.Value(nil), args...)
			}
			out[i] = rc
			continue
		}
		switch {
		case a.Dynamic && pt != nil && !pt.Equal(core.TAny) && a.Parent != nil &&
			!a.Parent.ConformsTo(pt):
			// A gradual arg whose bound is NOT already within the declared
			// (concrete) param type: re-bind the body's view of the param to a
			// dynamic carrier of the DECLARED type. The annotation is the
			// authoritative contract for body analysis — the body is written
			// assuming the param's type, and a dynamic arg means "statically
			// unknown, optimistically anything", so binding to dynamic(pt) keeps
			// optimism while letting the body's typed operations match. Covers
			// both a strictly-broader bound (`Any` → `String`) and a disjoint /
			// sibling bound (radix's edge splitter feeds a `slice`/`add` result
			// of unknown shape — a dynamic `List` — into `set-edge`'s
			// `newlabel:String`; without this the body's `newlabel slice 0 1`
			// stayed `List` and failed `set`'s String-key Map overload).
			if out == nil {
				out = append([]core.Value(nil), args...)
			}
			nc := core.NewCarrier(pt)
			nc.Dynamic = true
			out[i] = nc
		case !a.Dynamic && a.Carrier && core.IsDisjunct(a) && pt != nil && !pt.Equal(core.TAny):
			// A strict DISJUNCT carrier — the checker's ∀-abstraction of an
			// unresolved overload's returns — narrows to the declared param
			// type by the same contract as the armed-compile genArgs twin:
			// the entry guard / interpreter dispatch raises when the runtime
			// value misses pt, so assuming pt can only narrow (mini-s3's
			// `part` (a recovered slice's Bytes|List|String) into
			// s3-send-resp's body:Bytes — the chunk loop's `slice` then
			// matches mono instead of cascading no_signature).
			if out == nil {
				out = append([]core.Value(nil), args...)
			}
			out[i] = core.NewCarrier(pt)
		case !a.Dynamic && pt != nil && pt.Equal(core.TAny) && a.Parent != nil && a.Parent.Equal(core.TAny) && !core.IsBareTypeNode(a):
			// A STRICT Any arg bound to a declared-`Any` param. The param is
			// gradual by declaration (ParamInputCarrier gives dynamic Any), so a
			// body word over it must match optimistically — a strict Any conforms
			// to no concrete slot and would cascade no_signature. The case arises
			// when a fold accumulator that started `none` and was rebuilt into a
			// node is threaded into a `nd:Any` insert receiver (tst/radix's
			// `none entries [(acc … tst-insert)] fold`). Only a bare strict-Any
			// VALUE is lifted (not a typed/structural carrier).
			if out == nil {
				out = append([]core.Value(nil), args...)
			}
			nc := core.NewCarrier(core.TAny)
			nc.Dynamic = true
			out[i] = nc
		}
	}
	if out == nil {
		return args
	}
	return out
}

// stackHasFnValue reports whether any residual value is a Function —
// the shape whose static count can over-report (an unapplied fn-value call
// the interpreter applies at runtime; see emit.go's cluster-E refusal).
func stackHasFnValue(stk []core.Value) bool {
	for _, v := range stk {
		if v.Parent != nil && v.Parent.ConformsTo(core.TFunction) {
			return true
		}
	}
	return false
}

// stackHasDynamic reports whether any residual value is gradual (Dynamic)
// — the marker of a modelling seam where the analysis count may not equal
// the runtime count.
func stackHasDynamic(stk []core.Value) bool {
	for _, v := range stk {
		if v.Dynamic {
			return true
		}
	}
	return false
}

// stackHasApproxAny reports whether any residual value is a bare STRICT Any
// carrier — the lenient approximation BuildFnBodyReturnsFn leaves for a
// 0-net / undeclared call whose body unit declined (and the in-flight
// recursion bail). Such a value is a PHANTOM: the call nets zero (or an
// unknown count) at runtime, so a residual carrying one has a count the
// analysis does not know exactly — the same "inexact, skip the mirror"
// situation as a variadic / fn-value / dynamic residual. A declared `[Any]`
// return and the unbound-param degrade are marked Dynamic (caught by
// stackHasDynamic), so only the approximation phantom lands here.
func stackHasApproxAny(stk []core.Value) bool {
	for _, v := range stk {
		if v.Carrier && !v.Dynamic && v.Parent != nil && v.Parent.Equal(core.TAny) {
			return true
		}
	}
	return false
}

// posBefore reports whether source position (row, col) strictly precedes p
// in reading order. Used to test a diagnostic's attribution against a fn
// body's source span.
func posBefore(row, col int, p core.SrcPos) bool {
	return row < p.Row || (row == p.Row && col < p.Col)
}

// bodySpanEnd walks a parsed fn body's tokens (paren exprs, reaches, lists,
// map values — the exprRefsCarrier shapes) and returns the maximum source
// position seen, i.e. the start of the body's LAST token at any depth.
// Together with the first token's position it bounds the body's source span
// for diagnostic attribution. Zero when no token carries a position.
func bodySpanEnd(body []core.Value) core.SrcPos {
	var end core.SrcPos
	var walk func(vs []core.Value)
	walk = func(vs []core.Value) {
		for _, v := range vs {
			if posBefore(end.Row, end.Col, v.Pos()) {
				end = v.Pos()
			}
			if core.IsParenExpr(v) {
				if toks, err := core.AsParenExpr(v); err == nil {
					walk(toks)
				}
				continue
			}
			if core.IsReach(v) {
				if ri, err := core.AsReach(v); err == nil {
					walk(ri.Receiver)
					for i := range ri.Segments {
						if ri.Segments[i].Computed {
							walk(ri.Segments[i].KeyExpr)
						}
					}
				}
				continue
			}
			if lst, err := core.AsList(v); err == nil && !lst.IsNil() {
				walk(lst.Slice())
				continue
			}
			if mp, err := core.AsMap(v); err == nil && mp != nil {
				for _, k := range mp.Keys() {
					mv, _ := mp.Get(k)
					walk([]core.Value{mv})
				}
			}
		}
	}
	walk(body)
	return end
}

// residualProvablyDisjoint reports whether a body-residual value's type can
// NEVER inhabit the declared return type: no conformance in either direction
// AND a Never tand meet. The either-direction conformance mirrors
// sigTypeMatches' gradual rule (tand wrongly reports two container-family
// types as disjoint when one conforms to the other). A disjunct residual is
// disjoint only if every alternative is.
func residualProvablyDisjoint(got core.Value, exp *core.Type) bool {
	if core.IsDisjunct(got) {
		di, err := core.AsDisjunct(got)
		if err != nil || len(di.Alternatives) == 0 {
			return false
		}
		for _, alt := range di.Alternatives {
			probe := alt
			if core.IsBareTypeNode(alt) {
				probe = core.CarrierOfLiteral(alt)
			}
			if !residualProvablyDisjoint(probe, exp) {
				return false
			}
		}
		return true
	}
	p := got.Parent
	if p == nil || p.ConformsTo(exp) || exp.ConformsTo(p) {
		return false
	}
	// An OPAQUE union residual (Parent=Disjunct with no readable
	// alternatives — the payload was stripped upstream) denotes "one of
	// several types" — nothing is provable about it. And a Function
	// residual sits on the fn-value-call frontier: `v f/r apply` leaves an
	// UNAPPLIED Function in the abstract residual where the runtime calls
	// it (the "fn-value-call boundary" imprecision class), so a Function
	// residual routinely means "not modeled", not "returns a function".
	// Both stay with the runtime RET check.
	if p.ConformsTo(core.TDisjunct) || p.ConformsTo(core.TFunction) {
		return false
	}
	// A declared type that admits values by VALUE-level membership — a
	// disjunct/enum (`def T (Integer tor String)`), a negation, a predicate
	// type, a Go member type — accepts residuals whose TAG does not
	// nominally conform: the runtime `v.Is(T)` runs the membership, so
	// `[x]` against a declared T passes for an Integer x ∈ T. A nominal
	// disjointness proof says nothing there — skip. The carrier-Is probe is
	// the backstop for wrapped behaviors (a behave-augmented union still
	// answers membership through its Match chain).
	if membershipBeyondNominal(exp) || core.NewCarrier(p).Is(exp) {
		return false
	}
	return core.IsNeverShape(core.TandValues(core.NewCarrier(p), core.NewCarrier(exp)))
}

// membershipBeyondNominal reports whether t's installed Behavior admits
// values by VALUE-level membership rather than nominal tag conformance —
// the unifier families whose Match can accept a value from a foreign
// lattice family. Bare refines stay nominal (provable); DepScalar nodes
// are parented at their base scalar, so the conformance shortcut above
// already skips them — listed here as a belt.
func membershipBeyondNominal(t *core.Type) bool {
	if t == nil {
		return false
	}
	switch t.Behavior().(type) {
	case *core.DisjunctUnifier, *core.NegationUnifier, *core.PredicateUnifier, core.MemberUnifier, *core.DepScalarUnifier:
		return true
	}
	return false
}

// quoteParamCarrierBind reports whether a matched user-fn signature binds a
// COMPUTED (carrier) argument to an Atom-typed or /q param. Quote capture is
// forward-only: at the runtime pointer such a slot binds only a bare Word
// collected forward — a delivered stack value never matches (`def f fn
// [[k:Atom] [Atom] [k]] f 'meta'` raises no_signature on every engine).
// Check-mode matching is looser (a strict Atom carrier satisfies the slot),
// so a closure-unit compile that admits the bind records an application the
// runtime rejects. Consulted only inside closure-unit compilation — the one
// scope where the mismatch produced a compile≠interpret divergence.
func quoteParamCarrierBind(sigParams []core.FnParam, args []core.Value) bool {
	for i, p := range sigParams {
		if (p.Quote || (p.Type != nil && p.Type.ConformsTo(core.TAtom))) &&
			i < len(args) && args[i].Carrier {
			return true
		}
	}
	return false
}
