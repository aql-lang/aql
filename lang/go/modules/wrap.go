package modules

import (
	"github.com/aql-lang/aql/lang/go/native"
)

// wrap.go — the shared builders behind every native module's FnDef
// wrappers and the Build*Module scaffold.
//
// # The trivial-delegation contract
//
// Native modules under this package follow the sub-registry pattern:
// BuildXxxModule registers the module's Go words into an isolated
// sub-registry and exports FnDef WRAPPERS whose every FnSig delegates
// via the trivial body [Word(inner-name)] and carries
// `Registry: subReg`. execFnDefLiteral detects that shape and
// short-circuits to a direct execMatch dispatch of the inner native in
// the sub-registry — no body execution, no token splicing, no push
// reordering (see design/SIG-ORDER-REFACTOR.10.md and the
// "Module FnDef Wrappers" notes in lang/go/CLAUDE.md).
//
// Invariants every wrapper built here preserves:
//
//   - The FnDef's Name IS the body word. The short-circuit keys on
//     Body == [Word(fnDef.Name)], and dispatch resolves the inner
//     native via reg.Lookup(fnDef.Name) — so a clash-avoiding inner
//     name (e.g. TypeUtil's "_t_merge") is carried by the wrapper
//     name too, while the user-facing spelling is the EXPORT key.
//   - FnSig.Params is declared in the inner native's Signature.Args
//     order — top-first, sig order (Params[0] = top of stack under
//     matchSignature). The Params types feed the static analyser and
//     user-facing display; runtime dispatch consults the inner
//     native's own Signatures.
//   - The wrapper sig is all-forward (BarrierPos -1) unless a caller
//     explicitly opts into stack-only, because the INNER native's sig
//     governs dispatch and the wrapper's own sig must not veto the
//     swap form (`a pkg.word b`). Inner natives registered into a
//     module sub-registry MUST themselves use BarrierPos: -1 (the
//     rule pinned by wrapper_dispatch_test.go).
//
// makeModuleFnDef derives the wrapper sigs from the inner native's own
// Signatures — prefer it when the wrapper's surface mirrors the native
// exactly. makeWrapFnDef / makeTypedFnDef take explicit sigs for
// wrappers whose declared surface deliberately differs from the inner
// native's (e.g. MathUtil's Number facade over per-leaf Integer/Float
// overloads, or TypeUtil's Any-typed binary words whose handlers
// validate types themselves).

// makeModuleFnDef builds a FnDef value wrapping a moved-out native word. Each
// signature delegates via a trivial body [Word(name)], which execFnDefLiteral
// short-circuits to a direct dispatch of the inner native in the
// sub-registry. The wrapper FnSigs mirror the inner native's Signatures
// exactly — same arg types, returns, NoEvalArgs, and BarrierPos — so dispatch
// behaviour is identical to the former core word (dot-access dispatch keys
// off the inner native's signatures; see the "Module FnDef Wrappers" note in
// lang/go/CLAUDE.md). Shared by the fully-delegating modules (struct, io,
// logic, string, net, log, debug, …) and by the derived-wrapper loops of the
// partially-delegating ones (time's async words, binary's bitwise words,
// type's tpartial).
func makeModuleFnDef(n native.NativeFunc, subReg *native.Registry) native.Value {
	fnSigs := make([]native.FnSig, len(n.Signatures))
	for i, s := range n.Signatures {
		params := make([]native.FnParam, len(s.Args))
		for j, t := range s.Args {
			params[j] = native.FnParam{Type: t}
		}
		fnSigs[i] = native.FnSig{
			Params:     params,
			Returns:    s.Returns,
			Impl:       native.AQL([]native.Value{native.NewWord(n.Name)}),
			NoEvalArgs: s.NoEvalArgs,
			BarrierPos: s.BarrierPos,
			// Carry the inner native's check-mode ReturnsFn onto the wrapper
			// so static analysis of a dotted module call uses it instead of
			// the fixed Returns shape — e.g. the body-running debug words
			// analyse their quoted body for type errors.
			ReturnsFn: s.ReturnsFn,
		}
	}
	return native.NewFnDef(native.FnDefInfo{
		Name:       n.Name,
		Signatures: fnSigs,
		Registry:   subReg,
	})
}

// wrapSig is one explicit wrapper signature for makeWrapFnDef. params
// are in the inner native's Signature.Args order (top-first, sig
// order); returns is the static return tuple shown to the analyser.
// The three per-position maps mirror their FnSig fields so a wrapper
// can preserve quoted code bodies / schema maps / bare-word captures
// during its own forward collection (the inner native's Signature
// applies the same flags at the delegated dispatch).
type wrapSig struct {
	params    []native.FnParam
	returns   []*native.Type
	noEval    map[int]bool // FnSig.NoEvalArgs — quoted list-shaped code-body positions
	noEvalMap map[int]bool // FnSig.NoEvalMapArgs — quoted map-shaped schema positions
	quoteArgs map[int]bool // FnSig.QuoteArgs — bare-word-captured positions
	stackOnly bool         // BarrierPos 0 (explicit all-stack) instead of the -1 all-forward default
}

// makeWrapFnDef builds the trivial-delegation FnDef wrapper for the
// sub-registry native wordName, one FnSig per given wrapSig. wordName
// is both the FnDef name (what reg.Lookup resolves) and the body word
// (what the short-circuit dispatches), so it must be the inner
// native's registered name — the user-facing spelling is chosen by
// the exports.Set key at the call site.
func makeWrapFnDef(wordName string, subReg *native.Registry, sigs ...wrapSig) native.Value {
	fnSigs := make([]native.FnSig, len(sigs))
	for i, s := range sigs {
		barrier := -1 // all-forward eligible; see the contract note above
		if s.stackOnly {
			barrier = 0
		}
		fnSigs[i] = native.FnSig{
			Params:        s.params,
			Returns:       s.returns,
			Impl:          native.AQL([]native.Value{native.NewWord(wordName)}),
			NoEvalArgs:    s.noEval,
			NoEvalMapArgs: s.noEvalMap,
			QuoteArgs:     s.quoteArgs,
			BarrierPos:    barrier,
		}
	}
	return native.NewFnDef(native.FnDefInfo{
		Name:       wordName,
		Signatures: fnSigs,
		Registry:   subReg,
	})
}

// typeSig builds a wrapSig from unnamed param types (in the inner
// native's sig order, top-first) and a static return tuple.
func typeSig(params []*native.Type, returns ...*native.Type) wrapSig {
	fp := make([]native.FnParam, len(params))
	for i, t := range params {
		fp[i] = native.FnParam{Type: t}
	}
	return wrapSig{params: fp, returns: returns}
}

// makeTypedFnDef is the common-case convenience over makeWrapFnDef: a
// single signature of unnamed typed params (variadic, in the inner
// native's sig order, top-first) with one return type and no
// eval/quote flags.
func makeTypedFnDef(wordName string, subReg *native.Registry, ret *native.Type, params ...*native.Type) native.Value {
	return makeWrapFnDef(wordName, subReg, typeSig(params, ret))
}

// --- Build*Module scaffold pieces ---

// newModuleRegistry creates a module's isolated sub-registry (a fresh
// DefaultRegistry) and registers the given natives into it. Builders
// that must adopt the parent's type-ID sequence or mint module-owned
// types BEFORE registering their words keep constructing the registry
// by hand and register with a plain loop.
func newModuleRegistry(natives []native.NativeFunc) (*native.Registry, error) {
	subReg, err := newDefaultRegistry()
	if err != nil {
		return nil, err
	}
	for _, n := range natives {
		subReg.RegisterNativeFunc(n)
	}
	return subReg, nil
}

// delegatingExports builds an export map holding one derived
// (makeModuleFnDef) wrapper per native, keyed by the native's own
// name. Builders whose export keys differ from the inner names (log,
// debug) run their own rename loop instead.
func delegatingExports(natives []native.NativeFunc, subReg *native.Registry) *native.OrderedMap {
	exports := native.NewOrderedMap()
	for _, n := range natives {
		exports.Set(n.Name, makeModuleFnDef(n, subReg))
	}
	return exports
}

// moduleDesc assembles the ModuleDesc a native module builder returns:
// the sub-registry as the module's own Src, a module ID drawn from the
// importing registry's counter, and the export map bound under the
// module's namespace.
func moduleDesc(parent *native.Registry, namespace string, subReg *native.Registry, exports *native.OrderedMap) native.ModuleDesc {
	return native.ModuleDesc{
		Src:     subReg,
		ID:      parent.Modules.NextID(),
		Exports: map[string]*native.OrderedMap{namespace: exports},
	}
}

// buildDelegatingModule is the whole scaffold for a fully-delegating
// native module: an isolated sub-registry holding the given natives,
// and one derived FnDef wrapper per native exported under the
// module's namespace, keyed by the native's own name.
func buildDelegatingModule(parent *native.Registry, namespace string, natives []native.NativeFunc) (native.ModuleDesc, error) {
	subReg, err := newModuleRegistry(natives)
	if err != nil {
		return native.ModuleDesc{}, err
	}
	return moduleDesc(parent, namespace, subReg, delegatingExports(natives, subReg)), nil
}
