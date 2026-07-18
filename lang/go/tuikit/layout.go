package tuikit

import (
	"encoding/json"
	"fmt"
)

// The constraint solver of TUI.0.md §3.2, over the ValueToAny-shaped
// tree (maps/lists/strings/numbers — the single projection AQL values
// and wire JSON both arrive as). size: is data on each widget map:
//
//	<int>        fixed cells
//	{pct: n}     percent of the parent's extent
//	"fit"        intrinsic content size, capped by what remains
//	{flex: n}    weighted share of the remainder (default, weight 1)
//
// Resolution order fixed → pct → fit → flex; flex distributes the exact
// remainder with largest-remainder rounding; everything clamps at zero.

type sizeKind int

const (
	sizeFlex sizeKind = iota // the default (weight 1)
	sizeFixed
	sizePct
	sizeFit
)

type sizeSpec struct {
	kind sizeKind
	n    int
}

// toInt coerces the numeric shapes a ValueToAny / JSON tree can carry.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
		return 0, false
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i), true
		}
		return 0, false
	default:
		return 0, false
	}
}

// asNode reads a tree node as a widget map.
func asNode(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

// nodeStr / nodeInt / nodeBool / nodeList read optional fields with the
// recvDeadline convention: absent is fine, present-but-wrong-typed is a
// hard error.
func nodeStr(m map[string]any, key string) (string, bool, error) {
	v, ok := m[key]
	if !ok {
		return "", false, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", false, fmt.Errorf("%s: must be a String", key)
	}
	return s, true, nil
}

func nodeInt(m map[string]any, key string) (int, bool, error) {
	v, ok := m[key]
	if !ok {
		return 0, false, nil
	}
	n, ok := toInt(v)
	if !ok {
		return 0, false, fmt.Errorf("%s: must be an Integer", key)
	}
	return n, true, nil
}

func nodeBool(m map[string]any, key string) (bool, bool, error) {
	v, ok := m[key]
	if !ok {
		return false, false, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, false, fmt.Errorf("%s: must be a Boolean", key)
	}
	return b, true, nil
}

func nodeList(m map[string]any, key string) ([]any, bool, error) {
	v, ok := m[key]
	if !ok {
		return nil, false, nil
	}
	l, ok := v.([]any)
	if !ok {
		return nil, false, fmt.Errorf("%s: must be a List", key)
	}
	return l, true, nil
}

// parseSizeSpec reads a widget map's size: attribute.
func parseSizeSpec(m map[string]any) (sizeSpec, error) {
	v, ok := m["size"]
	if !ok {
		return sizeSpec{kind: sizeFlex, n: 1}, nil
	}
	if n, ok := toInt(v); ok {
		if n < 0 {
			n = 0
		}
		return sizeSpec{kind: sizeFixed, n: n}, nil
	}
	if s, ok := v.(string); ok {
		if s == "fit" {
			return sizeSpec{kind: sizeFit}, nil
		}
		return sizeSpec{}, fmt.Errorf("size: unknown keyword %q", s)
	}
	if sm, ok := asNode(v); ok {
		if n, has, err := nodeInt(sm, "pct"); err != nil {
			return sizeSpec{}, fmt.Errorf("size: %v", err)
		} else if has {
			return sizeSpec{kind: sizePct, n: n}, nil
		}
		if n, has, err := nodeInt(sm, "flex"); err != nil {
			return sizeSpec{}, fmt.Errorf("size: %v", err)
		} else if has {
			if n < 1 {
				n = 1
			}
			return sizeSpec{kind: sizeFlex, n: n}, nil
		}
		return sizeSpec{}, fmt.Errorf("size: a Map size needs a pct or flex key")
	}
	return sizeSpec{}, fmt.Errorf("size: must be an Integer, \"fit\", {pct: n} or {flex: n}")
}

// solveSizes assigns extents along one axis: total cells, a gap between
// neighbours, one spec per child, and each child's intrinsic fit size
// (consulted only for sizeFit).
func solveSizes(total, gap int, specs []sizeSpec, fits []int) []int {
	out := make([]int, len(specs))
	if len(specs) == 0 {
		return out
	}
	avail := total - gap*(len(specs)-1)
	if avail < 0 {
		avail = 0
	}
	remaining := avail

	claim := func(want int) int {
		if want < 0 {
			want = 0
		}
		if want > remaining {
			want = remaining
		}
		remaining -= want
		return want
	}
	for i, s := range specs {
		if s.kind == sizeFixed {
			out[i] = claim(s.n)
		}
	}
	for i, s := range specs {
		if s.kind == sizePct {
			out[i] = claim(avail * s.n / 100)
		}
	}
	for i, s := range specs {
		if s.kind == sizeFit {
			out[i] = claim(fits[i])
		}
	}
	// flex: distribute the exact remainder by weight, largest-remainder
	// rounding (ties broken by index) so the sizes sum exactly.
	totalWeight := 0
	for _, s := range specs {
		if s.kind == sizeFlex {
			totalWeight += s.n
		}
	}
	if totalWeight == 0 {
		return out
	}
	distributed := 0
	type frac struct{ i, rem int }
	fracs := make([]frac, 0, len(specs))
	for i, s := range specs {
		if s.kind != sizeFlex {
			continue
		}
		share := remaining * s.n
		out[i] = share / totalWeight
		distributed += out[i]
		fracs = append(fracs, frac{i: i, rem: share % totalWeight})
	}
	for left := remaining - distributed; left > 0; left-- {
		best := -1
		for j := range fracs {
			if best == -1 || fracs[j].rem > fracs[best].rem {
				best = j
			}
		}
		out[fracs[best].i]++
		fracs[best].rem = -1
	}
	return out
}
