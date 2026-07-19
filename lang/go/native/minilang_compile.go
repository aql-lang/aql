package native

import (
	"sync"

	eng "github.com/aql-lang/aql/eng/go"
)

// Compiled mini-languages (design/MINILANG.5.md §13). A kind may register an
// expansion-time COMPILE HOOK alongside its runtime transducer. When `mini
// <kind> <literal-src>` is expanded, the hook runs ONCE at the call site and
// returns the token list `mini` splices instead of the standard
// `MiniLang.lang_<kind> src opts end` call — so the DSL is parsed ahead of
// time (syntax errors surface with the call-site span) and the per-call cost
// drops to consuming a precompiled carrier (or inlined native ops).
//
// The transducer is never replaced: it is the semantic reference, the
// checker's target, and the fallback whenever src is not concrete. `mini` has
// no expansion cache, so a hook re-runs every time the call is stepped — a
// hook memoizes its own compile (as `re` does via miniCompiledPattern).
//
// Hooks are BUILT-IN (Go-only) machinery: they live in a per-registry table
// (RegisterMiniCompileGoHook) keyed by kind, discovered by miniHandler via
// miniGoHook. The AQL hook surface (MiniLang.register-compiled) died with
// the frozen kind namespace, and the kind set is fixed — so a hook can only
// belong to a built-in kind (`re` is the shipping example).

// miniHookToksMaterialisable reports whether a Go compile hook's expansion
// can be RECORDED by a compile pass: every token is either inert data
// (concrete / a bare type node) or plain code (words, markers). A runtime
// carrier in the splice (re's precompiled pattern, a partial over it) has no
// static materialisation; the caller then compiles the standard transducer
// call instead — sound per the §13 contract (the transducer is the semantic
// reference).
func miniHookToksMaterialisable(toks []Value) bool {
	for _, v := range toks {
		if v.Carrier || v.Dynamic {
			return false
		}
		// An Extension payload (re's precompiled *regexp behind a mini
		// carrier type) is CONCRETE by the payload predicate yet has no
		// const-pool materialisation — the exact shape this screen exists
		// to divert onto the standard call.
		if _, ext := v.Data.(eng.ExtensionPayload); ext {
			return false
		}
		// Everything else is either concrete data, a bare type node, or a
		// code token (word/marker) — recordable shapes (a shape the
		// recorder still cannot seat refuses downstream, which is the
		// sound fallback; this screen only diverts the KNOWN-unpoolable
		// runtime carriers).
	}
	return true
}

// MiniCompileHook is a Go expansion-time compiler: given the src and the
// (possibly deferred) opts value, it returns the tokens to splice.
type MiniCompileHook func(src string, opts Value, r *Registry) ([]Value, error)

const capMiniCompile = "engine.minilang.compile"

type miniCompileState struct {
	mu      sync.Mutex
	goHooks map[string]MiniCompileHook
}

func miniCompileStateFor(r *Registry, create bool) *miniCompileState {
	if s, ok, _ := eng.Cap[*miniCompileState](r, capMiniCompile); ok && s != nil {
		return s
	}
	if !create {
		return nil
	}
	s := &miniCompileState{goHooks: map[string]MiniCompileHook{}}
	_ = r.Capabilities.Set(capMiniCompile, s)
	return s
}

// RegisterMiniCompileGoHook installs a Go compile hook for kind on r.
func RegisterMiniCompileGoHook(r *Registry, kind string, hook MiniCompileHook) {
	s := miniCompileStateFor(r, true)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.goHooks[kind] = hook
}

// miniGoHook returns kind's Go compile hook, if registered.
func miniGoHook(r *Registry, kind string) (MiniCompileHook, bool) {
	s := miniCompileStateFor(r, false)
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.goHooks[kind]
	return h, ok
}
