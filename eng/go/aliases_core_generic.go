package eng

// Hand-written companions to the generated facade (aliases_core.go):
// generic functions cannot be aliased or var-wrapped, so each gets an
// explicit generic wrapper here.

import core "github.com/boru-lang/boru/core/go"

// Cap is the typed capability read — see core.Cap.
func Cap[T any](r *Registry, name string) (T, bool, error) {
	return core.Cap[T](r, name)
}
