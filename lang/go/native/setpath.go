package native

import (
	"fmt"

	voxgigstruct "github.com/voxgig/struct"
)

// The "setpath" word is registered via the consolidated Natives slice in
// natives.go.
//
// setpathHandler sets a nested value, returning a NEW structure. The path is
// either a dotted String (handled by voxgigstruct.SetPath) or a Reach lens
// (handled natively by setReachNative). Position-agnostic: it finds the path
// arg, then determines which of the remaining args is data vs new value.
func setpathHandler(args []Value, ctx map[string]Value, stack []Value, r *Registry) ([]Value, error) {
	// Find the path arg — a String dotted path, or a Reach lens.
	pathIdx := -1
	for i := range args {
		if args[i].Parent.Matches(TString) || IsReach(args[i]) {
			pathIdx = i
			break
		}
	}
	if pathIdx < 0 {
		return nil, r.AqlError("setpath_error", "setpath: path argument must be a string or a reach", "setpath")
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
	var data, newVal Value
	if others[0] < others[1] {
		if (a.Parent.Matches(TMap) || a.Parent.Matches(TList)) &&
			!(b.Parent.Matches(TMap) || b.Parent.Matches(TList)) {
			data, newVal = a, b
		} else if (b.Parent.Matches(TMap) || b.Parent.Matches(TList)) &&
			!(a.Parent.Matches(TMap) || a.Parent.Matches(TList)) {
			data, newVal = b, a
		} else {
			data, newVal = a, b
		}
	} else {
		data, newVal = b, a
	}

	// Reach lens: set natively so nesting is honored (an immutable nested
	// set, the lens "set" — the dotted-string voxgigstruct form does not
	// deep-set). The reach's receiver is ignored; data is the base.
	if IsReach(args[pathIdx]) {
		info, err := AsReach(args[pathIdx])
		if err != nil {
			return nil, fmt.Errorf("setpath: %w", err)
		}
		keys, err := reachPathKeys(r, info)
		if err != nil {
			return nil, err
		}
		out, err := setReachNative(data, keys, newVal)
		if err != nil {
			return nil, err
		}
		return []Value{out}, nil
	}

	path, err := AsString(args[pathIdx])
	if err != nil {
		return nil, fmt.Errorf("setpath: path: %w", err)
	}
	result := voxgigstruct.SetPath(valueToAny(data), path, valueToAny(newVal))
	val, err := anyToValue(result)
	if err != nil {
		return nil, fmt.Errorf("setpath: %w", err)
	}
	return []Value{val}, nil
}

// reachPathKeys decodes a Reach's segments into the key Values to walk
// (literal keys as-is; computed keys m.(expr) evaluated first). The get/getr
// distinction is irrelevant on a write path — setpath creates the path.
func reachPathKeys(r *Registry, info ReachInfo) ([]Value, error) {
	keys := make([]Value, 0, len(info.Segments))
	for i, seg := range info.Segments {
		if seg.Computed {
			sub := New(r)
			out, runErr := sub.Run(seg.KeyExpr)
			if runErr != nil {
				return nil, fmt.Errorf("setpath: computed key %d: %w", i, runErr)
			}
			if len(out) == 0 {
				return nil, fmt.Errorf("setpath: computed key %d produced no value", i)
			}
			keys = append(keys, out[len(out)-1])
		} else {
			keys = append(keys, seg.KeyLit)
		}
	}
	return keys, nil
}

// setReachNative immutably sets val at the key-path within data, returning a
// new structure. A numeric key into a concrete list updates that index (in a
// copy); any other key updates a map (shallow-cloned). Missing intermediate
// maps are created. Sibling values are shared by reference — safe because
// nothing is mutated in place.
func setReachNative(data Value, keys []Value, val Value) (Value, error) {
	if len(keys) == 0 {
		return val, nil
	}
	k := keys[0]
	rest := keys[1:]

	// List index.
	if k.Parent.Matches(TInteger) && data.Parent.Matches(TList) && IsConcrete(data) {
		lst, _ := AsList(data)
		n := lst.Len()
		idx64, _ := AsInteger(k)
		idx := int(idx64)
		if idx < 0 || idx >= n {
			return Value{}, fmt.Errorf("setpath: list index %d out of range (len %d)", idx, n)
		}
		cp := make([]Value, n)
		for i := 0; i < n; i++ {
			cp[i] = lst.Get(i)
		}
		child, err := setReachNative(cp[idx], rest, val)
		if err != nil {
			return Value{}, err
		}
		cp[idx] = child
		return NewList(cp), nil
	}

	// Map key (the default). Shallow-clone the source map (or start fresh
	// when the level is absent / not a map).
	out := NewOrderedMap()
	if data.Parent.Matches(TMap) && IsConcrete(data) {
		src, _ := AsMap(data)
		for _, key := range src.Keys() {
			v, _ := src.Get(key)
			out.Set(key, v)
		}
	}
	keyStr := reachKeyString(k)
	if len(rest) == 0 {
		out.Set(keyStr, val)
	} else {
		existing, _ := out.Get(keyStr)
		child, err := setReachNative(existing, rest, val)
		if err != nil {
			return Value{}, err
		}
		out.Set(keyStr, child)
	}
	return NewMap(out), nil
}

// reachKeyString renders a reach key Value as its map-key string (a word /
// atom by name, a string by value, anything else via ValToString).
func reachKeyString(k Value) string {
	switch {
	case IsWord(k):
		w, _ := AsWord(k)
		return w.Name
	case IsAtom(k):
		a, _ := AsAtom(k)
		return a
	case k.Parent.Matches(TString):
		s, _ := AsString(k)
		return s
	default:
		return ValToString(k)
	}
}
