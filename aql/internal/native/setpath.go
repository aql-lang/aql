package native

import (
	"fmt"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	voxgigstruct "github.com/voxgig/struct"
)

// The "setpath" word is registered via the consolidated Natives slice in
// natives.go.
//
// setpathHandler calls voxgigstruct.SetPath to set a nested value.
// Position-agnostic: finds the string arg (path), then determines
// which of the remaining args is data vs new value.
func setpathHandler(args []engine.Value, ctx map[string]engine.Value, stack []engine.Value, r *engine.Registry) ([]engine.Value, error) {
	// Find the string arg — that's always the path.
	pathIdx := -1
	for i := range args {
		if args[i].VType.Matches(engine.TString) {
			pathIdx = i
			break
		}
	}
	if pathIdx < 0 {
		return nil, fmt.Errorf("setpath: path argument must be a string")
	}
	path, err := args[pathIdx].AsString()
	if err != nil {
		return nil, fmt.Errorf("setpath: path: %w", err)
	}

	// Collect the two non-path args.
	var others [2]int
	oi := 0
	for i := range args {
		if i != pathIdx && oi < 2 {
			others[oi] = i
			oi++
		}
	}

	// Convention: with forward-first matching for `data setpath "path" newVal`:
	// Forward args fill first positions → args[0]="path", args[1]=newVal, args[2]=data
	// For `setpath data "path" newVal` (all forward): args[0]=data, args[1]="path", args[2]=newVal
	// The data arg is typically a map/list; newVal can be anything.
	// Use positional heuristic: first non-path arg = data, second = newVal.
	// But when data comes from stack (last position), swap.
	a, b := args[others[0]], args[others[1]]
	var data, newVal engine.Value
	if others[0] < others[1] {
		// Normal order: first=data, second=newVal
		// But if first is after the path and second is even later (all forward),
		// or if last arg is a map/list (likely data from stack), adjust.
		if (a.VType.Matches(engine.TMap) || a.VType.Matches(engine.TList)) &&
			!(b.VType.Matches(engine.TMap) || b.VType.Matches(engine.TList)) {
			data, newVal = a, b
		} else if (b.VType.Matches(engine.TMap) || b.VType.Matches(engine.TList)) &&
			!(a.VType.Matches(engine.TMap) || a.VType.Matches(engine.TList)) {
			data, newVal = b, a
		} else {
			// Both same type — use original convention: args[0]=data, args[2]=newVal
			// For infix, data is last (from stack). For prefix, data is first.
			data, newVal = a, b
		}
	} else {
		data, newVal = b, a
	}

	result := voxgigstruct.SetPath(valueToAny(data), path, valueToAny(newVal))

	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("setpath: %w", err)
	}
	return []engine.Value{val}, nil
}
