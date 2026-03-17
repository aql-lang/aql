package native

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// NativeHandler is the signature for built-in native functions.
// It receives the matched arguments, the current context map, the
// resolved stack (excluding matched args), and the registry (for
// invoking AQL callbacks via r.CallAQL).
type NativeHandler func(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error)

// NativeFunc describes a built-in native function with its name, signatures,
// and whether it uses suffix precedence.
type NativeFunc struct {
	Name             string
	SuffixPrecedence bool
	Signatures       []NativeSig
}

// NativeSig describes one overload of a native function.
type NativeSig struct {
	Args       []engine.Type
	Precedence int
	Handler    NativeHandler
	Patterns   map[int]engine.Value // optional structural patterns for args
}

// Register installs all built-in native functions into the given registry.
func Register(r *engine.Registry) {
	for _, fn := range All() {
		for _, sig := range fn.Signatures {
			handler := makeFullStackHandler(r, sig.Handler)
			s := engine.Signature{
				Args:             sig.Args,
				Precedence:       sig.Precedence,
				FullStackHandler: handler,
				Patterns:         sig.Patterns,
			}
			if fn.SuffixPrecedence {
				r.Register(fn.Name, s)
			} else {
				r.RegisterPrefixOnly(fn.Name, s)
			}
		}
	}
}

// makeFullStackHandler wraps a NativeHandler into an engine.FullStackHandler,
// passing the registry's current context and the resolved stack.
// The wrapper prepends the remaining stack to the handler's return values
// so that FullStackHandler semantics are satisfied (it replaces 0..pointer).
func makeFullStackHandler(r *engine.Registry, h NativeHandler) func(args []engine.Value, stack []engine.Value) ([]engine.Value, error) {
	return func(args []engine.Value, stack []engine.Value) ([]engine.Value, error) {
		ctx := r.Context()
		results, err := h(args, ctx, stack, r)
		if err != nil {
			return nil, err
		}
		out := make([]engine.Value, len(stack)+len(results))
		copy(out, stack)
		copy(out[len(stack):], results)
		return out, nil
	}
}

// All returns all built-in native functions.
func All() []NativeFunc {
	return []NativeFunc{
		listFunc(),
		createFunc(),
		loadFunc(),
		updateFunc(),
		removeFunc(),
		transformFunc(),
		mergeFunc(),
		validateFunc(),
		getpathFunc(),
		setpathFunc(),
		injectFunc(),
		cloneFunc(),
		walkFunc(),
		selectorFunc(),
		sizeFunc(),
		sliceFunc(),
		padFunc(),
		itemsFunc(),
		fetchFunc(),
		flattenFunc(),
		filterFunc(),
		joinFunc(),
		jsonifyFunc(),
	}
}
