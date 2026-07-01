package eng

// Signature implementation accessors.
//
// A signature's IMPLEMENTATION is one of two kinds — a Go `Handler` (a native
// word, a usurp/force wrapper, the synthetic fallback, a captured-native-fn
// bound to a name) or an AQL `Body` of tokens (an installed `def fn` body, an
// anonymous lambda, a module trivial-delegation ref). The two are mutually
// exclusive at run time; the discriminator is `dispatchHandler() == nil`
// (nil ⇒ the body-splice path).
//
// These accessors are the SINGLE way every reader reaches an implementation
// field, so the storage can move behind them. Stage 3a: they return the flat
// `Signature` fields unchanged (behaviour-preserving). Stage 3b: the fields
// move into a `GoImpl | AQLImpl` sum stored in `Signature.Impl`, and only
// these accessors change — the readers already route through them.

// dispatchHandler returns the Go function execMatch invokes for this sig, or
// nil when the sig is dispatched by splicing its AQL Body (module ref /
// un-installed lambda). It is also the native-vs-AQL discriminator.
func (s *Signature) dispatchHandler() Handler { return s.Handler }

// body returns the sig's AQL token body (nil for a native sig).
func (s *Signature) body() []Value { return s.Body }

// fnFrame returns the AQL fn-frame metadata stamped on an installed body /
// anonymous lambda (nil for natives and module refs).
func (s *Signature) fnFrame() *FnFrameMeta { return s.FnFrame }

// fullStack reports whether the handler receives the full resolved stack.
func (s *Signature) fullStack() bool { return s.FullStack }

// checkFullStackFn returns the check-mode full-stack handler, if any.
func (s *Signature) checkFullStackFn() CheckFullStackFunc { return s.CheckFullStackFn }

// parkResult reports whether the pointer advances past the spliced result
// instead of re-stepping it.
func (s *Signature) parkResult() bool { return s.ParkResult }

// runInCheckMode reports whether the handler runs even under check mode.
func (s *Signature) runInCheckMode() bool { return s.RunInCheckMode }
