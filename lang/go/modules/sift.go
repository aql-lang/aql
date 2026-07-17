package modules

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	eng "github.com/aql-lang/aql/eng/go"
	"github.com/aql-lang/aql/lang/go/native"
)

// The aql:sift module — the Sift namespace, the semi-structured-text tier of
// the parsing stack (design/SIFT.0.md). Between StringUtil / read {fmt:'lines'}
// (primitives) and parse json / aql:parse (rigid formats / grammars), sift
// parses the loose line- and field-oriented text Unix tools and /proc files
// emit. It is a PURE module — string in, value out, no capabilities — whose
// parsers are ordinary parselang kinds and whose extension mechanism is a
// declarative spec-map (a parser expressed as data).
//
// Six format families (kv, blocks, columns, dsv, fixed, pattern) register as
// parse kinds and read formats via the parselang/read bridge; a spec-map binds
// a family + options (or wraps a fn, family:'fn') under a name; the seven Sift
// words (define / parse / kinds / families / spec / detect / check) drive them.

// sift family atoms.
const (
	famKV      = "kv"
	famBlocks  = "blocks"
	famColumns = "columns"
	famDSV     = "dsv"
	famFixed   = "fixed"
	famPattern = "pattern"
	famFn      = "fn"
)

// siftFamilies is the ordered list of the six format families, surfaced by
// Sift.families and used to validate a spec's family atom.
var siftFamilies = []string{famKV, famBlocks, famColumns, famDSV, famFixed, famPattern}

// capSiftHost is the per-registry capability slot holding sift's own state:
// the registered spec catalog (for Sift.spec / Sift.kinds introspection) and
// the detection table, plus a guard so re-import is idempotent (families are
// registered into the shared parselang host state exactly once per registry).
const capSiftHost = "engine.sift.host"

type siftHostState struct {
	mu         sync.Mutex
	registered bool                 // builtin families installed into parselang state
	specs      map[string]*siftSpec // name → registered spec (families + presets + user)
	order      []string             // registration order, for Sift.kinds
	pathDetect map[string]string    // path glob → kind
	cmdDetect  map[string]string    // argv0 basename → kind
}

func siftHostStateFor(r *native.Registry) *siftHostState {
	if s, ok, _ := eng.Cap[*siftHostState](r, capSiftHost); ok && s != nil {
		return s
	}
	s := &siftHostState{
		specs:      map[string]*siftSpec{},
		pathDetect: map[string]string{},
		cmdDetect:  map[string]string{},
	}
	_ = r.Capabilities.Set(capSiftHost, s)
	return s
}

// siftSpec is a parser expressed as data (design/SIFT.0.md §6). A family spec
// names one of the six families plus its baked options; a fn spec (family:'fn')
// wraps a user function. detect metadata and doc are optional.
type siftSpec struct {
	name    string       // registered kind name; "" for an inline spec
	family  string       // one of siftFamilies, or famFn
	opts    native.Value // baked family options (a Map), or None
	fn      native.Value // the [source opts] fn, for family=='fn'
	pathPat []string     // detect: path globs
	cmdArgv []string     // detect: cmd.argv (canonical invocation)
	cmdName []string     // detect: cmd.match (argv0 basenames)
	doc     string       // one-line describe doc

	installIdent string // source-call identity (compiled-path idempotency)
	fromCheck    bool   // installed during the check pass; one VM re-run is idempotent
}

// ---------------------------------------------------------------------------
// Shared preprocessing (§5.1): skip / chop / comment / strict.
// ---------------------------------------------------------------------------

type siftOpts struct {
	m native.Value // the merged options Map (or None)
}

func (o siftOpts) get(key string) (native.Value, bool) {
	m, _ := native.AsMap(o.m)
	if m == nil {
		return native.Value{}, false
	}
	return m.Get(key)
}

// str reads an enumerated / literal String or Atom option, def when absent.
func (o siftOpts) str(key, def string) (string, error) {
	v, ok := o.get(key)
	if !ok {
		return def, nil
	}
	if v.Parent.ConformsTo(native.TAtom) {
		a, err := v.AsConcreteAtom()
		if err != nil {
			return "", fmt.Errorf("opts.%s: %w", key, err)
		}
		return a, nil
	}
	s, err := v.AsConcreteString()
	if err != nil {
		return "", fmt.Errorf("opts.%s must be a string or atom: %w", key, err)
	}
	return s, nil
}

func (o siftOpts) boolean(key string, def bool) (bool, error) {
	v, ok := o.get(key)
	if !ok {
		return def, nil
	}
	b, err := v.AsConcreteBoolean()
	if err != nil {
		return false, fmt.Errorf("opts.%s must be a boolean: %w", key, err)
	}
	return b, nil
}

func (o siftOpts) integer(key string, def int64) (int64, error) {
	v, ok := o.get(key)
	if !ok {
		return def, nil
	}
	n, err := v.AsConcreteInteger()
	if err != nil {
		return 0, fmt.Errorf("opts.%s must be an integer: %w", key, err)
	}
	return n, nil
}

// submap reads an option whose value is a Map (e.g. types), or nil.
func (o siftOpts) submap(key string) native.ReadMap {
	v, ok := o.get(key)
	if !ok {
		return nil
	}
	m, _ := native.AsMap(v)
	return m
}

// preprocess applies skip / chop / comment to the raw source, returning the
// surviving physical lines. strict is read separately by each family.
func (o siftOpts) preprocess(src string) ([]string, error) {
	skip, err := o.integer("skip", 0)
	if err != nil {
		return nil, err
	}
	chop, err := o.integer("chop", 0)
	if err != nil {
		return nil, err
	}
	comment, err := o.str("comment", "")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(src, "\n")
	// Drop a single trailing empty line (the final newline of a text file) so
	// row counts match intuition; interior blanks are preserved for blocks.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if skip < 0 || chop < 0 {
		return nil, fmt.Errorf("skip and chop must be non-negative")
	}
	if int(skip) < len(lines) {
		lines = lines[skip:]
	} else {
		lines = nil
	}
	if int(chop) < len(lines) {
		lines = lines[:len(lines)-int(chop)]
	} else {
		lines = nil
	}
	if comment != "" {
		kept := lines[:0:0]
		for _, ln := range lines {
			if strings.HasPrefix(strings.TrimLeft(ln, " \t"), comment) {
				continue
			}
			kept = append(kept, ln)
		}
		lines = kept
	}
	return lines, nil
}

func (o siftOpts) strict() (string, error) {
	s, err := o.str("strict", "error")
	if err != nil {
		return "", err
	}
	if s != "error" && s != "skip" {
		return "", fmt.Errorf("opts.strict must be 'error' or 'skip', got %q", s)
	}
	return s, nil
}

// ---------------------------------------------------------------------------
// Coercion vocabulary (§5.4).
// ---------------------------------------------------------------------------

var siftSizeRe = regexp.MustCompile(`^\s*([0-9]+(?:\.[0-9]+)?)\s*([a-zA-Z]*)\s*$`)

var siftSizeUnits = map[string]int64{
	"": 1, "b": 1,
	"k": 1 << 10, "kb": 1 << 10, "kib": 1 << 10,
	"m": 1 << 20, "mb": 1 << 20, "mib": 1 << 20,
	"g": 1 << 30, "gb": 1 << 30, "gib": 1 << 30,
	"t": 1 << 40, "tb": 1 << 40, "tib": 1 << 40,
	"p": 1 << 50, "pb": 1 << 50, "pib": 1 << 50,
}

// coerce applies a type atom to a raw cell string, returning the typed Value.
// An empty cell under a non-string type yields None; a conversion failure
// returns an error (the caller decides skip-vs-raise per strict).
func coerce(vtype, cell string) (native.Value, error) {
	if vtype == "" || vtype == "string" {
		return native.NewString(cell), nil
	}
	trimmed := strings.TrimSpace(cell)
	if trimmed == "" {
		return native.NewNone(), nil
	}
	switch vtype {
	case "integer":
		n, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return native.Value{}, fmt.Errorf("not an integer: %q", cell)
		}
		return native.NewInteger(n), nil
	case "float":
		f, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return native.Value{}, fmt.Errorf("not a float: %q", cell)
		}
		return native.NewFloat(f), nil
	case "boolean":
		switch strings.ToLower(trimmed) {
		case "true", "yes", "on", "1":
			return native.NewBoolean(true), nil
		case "false", "no", "off", "0":
			return native.NewBoolean(false), nil
		}
		return native.Value{}, fmt.Errorf("not a boolean: %q", cell)
	case "percent":
		f, err := strconv.ParseFloat(strings.TrimSuffix(trimmed, "%"), 64)
		if err != nil {
			return native.Value{}, fmt.Errorf("not a percentage: %q", cell)
		}
		return native.NewFloat(f), nil
	case "size":
		g := siftSizeRe.FindStringSubmatch(trimmed)
		if g == nil {
			return native.Value{}, fmt.Errorf("not a size: %q", cell)
		}
		mult, ok := siftSizeUnits[strings.ToLower(g[2])]
		if !ok {
			return native.Value{}, fmt.Errorf("unknown size unit in %q", cell)
		}
		f, err := strconv.ParseFloat(g[1], 64)
		if err != nil {
			return native.Value{}, fmt.Errorf("not a size: %q", cell)
		}
		return native.NewInteger(int64(f * float64(mult))), nil
	case "auto":
		if n, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return native.NewInteger(n), nil
		}
		if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return native.NewFloat(f), nil
		}
		switch strings.ToLower(trimmed) {
		case "true":
			return native.NewBoolean(true), nil
		case "false":
			return native.NewBoolean(false), nil
		}
		return native.NewString(cell), nil
	default:
		return native.Value{}, fmt.Errorf("unknown type %q", vtype)
	}
}

// isKnownType reports whether atom names a coercion type (for spec validation).
func isKnownType(t string) bool {
	switch t {
	case "string", "integer", "float", "boolean", "percent", "size", "auto":
		return true
	}
	return false
}

// typeLiteralFor maps a coercion type to the lattice type literal for a Table
// record schema; untyped/string columns stay TString (the csv precedent).
func typeLiteralFor(t string) native.Value {
	switch t {
	case "integer", "size":
		return native.NewTypeLiteral(native.TInteger)
	case "float", "percent":
		return native.NewTypeLiteral(native.TFloat)
	case "boolean":
		return native.NewTypeLiteral(native.TBoolean)
	default:
		return native.NewTypeLiteral(native.TString)
	}
}

// ---------------------------------------------------------------------------
// Name normalization (§5.3).
// ---------------------------------------------------------------------------

var siftWordBoundaryRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)
var siftCamelRe = regexp.MustCompile(`([a-z])([A-Z])`)

// normalizeName lowercases and kebab-cases a raw key/header, prefixing f- when
// it would start with a digit. Deterministic; the caller de-duplicates.
func normalizeName(raw string) string {
	s := siftCamelRe.ReplaceAllString(raw, "$1-$2")
	s = siftWordBoundaryRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	s = strings.ToLower(s)
	if s == "" {
		return s
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = "f-" + s
	}
	return s
}

// applyNames normalizes (or keeps) a name per the names option, de-duplicating
// collisions with -2, -3, … against the already-seen set.
func applyNames(raw, mode string, seen map[string]int) string {
	name := raw
	if mode == "normalize" {
		name = normalizeName(raw)
	}
	if n, clash := seen[name]; clash {
		seen[name] = n + 1
		name = fmt.Sprintf("%s-%d", name, n+1)
	} else {
		seen[name] = 1
	}
	return name
}
