package parser

import (
	jsonic "github.com/tabnas/jsonic/go"
)

const configSyntaxMessage = "invalid options syntax"

// configSyntaxError keeps ParseConfig's public message independent of the
// host jsonic port while preserving the dependency error for errors.Is/As.
// SafeParse intentionally remains the low-level dependency-native seam.
type configSyntaxError struct{ cause error }

func (e *configSyntaxError) Error() string { return configSyntaxMessage }
func (e *configSyntaxError) Unwrap() error { return e.cause }

func configValueCategory(v any) string {
	switch v.(type) {
	case []any:
		return "list"
	case string:
		return "string"
	default:
		return "scalar"
	}
}

// ParseConfig parses a jsonic configuration string into a nested map —
// the backing parser for the CLI's `--options` flag and any other
// host-supplied option blob. It uses PLAIN jsonic (no boru grammar
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
		return nil, &configSyntaxError{cause: err}
	}
	// Options are order-agnostic config; flatten the parser's ordered node
	// (and any nested ones) to plain maps so callers keep the simple
	// map[string]any contract.
	m, ok := Plainify(v).(map[string]any)
	if !ok {
		return nil, &configShapeError{category: configValueCategory(v)}
	}
	return m, nil
}

// configShapeError uses Boru's own data categories instead of leaking Go's
// []interface{} spelling (whose JavaScript twin would be [object Array]).
type configShapeError struct{ category string }

func (e *configShapeError) Error() string {
	return "options must be a map of key:value pairs, got " + e.category
}
