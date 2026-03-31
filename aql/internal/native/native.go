package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// NativeFunc describes a built-in native function with its name, signatures,
// and whether it uses forward precedence.
type NativeFunc struct {
	Name              string
	ForwardPrecedence bool
	Signatures        []NativeSig
}

// NativeSig describes one overload of a native function.
type NativeSig struct {
	Args     []engine.Type
	Handler  engine.Handler
	Patterns map[int]engine.Value // optional structural patterns for args
}

// Register installs all built-in native functions into the given registry.
func Register(r *engine.Registry) {
	for _, fn := range All() {
		for _, sig := range fn.Signatures {
			handler := wrapSafetyCheck(sig.Handler)
			s := engine.Signature{
				Args:     sig.Args,
				Handler:  handler,
				Patterns: sig.Patterns,
			}
			if fn.ForwardPrecedence {
				r.Register(fn.Name, s)
			} else {
				r.RegisterStackOnly(fn.Name, s)
			}
		}
	}
}

// wrapSafetyCheck wraps a Handler to reject type literals and Options types
// before the handler runs. This prevents nil pointer dereferences in native
// handlers that expect concrete data.
func wrapSafetyCheck(h engine.Handler) engine.Handler {
	return func(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
		for _, arg := range args {
			if arg.Data == nil && !arg.VType.Equal(engine.TNone) {
				return nil, fmt.Errorf("expected a concrete value, got type literal %s", arg.VType)
			}
			if arg.IsOptionsType() {
				return nil, fmt.Errorf("expected a concrete map, got options type %s", arg.String())
			}
		}
		return h(args, ctx, stack, r)
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
