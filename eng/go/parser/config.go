package parser

import (
	"fmt"

	jsonic "github.com/tabnas/jsonic/go"
)

// ParseConfig parses a jsonic configuration string into a nested map —
// the backing parser for the CLI's `--options` flag and any other
// host-supplied option blob. It uses PLAIN jsonic (no BORU grammar
// extensions), so it accepts ordinary relaxed-JSON / jsonic syntax:
//
//	a:1,b:2            => {a:1, b:2}
//	a:1,b:c:2          => {a:1, b:{c:2}}     (colon chains nest)
//	tape:initial:65536 => {tape:{initial:65536}}
//	{a:1,b:{c:2,d:3}}  => {a:1, b:{c:2, d:3}}
//	a:1, b:[1,2,3]     => {a:1, b:[1,2,3]}
//
// The top level must be a map (a bare `key:value,...` is an implicit
// map); a non-map top level is an error, since options are always
// keyed. Numbers come back as Go int or float64, strings as string,
// booleans as bool — the caller coerces per option.
// Plainify recursively converts the parser's insertion-ordered object nodes
// (jsonic.OrderedMap) to plain map[string]any, dropping key order — for
// order-agnostic consumers (config blobs, project manifests) that want the
// simple map[string]any shape. Re-exported so callers need not import jsonic.
func Plainify(v any) any { return jsonic.Plainify(v) }

func ParseConfig(s string) (map[string]any, error) {
	j := SafeMake(jsonic.Options{})
	v, err := j.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("invalid options syntax: %w", err)
	}
	// Options are order-agnostic config; flatten the parser's ordered node
	// (and any nested ones) to plain maps so callers keep the simple
	// map[string]any contract.
	m, ok := Plainify(v).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("options must be a map of key:value pairs, got %T", v)
	}
	return m, nil
}
