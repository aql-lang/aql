package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// NativeHandler is the signature for built-in native functions.
// It receives the matched arguments, the current context map, the
// resolved stack (excluding matched args), and the registry (for
// invoking AQL callbacks via r.CallAQL).
type NativeHandler func(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error)

// NativeFunc describes a built-in native function with its name, signatures,
// and whether it uses forward precedence.
type NativeFunc struct {
	Name             string
	ForwardPrecedence bool
	Signatures       []NativeSig
}

// NativeSig describes one overload of a native function.
type NativeSig struct {
	Args     []engine.Type
	Handler  NativeHandler
	Patterns map[int]engine.Value // optional structural patterns for args
}

// Register installs all built-in native functions into the given registry.
func Register(r *engine.Registry) {
	for _, fn := range All() {
		for _, sig := range fn.Signatures {
			handler := makeFullStackHandler(r, sig.Handler)
			s := engine.Signature{
				Args:             sig.Args,
				FullStackHandler: handler,
				Patterns:         sig.Patterns,
			}
			if fn.ForwardPrecedence {
				r.Register(fn.Name, s)
			} else {
				r.RegisterStackOnly(fn.Name, s)
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
		// Reject type literals (Data==nil) that passed signature matching
		// but would cause nil pointer dereferences in native handlers.
		// Also reject Options types which carry OptionsTypeInfo, not
		// *OrderedMap, so AsMap() returns nil.
		for _, arg := range args {
			if arg.Data == nil && !arg.VType.Equal(engine.TNone) {
				return nil, fmt.Errorf("expected a concrete value, got type literal %s", arg.VType)
			}
			if arg.IsOptionsType() {
				return nil, fmt.Errorf("expected a concrete map, got options type %s", arg.String())
			}
		}
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
		prepareFunc(),
		directFunc(),
		flattenFunc(),
		filterFunc(),
		joinFunc(),
		jsonifyFunc(),
		pushFunc(),
		popFunc(),
		unshiftFunc(),
		shiftFunc(),
	}
}
