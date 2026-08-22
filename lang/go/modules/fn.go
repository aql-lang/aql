package modules

import (
	"fmt"
	"sync"

	core "github.com/boru-lang/boru/core/go"

	"github.com/boru-lang/boru/lang/go/native"
)

// BuildFnUtilModule creates the "boru:fn-util" native module — the point-free
// function vocabulary the higher-order capability audit found missing
// (design/HIGHER-ORDER-FUNCTIONS.0.md §6.4): compose, pipe, curry, partial,
// const, identity, flip, on, memoize. Every word was writable in user boru in
// a handful of lines; shipping them native removes the friction AND the §5.8
// compile refusals the user-space spellings draw — a native word's produced
// wrapper is an ordinary Function value backed by a Go handler, invoked
// through the same callback seam filter uses (MatchFnSig + CallBoruFn on the
// fn's DEFINING registry, or InvokeBody for a compiled closure), so the
// FUNCTION-VALUE-SCOPE rule holds: a callback's free words resolve where the
// function was written.
//
// After import, words are accessed via dot notation: FnUtil.compose,
// FnUtil.partial, etc. `const` is a core word (the singleton-type maker), so
// its inner native is registered as `_f_const` — the same clash-avoiding
// convention as TypeUtil's `_t_merge` — while the export keeps the clean key.
//
// Application order convention, pinned in lang/spec/module-fn.tsv: the words
// follow the classical definitions —
//
//	(FnUtil.compose f g) x = f (g x)   — g first, mathematical composition
//	(FnUtil.pipe f g) x    = g (f x)   — f first, pipeline order
//	(FnUtil.on b u) x y    = b (u x) (u y)
//	(FnUtil.partial f a)   = a fn of f's remaining params with slot 0 bound to a
//	(FnUtil.curry f)       = the unary chain over f's single signature
//	(FnUtil.flip f)        = f with its signature argument order reversed
//	                         (UsurpFunction — the `/u` wrapper as a value word)
//	(FnUtil.memoize f)     = f behind a canon-keyed result cache
func BuildFnUtilModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := newModuleRegistry("boru:fn-util", fnUtilNatives)
	if err != nil {
		return native.ModuleDesc{}, err
	}

	// The function-taking slots are TFunction-typed so a `/v` reference (or
	// a bare fn name — NUR078's intercept) is COLLECTED forward instead of
	// dispatching mid-stream; a TAny slot would let the fn value start its
	// own call (the audit §2's forward-barrier rule). identity and const
	// keep TAny: they are the polymorphic words.
	exports := native.NewOrderedMap()
	exports.Set("identity", makeTypedFnDef("identity", subReg, native.TAny, native.TAny))
	exports.Set("const", makeTypedFnDef("_f_const", subReg, native.TFunction, native.TAny))
	exports.Set("compose", makeTypedFnDef("compose", subReg, native.TFunction, native.TFunction, native.TFunction))
	exports.Set("pipe", makeTypedFnDef("pipe", subReg, native.TFunction, native.TFunction, native.TFunction))
	exports.Set("flip", makeTypedFnDef("flip", subReg, native.TFunction, native.TFunction))
	exports.Set("curry", makeTypedFnDef("curry", subReg, native.TFunction, native.TFunction))
	exports.Set("partial", makeTypedFnDef("partial", subReg, native.TFunction, native.TFunction, native.TAny))
	exports.Set("on", makeTypedFnDef("on", subReg, native.TFunction, native.TFunction, native.TFunction))
	exports.Set("memoize", makeTypedFnDef("memoize", subReg, native.TFunction, native.TFunction))

	return moduleDesc(parent, "FnUtil", subReg, exports), nil
}

// ---- Shared helpers ----

// fnUtilArg validates a callable operand: an interpreter Function value
// (FnDefInfo). A compiled closure cannot reach these words today — every
// fn-util row refuses compilation (the frontier ledger) — so the closure
// arm waits for the §5.8 campaign to make it reachable, with coverage.
func fnUtilArg(v native.Value, opName string, r *native.Registry) (native.Value, error) {
	if _, ok := v.Data.(native.FnDefInfo); ok {
		return v, nil
	}
	return native.Value{}, r.BoruError("type_error",
		fmt.Sprintf("%s: argument must be a function value, got %s", opName, v.String()), opName)
}

// fnUtilSigArg validates a callable operand for the words that must
// INTROSPECT a signature (curry, partial, flip, memoize): only an
// interpreter FnDefInfo qualifies — a compiled closure carries no
// signature surface to reshape.
func fnUtilSigArg(v native.Value, opName string, r *native.Registry) (native.FnDefInfo, error) {
	if fd, ok := v.Data.(native.FnDefInfo); ok {
		return fd, nil
	}
	return native.FnDefInfo{}, r.BoruError("type_error",
		fmt.Sprintf("%s: argument must be a function value with a signature (a compiled closure or non-function cannot be reshaped), got %s", opName, v.String()), opName)
}

// fnUtilSingleSig returns the operand's one own signature, refusing
// multi-overload functions — reshaping picks ONE parameter list, and
// choosing among overloads silently would be a wrong-answer generator.
func fnUtilSingleSig(fd native.FnDefInfo, opName string, r *native.Registry) (*native.Signature, error) {
	own := fd.OwnSigs()
	if len(own) != 1 {
		return nil, r.BoruError("type_error",
			fmt.Sprintf("%s: the function must have exactly one signature, got %d overloads", opName, len(own)), opName)
	}
	return &own[0], nil
}

// invokeFnUtil applies a fn operand to args — the filter/parse callback
// seam's interpreter half: match a signature, run CallBoruFn on the fn's
// DEFINING registry (design/FUNCTION-VALUE-SCOPE.0.md). The InvokeBody
// closure half joins when a compiled closure can actually reach here.
func invokeFnUtil(r *native.Registry, opName string, fn native.Value, args []native.Value) ([]native.Value, error) {
	sig := native.MatchFnSig(fn, args)
	if sig == nil {
		return nil, r.BoruError("signature_error",
			fmt.Sprintf("%s: no signature of the applied function matches %d argument(s)", opName, len(args)), opName)
	}
	// A Go-impl signature (another fn-util wrapper, a usurp product) runs
	// its handler directly — CallBoru interprets BORU bodies only.
	if gi, ok := sig.Impl.(*core.GoImpl); ok {
		return gi.Handler(args, nil, nil, r)
	}
	var fnDef *native.FnDefInfo
	if fd, ok := fn.Data.(native.FnDefInfo); ok {
		fnDef = &fd
	}
	return core.CallBoruFn(r, fnDef, sig, args)
}

// invokeFnUtilOne is invokeFnUtil for the positions that feed a value
// onward (an inner stage of compose/pipe/on): exactly one result.
func invokeFnUtilOne(r *native.Registry, opName string, fn native.Value, args []native.Value) (native.Value, error) {
	vals, err := invokeFnUtil(r, opName, fn, args)
	if err != nil {
		return native.Value{}, err
	}
	if len(vals) != 1 {
		return native.Value{}, r.BoruError("type_error",
			fmt.Sprintf("%s: an inner function returned %d values where the pipeline needs exactly 1", opName, len(vals)), opName)
	}
	return vals[0], nil
}

// goFnValue builds a fresh anonymous Function value with one all-forward
// signature of nParams untyped (Any) parameters, backed by a Go handler —
// the shape every produced wrapper here takes. The wrapper is an ordinary
// first-class value: storable, passable, applied by the same dispatch that
// applies a user fn.
func goFnValue(name string, nParams int, h native.Handler) native.Value {
	params := make([]core.FnParam, nParams)
	for i := range params {
		params[i] = core.FnParam{Type: native.TAny}
	}
	sig := core.Signature{
		Params:     params,
		BarrierPos: nParams,
		Impl:       core.Go(h),
	}
	core.NormalizeSig(&sig)
	return native.NewFunction(native.FnDefInfo{
		Name:           name,
		Anonymous:      true,
		Signatures:     []core.Signature{sig},
		MaxForwardArgs: nParams,
	})
}

// ---- The natives ----

var fnUtilNatives = []native.NativeFunc{
	// identity — the total identity, any kind (a Function value passes
	// through inert; /v made the user-space spelling writable, this is
	// the named word).
	{
		Name: "identity",
		Signatures: []native.Signature{{
			BarrierPos: -1,
			Args:       []*native.Type{native.TAny},
			Returns:    []*native.Type{native.TAny},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				return []native.Value{args[0]}, nil
			}),
		}},
	},

	// _f_const (exported as `const`) — a one-argument function that
	// ignores its argument and returns the captured value.
	{
		Name: "_f_const",
		Signatures: []native.Signature{{
			BarrierPos: -1,
			Args:       []*native.Type{native.TAny},
			Returns:    []*native.Type{native.TFunction},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				x := args[0]
				return []native.Value{goFnValue("const", 1,
					func(_ []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
						return []native.Value{x}, nil
					})}, nil
			}),
		}},
	},

	// compose — (compose f g) x = f (g x).
	{
		Name: "compose",
		Signatures: []native.Signature{{
			BarrierPos: -1,
			Args:       []*native.Type{native.TFunction, native.TFunction},
			Returns:    []*native.Type{native.TFunction},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				f, err := fnUtilArg(args[0], "FnUtil.compose", r)
				if err != nil {
					return nil, err
				}
				g, err := fnUtilArg(args[1], "FnUtil.compose", r)
				if err != nil {
					return nil, err
				}
				return []native.Value{goFnValue("compose", 1,
					func(hargs []native.Value, _ map[string]native.Value, _ []native.Value, hr *native.Registry) ([]native.Value, error) {
						mid, err := invokeFnUtilOne(hr, "FnUtil.compose", g, []native.Value{hargs[0]})
						if err != nil {
							return nil, err
						}
						return invokeFnUtil(hr, "FnUtil.compose", f, []native.Value{mid})
					})}, nil
			}),
		}},
	},

	// pipe — (pipe f g) x = g (f x): compose in pipeline order.
	{
		Name: "pipe",
		Signatures: []native.Signature{{
			BarrierPos: -1,
			Args:       []*native.Type{native.TFunction, native.TFunction},
			Returns:    []*native.Type{native.TFunction},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				f, err := fnUtilArg(args[0], "FnUtil.pipe", r)
				if err != nil {
					return nil, err
				}
				g, err := fnUtilArg(args[1], "FnUtil.pipe", r)
				if err != nil {
					return nil, err
				}
				return []native.Value{goFnValue("pipe", 1,
					func(hargs []native.Value, _ map[string]native.Value, _ []native.Value, hr *native.Registry) ([]native.Value, error) {
						mid, err := invokeFnUtilOne(hr, "FnUtil.pipe", f, []native.Value{hargs[0]})
						if err != nil {
							return nil, err
						}
						return invokeFnUtil(hr, "FnUtil.pipe", g, []native.Value{mid})
					})}, nil
			}),
		}},
	},

	// flip — the function-value form of `/u`: the signature argument
	// order reversed (for a 2-arg fn, the classical flip).
	{
		Name: "flip",
		Signatures: []native.Signature{{
			BarrierPos: -1,
			Args:       []*native.Type{native.TFunction},
			Returns:    []*native.Type{native.TFunction},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				if _, err := fnUtilSigArg(args[0], "FnUtil.flip", r); err != nil {
					return nil, err
				}
				wrapped, ok := core.UsurpFunction(args[0])
				if !ok { //covergate:allow fnUtilSigArg already proved args[0] is a Function with FnDefInfo Data, the exact precondition UsurpFunction re-checks (§modules)
					return nil, r.BoruError("type_error", "FnUtil.flip: argument is not a function value", "FnUtil.flip")
				}
				return []native.Value{wrapped}, nil
			}),
		}},
	},

	// curry — the unary chain over a single-signature fn of 2+ params.
	{
		Name: "curry",
		Signatures: []native.Signature{{
			BarrierPos: -1,
			Args:       []*native.Type{native.TFunction},
			Returns:    []*native.Type{native.TFunction},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				fd, err := fnUtilSigArg(args[0], "FnUtil.curry", r)
				if err != nil {
					return nil, err
				}
				sig, err := fnUtilSingleSig(fd, "FnUtil.curry", r)
				if err != nil {
					return nil, err
				}
				n := len(sig.Params)
				if n < 2 {
					return nil, r.BoruError("type_error",
						fmt.Sprintf("FnUtil.curry: the function must take at least 2 parameters, got %d", n), "FnUtil.curry")
				}
				return []native.Value{curryLevel(args[0], nil, n)}, nil
			}),
		}},
	},

	// partial — bind the function's FIRST signature slot, returning a
	// fn of the remaining params. Chain calls to bind more.
	{
		Name: "partial",
		Signatures: []native.Signature{{
			BarrierPos: -1,
			Args:       []*native.Type{native.TFunction, native.TAny},
			Returns:    []*native.Type{native.TFunction},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				fd, err := fnUtilSigArg(args[0], "FnUtil.partial", r)
				if err != nil {
					return nil, err
				}
				sig, err := fnUtilSingleSig(fd, "FnUtil.partial", r)
				if err != nil {
					return nil, err
				}
				n := len(sig.Params)
				if n < 1 {
					return nil, r.BoruError("type_error",
						"FnUtil.partial: the function takes no parameters — nothing to bind", "FnUtil.partial")
				}
				fn := args[0]
				bound := args[1]
				return []native.Value{goFnValue("partial", n-1,
					func(hargs []native.Value, _ map[string]native.Value, _ []native.Value, hr *native.Registry) ([]native.Value, error) {
						full := make([]native.Value, 0, n)
						full = append(full, bound)
						full = append(full, hargs...)
						return invokeFnUtil(hr, "FnUtil.partial", fn, full)
					})}, nil
			}),
		}},
	},

	// on — (on b u) x y = b (u x) (u y): the binary op through a
	// unary projection, Haskell's `on`.
	{
		Name: "on",
		Signatures: []native.Signature{{
			BarrierPos: -1,
			Args:       []*native.Type{native.TFunction, native.TFunction},
			Returns:    []*native.Type{native.TFunction},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				b, err := fnUtilArg(args[0], "FnUtil.on", r)
				if err != nil {
					return nil, err
				}
				u, err := fnUtilArg(args[1], "FnUtil.on", r)
				if err != nil {
					return nil, err
				}
				return []native.Value{goFnValue("on", 2,
					func(hargs []native.Value, _ map[string]native.Value, _ []native.Value, hr *native.Registry) ([]native.Value, error) {
						ux, err := invokeFnUtilOne(hr, "FnUtil.on", u, []native.Value{hargs[0]})
						if err != nil {
							return nil, err
						}
						uy, err := invokeFnUtilOne(hr, "FnUtil.on", u, []native.Value{hargs[1]})
						if err != nil {
							return nil, err
						}
						return invokeFnUtil(hr, "FnUtil.on", b, []native.Value{ux, uy})
					})}, nil
			}),
		}},
	},

	// memoize — the fn behind a canon-keyed result cache. The cache is
	// per-wrapper (each memoize call mints a fresh one) and keys on the
	// canon of the full argument list, so any canon-distinct argument
	// re-invokes. Multi-return fns memoize the whole result stack.
	{
		Name: "memoize",
		Signatures: []native.Signature{{
			BarrierPos: -1,
			Args:       []*native.Type{native.TFunction},
			Returns:    []*native.Type{native.TFunction},
			Impl: native.Go(func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				fd, err := fnUtilSigArg(args[0], "FnUtil.memoize", r)
				if err != nil {
					return nil, err
				}
				sig, err := fnUtilSingleSig(fd, "FnUtil.memoize", r)
				if err != nil {
					return nil, err
				}
				fn := args[0]
				n := len(sig.Params)
				// The cache is captured by the returned handler, and that
				// handler rides a Function VALUE — which a ForkConcurrent
				// registry inherits, so `await`, timers, services and network
				// callbacks can drive the SAME closure from several
				// goroutines. An unsynchronised map there is not merely racy:
				// Go terminates the process on a concurrent map read/write.
				// The invoke deliberately runs OUTSIDE the lock — it re-enters
				// the interpreter and may itself call a memoized fn, which
				// under a held lock would deadlock. Two goroutines racing the
				// same cold key therefore both compute; last writer wins, and
				// since memoize's contract is a PURE fn keyed by argument
				// canon, either result is the same value.
				var mu sync.Mutex
				cache := map[string][]native.Value{}
				return []native.Value{goFnValue("memoize", n,
					func(hargs []native.Value, _ map[string]native.Value, _ []native.Value, hr *native.Registry) ([]native.Value, error) {
						key := native.Canon(hargs)
						mu.Lock()
						hit, ok := cache[key]
						mu.Unlock()
						if ok {
							out := make([]native.Value, len(hit))
							copy(out, hit)
							return out, nil
						}
						vals, err := invokeFnUtil(hr, "FnUtil.memoize", fn, hargs)
						if err != nil {
							return nil, err
						}
						stored := make([]native.Value, len(vals))
						copy(stored, vals)
						mu.Lock()
						cache[key] = stored
						mu.Unlock()
						return vals, nil
					})}, nil
			}),
		}},
	},
}

// curryLevel builds one level of the curry chain: a 1-param fn that,
// with the final argument, applies the original; before that, returns
// the next level. bound is copied per call so sibling partial
// applications never share a backing array.
func curryLevel(fn native.Value, bound []native.Value, total int) native.Value {
	return goFnValue("curry", 1,
		func(hargs []native.Value, _ map[string]native.Value, _ []native.Value, hr *native.Registry) ([]native.Value, error) {
			next := make([]native.Value, len(bound), len(bound)+1)
			copy(next, bound)
			next = append(next, hargs[0])
			if len(next) == total {
				return invokeFnUtil(hr, "FnUtil.curry", fn, next)
			}
			return []native.Value{curryLevel(fn, next, total)}, nil
		})
}
