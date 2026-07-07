package eng

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/apd/v3"
)

// ReadList is a read-only view of a list of Values.
// Node list values expose this via AsList(). To mutate, use AsMutableList().
type ReadList struct {
	elems []Value
}

// NewReadList wraps a slice of Values as a ReadList. External callers
// (outside the aqleng package) use this constructor because the elems
// field is unexported.
func NewReadList(elems []Value) ReadList {
	return ReadList{elems: elems}
}

// Get returns the element at index i.
// Internal use only — caller must ensure 0 <= i < Len().
func (l ReadList) Get(i int) Value {
	return l.elems[i]
}

// GetOk returns the element at index i and true, or the zero Value and false
// if i is out of bounds. Safe for use at system boundaries.
func (l ReadList) GetOk(i int) (Value, bool) {
	if i < 0 || i >= len(l.elems) {
		return Value{}, false
	}
	return l.elems[i], true
}

// Len returns the number of elements.
func (l ReadList) Len() int {
	return len(l.elems)
}

// Slice returns a copy of the underlying slice.
func (l ReadList) Slice() []Value {
	out := make([]Value, len(l.elems))
	copy(out, l.elems)
	return out
}

// IsNil reports whether this ReadList has no backing data.
func (l ReadList) IsNil() bool {
	return l.elems == nil
}

// ReadMap is a read-only view of an ordered key-value map.
// Node values (Map, Options) expose this interface via AsMap().
// To mutate, use AsMutableMap() which is only valid for Object instances.
type ReadMap interface {
	Get(key string) (Value, bool)
	Keys() []string
	SortedKeys() []string
	Len() int
}

// OrderedMap is a map that preserves insertion order of keys.
type OrderedMap struct {
	keys     []string
	vals     map[string]Value
	Implicit bool           // true when created from implicit pair syntax (e.g., [x:Integer])
	Meta     map[string]any // optional metadata for parser/engine communication
}

// NewOrderedMap creates an empty OrderedMap.
func NewOrderedMap() *OrderedMap {
	return &OrderedMap{vals: make(map[string]Value)}
}

// Set adds or updates a key-value pair, preserving insertion order.
func (m *OrderedMap) Set(key string, val Value) {
	if _, exists := m.vals[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.vals[key] = val
}

// Get retrieves a value by key.
func (m *OrderedMap) Get(key string) (Value, bool) {
	v, ok := m.vals[key]
	return v, ok
}

// Keys returns the keys in insertion order (defensive copy).
func (m *OrderedMap) Keys() []string {
	out := make([]string, len(m.keys))
	copy(out, m.keys)
	return out
}

// SortedKeys returns the keys in sorted order (for deterministic comparison).
func (m *OrderedMap) SortedKeys() []string {
	sorted := make([]string, len(m.keys))
	copy(sorted, m.keys)
	sort.Strings(sorted)
	return sorted
}

// Len returns the number of entries.
func (m *OrderedMap) Len() int {
	return len(m.keys)
}

// Delete removes a key-value pair. Returns true if the key existed.
func (m *OrderedMap) Delete(key string) bool {
	if _, exists := m.vals[key]; !exists {
		return false
	}
	delete(m.vals, key)
	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}
	return true
}

// PathonInfo holds the data for a Scalar/Micron/Pathon value.
// A Path represents a filesystem path as a sequence of parts.
// Absolute paths start from the root (Abs = true).
type PathonInfo struct {
	Parts []string // path segments (e.g. ["usr", "local", "bin"])
	Abs   bool     // true for absolute paths (e.g. /usr/local/bin)
}

// String returns the OS path string for this path.
func (p PathonInfo) String() string {
	joined := strings.Join(p.Parts, "/")
	if p.Abs {
		return "/" + joined
	}
	return joined
}

// ChildTypeInfo holds the child-type constraint for a typed list or
// typed map.
//
// For example, [:String] constrains all list elements to be strings,
// and {:String} constrains all map values to be strings. Elements is
// non-nil when the source carried both literal elements AND a child
// constraint (e.g. `[{x:1} :{x:Integer} {x:2}]`); each element is
// validated against Child by `is` and similar predicates.
type ChildTypeInfo struct {
	Child    Value
	Elements []Value // optional: concrete elements alongside the child constraint
	Entries  []ChildEntry
	// Len is an optional statically-known length for a typed-list
	// carrier (nil = unknown). Set by length-producing words whose
	// result length can be computed exactly in check mode (e.g. iota),
	// it lets StaticListLen recover a bound for a computed list so the
	// index checker can flag a provably out-of-range access. It MUST be
	// an exact length or an upper bound — never an underestimate, which
	// would turn an in-bounds access into a false positive.
	Len *int
}

// ChildEntry is a (key, value) pair retained for typed maps that
// carry concrete entries alongside their child constraint
// (`{a:1 :{x:Integer} b:2}`).
type ChildEntry struct {
	Key   string
	Value Value
}

// RecordTypeInfo holds the field schema for a record type.
// Each field maps a name to a type-constraint Value (e.g. a type literal).
// A record type unifies with a concrete map if it has exactly the right keys
// and each value unifies with the corresponding field type.
type RecordTypeInfo struct {
	Fields *OrderedMap // field name → type-constraint Value
}

// OptionsTypeInfo holds the field schema for an options type.
// Each field maps a name to a default value or type constraint.
// Concrete values serve as defaults when the key is absent during unification;
// type literals require the caller to provide a value.
type OptionsTypeInfo struct {
	Fields *OrderedMap // field name → default value or type constraint
}

// TableTypeInfo holds the record schema for a table type.
// A table represents a list of record instances that all conform to the
// same record type.
type TableTypeInfo struct {
	Record RecordTypeInfo // the record type that each row must match
}

// FnParam describes one parameter in a function signature.
type FnParam struct {
	Name     string // empty for unnamed positional parameters
	Type     *Type
	Pattern  *Value // optional: map/list pattern for structural matching
	Optional bool   // true if this param was marked optional via ?
	// Quote marks a /q param (`name:Atom/q` in an AQL input sig): the
	// slot captures an upcoming bare Word as its Atom NAME during
	// collection — the arg is presented as if quoted — with the same
	// binding-agnostic rule native QuoteArgs slots follow (`def`,
	// `inspect`, `quote`: the capture trumps any def binding of the
	// word). normalizeSig merges the flag into Signature.QuoteArgs,
	// which is the field every dispatch-side reader consults.
	Quote bool
}

// BarrierAllForward is the canonical sentinel for "no `|` boundary
// specified — default this sig to all-forward dispatch." Resolved
// at registration time (upsertFnDef) to len(Args). Stack-only sigs
// must set BarrierPos: 0 explicitly; this constant exists only for
// the all-forward default.
const BarrierAllForward = -1

// FnSig describes one overload of a function definition.
type FnSig struct {
	Params  []FnParam
	Returns []*Type // declared return types (nil = unchecked)
	// BarrierPos is the forward/stack boundary expressed by `|` in
	// an AQL fn parameter list. Three values carry distinct meaning:
	//
	//   -1 — unset (no `|` token in the source). Consumers
	//        (InstallFnDef, fnSigsToSignatures) default this to
	//        len(Params) so the fn behaves as all-forward, matching
	//        the convention `RegisterNativeFunc` already applies to
	//        natives that omit the field.
	//    0 — explicit all-stack. The user wrote `[| a b]` so every
	//        arg comes from the stack. Distinct from -1 because the
	//        default bump must NOT fire here.
	//   >0 — explicit barrier at position N. Args[0..N-1] are
	//        forward-eligible; args[N..] come from the stack.
	//
	// Go-side construction sites that don't go through ParseFnParams
	// (modules/math.go, modules/matrix.go, etc.) leave the field at
	// the Go zero value (0); the consumer treats that as "default"
	// for backwards compatibility — only the AQL-source path emits
	// the -1 sentinel.
	BarrierPos int
	// NoEvalArgs lists per-sig-position the list-shaped args that
	// must NOT be auto-evaluated when consumed by this fn. Mirrors
	// Signature.NoEvalArgs so module FnDef wrappers can preserve
	// quoted code bodies passed at unnamed-param positions (e.g.
	// `rand.list-of [body] N` — body is code, not data).
	//
	// Honored by fnSigsToSignatures (forwards to Signature.NoEvalArgs)
	// and by execFnDefSig's auto-eval guard before CallAQL. Without
	// this, an Eval=true list passed in would be silently sub-Run'd,
	// breaking the quoted-body contract.
	NoEvalArgs map[int]bool
	// NoEvalMapArgs is the map-shaped counterpart to NoEvalArgs.
	// Used by sigs that take a Map at a code-body slot (e.g. a spec
	// schema where map values are quoted generators).
	NoEvalMapArgs map[int]bool
	// RawParens marks arg positions where a forward ParenExpr is captured
	// RAW (not pre-evaluated) so the handler receives the paren as code.
	// Opt-in; see Signature.RawParens and design/PAREN-REPRESENTATION.9.md.
	RawParens map[int]bool
	// FormArgs marks arg positions captured as a raw FORM — a generalization
	// of RawParens (don't pre-eval a paren) and QuoteArgs (capture a bare word
	// as data) to ANY operand: a word stays a Word, a paren/list/literal is
	// captured unevaluated, with no def resolution, no dispatch, and no
	// Word→Atom coercion. The macro definer sets it on every param so a macro
	// receives its operands as code. See design/MACROS-PHASE1.10.md §3.
	FormArgs map[int]bool

	// --- Run implementation + dispatch metadata. ---

	// Impl is the signature's run implementation as a sealed sum
	// (GoImpl | AQLImpl) — the SINGLE representation every reader consults
	// through the Signature accessors (dispatchHandler / body / fnFrame /
	// fullStack / checkFullStackFn / parkResult / runInCheckMode in
	// sigimpl.go). Native words and internal Go sites author `Go(handler,
	// opts...)`; module refs / un-installed lambdas author `AQL(body)`;
	// InstallFnDef / compileFnDefLiteral build the installed-fn `AQLImpl`
	// (with a derived body-splicer + frame meta) directly. A Signature with
	// a nil Impl has no implementation (a check-mode shape synth, a raw
	// match-only test fixture) and dispatchHandler() returns nil.
	Impl SigImpl
	// Args / Patterns are the exported positional constructor-convenience
	// mirrors of Params (see signature.go; kernel reads Params).
	Args     []*Type
	Patterns map[int]Value
	// QuoteArgs / TypeArgs are per-position dispatch modifiers.
	QuoteArgs map[int]bool
	TypeArgs  map[int]bool
	// FnInertArgs marks per-position slots where a FN-VALUED operand is INERT
	// DATA for the bytecode recorder — read or compared, never invoked — on a
	// word that is NOT wholesale CompileReadsFn because ANOTHER slot does
	// invoke a fn. The one holder today is `is` (Stage M2d): its VALUE slot
	// only reads the operand's lattice tag (`(+re/…/) is (MiniLang.Re)`),
	// while a Function in its TYPE slot is a predicate the handler INVOKES
	// (`5 is Positive` — RunPredicate) and must keep refusing. Read by
	// recordCallOperands alongside CompileReadsFn/CompileStoresFn.
	FnInertArgs map[int]bool
	// Fallback marks the synthesized 0-arg catch-all sig.
	Fallback bool
	// ReturnsFn is the check-mode return computer (native-authored or
	// AQL-derived); orthogonal to the run implementation in Impl.
	ReturnsFn ReturnsFunc
	// CompileEffect declares the word's compile-relevant semantics for the
	// bytecode recorder, so it can classify the word WITHOUT a name-keyed table
	// in eng (which couples the engine to specific, often module, word names).
	// The zero value is CompileDefault (an ordinary word). Set on the Signature
	// at registration; copied here by RegisterNativeFunc.
	CompileEffect CompileEffect
	// Callable, when non-nil, declares this word as a code-body higher-order
	// word whose body compiles to a closure unit (each / fold / do / a test
	// case body, …). The bytecode recorder reads the closure shape from here —
	// the body operand's position, its per-invocation output count, and the
	// input carriers it consumes — instead of a name-keyed eng table, so eng
	// names no specific (often module) word. Copied from NativeFunc.Callable
	// onto every signature at registration. nil = not closure-eligible.
	Callable *CallableSpec

	// Locked marks a signature registered through the Go registration
	// layer (Registry.Register — every native / kernel word plus host
	// words). Locked signatures can never be replaced or removed by a
	// def-merge (design/OPEN-WORDS.0.md §2.3), and they sort FIRST in
	// match order (CompareSignatures), so an unlocked merged addition
	// can never pre-empt a locked match — no previously-valid call
	// changes its dispatch. Locking is a property of the Go layer, not
	// an AQL language ability; InstallFnDef (user `def … fn`) never
	// sets it.
	Locked bool
	// Origin records which module contributed this signature via an
	// export transplant (the module ref, e.g. "./ext.aql" or
	// "aql:time-util"). Empty for native registrations and direct user
	// defs. Read by the transplant collision check: the same tuple
	// arriving from a DIFFERENT module raises [aql/extend_conflict],
	// while identical provenance (diamond re-import) is idempotent.
	Origin string
}

// CompileEffect is a set of compile-relevant capability flags a word declares
// so the bytecode recorder can classify it from the Signature rather than from
// name-keyed tables — decoupling eng from specific (often module) word names.
// It is a BITFIELD: the flags are orthogonal and a word may carry several (e.g.
// `typeof` reads a fn value AND is a pure module reader AND re-dispatches as an
// island-pure word). The zero value, CompileDefault, is an ordinary word.
type CompileEffect uint16

const (
	// CompileReadsFn marks an INTROSPECTION word that READS a fn value's
	// immutable shape (typeof / inspect / arityof / type-algebra) and never
	// invokes it. The fn bakes as a const the handler inspects — and because only
	// the shape is read, even a CAPTURING fn is safe to bake (its captures are
	// irrelevant to its signature).
	CompileReadsFn CompileEffect = 1 << iota
	// CompileStoresFn marks a word that STORES a fn value for later
	// interpreter-side invocation (minilang / parselang register), never the VM
	// tape. A fn-valued operand rides as an inert const; but unlike
	// CompileReadsFn, only a PURE fn literal bakes (a capturing / sub-registry fn
	// declines at isInertConst), because the stored fn is invoked later and must
	// keep its real binding.
	CompileStoresFn
	// CompileModuleFold marks a PURE reader word (get / getr / convert / typeof /
	// is / size / has) whose result over an import-bound module value plus inert
	// const operands is deterministic — so the recorder const-folds it.
	CompileModuleFold
	// CompileIslandPure marks a PURE typed-dispatch word (get / getr / size /
	// make / is / typeof / type-algebra) with no side effects: where the checker
	// could not commit to one overload, the word may run as an interpreter island
	// that re-dispatches on the real runtime value (and a stack-form island when
	// it has no threaded body inputs).
	CompileIslandPure
	// CompileFallbackBody marks a code-body higher-order word (each / fold / scan
	// / filter / select / group / do / case / where / having / order / …) that
	// may compile its body as a Stage-5 interpreter island.
	CompileFallbackBody
	// CompileQuoteInert marks a word whose implicit-quote (QuoteArgs) operand is
	// INERT DATA the handler consumes verbatim — a quoted symbol (`quote name`,
	// `raise bad_input …`) or a quoted code body held as data (`timeout 1000
	// [body]`) — so the word bakes as a plain CALL_NATIVE once each quoted operand
	// resolves to an inert const: the VM runs the SAME handler over the SAME baked
	// value as the interpreter. It is the opt-in for the QuoteArgs refusal, the
	// declared analogue of the get/getr/set exemption. Do NOT set it on a
	// dispatch-manipulating meta word (usurp / force-arity / ref) whose quoted
	// operand drives a RE-STEPPING result the VM cannot reproduce by re-running
	// the handler.
	CompileQuoteInert
	// CompileDiverges marks a word whose handler ALWAYS raises (it never returns
	// normally) — `raise`, the user-error constructor. A call to it is recorded as
	// a CALL_NATIVE (the handler raises the byte-identical error at run time) but
	// the recorder treats it as a DIVERGENT terminal: control never reaches the
	// fragment's end, so a closure body ending in it (`do [raise …]`, `error`'s
	// handler tail) compiles with NO RET — the error propagates out of the VM and
	// the catching word (`do` / `error`) turns it into the Error value via
	// InvokeBody, exactly as the interpreter does. This is the bytecode side of
	// structured exception handling: a trapping closure body is catchable rather
	// than uncompilable. The divergence is sound in a branch/loop arm too (the
	// arm never produces a value, like break/continue).
	CompileDiverges
	// CompileExecutesBody marks a word whose NoEvalArgs body is CODE the handler
	// SPLICES onto the tape for re-execution (a block-with-locals word like `var`),
	// as opposed to one it READS or STORES as data (a query clause, a Test.prop
	// spec, a timeout body). Such a handler RETURNS tape-coupled tokens
	// (def/body/undef, mark/move) the interpreter re-steps — which the VM cannot
	// run — so the recorder must REFUSE it (Stage 2 code-body) even when the body
	// is an inert word-list that would otherwise pass noEvalBodiesInert and bake as
	// a CALL_NATIVE. Without this flag `var [[v] …]` baked to a CALL_NATIVE whose
	// handler then tripped the VM's tape-coupled-result screen at run time. Closure-
	// compilable body words (each / fold / do — they declare a CallableSpec) take
	// the closure path before reaching the refusal, so they do NOT set this.
	CompileExecutesBody
	// CompileRunsBodyIsolated marks a word whose NoEvalArgs body(ies) are NEITHER
	// spliced onto the tape NOR const-baked and re-run in the enclosing sub-engine.
	// Instead the handler executes each body via a fresh, ISOLATED CallAQL frame
	// against a registry the word CAPTURED at registration (not the passed-in `r`),
	// binding only that body's own parameters. Test.check-prop is the canonical
	// case: runCheckProp runs the generator body (param `r` = a seeded rand
	// instance) and the property body (one unnamed param) each through
	// parent.CallAQL, so name resolution inside a body is IDENTICAL under the
	// interpreter and the VM -- it never touches a compiled frame local, because the
	// CallAQL frame binds only the body's params and resolves everything else
	// against the captured parent (module / global scope), exactly as it does when
	// the interpreter drives the same handler. So a plain CALL_NATIVE bake is sound
	// even when the body is a DYNAMIC value (a map get, `p get "gen"`) whose tokens
	// are not statically inert -- the VM evaluates the operand to the same List value
	// and hands it to the same handler. The flag exempts ONLY the inert-scoped
	// disjunct of the code-body refusal; a word that ALSO splices its body
	// (CompileExecutesBody) or declares a body-executing CallableSpec still refuses.
	CompileRunsBodyIsolated

	// CompileScalarFold marks a PURE value-level word (the comparison family:
	// eq/neq/deq/cmp/tcmp/lt/lte/gt/gte) whose dispatch over ALL-inert-const
	// operands the check pass may CONST-FOLD by running the real handler
	// (tryFoldScalarConst, carrier.go): the concrete result replaces the
	// declared-type carrier, so a literal condition like `(n eq 0)` with a
	// const-bound n folds to a concrete boolean and downstream `if` analysis
	// sees a statically-determined branch instead of a Disjunct residual
	// (the forward-barrier.tsv:83 soundness pin). Folding is double-evaluated
	// with an agreement guard exactly like CompileModuleFold, so a
	// non-deterministic handler never freezes; an erroring dispatch (the
	// family-restricted `lt` on cross-family operands) declines the fold and
	// keeps today's diagnostic path.
	CompileScalarFold
	// CompileValueDiverges marks a word that diverges VALUE-DEPENDENTLY: it
	// returns its declared result for most operands but RAISES for a specific
	// statically-decidable operand shape — `div` / `mod` by a static-zero
	// integer divisor (`1 div 0`). Unlike CompileDiverges (ALWAYS raises), the
	// divergence is per-call, so the recorder infers it from the word's own
	// check-mode ReturnsFn: when the ReturnsFn produced NO result (the divergent
	// path — returnsDivMod returns nil for a static-zero divisor), the call
	// raises and is recorded as a divergent terminal (like raise), so a closure
	// body ending in it (`do [1 div 0]`) compiles with no RET and the catching
	// `do` turns the raised error into an Error value — instead of islanding.
	// The flag scopes the "0 results ⇒ diverges" inference to these words: a
	// genuinely void word (print/set, declared 0-result) is unaffected.
	CompileValueDiverges
)

// CompileDefault is an ordinary word: no compile-relevant capability. A
// fn-valued operand reaching it means the handler invokes the fn on the tape,
// which the VM cannot honour, so the recorder refuses (Stage 3).
const CompileDefault CompileEffect = 0

// Has reports whether the effect set includes flag f.
func (e CompileEffect) Has(f CompileEffect) bool { return e&f != 0 }

// CallableSpec declares a code-body higher-order word's closure-compilation
// shape, so the bytecode recorder reads it from the resolved Signature
// (sig.Callable) rather than a name-keyed eng table — decoupling eng from the
// (often module) word names that own these words. A word declares one on its
// NativeFunc; RegisterNativeFunc copies the pointer onto every signature. nil
// on a Signature means the word is not closure-eligible.
type CallableSpec struct {
	// BodyPos is the body operand's sig position (the code list / lambda).
	BodyPos int
	// BodyOut is how many values the body nets per invocation: 1 for a
	// map/transform body (each / fold / do), 0 for a SIDE-EFFECT body (a test
	// case whose assertions raise on failure and otherwise leave nothing). It
	// sets the compiled unit's declared return count.
	BodyOut int
	// EmptyBodyErrors marks a word whose driving HANDLER itself raises a
	// runtime error when an invocation's body nets 0 values — each / fold /
	// scan raise `<word>_error "body produced no result"` from their InvokeBody
	// loop (invokeBodyTop / the list eachHandler return the empty residual and
	// the handler raises). For such a word a 0-net body is NOT a compile
	// refusal: the body compiles as a count-AGNOSTIC closure (declared nil
	// returns, like a side-effect body) so the handler raises the byte-identical
	// error at run time, instead of refusing the closure and islanding to let
	// the interpreter raise. The handler arbitrates the residual uniformly —
	// it takes the LAST value and errors on none — so a 1- or N-value body is
	// handled identically to before; only the unenforced count differs.
	EmptyBodyErrors bool
	// Inputs returns the per-invocation input carriers the body consumes, in the
	// order the driving handler supplies them via InvokeBody. The carriers are
	// GENERALISED types (not one call's concrete values) so the body compiles
	// once for every invocation. args is the call's full operand list, so a word
	// can derive the input type from its data operand (e.g. each's element type)
	// or its seed (fold's accumulator). Returns an empty slice for a 0-input body
	// (do / a test case). nil declines the compile (an unexpected operand shape).
	Inputs func(args []Value) []Value
	// BodyResultTop marks a driving handler that reads only the TOP of each
	// invocation's body residual (`res[len(res)-1]`) — each / fold / scan / filter.
	// The body may then leave UNCONSUMED values below that top (notably the
	// per-invocation INPUT a body that ignores its element leaves on the stack,
	// `each [add 1 0]` → `[input, 3]`), and the compiler may safely DROP them at
	// the RET: the handler never reads below the top. A handler that reads the
	// WHOLE residual (`do`, returning every value) leaves this false, so its
	// closure keeps the strict in-order reconciliation. Verified against the
	// handlers (native_array.go each/fold/scan, native/filter.go) — each takes
	// res[len(res)-1].
	BodyResultTop bool
	// CrossCollectionTokenShape marks a word whose TOKEN-quotation body is
	// SHAPE-GENERIC across the word's List-vs-Map overloads: both present the
	// closure the bare element/value (ClosureInValue), never a KeyVal. So a
	// gradual-Any (Dynamic) collection — statically ambiguous between the List and
	// Map overload — can still compile: the recorder commits the first reachable
	// (List) overload and lowers the token body to ONE closure, and the committed
	// handler is RUNTIME-ROBUST (it delegates to the sibling collection's iteration
	// when the value's concrete type is the other one), so the SAME closure drives
	// either shape == the interpreter, which dispatches the overload by the runtime
	// type. Without this flag a gradual collection refuses (the ambiguous-overload
	// MarkUncompilable below). Set ONLY on words whose every List/Map token overload
	// is ClosureInValue AND whose handlers cross-delegate (each/fold/scan). A LAMBDA
	// body never reaches this — it matches only the single TFunction overload, so
	// the ≥2-reachable ambiguity gate never fires for it.
	CrossCollectionTokenShape bool
	// LambdaSharesTokenShape marks a word that presents a LAMBDA callback the
	// SAME per-invocation inputs as a token-quotation body — Inputs(args) is the
	// single callback convention (walk's `{key value path parent depth}` payload
	// map, handed to a quotation on the stack and to a lambda as its one named
	// param). The recorder then compiles a lambda body against Inputs(args)
	// directly (ClosureInValue), instead of consulting the per-word
	// lambdaCallbackInputs table whose shapes (pair maps, KeyVals) only fit the
	// words that present DIFFERENT views to lambdas vs quotations.
	LambdaSharesTokenShape bool
}

// FnDefInfo holds the function specification for a def-defined function.
// Name is the function's registered name (set by InstallDef). If Registry is
// non-nil, the function was defined in a module and should execute in that
// registry's context (closure semantics).
//
// Signatures is the SINGLE per-function signature slice — one full-fidelity
// overload per entry. Each Signature carries the authored shape (Params with
// names, Returns, and the AQL Body) AND, once compiled, the dispatch fields
// (a Go Handler, resolved BarrierPos, sorted order). Body vs Handler is the
// only Go-vs-AQL distinction: a Go builtin has a Handler and no Body, an AQL
// fn carries Body tokens and (after install/compile) a body-splicing Handler.
//
// The slice on a DefStack entry or a constructed Function value holds only
// THAT definition's own overloads — it is NOT the cross-stack dispatch table.
// The accumulated, sorted, fallback-bearing dispatch table is built on demand
// at the registry boundary (Registry.Lookup → aggregateDispatch); every
// matcher / forward-planner reads the aggregate's Signatures, while the
// authored-shape readers (canon, inspect, predicate probes, trivial-delegation,
// targeted undef, overlap) read a single definition's own Signatures, skipping
// any synthetic Fallback. Whether the engine tries forward collection is
// determined per-signature via Signature.BarrierPos — derive the word-level
// summary via fn.HasForwardSigs.
type FnDefInfo struct {
	Name           string
	Signatures     []Signature // own full-fidelity overloads (see doc above)
	MaxForwardArgs int         // longest forward arg count across all sigs (respecting barriers)
	Registry       *Registry
	// Module/Export/Doc record a function's origin and one-line summary
	// when it is a native-module export (e.g. ArrayUtil.indices). They are
	// the provenance `describe` renders for a qualified name. Module is the
	// import id ("aql:array-util"), Export the namespace ("ArrayUtil"), Doc
	// a one-line summary. All three are empty for user/anonymous fns and
	// core words, which carry no module origin.
	Module string
	Export string
	Doc    string
	// MiniKind names the mini-language kind whose expansion produced
	// this partially-applied Function ("" for everything else). The
	// per-kind member types the aql:minilang module exports
	// (MiniLang.Re, MiniLang.Gex, …) match on it, so a typed fn param
	// can require a specific kind's partial (e.g. a regexp matcher).
	MiniKind string
	// Anonymous is true iff the FnDef was produced by the `afn` word (i.e.
	// via the `=>` lambda sugar). The flag is read only in check mode: an
	// anonymous fn's static Returns is the conservative [Any], and the
	// check-mode dispatch path runs AnalyseFnBody to recover the real
	// return type for downstream type propagation. Named fns leave this
	// false and the check-mode path uses sig.Returns as authored.
	Anonymous bool
	// Macro is true iff the FnDef was produced by the `macro` definer. A
	// macro is an fn the expander runs on UNEVALUATED operand forms (every
	// param is FormArgs raw-capture; §3 of design/MACROS-PHASE1.10.md), whose
	// returned token list is spliced into the call site rather than left as a
	// value. Read at dispatch (stepWord / execFnDefLiteral) to branch to the
	// expander before normal forward collection. Unlike Anonymous (check-mode
	// only), Macro gates runtime dispatch.
	Macro bool
	// Gen carries the generic-parameter spec for a generic fn
	// (`def identity gen [T] fn [[x:T] [T] [x]]`). Nil for ordinary
	// fns. Dispatch admission rides the placeholder nodes' Behaviors;
	// at each call the inferred bindings are installed as body-scoped
	// type bindings so `of [T]` / `make (Box of [T])` resolve. See
	// design/GENERICS.10.md Phase 4.
	Gen *GenSpecInfo
	// Captured holds enclosing-fn-local bindings snapshotted at fn-
	// construction time — the implementation of lexical closures.
	// Populated by computeCaptures during afn / fn handler execution
	// (fn_capture.go) when at least one body-Word resolves to a name
	// bound by an enclosing fn (Depth > TopFnBaseline). Installed as
	// defs in the per-call scope BEFORE body execution and torn down
	// by the same DefCleanup mechanism that pops named params; the
	// install/cleanup wiring lives in InstallFnDef (core_helpers.go),
	// execFnDefSig + spliceAnonCheckResult (engine.go), and CallAQL
	// (registry.go). Nil for top-level constructions and any inner
	// fn whose body references only params, module-global names, or
	// forward refs. See lang/go/CLAUDE.md "Closures and Capture".
	Captured []CapturedBinding
	// Extends marks this FnDefInfo as a WORD-EXTENSION CLONE: the result
	// of `def <word> fn […]` on a word that carries locked signatures
	// (design/OPEN-WORDS.0.md §2.1). The value is the base word's name.
	// A clone carries the base word's COMPLETE signature list plus the
	// merge, so Registry.Lookup stops aggregating at a clone — deeper
	// def-stack entries for the name are occluded, which is what makes
	// an unlocked-tuple REPLACEMENT effective and `undef` restore the
	// exact previous state. Detected via IsWordExtension (the named-
	// helper protocol — never probe the field inline); recognised at
	// module import for the export transplant. Empty for every ordinary
	// fn / native registration.
	Extends string
}

// CapturedBinding is one lexically-captured name in a closure. The
// list is sorted by Name for deterministic install order so that two
// captures with the same name (impossible today but cheap to keep
// reproducible) get a stable shadowing order.
type CapturedBinding struct {
	Name  string
	Value Value
}

// HasForwardSigs reports whether any compiled signature has a non-zero
// BarrierPos — i.e. at least one signature wants to collect args from
// tokens following the word. Used by the dispatcher to decide whether
// pre-evaluation of upcoming parens is worthwhile and whether a
// stack-only retry should be attempted on first-pass match failure.
// The result is derived from the sigs, not stored — Signature is the
// source of truth.
func (fn *FnDefInfo) HasForwardSigs() bool {
	if fn == nil {
		return false
	}
	for i := range fn.Signatures {
		if fn.Signatures[i].BarrierPos > 0 {
			return true
		}
	}
	return false
}

// OwnSigs returns the function's authored overloads — its Signatures with
// any synthetic 0-arg Fallback sig filtered out. This is the view the
// authored-shape readers (canon, inspect, predicate probes, trivial-
// delegation, targeted undef, overlap detection) want: the signatures the
// user actually declared, never the dispatch-time fallback the aggregate
// injects.
func (fn *FnDefInfo) OwnSigs() []Signature {
	if fn == nil {
		return nil
	}
	hasFallback := false
	for i := range fn.Signatures {
		if fn.Signatures[i].Fallback {
			hasFallback = true
			break
		}
	}
	if !hasFallback {
		return fn.Signatures
	}
	out := make([]Signature, 0, len(fn.Signatures))
	for i := range fn.Signatures {
		if fn.Signatures[i].Fallback {
			continue
		}
		out = append(out, fn.Signatures[i])
	}
	return out
}

// FirstOwnSig returns a pointer to the first non-fallback signature and
// whether one exists. Used by the single-overload readers (predicate
// input-type gate, refine single-param probe).
func (fn *FnDefInfo) FirstOwnSig() (*Signature, bool) {
	if fn == nil {
		return nil, false
	}
	for i := range fn.Signatures {
		if !fn.Signatures[i].Fallback {
			return &fn.Signatures[i], true
		}
	}
	return nil, false
}

// FnSigSpec describes a signature specification without a body, used for
// targeted undef of specific function signatures.
type FnSigSpec struct {
	Params  []FnParam
	Returns []*Type
}

// FnUndefInfo holds signature specs for targeted undef of function signatures.
type FnUndefInfo struct {
	Sigs []FnSigSpec
}

// ReturnCheckInfo carries expected return types for fn-defined function validation.
type ReturnCheckInfo struct {
	FuncName     string
	Returns      []*Type
	UnnamedCount int    // number of unnamed params pushed onto the stack before the body
	Pos          SrcPos // source position of the fn call site, for return errors
}

// DisjunctInfo holds the alternatives for a disjunction (union) type.
// A disjunct unifies if any of its alternatives unifies with the target.
type DisjunctInfo struct {
	Alternatives []Value
	// Declared marks a check-mode carrier disjunct denoting a DECLARED
	// union domain — a named-union fn parameter binding
	// (ParamInputCarrier), where every alternative is an author-claimed
	// valid input. A body dispatch that fails for such an alternative is
	// an ERROR (the fn breaks its contract for a valid argument), where
	// the same partial failure on an ANALYSIS-join disjunct (an if/else
	// branch join, a heterogeneous element join) stays a warning — there
	// the runtime materialises only one alternative, so the failing path
	// may be dead. Never set on runtime disjunct values.
	Declared bool
}

// NegationInfo holds the inner type of a negation (complement) type. A
// value satisfies `tnot Inner` iff it does NOT satisfy Inner. The
// negation is the kernel's set-theoretic complement; together with
// DisjunctInfo (union) and TandValues (intersection) it closes the type
// algebra under Boolean operations — see
// design/elixir-types-in-aql-report.10.md.
type NegationInfo struct {
	Inner Value
}

// ClassTypeInfo holds the type definition for an object type.
// Object types form an inheritance hierarchy analogous to class inheritance.
// For example, Object/Foo has parent Object, Object/Foo/Bar has parent Foo.
// Fields are the type's own fields (not including inherited ones).
// Parent points to the parent object type (nil for direct children of Object root).
// ID is a unique internal identifier: "T_" followed by 12 lowercase hex characters.
// Name is the full type path (e.g. "Object/Foo/Bar"), set when the type is
// registered via def.
type ClassTypeInfo struct {
	Fields          *OrderedMap    // own fields (field name → type-constraint Value)
	Parent          *ClassTypeInfo // parent class type (nil if direct child of Ideal/Class)
	ID              string         // unique internal ID: "T_" + 12 hex chars
	Name            string         // full type path (e.g. "Class/Foo/Bar")
	Type            *Type          // canonical *Type identity; populated by MintType during installation
	BinaryLayout    Value          // for a binary-frame spec (a class carrying a Scalar/Bytes wire layout): the raw layout List<Map>; zero Value otherwise. Read by the Bytes codec (make/unpack/convert) and the Binary/BinarySpec membership predicates. See lang/go/native/native_bytes.go and design/go-modules/BYTES.10.md.
	cachedAllFields *OrderedMap    // lazily computed merged field map (immutable after first call)
}

// AllFields returns all fields including inherited ones. Parent fields come
// first, followed by the type's own fields. Own fields override inherited
// fields with the same name. The result is cached since ClassTypeInfo is
// immutable after registration.
func (o *ClassTypeInfo) AllFields() *OrderedMap {
	if o.cachedAllFields != nil {
		return o.cachedAllFields
	}
	result := NewOrderedMap()
	if o.Parent != nil {
		parentFields := o.Parent.AllFields()
		for _, k := range parentFields.Keys() {
			v, _ := parentFields.Get(k)
			result.Set(k, v)
		}
	}
	for _, k := range o.Fields.Keys() {
		v, _ := o.Fields.Get(k)
		result.Set(k, v)
	}
	o.cachedAllFields = result
	return result
}

// ClassInstanceInfo holds a concrete instance of an object (class)
// type. TypeRef points back to the ClassTypeInfo that created this
// instance; Fields holds every resolved field. Instances are FLAT —
// class construction resolves the full field set (own + inherited)
// eagerly at make, so there is no prototype chain (open objects, which
// used one, were removed).
type ClassInstanceInfo struct {
	TypeRef *ClassTypeInfo // the object type this is an instance of
	Fields  *OrderedMap    // resolved field values (flat: own + inherited)
}

// GetField returns a field value (flat lookup — instances carry every
// field resolved at make).
func (oi ClassInstanceInfo) GetField(name string) (Value, bool) {
	return oi.Fields.Get(name)
}

// AllFields returns the instance's resolved fields (flat).
func (oi ClassInstanceInfo) AllFields() *OrderedMap {
	result := NewOrderedMap()
	for _, k := range oi.Fields.Keys() {
		v, _ := oi.Fields.Get(k)
		result.Set(k, v)
	}
	return result
}

// ResourceTypeInfo holds the type definition for the SDK Resource /
// Entity object-type hierarchy (Ideal/Resource, Ideal/Resource/Entity).
// It mirrors the shape of a class type — own fields, a parent chain, a
// minted lattice identity — but is a distinct payload so the shared
// class-instance representation stays class-only. Instances are flat
// (schema resolved eagerly at make; no prototype delegation).
type ResourceTypeInfo struct {
	Fields          *OrderedMap       // own fields (field name → type-constraint Value)
	Parent          *ResourceTypeInfo // parent resource type (nil for Resource root)
	ID              string            // unique internal ID: "T_" + 12 hex chars
	Name            string            // full type path (e.g. "Ideal/Resource/Entity")
	Type            *Type             // canonical *Type identity; populated by MintType during installation
	cachedAllFields *OrderedMap       // lazily computed merged field map
}

// AllFields returns all fields including inherited ones (parent first,
// own overriding), mirroring ClassTypeInfo.AllFields.
func (o *ResourceTypeInfo) AllFields() *OrderedMap {
	if o.cachedAllFields != nil {
		return o.cachedAllFields
	}
	result := NewOrderedMap()
	if o.Parent != nil {
		for _, k := range o.Parent.AllFields().Keys() {
			v, _ := o.Parent.AllFields().Get(k)
			result.Set(k, v)
		}
	}
	for _, k := range o.Fields.Keys() {
		v, _ := o.Fields.Get(k)
		result.Set(k, v)
	}
	o.cachedAllFields = result
	return result
}

// ResourceInstanceInfo is a flat, schema-resolved instance of a
// ResourceType. Like a class instance it has no prototype chain — every
// field (own + inherited) is resolved at make into a single map.
type ResourceInstanceInfo struct {
	TypeRef *ResourceTypeInfo // the resource type this is an instance of
	Fields  *OrderedMap       // resolved field values (flat: own + inherited)
}

// GetField returns a field value (flat lookup — no prototype chain).
func (ri ResourceInstanceInfo) GetField(name string) (Value, bool) {
	return ri.Fields.Get(name)
}

// StoreInstanceInfo is a copy-on-write key-value store (Object/Store).
// Unlike regular Object instances which have typed fields, Store instances
// hold arbitrary key-value pairs. Key resolution walks the prototype chain,
// enabling scope-like lookup when contexts are nested.
//
// Copy-on-write: the AQL `set` word creates a new Store layer (prototype =
// old Store) instead of mutating in place. If this Store is nested inside
// a parent Store, the parent is COW'd too, propagating up to the ctxStack.
type StoreInstanceInfo struct {
	TypeName  string             // full type path, e.g. "Object/Store" or "Object/Store/System"
	Data      map[string]Value   // own key-value pairs (COW layer)
	Prototype *StoreInstanceInfo // prototype chain for key lookup / COW base
	Parent    *StoreInstanceInfo // containing Store (for COW propagation), nil if root
	ParentKey string             // key in Parent that references this Store
}

// Get looks up a key in this store, walking the prototype chain if not found.
func (si *StoreInstanceInfo) Get(key string) (Value, bool) {
	if v, ok := si.Data[key]; ok {
		return v, true
	}
	if si.Prototype != nil {
		return si.Prototype.Get(key)
	}
	return Value{}, false
}

// Set stores a key-value pair directly (for internal/init use only).
// AQL code should use the set word which does COW via CowSet.
func (si *StoreInstanceInfo) Set(key string, val Value) {
	si.Data[key] = val
	// Track parent relationship for nested Stores.
	if childStore, ok := val.Data.(*StoreInstanceInfo); ok {
		childStore.Parent = si
		childStore.ParentKey = key
	}
}

// "T_" followed by 12 lowercase hex characters (6 random bytes).
func GenerateObjectTypeID() string {
	return GenerateID("T_")
}

// markCounter is a global counter for generating unique mark IDs.
var markCounter atomic.Int64

// NextMarkID generates a unique mark ID.
func NextMarkID() string {
	n := markCounter.Add(1)
	return fmt.Sprintf("_m%d", n)
}

// MarkInfo identifies a mark on the stack. Marks are internal control-flow
// anchors placed by constructs like for-loops. Each mark has a unique ID
// so that a corresponding move can jump the pointer back to it.
// Body stores the original values between the mark and its paired move,
// enabling replay when the move fires.
type MarkInfo struct {
	ID   string  // unique identifier for this mark
	Body []Value // original content to replay (set by the move on first encounter)
}

// SpliceInfo carries an __SP (splice) marker's payload. When an __SP value
// reaches the engine pointer it is replaced, unevaluated, by its Data: a
// plain list contributes its top-level elements, any other value contributes
// itself. The spliced content is then re-stepped against the live stack, so a
// word-bearing list behaves as a Forth-style macro. Produced by the `word`
// native and by `def name word value`. See engine.go::stepLiteral.
type SpliceInfo struct {
	Data Value // the wrapped payload to splice in
}

// MoveInfo identifies a move on the stack. When the stack pointer reaches
// a move, it jumps back to the corresponding mark. The Reason field
// describes why the move exists (e.g. "for loop") and is used in error
// messages when the target mark cannot be found.
//
// Cont optionally carries for-loop continuation state. When set, stepMove
// uses it to drive multi-iteration loops: each firing advances the iterator,
// conditionally re-inserts mark+body+move for the next iteration, and
// accumulates results across iterations.
type MoveInfo struct {
	To     string   // ID of the target mark
	Reason string   // human-readable reason (for error messages)
	Cont   *ForCont // optional: for-loop iteration state
	IfCont *IfCont  // optional: if-statement continuation state
}

// ForCont holds the iteration state for a mark/move-driven for loop.
// It is carried by the MoveInfo and mutated across iterations.
type ForCont struct {
	Registry *Registry
	IterName string  // name of the iterator variable (e.g. "i")
	Current  int64   // current iteration value
	End      int64   // exclusive bound
	Step     int64   // increment per iteration
	Body     []Value // original body tokens (replayed each iteration)
	Results  []Value // accumulated results from completed iterations
}

// IfCont holds the continuation state for a mark/move-driven if statement.
// When the move fires, the condition result (between mark and move) is
// evaluated for truthiness to select the appropriate branch.
type IfCont struct {
	Then []Value // tokens to splice if condition is truthy
	Else []Value // tokens to splice if condition is falsy (nil for 2-arg if)
}

// ModuleDesc describes a module: its generated ID and named exports.
// Each export call adds a named entry mapping export name → export map.
type ModuleDesc struct {
	ID      string                 // generated internal identifier
	Exports map[string]*OrderedMap // export name → export map (name → value)
	// Descriptor metadata, populated by the loader (Resolve / loadFileModule
	// / RunModuleBody) and surfaced on the Ideal/Module instance at import.
	Ref    string // external module reference ("aql:math-util", "./lib.aql"); "" inline
	Kind   string // "native" | "file" | "inline"
	File   string // source file path ("" for native/inline)
	Folder string // source folder ("" for native/inline)
	// Src is the module's OWN registry — the sub-registry a native
	// builder registered its words/types/ideals into, or the body
	// registry an inline/file module ran in. Import uses it to
	// transport ESCAPED type machinery to the importer: each exported
	// bare type literal adopts its canonical node into the importer's
	// TypeTable (the compiled OpPushType path) and brings its matching
	// Ideal across, so a facade module re-exporting a constructible
	// type keeps `make` working. In-process only — never serialized.
	Src *Registry
}

// WordInfo carries the name and optional modifiers for a function reference.
type WordInfo struct {
	Name         string
	ArgCount     int  // -1 = unspecified
	ForceStack   bool // lower/s
	ForceForward bool // lower/f
	ForceRef     bool // lower/r — resolve to the bound value without invoking
	ForceUsurp   bool // lower/u — wrap the bound fn so its sig arg order is reversed
}

// ForwardInfo tracks forward argument collection for a deferred function call.
type ForwardInfo struct {
	FuncName      string
	ExpectedArgs  int
	CollectedArgs int
	StackArgs     int // number of sig args already consumed from the stack
	// FuncIndex records where the deferred function word sits in the stack.
	FuncIndex int
	Sig       *Signature // the matched signature, for direct execution on completion
	Pos       SrcPos     // source position of the forward-collecting word, for errors

	// Speculative records the planner's stop condition for the arrival
	// loop (design/FORWARD-COLLECTION-PHASES.10.md): matchSignature
	// filled at least one forward slot with a WORD bound to a
	// dispatching definition (an FnDefInfo binding accepted through the
	// Any-slot escape) — the plan treats that token as an operand, but
	// at runtime it dispatches as an operator. SpeculativeAt is the
	// sig-order index of the first such slot and is meaningful only
	// when Speculative is set (the bool guards the int so the struct's
	// zero value means "none" — No-Zero-Overload rule). The index is
	// plan-side bookkeeping: under zero/multi-value paren collapse the
	// arrival count can drift from plan positions, so consumers may use
	// it for tracing/diagnostics, never for slot arithmetic. The
	// commitBarrierForward scan must NOT be gated on this field — the
	// barrier commit doubles as zero-value-collapse recovery where the
	// plan had no speculative word at all.
	Speculative   bool
	SpeculativeAt int
}

// Value is the single node type of the AQL kernel: it is at once a
// runtime value (an entry on the stack) and a node in the type
// lattice. Value and Type were historically separate structs; they
// are now one, with Type an alias for Value (see typetable.go). A
// leaf (5, 'hello', [1 2 3]) and a type node (Integer, List, Any)
// are the same kind of thing, differing only in which fields carry
// data.
//
// The lattice is encoded by Parent: a leaf's Parent is its type, a
// type node's Parent is its supertype.
//
// Every value carries a unique ID with a prefix indicating its
// category:
//   - "S_" for scalar values (String, Number, Boolean)
//   - "N_" for node values (List, Map, Table, Record)
//   - "W_" for word values (Word, Atom, Function, Internal/*)
//   - "T_" for type/object values (Object/*, type literals, Any, None)
//
// Each ID is the prefix followed by 12 lowercase hex characters.
type Value struct {
	ID string

	// Parent is the node directly above this one in the unified
	// lattice: for an ordinary value it is the value's type, for a
	// type node it is the supertype. nil only for lattice roots.
	Parent *Type

	// Type-lattice metadata — populated on type nodes, zero on
	// ordinary values. A non-nil Behavior is the marker of a type
	// node.
	Name    string // type-node leaf name (e.g. "ProperString")
	FixedID int    // >0 for builtin type nodes; 0 otherwise
	Rank    int    // unified lattice rank — total order for CompareValues/compareTypes
	Depth   int    // parent-chain length (root = 1); 0 = unset (ad-hoc *Type). Cached for typeDepth / LCA.
	// In/Out are the DFS nested-set interval of a type node within the
	// STATIC builtin lattice: In is the pre-order entry number, Out the
	// largest entry number in the node's subtree. A descendant d of an
	// ancestor a satisfies a.In <= d.In <= a.Out, so IsAncestor is an O(1)
	// range test instead of a parent-chain walk. Assigned once by
	// labelIntervals at builtin-table construction; 0 = unlabelled (minted,
	// external, or ad-hoc types), which routes IsAncestor through the walk.
	In         int
	Out        int
	IsInternal bool         // Word/__XX runtime markers — not user-facing
	Origin     OriginKind   // builtin / userdef
	Behavior   TypeBehavior // pluggable dispatch — non-nil exactly on type nodes

	// Payload and evaluation state — populated on ordinary values.
	Data      Payload // the kernel-known data payload; see payload.go for variants
	Quoted    bool    // produced by the quote word; prevents auto-evaluation
	Eval      bool    // parser-created list that should auto-evaluate at end of Run
	Pos       SrcPos  // source position for error reporting (zero value = unknown)
	Undefined bool    // atom created from an undefined word (error if left on result stack)
	// FailedDispatch marks a named Function value that a dispatch
	// attempt left on the stack as data because no signature matched
	// (the silent-failure shape of design/ERRORS.8.md §5, VOXGIG T1).
	// Harmless while anything consumes the value (higher-order use,
	// def); if it survives unconsumed to the top-level end-of-Run
	// drain, the engine raises uncalled_function at the original call
	// site — the same bug check mode diagnoses under that name.
	FailedDispatch bool
	Carrier        bool // static-typecheck carrier (type-only, Data stripped of concrete payload)
	// Dynamic marks a carrier as a bounded gradual value (Elixir-style
	// dynamic(T) — design/dynamic-modality-report.10.md). Implies Carrier.
	// Its Parent/Data is a BOUND, not a proven type: at a signature
	// boundary it matches the slot unless PROVABLY disjoint from it
	// (not-disjoint rule), rather than by strict ConformsTo. Set only on
	// carriers the checker cannot prove exactly (escape hatches); cleared
	// by a successful guard, which discharges the gradual obligation.
	Dynamic bool
	// DynFrom is the binding name a dynamic carrier was resolved from
	// (check mode only). It lets narrowing-through-use tighten that
	// binding to dynamic(bound ∩ slot) at a typed use, so a later
	// provably-disjoint use of the same name is caught. Empty for
	// non-binding-derived carriers; never read at runtime.
	DynFrom string
}

// idRand is the package-level RNG used for ID generation.
// Defaults to time-seeded; can be overridden via SetIDSeed.
//
// idRandMu guards it: GenerateID is called from concurrently-running
// engine forks (await branches, timer callbacks), and an *rand.Rand is
// not safe for concurrent use. The mutex keeps ID generation race-free
// without each call paying for its own RNG.
var (
	idRandMu sync.Mutex
	idRand   = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0))
)

// idMin is the minimum random value for generated IDs (0x100000000000).
const idMin uint64 = 0x100000000000

// idMax is the exclusive upper bound so values fit in 12 hex chars.
const idMax uint64 = 0x1000000000000 - 0x100000000000

// SetIDSeed configures the package-level RNG with the given seed.
func SetIDSeed(seed int64) {
	idRandMu.Lock()
	defer idRandMu.Unlock()
	idRand = rand.New(rand.NewPCG(uint64(seed), 0))
}

// GenerateID creates a unique ID with the given prefix followed by 12
// lowercase hex characters. The random value is >= 0x100000000000.
func GenerateID(prefix string) string {
	idRandMu.Lock()
	n := idMin + idRand.Uint64N(idMax)
	idRandMu.Unlock()
	var buf [6]byte
	buf[0] = byte(n >> 40)
	buf[1] = byte(n >> 32)
	buf[2] = byte(n >> 24)
	buf[3] = byte(n >> 16)
	buf[4] = byte(n >> 8)
	buf[5] = byte(n)
	return prefix + hex.EncodeToString(buf[:])
}

// IDPrefixForType returns the ID prefix for a given type:
// "S_" for Scalar, "N_" for Node, "W_" for Word, "T_" for Object/Any/None.
//
// Under the Any-root lattice the universal Root() of every connected
// type is Any; the historical category prefix lives one step below —
// the topmost ancestor that's NOT Any. Walk the parent chain until
// the next step would land on Any (or nil) and read the name there.
func IDPrefixForType(t *Type) string {
	if t == nil {
		return "T_"
	}
	for d := t; d != nil; d = d.Parent {
		if d.Parent == nil || d.Parent.FixedID == anyFixedID {
			switch d.Name {
			case "Scalar":
				return "S_"
			case "Node":
				return "N_"
			case "Word":
				return "W_"
			}
			return "T_"
		}
	}
	return "T_"
}

// NewValueRaw creates a Value with an auto-generated ID based on the
// type category. data must be a Payload — the sealed interface
// implemented by all kernel-known payload variants and by every
// eng-defined struct/pointer type used as a payload. After Step 5g,
// passing a raw int64 / string / time.Time / etc. is a compile error
// — wrap it in IntPayload / StrPayload / TimePayload / etc. first.
func NewValueRaw(t *Type, data Payload) Value {
	return Value{
		ID:     GenerateID(IDPrefixForType(t)),
		Parent: t,
		Data:   data,
	}
}

// NewString creates a string value tagged with the appropriate
// String subtype: EmptyString for "" (the unique inhabitant of the
// EmptyString singleton type) and ProperString for any non-empty
// payload. Both subtypes match Scalar/String via the type lattice
// (TStringProper.ConformsTo(TString) and TStringEmpty.ConformsTo(TString)
// are true), so signatures declared on TString continue to dispatch
// transparently — the difference is observable only via typeof,
// pattern dispatch, or explicit subtype-equality checks.
//
// Specific-value dispatch is still primarily routed through
// Signature.Patterns; the empty/proper split provides a coarser
// "value-shape at the type level" signal so user code can branch on
// emptiness without resorting to a length comparison.
func NewString(s string) Value {
	if s == "" {
		return NewValueRaw(TStringEmpty, StrPayload{S: s})
	}
	return NewValueRaw(TStringProper, StrPayload{S: s})
}

// NewInteger creates a number/integer value with Parent = Scalar/Number/Integer.
// Specific-value dispatch (e.g. `def fact[0] (1)`) routes through
// Signature.Patterns, not through a per-value type-path leaf. See
// the NewString comment for the rationale.
func NewInteger(n int64) Value {
	return NewValueRaw(TInteger, IntPayload{N: n})
}

// NewFloat creates a number/float value with a float64 payload.
func NewFloat(f float64) Value {
	return NewValueRaw(TFloat, FloatPayload{F: f})
}

// NewBigInteger creates a Scalar/Number/BigInteger value wrapping an
// arbitrary-precision integer. The caller must not mutate n afterwards.
func NewBigInteger(n *big.Int) Value {
	return NewValueRaw(TBigInteger, BigIntPayload{N: n})
}

// NewBigDecimal creates a Scalar/Number/BigDecimal value wrapping an
// arbitrary-precision base-10 decimal. The caller must not mutate d.
func NewBigDecimal(d *apd.Decimal) Value {
	return NewValueRaw(TBigDecimal, DecimalPayload{D: d})
}

// FormatFloat renders a float64 with a guaranteed decimal point so the
// type stays visually distinct from Integer. Uses 'f' format with -1
// precision (shortest round-trip), then appends ".0" when the result
// has neither a fractional part nor an exponent. Float artefacts like
// 0.1 + 0.2 = 0.30000000000000004 are preserved verbatim — see the
// note in spec/SPEC_REPORT.md §2 on the apd-port plan if exact
// decimal arithmetic is required.
func FormatFloat(f float64) string {
	// Special values render as the parseable literals inf / -inf / nan
	// (matching the parser's word-context literals) so print∘parse is
	// identity. The historical +Inf.0 / NaN.0 forms could not be re-read.
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	// Use scientific notation only for genuinely extreme magnitudes, where
	// plain decimal would be gratuitously long (hundreds of digits). The
	// bounds deliberately leave the everyday range — including small
	// decimals like 0.00001 (1e-5) and large values up to ~1e20 — rendered
	// in full, exactly as before. 1e21 (Go's own 'g' upper threshold) and
	// magnitudes below 1e-10 switch to 'e'. Both forms re-parse to the same
	// Float, so the choice is purely about readability.
	if f != 0 {
		if a := math.Abs(f); a >= 1e21 || a < 1e-10 {
			return strconv.FormatFloat(f, 'e', -1, 64)
		}
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// formatFloat is the lowercase alias retained for in-package call
// sites that pre-date the exported form.
func formatFloat(f float64) string { return FormatFloat(f) }

// FormatBigInteger renders a BigInteger as the parseable literal `0d…`,
// with the sign before the marker (`-0d5`), so print∘parse is identity.
func FormatBigInteger(n *big.Int) string {
	if n == nil {
		return "0d0"
	}
	if n.Sign() < 0 {
		return "-0d" + new(big.Int).Abs(n).String()
	}
	return "0d" + n.String()
}

// FormatBigDecimal renders a BigDecimal as the parseable literal `0d…`
// using apd's plain 'f' form (preserving scale, e.g. 0d0.30), with the
// sign before the marker.
func FormatBigDecimal(d *apd.Decimal) string {
	if d == nil {
		return "0d0"
	}
	s := d.Text('f')
	if strings.HasPrefix(s, "-") {
		return "-0d" + s[1:]
	}
	return "0d" + s
}

// NewBoolean creates a boolean value. The boolean payload (true/false) is the
// value; there are no Boolean/True or Boolean/False sub-types.
func NewBoolean(b bool) Value {
	return NewValueRaw(TBoolean, BoolPayload{B: b})
}

// NewList creates a list value from a slice of Values.
func NewList(elems []Value) Value {
	return NewValueRaw(TList, ListPayload{Elems: elems})
}

// NewEvalList creates a list value that is marked for auto-evaluation
// at the end of execution. Used by the parser for source-code lists.
func NewEvalList(elems []Value) Value {
	v := NewValueRaw(TList, ListPayload{Elems: elems})
	v.Eval = true
	return v
}

// NewTypedList creates a typed list value with a child type constraint.
// For example, NewTypedList(NewTypeLiteral(TString)) represents [:string].
func NewTypedList(child Value) Value {
	return NewValueRaw(TList, ChildTypeInfo{Child: child})
}

// NewMap creates a map value from an ordered map of string keys to Values.
func NewMap(entries *OrderedMap) Value {
	return NewValueRaw(TMap, MapPayload{M: entries})
}

// NewFlexList creates a mutable FlexList value over a pointer-backed
// element store. Flex nodes are runtime data, never parser output, so
// Eval is never set on them.
func NewFlexList(elems []Value) Value {
	return NewValueRaw(TFlexList, &FlexListData{Elems: elems})
}

// NewXmlElement creates an immutable Node/Xml element value. attr may
// be nil (treated as no attributes); cren may be nil (no children).
// See design/XML-LITERAL.0.md and core_xml.go.
func NewXmlElement(tag string, attr *OrderedMap, cren []Value) Value {
	if attr == nil {
		attr = NewOrderedMap()
	}
	return NewValueRaw(TXml, XmlElementPayload{Tag: tag, Attr: attr, Cren: cren})
}

// NewXmlInterp creates an interpolated XML literal skeleton (Word/__XI).
// The engine evaluates it in place to a concrete Node/Xml at runtime,
// like an InterpString. See payload.go::XmlInterpPayload and
// engine.go::evalXmlInterp.
func NewXmlInterp(tmpl XmlTmpl) Value {
	return NewValueRaw(TXmlInterp, XmlInterpPayload{Tmpl: tmpl})
}

// IsXmlInterp reports whether v is an interpolated XML literal skeleton.
func IsXmlInterp(v Value) bool {
	return v.Parent.Equal(TXmlInterp)
}

// AsXmlInterp returns the template backing an interpolated XML skeleton.
func AsXmlInterp(v Value) (XmlTmpl, error) {
	if xi, ok := v.Data.(XmlInterpPayload); ok {
		return xi.Tmpl, nil
	}
	return XmlTmpl{}, fmt.Errorf("AsXmlInterp: not an xml-interp value (got %T)", v.Data)
}

// NewFlexMap creates a mutable FlexMap value. It reuses MapPayload —
// the *OrderedMap is pointer-backed, so in-place mutation is visible
// through every Value copy sharing the payload. Like NewFlexList,
// Eval is never set.
func NewFlexMap(entries *OrderedMap) Value {
	return NewValueRaw(TFlexMap, MapPayload{M: entries})
}

// NewEvalMap creates a map value marked for auto-evaluation at end of
// execution. Used by the parser for source-code maps.
func NewEvalMap(entries *OrderedMap) Value {
	v := NewValueRaw(TMap, MapPayload{M: entries})
	v.Eval = true
	return v
}

// NewTypedListWithElements creates a typed list value carrying both
// concrete elements and a child-type constraint. Used by the parser
// for `[v0 :T v1]` syntax. Each element is validated against the
// child constraint by `is` and similar predicates.
func NewTypedListWithElements(child Value, elems []Value) Value {
	return NewValueRaw(TList, ChildTypeInfo{Child: child, Elements: elems})
}

// NewTypedMapWithEntries creates a typed map value carrying both
// concrete (key, value) entries and a child-type constraint. Used by
// the parser for `{k:v :T}` syntax.
func NewTypedMapWithEntries(child Value, entries []ChildEntry) Value {
	return NewValueRaw(TMap, ChildTypeInfo{Child: child, Entries: entries})
}

// NewImplicitMap creates a map value marked as implicit (from pair syntax).
// In fn signatures, implicit maps are treated as named parameter declarations
// (e.g., [x:Integer]), while explicit maps are structural patterns.
func NewImplicitMap(entries *OrderedMap) Value {
	entries.Implicit = true
	return NewValueRaw(TMap, MapPayload{M: entries})
}

// IsImplicitMap reports whether v is a Map value whose backing
// OrderedMap was constructed from implicit-pair syntax (e.g.
// `{x:Integer}` or `[x:Integer]` inside an fn sig). Used to
// discriminate record-shape patterns from concrete maps.
func IsImplicitMap(v Value) bool {
	if !v.Parent.Equal(TMap) || v.Data == nil {
		return false
	}
	if mp, ok := v.Data.(MapPayload); ok {
		return mp.M != nil && mp.M.Implicit
	}
	return false
}

// NewTypedMap creates a typed map value with a child type constraint.
// For example, NewTypedMap(NewTypeLiteral(TString)) represents {:string}.
func NewTypedMap(child Value) Value {
	return NewValueRaw(TMap, ChildTypeInfo{Child: child})
}

// NewRecordType creates a record type value from a field schema.
// The fields map contains field names as keys and type-constraint Values as values.
// For example, record{x:number, y:number} constrains maps to have exactly
// keys x and y with number-typed values.
func NewRecordType(fields *OrderedMap) Value {
	return NewValueRaw(TMap, RecordTypeInfo{Fields: fields})
}

// NewOptionsType creates an options type value from a field schema map.
func NewOptionsType(fields *OrderedMap) Value {
	return NewValueRaw(TMap, OptionsTypeInfo{Fields: fields})
}

// NewTableType creates a table type value from a record type.
// A table type constrains a list so that each element is a map conforming
// to the given record schema.
func NewTableType(record RecordTypeInfo) Value {
	return NewValueRaw(TList, TableTypeInfo{Record: record})
}

// NewAtom creates an atom value from a bare unquoted word.
func NewAtom(name string) Value {
	return NewValueRaw(TAtom, AtomPayload{Name: name})
}

// AtomReferent returns the value an atom's name was snapshotted to refer to,
// if one was captured (by `quote` or the run-start resolution pass). ok=false
// when v is not an atom or carries no referent.
func AtomReferent(v Value) (Value, bool) {
	ap, ok := v.Data.(AtomPayload)
	if !ok || ap.Referent == nil {
		return Value{}, false
	}
	return *ap.Referent, true
}

// SetAtomReferent returns a copy of atom v carrying ref as its referent (a
// snapshot of what its name refers to). Name, Quoted, and Pos are preserved.
// Returns v unchanged when it is not an atom.
func SetAtomReferent(v Value, ref Value) Value {
	ap, ok := v.Data.(AtomPayload)
	if !ok {
		return v
	}
	snap := ref
	ap.Referent = &snap
	v.Data = ap
	return v
}

// NewPathon creates a Pathon value from parts and an absolute flag.
func NewPathon(parts []string, abs bool) Value {
	p := make([]string, len(parts))
	copy(p, parts)
	return NewValueRaw(TPathon, PathonPayload{Info: PathonInfo{Parts: p, Abs: abs}})
}

// NewTypeLiteral returns the type t expressed as a Value. After the
// type/value merge a type literal IS its lattice node, so this
// returns a by-value copy of the node itself: the literal's Parent
// is the supertype (so typeof is uniformly Parent), its Name is the
// type's own name, its Data is nil.
func NewTypeLiteral(t *Type) Value {
	if t == nil {
		return Value{}
	}
	return *t
}

// typeNodeOf returns the lattice node a Data==nil value stands for.
// A type literal IS its node. A carrier's node is its Parent — a
// carrier keeps Parent pointing at the type it carries. Used by the
// renderers to recover the type name from either shape.
func typeNodeOf(v Value) *Type {
	if v.Carrier {
		return v.Parent
	}
	return &v
}

// TypeNameOf returns the leaf name of the type v represents: a type
// literal's own name, or any other value's Parent leaf. Used by
// inspect and rendering paths that display a type's name.
func TypeNameOf(v Value) string {
	if IsBareTypeNode(v) {
		return v.Leaf()
	}
	return v.Parent.Leaf()
}

// TypePathOf returns the slash path of the type v represents: a type
// literal's own path, or any other value's Parent path.
func TypePathOf(v Value) string {
	if IsBareTypeNode(v) {
		return v.Path()
	}
	return v.Parent.Path()
}

// ValueType returns the lattice node that is v's type: v itself when
// v is a bare type literal (a type literal IS a lattice node), and
// v's Parent for any concrete value or carrier.
func ValueType(v Value) *Type {
	if IsBareTypeNode(v) {
		return &v
	}
	return v.Parent
}

// noneSentinel is the non-nil Data payload that distinguishes the
// VALUE `none` (the unique inhabitant of None) from the TYPE LITERAL
// `None` (which has Data == nil like every other type literal).
// Renderers and the matcher use the Data!=nil discriminator to print
// the value as "none" and the type literal as "None".
type noneSentinel struct{}

// NewNone creates the value `none` — the unique inhabitant of the
// None type. Distinct from NewTypeLiteral(TNone) (the type itself).
func NewNone() Value {
	return NewValueRaw(TNone, NonePayload{})
}

// Is reports whether v satisfies type t, routed through t.Behavior.
// The canonical dispatch point for "is v a T?" — used by handlers,
// the matcher, and `is` / `guard`. Default Behavior delegates to the
// lattice walk (v.Parent.ConformsTo(t)); types with custom Behavior
// override (predicate types invoke their body, record types check
// field-by-field conformance, etc.).
//
// Safe on nil t: returns false. Safe on nil Behavior: callers should
// not encounter this state because every Type registered through
// the kernel paths carries a non-nil Behavior, but the method
// defends against it anyway.
func (v Value) Is(t *Type) bool {
	if t == nil {
		return false
	}
	if t.Behavior == nil {
		return v.Parent.ConformsTo(t)
	}
	return t.Behavior.Match(v, t)
}

// IsNone reports whether v is the value `none` (not the None type
// literal). The check distinguishes the inhabitant from the type:
// `none` carries a NonePayload/noneSentinel marker; the bare type
// literal `None` is just a by-value copy of TNone with Data=nil.
// Use IsNoneShape for the broader "is this any form of None" check.
func IsNone(v Value) bool {
	if !v.Parent.Equal(TNone) {
		return false
	}
	if _, ok := v.Data.(NonePayload); ok {
		return true
	}
	_, ok := v.Data.(noneSentinel)
	return ok
}

// IsNoneShape reports whether v is any form of None — the canonical
// "did we get back None?" check that covers:
//
//   - the sentinel value `none` (IsNone — Data is NonePayload or
//     noneSentinel, Parent=TNone).
//   - the bare type literal `None` (NewTypeLiteral(TNone), Data=nil,
//     value IS the TNone lattice node).
//   - manually-constructed Value{Parent: TNone} carrier-shaped
//     values (Data=nil, Parent=TNone — used by some host code and
//     test fixtures as a "no value" sentinel).
//
// Renderers that want to distinguish "none" (lowercase, the value)
// from "None" (capital, the type) use IsNone for the strict check;
// dispatch and comparison sites use IsNoneShape.
func IsNoneShape(v Value) bool {
	if IsNone(v) {
		return true
	}
	if v.Data != nil {
		return false
	}
	if v.Parent.Equal(TNone) {
		return true
	}
	return !v.Carrier && (&v).Equal(TNone)
}

// NewWord creates a word value (function reference) with no modifiers.
func NewWord(name string) Value {
	return NewValueRaw(TWord, WordInfo{Name: name, ArgCount: -1})
}

// NewWordModified creates a word value with explicit modifiers.
func NewWordModified(name string, argCount int, forceStack, forceForward bool) Value {
	return NewValueRaw(TWord, WordInfo{
		Name:         name,
		ArgCount:     argCount,
		ForceStack:   forceStack,
		ForceForward: forceForward,
	})
}

// NewWordRef creates a word value marked with the /r modifier: when
// reached at the pointer it resolves the name to its bound Function
// value without entering function dispatch. /r is legal ONLY for
// function words — a name bound to a non-fn value (plain value, type
// body) raises [aql/illegal_ref] (see eng.IsFunctionRef). ArgCount
// stays unspecified because /r short-circuits argument collection.
func NewWordRef(name string) Value {
	return NewValueRaw(TWord, WordInfo{
		Name:     name,
		ArgCount: -1,
		ForceRef: true,
	})
}

// NewWordUsurp creates a word value marked with the /u modifier: when
// reached at the pointer it resolves the name to its bound Function value
// and wraps it so its signature argument order is reversed (usurped a b c
// ≡ f c b a). Like /r, /u is legal ONLY for function words. The usurped
// wrapper is left UNQUOTED, so it dispatches immediately when args are
// available; combine with /r (name/ur) to leave it as inert data instead.
func NewWordUsurp(name string, ref bool) Value {
	return NewValueRaw(TWord, WordInfo{
		Name:       name,
		ArgCount:   -1,
		ForceUsurp: true,
		ForceRef:   ref,
	})
}

// NewForward creates a forward primitive value for forward argument tracking.
func NewForward(info ForwardInfo) Value {
	return NewValueRaw(TForward, info)
}

// NewOpenParen creates an open-paren marker value for sub-expression scoping.
func NewOpenParen() Value {
	return NewValueRaw(TOpenParen, nil)
}

// NewCloseParen creates a close-paren marker value. Emitted by the
// parser for `)` so the engine can recognise it by Parent identity
// instead of by string compare.
func NewCloseParen() Value {
	return NewValueRaw(TCloseParen, nil)
}

// NewEnd creates an end-marker value (the `end` / `;` keyword).
// Emitted by the parser so the engine can recognise it by Parent
// identity instead of by string compare.
func NewEnd() Value {
	return NewValueRaw(TEnd, nil)
}

// NewParenExpr creates a paren expression value containing items to evaluate.
// Used by the parser for paren groups in map/list value positions.
// autoEvalMap evaluates these by running the items in a sub-engine with
// paren markers, producing a single result value.
func NewParenExpr(items []Value) Value {
	return NewValueRaw(TParenExpr, ParenExprPayload{Toks: items})
}

// NewReach creates an Ideal/Reach value — a first-class dot-access node
// (m.a.b). receiver is the base expression's tokens (nil/empty for a
// receiverless reach); segments are the .key / !.key steps; eval marks it
// evaluate-by-default. See design/REACH.10.md.
func NewReach(info ReachInfo) Value {
	return NewValueRaw(TReach, info)
}

// NewReachFromKeys builds an inert (non-evaluating, Eval=false) Reach over a
// concrete receiver value with literal `get` segments — the programmatic
// `reach` constructor. The result is data (a lens): it does not auto-evaluate
// like a parsed m.a.b. See design/REACH.10.md §7.
func NewReachFromKeys(receiver Value, keys []Value) Value {
	segs := make([]ReachSeg, len(keys))
	for i, k := range keys {
		segs[i] = ReachSeg{KeyLit: k}
	}
	return NewReach(ReachInfo{Receiver: []Value{receiver}, Segments: segs, Eval: false})
}

// IsReach reports whether v is an Ideal/Reach value.
func IsReach(v Value) bool {
	return v.Parent.Equal(TReach)
}

// AsReach returns the ReachInfo of a Reach value.
func AsReach(v Value) (ReachInfo, error) {
	if ri, ok := v.Data.(ReachInfo); ok {
		return ri, nil
	}
	return ReachInfo{}, fmt.Errorf("AsReach: not a reach value (got %T)", v.Data)
}

// InterpPart represents one segment of an interpolated string.
// If Expr is nil, Lit is a literal string segment.
// If Expr is non-nil, it contains parsed AQL values to evaluate.
type InterpPart struct {
	Lit  string
	Expr []Value
}

// NewInterpString creates an interpolated string value from alternating
// literal and expression parts. The engine evaluates expression parts in
// a sub-engine, converts results to strings, and concatenates everything.
func NewInterpString(parts []InterpPart) Value {
	return NewValueRaw(TInterpString, InterpStringPayload{Parts: parts})
}

// NewMark creates a mark value with the given unique ID and the body to
// replay when the corresponding move fires. The body should contain
// the original values between the mark and its paired move.
func NewMark(id string, body ...Value) Value {
	b := make([]Value, len(body))
	copy(b, body)
	return NewValueRaw(TMark, MarkInfo{ID: id, Body: b})
}

// NewMove creates a move value targeting the mark with the given ID.
// The reason string describes why this move exists (used in error messages).
func NewMove(to string, reason string) Value {
	return NewValueRaw(TMove, MoveInfo{To: to, Reason: reason})
}

// NewMoveCont creates a move value with for-loop continuation state.
func NewMoveCont(to, reason string, cont *ForCont) Value {
	return NewValueRaw(TMove, MoveInfo{To: to, Reason: reason, Cont: cont})
}

// NewMoveIf creates a move value with if-statement continuation state.
func NewMoveIf(to, reason string, ifCont *IfCont) Value {
	return NewValueRaw(TMove, MoveInfo{To: to, Reason: reason, IfCont: ifCont})
}

// NewFnDef creates a function definition value for storage on DefStacks.
func NewFnDef(info FnDefInfo) Value {
	return NewValueRaw(TFnDef, info)
}

// NewFunction creates a function reference value. The underlying data is a
// FnDefInfo, but the type is TFunction so it can be matched by function-typed
// parameters and passed to other functions without being called.
func NewFunction(info FnDefInfo) Value {
	return NewValueRaw(TFunction, info)
}

// NewFnUndef creates a function undef spec value for targeted signature removal.
func NewFnUndef(info FnUndefInfo) Value {
	return NewValueRaw(TFnUndef, info)
}

// NewReturnCheck creates a return-check marker for fn return type validation.
func NewReturnCheck(info ReturnCheckInfo) Value {
	return NewValueRaw(TReturnCheck, info)
}

// DefCleanupInfo holds a snapshot of DefStacks lengths taken before fn body
// execution. When the engine encounters a DefCleanup marker, it pops any
// defs that were added during body execution back to the snapshot state.
type DefCleanupInfo struct {
	Snapshot map[string]int
	Registry *Registry
}

// NewDefCleanup creates a def-cleanup marker for fn body local def cleanup.
func NewDefCleanup(info DefCleanupInfo) Value {
	return NewValueRaw(TDefCleanup, info)
}

// NewDisjunct creates a disjunction type value from a list of
// alternatives. The low-level constructor preserves the caller's order —
// some internal callers build order-bearing disjuncts (a DepScalar
// `[low, high]` bound, a dynamic-dispatch reachable-returns partition).
// Canonical tcmp ordering for `tor` type UNIONS is applied one layer up,
// in SimplifyDisjunctAlts (core_helpers.go), which every user-facing
// union construction routes through — so `None tor 1` and `1 tor None`
// build equal values that print identically and compare equal under
// `tcmp`. (NewEnum also preserves order — an enum's member order is
// significant.)
func NewDisjunct(alternatives []Value) Value {
	return NewValueRaw(TDisjunct, DisjunctInfo{Alternatives: alternatives})
}

// disjunctAltLess is the canonical ordering predicate for disjunct
// alternatives: primarily CompareValues (the `tcmp` total order); a CanonValue
// lexical tiebreak resolves both incomparable pairs (CompareValues errs) AND
// value-equal-but-distinct pairs (CompareValues returns 0). The tie case is
// real: cross-leaf numeric magnitude equality (`1 cmp 1.0 == 0`) leaves two
// type-distinct alts the primary order cannot separate, so without the tiebreak
// `1 tor 1.0` and `1.0 tor 1` would keep the caller's order and render/store
// differently despite `tor` being commutative. CanonValue distinguishes them
// (`1` vs `1.0`), keeping the sort a deterministic strict weak ordering.
func disjunctAltLess(a, b Value) bool {
	if c, err := CompareValues(a, b); err == nil && c != 0 {
		return c < 0
	}
	return CanonValue(a) < CanonValue(b)
}

// NewEnum creates an Enum value (Type/Disjunct/Enum) — a fixed
// enumeration of named values. Structurally identical to a Disjunct
// (same DisjunctInfo payload) but tagged with the more specific Enum
// type so `typeof` reports `Enum` and the value can be distinguished
// from a general type-disjunct.
func NewEnum(alternatives []Value) Value {
	return NewValueRaw(TEnum, DisjunctInfo{Alternatives: alternatives})
}

// NewNegation creates a negation (complement) type value whose
// inhabitants are exactly the values that do NOT satisfy inner.
func NewNegation(inner Value) Value {
	return NewValueRaw(TNegation, NegationInfo{Inner: inner})
}

// NewClassType creates an object type value. The caller must
// provide the canonical *Type identity — typically minted via
// r.Types.MintType for named types being installed, or for anonymous
// `object {…}` declarations. The info's Type field is set to t for
// downstream code that needs the parent's *Type when extending.
func NewClassType(t *Type, info ClassTypeInfo) Value {
	info.Type = t
	return NewValueRaw(t, info)
}

// NewClassInstance creates an object instance value of the given
// type. The caller must provide the type's *Type identity (typically
// info.TypeRef.Type for the type currently being instantiated).
func NewClassInstance(t *Type, info ClassInstanceInfo) Value {
	return NewValueRaw(t, info)
}

// NewResourceType creates a Resource/Entity type value; sets info.Type to t.
func NewResourceType(t *Type, info ResourceTypeInfo) Value {
	info.Type = t
	return NewValueRaw(t, info)
}

// NewResourceInstance creates a resource instance value of the given type.
func NewResourceInstance(t *Type, info ResourceInstanceInfo) Value {
	return NewValueRaw(t, info)
}

// NewStore creates a Store value of the given type. Pass TStore for
// the builtin Object/Store, TStoreSystem for Object/Store/System, or
// a user-defined *Type for a custom store subtype. The Value's
// TypeName is derived from t.Path() for legacy prototype-chain code
// that compares store types by path string.
func NewStore(t *Type) Value {
	if t == nil {
		t = TStore
	}
	return NewValueRaw(t, &StoreInstanceInfo{
		TypeName: t.Path(),
		Data:     make(map[string]Value),
	})
}

// NewStoreValue wraps an existing StoreInstanceInfo into a Value.
// Pass t = nil to default to TStore.
func NewStoreValue(t *Type, si *StoreInstanceInfo) Value {
	if t == nil {
		t = TStore
	}
	return NewValueRaw(t, si)
}

// NewStoreWithPrototype creates a Store value of the given type with
// a prototype chain. Pass t = nil to default to TStore.
func NewStoreWithPrototype(t *Type, prototype *StoreInstanceInfo) Value {
	if t == nil {
		t = TStore
	}
	return NewValueRaw(t, &StoreInstanceInfo{
		TypeName:  t.Path(),
		Data:      make(map[string]Value),
		Prototype: prototype,
	})
}

// As* accessors for Scalar/Time/* moved to
// lang/go/engine/native_temporal.go (Step 6/7). The kernel no longer
// carries methods named for types it doesn't own. CalDurationData
// stays here because the payload struct is kernel-owned (it has the
// payloadMarker) — only the user-facing constructor / accessor
// surface moved.

// CalDurationData holds a calendar duration (years, months, days).
type CalDurationData struct {
	Years  int
	Months int
	Days   int
}

// ErrorInfo holds the details of an AQL error value.
type ErrorInfo struct {
	Message string // the short error description (an AqlError's Detail)
	// Code is the stable, dispatchable error code ("user_error",
	// "type_error", …) when the source was an AqlError (native or
	// `raise`d); empty for plain Go errors. Handlers branch on it via
	// `e.code` / `convert Map`.
	Code string
	// Data carries the extra keys of a `raise {code:… message:… …}`
	// spec map for programmatic handlers; nil otherwise. The formatter
	// prints code + message only.
	Data *OrderedMap
}

// NewError creates an error value from a Go error. An AqlError (the
// engine's structured error, including everything `raise` produces)
// contributes its stable Code, its SHORT Detail as the message (not
// the formatted multi-line report), and any raise payload; a plain Go
// error contributes only its text.
func NewError(err error) Value {
	info := ErrorInfo{Message: err.Error()}
	var ae *AqlError
	if errors.As(err, &ae) {
		info.Code = ae.Code
		info.Message = ae.Detail
		info.Data = ae.Data
	}
	return NewValueRaw(TError, info)
}

// IsError reports whether this value is an error.
func IsError(v Value) bool {
	return v.Parent.Equal(TError)
}

// AsError returns the ErrorInfo for an error value.
func AsError(v Value) (ErrorInfo, error) {
	info, ok := v.Data.(ErrorInfo)
	if !ok {
		return ErrorInfo{}, fmt.Errorf("AsError: not an error value (got %T)", v.Data)
	}
	return info, nil
}

// TimeoutInfo holds a pending timeout handle.
type TimeoutInfo struct {
	ID    string      // unique identifier
	Ms    int64       // delay in milliseconds
	Timer *time.Timer // underlying Go timer (nil after cancel)
}

// NewTimeout, IsTimeout, AsTimeout moved to lang/go/engine/native_misc.go
// (Step 8). Callers that need them use engine.NewTimeout /
// engine.AsTimeout, etc. The IsTimeout method is replaced by
// `v.Parent.Equal(engine.TTimeout)` at call sites.

// IntervalInfo holds a repeating interval handle.
type IntervalInfo struct {
	ID     string        // unique identifier
	Ms     int64         // interval in milliseconds
	Ticker *time.Ticker  // underlying Go ticker (nil after cancel)
	Done   chan struct{} // closed to signal cancellation
}

// NewInterval moved to lang/go/engine/native_misc.go (Step 8).

// IsInterval and AsInterval moved to lang/go/engine/native_misc.go (Step 8).

// IsWord reports whether this value is a word (function reference).
func IsWord(v Value) bool {
	return v.Parent.Equal(TWord)
}

// IsForward reports whether this value is a forward primitive.
func IsForward(v Value) bool {
	return v.Parent.Equal(TForward)
}

// IsBoolean reports whether this value is a boolean type.
func IsBoolean(v Value) bool {
	return v.Parent.ConformsTo(TBoolean)
}

// IsOpenParen reports whether this value is an open-paren marker.
func IsOpenParen(v Value) bool {
	return v.Parent.Equal(TOpenParen)
}

// IsCloseParen reports whether this value is a close-paren marker.
func IsCloseParen(v Value) bool {
	return v.Parent.Equal(TCloseParen)
}

// IsEnd reports whether this value is an end-marker.
func IsEnd(v Value) bool {
	return v.Parent.Equal(TEnd)
}

// IsParenExpr reports whether this value is a paren expression.
func IsParenExpr(v Value) bool {
	return v.Parent.Equal(TParenExpr)
}

// AsParenExpr returns the items in a paren expression value.
func AsParenExpr(v Value) ([]Value, error) {
	if pp, ok := v.Data.(ParenExprPayload); ok {
		return pp.Toks, nil
	}
	return nil, fmt.Errorf("AsParenExpr: not a paren-expr value (got %T)", v.Data)
}

// IsInterpString reports whether this value is an interpolated string.
func IsInterpString(v Value) bool {
	return v.Parent.Equal(TInterpString)
}

// AsInterpString returns the parts of an interpolated string value.
func AsInterpString(v Value) ([]InterpPart, error) {
	if ip, ok := v.Data.(InterpStringPayload); ok {
		return ip.Parts, nil
	}
	return nil, fmt.Errorf("AsInterpString: not an interp-string value (got %T)", v.Data)
}

// IsMark reports whether this value is a mark.
func IsMark(v Value) bool {
	return v.Parent.Equal(TMark)
}

// AsMark returns the MarkInfo, panics if not a mark.
func AsMark(v Value) (MarkInfo, error) {
	info, ok := v.Data.(MarkInfo)
	if !ok {
		return MarkInfo{}, fmt.Errorf("AsMark: not a mark value (got %T)", v.Data)
	}
	return info, nil
}

// IsMove reports whether this value is a move.
func IsMove(v Value) bool {
	return v.Parent.Equal(TMove)
}

// NewSplice wraps a value in an __SP (splice) marker. When the marker reaches
// the engine pointer its payload is spliced, unevaluated, into the stack.
func NewSplice(v Value) Value {
	return NewValueRaw(TSplice, SpliceInfo{Data: v})
}

// IsSplice reports whether this value is an __SP splice marker.
func IsSplice(v Value) bool {
	return v.Parent.Equal(TSplice)
}

// AsSplice returns the SpliceInfo, erroring if not a splice value.
func AsSplice(v Value) (SpliceInfo, error) {
	info, ok := v.Data.(SpliceInfo)
	if !ok {
		return SpliceInfo{}, fmt.Errorf("AsSplice: not a splice value (got %T)", v.Data)
	}
	return info, nil
}

// DispatchModInfo carries a `/`-modifier applied to a paren / dotted-path
// RESULT (a value), e.g. `(m.f)/s`, `path/3`, `m.a/r`. The parser emits a
// Word/__DM marker right after the group; execFnDefLiteral peeks and
// consumes it to dispatch the result function with these flags (or, for
// Ref/Quote, to leave it as inert data). ArgCount is -1 when unset. The
// `/u` (usurp) modifier is NOT carried here — it is emitted as the `usurp`
// word.
type DispatchModInfo struct {
	Ref   bool // /r — leave the function as data (do not invoke)
	Quote bool // /q — treat the result as data
}

func (DispatchModInfo) payloadMarker() {}

// NewDispatchMod builds a Word/__DM dispatch-modifier marker.
func NewDispatchMod(info DispatchModInfo) Value {
	return NewValueRaw(TDispatchMod, info)
}

// IsDispatchMod reports whether v is a Word/__DM marker.
func IsDispatchMod(v Value) bool { return v.Parent.Equal(TDispatchMod) }

// AsDispatchMod returns the DispatchModInfo if v is a Word/__DM marker.
func AsDispatchMod(v Value) (DispatchModInfo, bool) {
	info, ok := v.Data.(DispatchModInfo)
	return info, ok
}

// AsMove returns the MoveInfo, panics if not a move.
func AsMove(v Value) (MoveInfo, error) {
	info, ok := v.Data.(MoveInfo)
	if !ok {
		return MoveInfo{}, fmt.Errorf("AsMove: not a move value (got %T)", v.Data)
	}
	return info, nil
}

// IsReturnCheck reports whether this value is a return-check marker.
func IsReturnCheck(v Value) bool {
	return v.Parent.Equal(TReturnCheck)
}

// AsReturnCheck returns the ReturnCheckInfo, panics if not a return-check.
func AsReturnCheck(v Value) (ReturnCheckInfo, error) {
	info, ok := v.Data.(ReturnCheckInfo)
	if !ok {
		return ReturnCheckInfo{}, fmt.Errorf("AsReturnCheck: not a return-check value (got %T)", v.Data)
	}
	return info, nil
}

// IsDefCleanup reports whether this value is a def-cleanup marker.
func IsDefCleanup(v Value) bool {
	return v.Parent.Equal(TDefCleanup)
}

// AsDefCleanup returns the DefCleanupInfo, panics if not a def-cleanup.
func AsDefCleanup(v Value) (DefCleanupInfo, error) {
	info, ok := v.Data.(DefCleanupInfo)
	if !ok {
		return DefCleanupInfo{}, fmt.Errorf("AsDefCleanup: not a def-cleanup value (got %T)", v.Data)
	}
	return info, nil
}

// IsDisjunct reports whether this value is a disjunction type — a
// plain Disjunct (Type/Disjunct) or any subtype such as an Enum
// (Type/Disjunct/Enum).
func IsDisjunct(v Value) bool {
	_, ok := v.Data.(DisjunctInfo)
	return ok && v.Parent.ConformsTo(TDisjunct)
}

// AsDisjunct returns the DisjunctInfo, panics if not a disjunct.
func AsDisjunct(v Value) (DisjunctInfo, error) {
	info, ok := v.Data.(DisjunctInfo)
	if !ok {
		return DisjunctInfo{}, fmt.Errorf("AsDisjunct: not a disjunct value (got %T)", v.Data)
	}
	return info, nil
}

// IsNegation reports whether v is a negation (complement) type value.
func IsNegation(v Value) bool {
	_, ok := v.Data.(NegationInfo)
	return ok && v.Parent.ConformsTo(TNegation)
}

// AsNegation returns the NegationInfo payload, or an error if v is not
// a negation value.
func AsNegation(v Value) (NegationInfo, error) {
	info, ok := v.Data.(NegationInfo)
	if !ok {
		return NegationInfo{}, fmt.Errorf("AsNegation: not a negation value (got %T)", v.Data)
	}
	return info, nil
}

// IsClassType reports whether this value is an object type definition.
// The test is payload-based: any value carrying ClassTypeInfo is an
// object type, regardless of where its Parent sits in the lattice — a
// builtin like Resource is an object type whose Parent is the peer
// Ideal kind Ideal/Resource, not a descendant of Ideal/Object.
func IsClassType(v Value) bool {
	_, ok := v.Data.(ClassTypeInfo)
	return ok
}

// AsClassType returns the ClassTypeInfo, panics if not an object type.
func AsClassType(v Value) (ClassTypeInfo, error) {
	info, ok := v.Data.(ClassTypeInfo)
	if !ok {
		return ClassTypeInfo{}, fmt.Errorf("AsClassType: not an object type value (got %T)", v.Data)
	}
	return info, nil
}

// IsStore reports whether this value is a Store instance.
func IsStore(v Value) bool {
	_, ok := v.Data.(*StoreInstanceInfo)
	return ok && v.Parent.ConformsTo(TStore)
}

// AsStore returns the StoreInstanceInfo pointer. Returns an error if not a store.
func AsStore(v Value) (*StoreInstanceInfo, error) {
	si, ok := v.Data.(*StoreInstanceInfo)
	if !ok {
		return nil, fmt.Errorf("AsStore: not a store value (got %T)", v.Data)
	}
	return si, nil
}

// IsClassInstance reports whether this value is an object instance.
// Payload-based — see IsClassType.
func IsClassInstance(v Value) bool {
	_, ok := v.Data.(ClassInstanceInfo)
	return ok
}

// AsClassInstance returns the ClassInstanceInfo, panics if not an object instance.
func AsClassInstance(v Value) (ClassInstanceInfo, error) {
	info, ok := v.Data.(ClassInstanceInfo)
	if !ok {
		return ClassInstanceInfo{}, fmt.Errorf("AsClassInstance: not an object instance value (got %T)", v.Data)
	}
	return info, nil
}

// IsResourceType reports whether v is a Resource/Entity type descriptor.
func IsResourceType(v Value) bool {
	_, ok := v.Data.(ResourceTypeInfo)
	return ok
}

// AsResourceType returns the ResourceTypeInfo, or an error if v is not one.
func AsResourceType(v Value) (ResourceTypeInfo, error) {
	info, ok := v.Data.(ResourceTypeInfo)
	if !ok {
		return ResourceTypeInfo{}, fmt.Errorf("AsResourceType: not a resource type value (got %T)", v.Data)
	}
	return info, nil
}

// IsResourceInstance reports whether v is a Resource/Entity instance.
func IsResourceInstance(v Value) bool {
	_, ok := v.Data.(ResourceInstanceInfo)
	return ok
}

// AsResourceInstance returns the ResourceInstanceInfo, or an error if v is not one.
func AsResourceInstance(v Value) (ResourceInstanceInfo, error) {
	info, ok := v.Data.(ResourceInstanceInfo)
	if !ok {
		return ResourceInstanceInfo{}, fmt.Errorf("AsResourceInstance: not a resource instance value (got %T)", v.Data)
	}
	return info, nil
}

// IsFlatInstance reports whether v is a flat field-map instance — a
// class instance or a Resource/Entity instance. Both resolve every
// field (own + inherited) into a single map at make time with no
// prototype chain and carry an optional schema TypeRef, so container-
// shaped operations (field projection, size, deq, eq, serialization)
// treat them uniformly. Sites that special-case one MUST use this (or
// FlatInstanceFields / flatInstanceParts) so the two representations
// stay in lockstep.
func IsFlatInstance(v Value) bool {
	switch v.Data.(type) {
	case ClassInstanceInfo, ResourceInstanceInfo:
		return true
	}
	return false
}

// FlatInstanceFields returns a fresh copy of a flat instance's field
// map (class or Resource/Entity) and true; (nil, false) otherwise.
// Field order follows the instance's own order.
func FlatInstanceFields(v Value) (*OrderedMap, bool) {
	fields, _, ok := flatInstanceParts(v)
	if !ok {
		return nil, false
	}
	out := NewOrderedMap()
	if fields != nil {
		for _, k := range fields.Keys() {
			val, _ := fields.Get(k)
			out.Set(k, val)
		}
	}
	return out, true
}

// flatInstanceParts returns the LIVE (shared) flat Fields map and the
// declared schema key order (own + inherited) for a class or resource
// instance. The Fields map is not copied — callers that mutate must
// copy first (see FlatInstanceFields).
func flatInstanceParts(v Value) (fields *OrderedMap, schemaKeys []string, ok bool) {
	switch d := v.Data.(type) {
	case ClassInstanceInfo:
		if d.TypeRef != nil {
			schemaKeys = d.TypeRef.AllFields().Keys()
		}
		return d.Fields, schemaKeys, true
	case ResourceInstanceInfo:
		if d.TypeRef != nil {
			schemaKeys = d.TypeRef.AllFields().Keys()
		}
		return d.Fields, schemaKeys, true
	}
	return nil, nil, false
}

// IsAtom reports whether this value is an atom.
// IsPathon reports whether this value is a Path.
func IsPathon(v Value) bool {
	_, ok := v.Data.(PathonPayload)
	return ok && v.Parent.Equal(TPathon)
}

// AsPathon returns the PathonInfo, or an error if the value is not a path.
func AsPathon(v Value) (PathonInfo, error) {
	if pp, ok := v.Data.(PathonPayload); ok {
		return pp.Info, nil
	}
	return PathonInfo{}, fmt.Errorf("AsPathon: not a path value (got %T)", v.Data)
}

func IsAtom(v Value) bool {
	return v.Parent.ConformsTo(TAtom)
}

// payloadOf unwraps the payload variant P from v — the ONE nil-check +
// type-assertion + error shape every scalar accessor shares. name is the
// accessor name and kind the payload description for the error text
// ("AsString: not a string value (got %T)"), kept identical to the
// hand-rolled messages the accessors carried before unification.
func payloadOf[P Payload](v Value, name, kind string) (P, error) {
	var zero P
	if v.Data == nil {
		return zero, fmt.Errorf("%s: nil data", name)
	}
	p, ok := v.Data.(P)
	if !ok {
		return zero, fmt.Errorf("%s: not %s value (got %T)", name, kind, v.Data)
	}
	return p, nil
}

// AsAtom returns the string payload. Returns "" if Data is nil.
func AsAtom(v Value) (string, error) {
	p, err := payloadOf[AtomPayload](v, "AsAtom", "an atom")
	return p.Name, err
}

// IsTypedList reports whether this value is a typed list (has child type constraint).
func IsTypedList(v Value) bool {
	_, ok := v.Data.(ChildTypeInfo)
	return ok && v.Parent.Equal(TList)
}

// IsTypedMap reports whether this value is a typed map (has child type constraint).
func IsTypedMap(v Value) bool {
	_, ok := v.Data.(ChildTypeInfo)
	return ok && v.Parent.Equal(TMap)
}

// IsRecordType reports whether this value is a record type (map with field schema).
func IsRecordType(v Value) bool {
	_, ok := v.Data.(RecordTypeInfo)
	return ok && v.Parent.Equal(TMap)
}

// AsRecordType returns the RecordTypeInfo, panics if not a record type.
func AsRecordType(v Value) (RecordTypeInfo, error) {
	info, ok := v.Data.(RecordTypeInfo)
	if !ok {
		return RecordTypeInfo{}, fmt.Errorf("AsRecordType: not a record type value (got %T)", v.Data)
	}
	return info, nil
}

// IsOptionsType reports whether this value is an options type (map with defaults/constraints).
func IsOptionsType(v Value) bool {
	_, ok := v.Data.(OptionsTypeInfo)
	return ok && v.Parent.Equal(TMap)
}

// AsOptionsType returns the OptionsTypeInfo, panics if not an options type.
func AsOptionsType(v Value) (OptionsTypeInfo, error) {
	info, ok := v.Data.(OptionsTypeInfo)
	if !ok {
		return OptionsTypeInfo{}, fmt.Errorf("AsOptionsType: not an options type value (got %T)", v.Data)
	}
	return info, nil
}

// IsTableType reports whether this value is a table type (list with record schema).
func IsTableType(v Value) bool {
	if v.Parent.Equal(TList) {
		if _, ok := v.Data.(TableTypeInfo); ok {
			return true
		}
		if _, ok := v.Data.(TableData); ok {
			return true
		}
		if _, ok := v.Data.(MaterializerPayload); ok {
			return true
		}
		if _, ok := v.Data.(Materializer); ok {
			return true
		}
	}
	return false
}

// AsTableType returns the TableTypeInfo, panics if not a table type.
func AsTableType(v Value) (TableTypeInfo, error) {
	if td, ok := v.Data.(TableData); ok {
		return TableTypeInfo{Record: td.Record}, nil
	}
	if mp, ok := v.Data.(MaterializerPayload); ok {
		return TableTypeInfo{Record: mp.M.SourceRecord()}, nil
	}
	if mz, ok := v.Data.(Materializer); ok {
		return TableTypeInfo{Record: mz.SourceRecord()}, nil
	}
	info, ok := v.Data.(TableTypeInfo)
	if !ok {
		return TableTypeInfo{}, fmt.Errorf("AsTableType: not a table type value (got %T)", v.Data)
	}
	return info, nil
}

// AsChildType returns the ChildTypeInfo, panics if not a typed list or typed map.
func AsChildType(v Value) (ChildTypeInfo, error) {
	info, ok := v.Data.(ChildTypeInfo)
	if !ok {
		return ChildTypeInfo{}, fmt.Errorf("AsChildType: not a child type value (got %T)", v.Data)
	}
	return info, nil
}

// AsWord returns the WordInfo, panics if not a word.
func AsWord(v Value) (WordInfo, error) {
	info, ok := v.Data.(WordInfo)
	if !ok {
		return WordInfo{}, fmt.Errorf("AsWord: not a word value (got %T)", v.Data)
	}
	return info, nil
}

// AsForward returns the ForwardInfo, panics if not a forward.
func AsForward(v Value) (ForwardInfo, error) {
	info, ok := v.Data.(ForwardInfo)
	if !ok {
		return ForwardInfo{}, fmt.Errorf("AsForward: not a forward value (got %T)", v.Data)
	}
	return info, nil
}

// AsString returns the string payload. Returns "" if Data is nil (type literal).
func AsString(v Value) (string, error) {
	p, err := payloadOf[StrPayload](v, "AsString", "a string")
	return p.S, err
}

// AsInteger returns the int64 payload. Returns 0 if Data is nil (type literal).
func AsInteger(v Value) (int64, error) {
	p, err := payloadOf[IntPayload](v, "AsInteger", "an integer")
	return p.N, err
}

// AsFloat returns the float64 payload. Returns 0.0 if Data is nil (type literal).
func AsFloat(v Value) (float64, error) {
	p, err := payloadOf[FloatPayload](v, "AsFloat", "a float")
	return p.F, err
}

// AsBigInteger returns the *big.Int payload of a BigInteger value.
func AsBigInteger(v Value) (*big.Int, error) {
	p, err := payloadOf[BigIntPayload](v, "AsBigInteger", "a BigInteger")
	return p.N, err
}

// AsBigDecimal returns the *apd.Decimal payload of a BigDecimal value.
func AsBigDecimal(v Value) (*apd.Decimal, error) {
	p, err := payloadOf[DecimalPayload](v, "AsBigDecimal", "a BigDecimal")
	return p.D, err
}

// AsNumber returns the numeric value as float64. It is defined ONLY for
// the fixed-width leaves Integer and Float; the arbitrary-precision Big
// leaves return an error rather than silently projecting to float64 (that
// would reopen the precision-loss trap across ~60 callers). Code that
// genuinely wants a lossy Big→float64 projection must call AsFloatApprox.
func AsNumber(v Value) (float64, error) {
	if v.Parent.ConformsTo(TFloat) {
		return AsFloat(v)
	}
	if v.Parent.ConformsTo(TBigInteger) || v.Parent.ConformsTo(TBigDecimal) {
		return 0, fmt.Errorf("AsNumber: %s is arbitrary-precision; use AsFloatApprox for a lossy float64", v.Parent)
	}
	n, err := AsInteger(v)
	return float64(n), err
}

// AsFloatApprox projects any numeric leaf to float64, accepting the
// precision loss for the Big leaves. This is the explicit, auditable
// home for the lossy projection (e.g. transcendental math on Big values,
// IEEE classifiers) — never reach for it where exactness is required.
func AsFloatApprox(v Value) (float64, error) {
	switch {
	case v.Parent.ConformsTo(TBigInteger):
		n, err := AsBigInteger(v)
		if err != nil {
			return 0, err
		}
		f := new(big.Float).SetInt(n)
		out, _ := f.Float64()
		return out, nil
	case v.Parent.ConformsTo(TBigDecimal):
		d, err := AsBigDecimal(v)
		if err != nil {
			return 0, err
		}
		return d.Float64()
	default:
		return AsNumber(v)
	}
}

// AsBoolean returns the bool payload. Returns false if Data is nil (type literal).
func AsBoolean(v Value) (bool, error) {
	p, err := payloadOf[BoolPayload](v, "AsBoolean", "a boolean")
	return p.B, err
}

// AsList returns the []Value payload, or nil if the data is not a []Value.
// Also works for TableData and Materializer, returning the rows.
// For Materializer, this triggers materialization.
// AsList returns a read-only view of the list payload.
// Returns an error if the value lacks a list-shaped payload.
// Type literals (Data == nil) and unsupported payload types are
// reported as errors so callers can't silently treat a non-list
// value as an empty list.
func AsList(v Value) (ReadList, error) {
	if v.Data == nil {
		return ReadList{}, fmt.Errorf("AsList: nil data")
	}
	// Post Step 5c variant first; legacy []Value second.
	if lp, ok := v.Data.(ListPayload); ok {
		return ReadList{elems: lp.Elems}, nil
	}
	if fd, ok := v.Data.(*FlexListData); ok {
		return ReadList{elems: fd.Elems}, nil
	}
	if td, ok := v.Data.(TableData); ok {
		return ReadList{elems: td.Rows}, nil
	}
	if mp, ok := v.Data.(MaterializerPayload); ok {
		td, err := mp.M.Materialize()
		if err != nil {
			return ReadList{}, fmt.Errorf("AsList: materialize: %w", err)
		}
		return ReadList{elems: td.Rows}, nil
	}
	if mz, ok := v.Data.(Materializer); ok {
		td, err := mz.Materialize()
		if err != nil {
			return ReadList{}, fmt.Errorf("AsList: materialize: %w", err)
		}
		return ReadList{elems: td.Rows}, nil
	}
	// Typed list carrying both a child constraint and concrete
	// elements (`[v0 :T v1]`). Surface the elements so list-aware
	// operations (lengthq, firstq, is) see them.
	if ci, ok := v.Data.(ChildTypeInfo); ok && len(ci.Elements) > 0 {
		return ReadList{elems: ci.Elements}, nil
	}
	return ReadList{}, fmt.Errorf("AsList: not a list payload (got %T)", v.Data)
}

// AsMutableList returns the underlying []Value slice for mutation.
// Only valid for internal construction paths — never for immutable Node values.
// Returns an error if the value lacks a mutable list payload.
func AsMutableList(v Value) ([]Value, error) {
	if v.Data == nil {
		return nil, fmt.Errorf("AsMutableList: nil data")
	}
	if lp, ok := v.Data.(ListPayload); ok {
		return lp.Elems, nil
	}
	if fd, ok := v.Data.(*FlexListData); ok {
		return fd.Elems, nil
	}
	return nil, fmt.Errorf("AsMutableList: not a list payload (got %T)", v.Data)
}

// AsFlexList returns the pointer-backed element store of a FlexList
// value. Mutating words need the pointer — element growth (append,
// push) must reassign fd.Elems through it so every Value copy sharing
// the payload observes the change. Returns an error for anything that
// is not a concrete FlexList.
func AsFlexList(v Value) (*FlexListData, error) {
	if v.Data == nil {
		return nil, fmt.Errorf("AsFlexList: nil data")
	}
	if fd, ok := v.Data.(*FlexListData); ok {
		return fd, nil
	}
	return nil, fmt.Errorf("AsFlexList: not a flex list payload (got %T)", v.Data)
}

// NewFlexXml creates a mutable Node/Xml/FlexXml element. attr may be nil
// (treated as no attributes); cren may be nil (no children). The payload
// is pointer-backed so append/set mutate in place. See core_flex.go.
func NewFlexXml(tag string, attr *OrderedMap, cren []Value) Value {
	if attr == nil {
		attr = NewOrderedMap()
	}
	return NewValueRaw(TFlexXml, &FlexXmlData{Tag: tag, Attr: attr, Cren: cren})
}

// AsFlexXml returns the pointer-backed FlexXmlData of a FlexXml value;
// mutators reassign through it so every Value copy observes the change.
func AsFlexXml(v Value) (*FlexXmlData, error) {
	if fd, ok := v.Data.(*FlexXmlData); ok {
		return fd, nil
	}
	return nil, fmt.Errorf("AsFlexXml: not a flex xml payload (got %T)", v.Data)
}

// AsXmlElement returns the XmlElementPayload backing a Node/Xml value,
// or an error when v is a type literal or a non-Xml value.
func AsXmlElement(v Value) (XmlElementPayload, error) {
	if x, ok := v.Data.(XmlElementPayload); ok {
		return x, nil
	}
	return XmlElementPayload{}, fmt.Errorf("AsXmlElement: not an Xml element payload (got %T)", v.Data)
}

// AsMap returns a read-only view of the map payload.
// Returns an error if the value lacks a map-shaped payload.
// Type literals (Data == nil) and unsupported payload types are
// reported as errors so callers can't silently treat a non-map
// value as an empty map.
func AsMap(v Value) (ReadMap, error) {
	if v.Data == nil {
		return nil, fmt.Errorf("AsMap: nil data")
	}
	if mp, ok := v.Data.(MapPayload); ok {
		return mp.M, nil
	}
	// Typed map carrying both a child constraint and concrete entries
	// (`{k:v :T}`). Surface the entries as an OrderedMap so map-aware
	// operations see them.
	if ci, ok := v.Data.(ChildTypeInfo); ok && len(ci.Entries) > 0 {
		om := NewOrderedMap()
		for _, e := range ci.Entries {
			om.Set(e.Key, e.Value)
		}
		return om, nil
	}
	return nil, fmt.Errorf("AsMap: not a map payload (got %T)", v.Data)
}

// AsMutableMap returns the underlying *OrderedMap for mutation. Only valid
// for Object instances and internal construction — never for Node values.
// Returns an error if the value lacks a mutable map payload.
func AsMutableMap(v Value) (*OrderedMap, error) {
	if v.Data == nil {
		return nil, fmt.Errorf("AsMutableMap: nil data")
	}
	if mp, ok := v.Data.(MapPayload); ok {
		return mp.M, nil
	}
	return nil, fmt.Errorf("AsMutableMap: not a map payload (got %T)", v.Data)
}

// String returns a human-readable representation.
func (v Value) String() string {
	// A dynamic carrier renders as dynamic(<bound>) so the gradual
	// modality is legible in traces / `aql check` output instead of
	// masquerading as its bare bound (design/dynamic-modality-report.10.md).
	// Render the bound by clearing the flag and recursing.
	if v.Dynamic {
		inner := v
		inner.Dynamic = false
		return "dynamic(" + inner.String() + ")"
	}
	// Behavior-driven format delegation: types that supply a custom
	// TypeBehavior route through their Format. Walks the Parent
	// chain so descendants of a type with a custom Behavior inherit
	// it (e.g. TInspect inherits mapFormatBehavior via its TMap
	// parent). The DefaultBehavior sentinel — and any Behavior that
	// embeds defaultBehavior without overriding Format (e.g. the
	// scalar Comparer behaviors) — falls through to the kernel
	// renderer below.
	//
	// Type literals (Data==nil) are NOT delegated — they render as
	// their leaf type name uniformly across all types, including
	// types with custom Behaviors. See the Data==nil arm in
	// kernelFormatDefault.
	if v.Data != nil && v.Parent != nil {
		for t := v.Parent; t != nil; t = t.Parent {
			if t.Behavior == nil || t.Behavior == DefaultBehavior {
				continue
			}
			if delegatesFormat(t.Behavior) {
				continue
			}
			return t.Behavior.Format(v)
		}
	}
	return kernelFormatDefault(v)
}

// formatDelegatesToDefault is an opt-in marker for TypeBehavior impls
// that embed defaultBehavior without overriding Format. Without it,
// Value.String would dispatch into the embedded defaultBehavior.Format,
// which calls v.String() — a stack-overflow. Marker-tagged Behaviors
// are skipped so the walk falls through to kernelFormatDefault.
type formatDelegatesToDefault interface {
	formatDelegate()
}

// FormatDelegate is the EXPORTED counterpart of formatDelegatesToDefault,
// for external (lang-layer / plugin) Behaviors that delegate Format to
// DefaultBehavior and want canon / Value.String to fall through to the
// kernel switch — so the value renders by its lattice family (e.g. an
// Atom subtype as `name` / `name/q`) instead of routing through the
// delegating Format. Unexported interface methods can only be satisfied
// from this package, so out-of-package Behaviors need this hook.
type FormatDelegate interface {
	FormatDelegate()
}

// delegatesFormat reports whether b opts out of Format routing via
// either the internal or exported marker.
func delegatesFormat(b TypeBehavior) bool {
	if _, ok := b.(formatDelegatesToDefault); ok {
		return true
	}
	_, ok := b.(FormatDelegate)
	return ok
}

// kernelFormatDefault renders v using the kernel's canonical switch.
// Used both as the terminal of Value.String's Behavior walk and from
// defaultBehavior.Format so the two paths produce identical output
// without recursing through Value.String.
func kernelFormatDefault(v Value) string {
	switch {
	case IsBoundedType(v):
		// `Type of [B]` renders as the suffix sugar B/t — the shortest
		// round-trippable spelling, mirroring atoms rendering as name/q.
		n, err := AsBoundedType(v)
		if err != nil {
			return "Type"
		}
		return n.Leaf() + "/t"
	case IsWord(v):
		w, _ := AsWord(v)
		return fmt.Sprintf("word(%s)", w.Name)
	case IsForward(v):
		f, _ := AsForward(v)
		return fmt.Sprintf("forward(%s,%d/%d)", f.FuncName, f.CollectedArgs, f.ExpectedArgs)
	case IsOpenParen(v):
		return "("
	case IsCloseParen(v):
		return ")"
	case IsEnd(v):
		return "end"
	case IsParenExpr(v):
		_pe, _ := AsParenExpr(v)
		return fmt.Sprintf("paren(%v)", _pe)
	case IsMark(v):
		_as2, _ := AsMark(v)
		return fmt.Sprintf("mark(%s)", _as2.ID)
	case IsMove(v):
		m, _ := AsMove(v)
		return fmt.Sprintf("move(%s,%s)", m.To, m.Reason)
	case IsReturnCheck(v):
		rc, _ := AsReturnCheck(v)
		return fmt.Sprintf("returncheck(%s)", rc.FuncName)
	case IsDefCleanup(v):
		return "__dc"
	case IsError(v):
		_as3, _ := AsError(v)
		return fmt.Sprintf("error(%s)", _as3.Message)
	case IsNone(v):
		// The VALUE none (NonePayload, Data non-nil) — lowercase, like
		// the source literal and eng.Canon. The None TYPE literal has
		// Data==nil and takes the next arm, rendering capital `None`.
		return "none"
	case v.Data == nil:
		// Type literal (or carrier) with no specific value — render as
		// the type's leaf name; type names are globally unique so the
		// leaf alone is unambiguous.
		return typeNodeOf(v).Leaf()
	case v.IsDepScalar():
		// Must come before TString / TInteger / TFloat matches: the
		// lattice override makes DepString.ConformsTo(TString) (and the
		// numeric counterparts) true, so without this case the value
		// payload would be cast to the wrong concrete type.
		return renderDepScalar(v)
	case v.Parent.ConformsTo(TString):
		s, _ := AsString(v)
		return fmt.Sprintf("'%s'", s)
	case v.Parent.ConformsTo(TAtom):
		s, _ := AsAtom(v)
		return s
	// Big leaves come before Integer/Float: they don't conform to either,
	// but listing them first keeps the numeric arms grouped and guards
	// against a future widening of those checks.
	case v.Parent.ConformsTo(TBigInteger):
		n, _ := AsBigInteger(v)
		return FormatBigInteger(n)
	case v.Parent.ConformsTo(TBigDecimal):
		d, _ := AsBigDecimal(v)
		return FormatBigDecimal(d)
	case v.Parent.ConformsTo(TFloat):
		_as4, _ := AsFloat(v)
		return formatFloat(_as4)
	case v.Parent.ConformsTo(TInteger):
		n, _ := AsInteger(v)
		return fmt.Sprintf("%d", n)
	case v.Parent.ConformsTo(TBoolean):
		_as5, _ := AsBoolean(v)
		if _as5 {
			return "true"
		}
		return "false"
	// Domain types (Instant, DateTime, Date, TimeOfDay, CalendarDuration,
	// ClockDuration, Timezone, Matrix, Timeout, Interval) now render
	// via their per-Type Behavior installed by
	// coretype_format_behaviors.go and dispatched at the top of this
	// function. Their old switch arms have been removed.
	case IsPathon(v):
		_as6, _ := AsPathon(v)
		return _as6.String()
	case IsMicronValue(v):
		// Backstop for Micron instances whose minted node carries a
		// wrapper Behavior that delegates Format to the kernel default
		// (a bare-nominal newtype's bareRefineUnifier wraps the fresh
		// mint's DefaultBehavior) — without this arm they would fall
		// to the %v branch and render the payload struct.
		return micronRender(v)
	// TList rendering moved to listFormatBehavior in
	// coretype_list_map_behaviors.go (Step 10). The top-of-function
	// Behavior dispatch routes List values there.
	// Timeout / Interval render via their per-Type Behavior — see
	// coretype_format_behaviors.go. Their arms have been removed
	// from this switch.
	case IsClassInstance(v):
		oi, _ := AsClassInstance(v)
		// A class instance normally carries its schema TypeRef; guard
		// defensively against a nil/anonymous TypeRef by rendering under
		// the lattice leaf or ID instead of dereferencing an absent
		// schema (was a SIGSEGV).
		name := ""
		switch {
		case oi.TypeRef != nil && oi.TypeRef.Name != "":
			name = oi.TypeRef.Name
		case oi.TypeRef != nil:
			name = "Ideal/Class/" + oi.TypeRef.ID
		case v.Parent != nil:
			name = v.Parent.Leaf()
		default:
			name = "Class"
		}
		return formatFieldBag(name, oi.AllFields())
	case IsResourceInstance(v):
		ri, _ := AsResourceInstance(v)
		name := "Resource"
		switch {
		case ri.TypeRef != nil && ri.TypeRef.Name != "":
			name = ri.TypeRef.Name
		case v.Parent != nil:
			name = v.Parent.Leaf()
		}
		return formatFieldBag(name, ri.Fields)
	case IsClassType(v):
		ot, _ := AsClassType(v)
		name := ot.Name
		if name == "" {
			name = "Ideal/Class/" + ot.ID
		}
		return formatFieldBag("object<"+name+">", ot.AllFields())
	case IsDisjunct(v):
		di, _ := AsDisjunct(v)
		parts := make([]string, len(di.Alternatives))
		for i, alt := range di.Alternatives {
			parts[i] = alt.String()
		}
		return strings.Join(parts, " tor ")
	case IsNegation(v):
		ni, _ := AsNegation(v)
		// Parenthesise a compound inner so `tnot (A tor B)` doesn't misread
		// as `(tnot A) tor B`.
		if IsDisjunct(ni.Inner) || IsNegation(ni.Inner) {
			return "tnot (" + ni.Inner.String() + ")"
		}
		return "tnot " + ni.Inner.String()
	// A function value (TFnDef / TFunction, payload FnDefInfo) renders
	// as a compact `fn name(sig…)` summary. Crucially it does NOT fall
	// through to the `%v` default, which would dump the whole FnDefInfo
	// — including its captured *Registry and the module's entire exports
	// map (a 600-line spill into error messages). See formatFnDef.
	case isFnDefValue(v):
		fd, _ := v.Data.(FnDefInfo)
		return formatFnDef(fd)
	// TMap rendering moved to mapFormatBehavior in
	// coretype_list_map_behaviors.go (Step 10). The top-of-function
	// Behavior dispatch routes Map values (and TMap-rooted subtypes
	// like RecordType/OptionsType) there.
	default:
		return fmt.Sprintf("%v(%v)", v.Parent, v.Data)
	}
}

// isFnDefValue reports whether v carries an FnDefInfo payload (a fn or
// lambda value, Parent TFnDef or TFunction).
func isFnDefValue(v Value) bool {
	_, ok := v.Data.(FnDefInfo)
	return ok
}

// formatFnDef renders a function value as a compact one-line summary:
// `fn name(argTypes…)` per overload, e.g. `fn inc(Integer)` or
// `fn (Integer, String) or (Float)`. It reads ONLY Name and the
// argument types of each Signature — never Registry, Captured, or the
// raw FnSig bodies — so a function value can appear in an error message
// or a printed list without spilling its closure environment (the
// module exports map in particular). Anonymous lambdas have no Name and
// render as `fn(args…)`.
func formatFnDef(fd FnDefInfo) string {
	name := fd.Name
	// Summarise the argument shapes. Prefer non-empty signatures; a
	// synthesized 0-arg fallback alongside real overloads is noise here
	// (it is not shown by `help` either), so drop empty sigs when any
	// non-empty one exists.
	var sigParts []string
	for i := range fd.Signatures {
		if fd.Signatures[i].TotalArgs() > 0 {
			sigParts = append(sigParts, describeSigArgs(&fd.Signatures[i]))
		}
	}
	sigPart := strings.Join(sigParts, " or ")
	switch {
	case name == "" && sigPart == "":
		return "fn"
	case name == "":
		return "fn " + sigPart
	case sigPart == "":
		return "fn " + name
	default:
		return "fn " + name + sigPart
	}
}

// IsTypeValue returns true if v is a type literal, an Options instance,
// or a Node that contains a leaf that is a type.
func IsTypeValue(v Value) bool {
	// Type literal: Data==nil with a real type (not None).
	if v.Data == nil && !IsNoneShape(v) {
		return true
	}

	// Options type, record type, typed list/map, table type, object type.
	if IsOptionsType(v) || IsRecordType(v) || IsTypedList(v) ||
		IsTypedMap(v) || IsTableType(v) || IsClassType(v) {
		return true
	}

	// Concrete list: check each element recursively.
	if v.Parent.ConformsTo(TList) && v.Data != nil {
		elems, _ := AsList(v)
		if !elems.IsNil() {
			for _, elem := range elems.Slice() {
				if IsTypeValue(elem) {
					return true
				}
			}
		}
	}

	// Concrete map: check each value recursively.
	if v.Parent.ConformsTo(TMap) && v.Data != nil {
		m, _ := AsMap(v)
		if m != nil {
			for _, key := range m.Keys() {
				val, _ := m.Get(key)
				if IsTypeValue(val) {
					return true
				}
			}
		}
	}

	return false
}
