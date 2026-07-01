package eng

// Signature implementation — a sealed sum type.
//
// A signature's run IMPLEMENTATION is one of two mutually-exclusive kinds:
//
//   - GoImpl — a Go `Handler` plus its dispatch knobs: a native word, a
//     usurp/force wrapper, the synthetic fallback, or a captured-native-fn
//     bound to a name.
//   - AQLImpl — an AQL `Body` of tokens: an installed `def fn` body, an
//     anonymous lambda, or a module-ref trivial-delegation (`Body=[Word(inner)]`).
//
// The discriminator is `dispatchHandler() == nil` — nil selects the
// body-splice path (a module-ref / un-installed lambda). An INSTALLED AQL fn
// is still an `AQLImpl`, but carries a derived body-splicing handler
// (buildFnBodyHandler) in `dispatch`, so its `dispatchHandler()` is non-nil.
//
// `normalizeSig` synthesizes `Impl` from the flat authoring fields at the
// install boundary — the SAME install-boundary demotion `Params` gets from the
// legacy `Args`. The flat `Handler`/`Body`/`FnFrame`/knob fields survive only
// as those authoring INPUTS; every reader consults `Impl` through the
// `Signature` accessors below (all reads were routed through them in Stage 3a).

// SigImpl is the sealed sum of a signature's run implementation. Only the two
// package-local variants satisfy it (the unexported `sigImpl()` seals the set,
// so the compiler enforces exhaustiveness at every type switch below).
type SigImpl interface {
	dispatchHandler() Handler
	sigImpl()
}

// GoImpl is the implementation of a native word, a usurp/force wrapper, the
// synthetic fallback, or a captured-native-fn bound to a name: a Go Handler
// plus the dispatch knobs that only a Go handler carries.
type GoImpl struct {
	Handler          Handler
	FullStack        bool
	CheckFullStackFn CheckFullStackFunc
	ParkResult       bool
	RunInCheckMode   bool
}

func (g *GoImpl) dispatchHandler() Handler { return g.Handler }
func (g *GoImpl) sigImpl()                 {}

// AQLImpl is the implementation of an installed AQL fn body, an anonymous
// lambda, or a module-ref delegation. `dispatch` is the derived body-splicer
// (buildFnBodyHandler) for an installed fn, or nil for a module-ref /
// un-installed lambda — in which case execFnDefLiteral splices `Body` directly.
// A nil `dispatch` reproduces today's `Handler==nil` splice discriminator.
type AQLImpl struct {
	Body     []Value
	FnFrame  *FnFrameMeta
	dispatch Handler
}

func (a *AQLImpl) dispatchHandler() Handler { return a.dispatch }
func (a *AQLImpl) sigImpl()                 {}

// --- Authoring constructors. Native words and internal Go-handler sites write
// `Impl: Go(handler, opts...)`; module-refs / un-installed bodies write
// `Impl: AQL(body)`. These are the ONLY way an implementation is spelled once
// the flat authoring fields are retired. ---

// GoOpt sets an optional dispatch knob on a GoImpl built by Go.
type GoOpt func(*GoImpl)

// Go builds the implementation of a native word / internal Go handler.
func Go(h Handler, opts ...GoOpt) *GoImpl {
	g := &GoImpl{Handler: h}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// RunInCheck marks the handler to run even under check mode (usurp/force
// re-dispatch wrappers, def/undef, refine, depscalar predicates).
func RunInCheck() GoOpt { return func(g *GoImpl) { g.RunInCheckMode = true } }

// Park makes execMatch advance past the spliced result instead of re-stepping
// it (the `ref` word).
func Park() GoOpt { return func(g *GoImpl) { g.ParkResult = true } }

// FullStack passes the full resolved stack to the handler (depth/pick/roll/…).
func FullStack() GoOpt { return func(g *GoImpl) { g.FullStack = true } }

// CheckFullStack sets the check-mode full-stack replacement handler (paired
// with FullStack on the stack-shuffling words).
func CheckFullStack(fn CheckFullStackFunc) GoOpt {
	return func(g *GoImpl) { g.CheckFullStackFn = fn }
}

// AQL builds the implementation of a module-ref trivial delegation or an
// un-installed lambda body — a token Body spliced by execFnDefLiteral (dispatch
// nil, no frame). InstallFnDef / compileFnDefLiteral build `&AQLImpl{…}`
// directly instead, so they can attach the derived body-splicer and frame meta.
func AQL(body []Value) *AQLImpl { return &AQLImpl{Body: body} }

// hasFlatImplInputs reports whether any legacy flat implementation field is
// set. normalizeSig synthesizes Impl from these inputs whenever any is present
// (so a copy-then-mutate site that changes a flat field re-derives Impl); an
// author that writes Impl directly leaves them all zero, and normalizeSig keeps
// the authored Impl untouched. Retired together with the flat fields.
func (s *Signature) hasFlatImplInputs() bool {
	return s.Handler != nil || len(s.Body) > 0 || s.FnFrame != nil ||
		s.FullStack || s.CheckFullStackFn != nil || s.ParkResult || s.RunInCheckMode
}

// --- Signature accessors: the SINGLE way every reader reaches the run
// implementation. They read `Impl` once normalizeSig has synthesized it, and
// fall back to the flat authoring fields for a Signature built in a test that
// never went through normalizeSig (Impl == nil). ---

// dispatchHandler returns the Go function execMatch invokes for this sig, or
// nil when the sig is dispatched by splicing its AQL Body (module ref /
// un-installed lambda). It is also the native-vs-AQL discriminator.
func (s *Signature) dispatchHandler() Handler {
	if s.Impl != nil {
		return s.Impl.dispatchHandler()
	}
	return s.Handler
}

// body returns the sig's AQL token body (nil for a Go sig).
func (s *Signature) body() []Value {
	switch impl := s.Impl.(type) {
	case *AQLImpl:
		return impl.Body
	case *GoImpl:
		return nil
	default:
		return s.Body
	}
}

// fnFrame returns the AQL fn-frame metadata stamped on an installed body /
// anonymous lambda (nil for Go sigs and module refs).
func (s *Signature) fnFrame() *FnFrameMeta {
	switch impl := s.Impl.(type) {
	case *AQLImpl:
		return impl.FnFrame
	case *GoImpl:
		return nil
	default:
		return s.FnFrame
	}
}

// fullStack reports whether the handler receives the full resolved stack.
func (s *Signature) fullStack() bool {
	switch impl := s.Impl.(type) {
	case *GoImpl:
		return impl.FullStack
	case *AQLImpl:
		return false
	default:
		return s.FullStack
	}
}

// checkFullStackFn returns the check-mode full-stack handler, if any.
func (s *Signature) checkFullStackFn() CheckFullStackFunc {
	switch impl := s.Impl.(type) {
	case *GoImpl:
		return impl.CheckFullStackFn
	case *AQLImpl:
		return nil
	default:
		return s.CheckFullStackFn
	}
}

// parkResult reports whether the pointer advances past the spliced result
// instead of re-stepping it.
func (s *Signature) parkResult() bool {
	switch impl := s.Impl.(type) {
	case *GoImpl:
		return impl.ParkResult
	case *AQLImpl:
		return false
	default:
		return s.ParkResult
	}
}

// runInCheckMode reports whether the handler runs even under check mode.
func (s *Signature) runInCheckMode() bool {
	switch impl := s.Impl.(type) {
	case *GoImpl:
		return impl.RunInCheckMode
	case *AQLImpl:
		return false
	default:
		return s.RunInCheckMode
	}
}
