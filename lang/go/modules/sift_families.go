package modules

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/aql-lang/aql/lang/go/native"
)

// The six format-family parsers (design/SIFT.0.md §5). Each is a pure function
// of (source, options): no registry state, no I/O — so a literal-source parse
// folds at check time (ParseLangSpec.Pure). siftParseFamily dispatches by
// family atom; a bad option or malformed line raises a sift_* error carrying
// the 1-based line number where applicable.

func siftParseFamily(family, src string, o siftOpts, r *native.Registry) (native.Value, error) {
	switch family {
	case famKV:
		return famParseKV(src, o, r)
	case famBlocks:
		return famParseBlocks(src, o, r)
	case famColumns:
		return famParseColumns(src, o, r)
	case famDSV:
		return famParseDSV(src, o, r)
	case famFixed:
		return famParseFixed(src, o, r)
	case famPattern:
		return famParsePattern(src, o, r)
	default:
		return native.Value{}, r.AqlError("sift_spec_error", "unknown family: "+family, "sift")
	}
}

// typesMap reads the `types` option into a name→type map, validating each.
func typesMap(o siftOpts, r *native.Registry) (map[string]string, error) {
	out := map[string]string{}
	sm := o.submap("types")
	if sm == nil {
		return out, nil
	}
	for _, k := range sm.Keys() {
		v, _ := sm.Get(k)
		t := native.ValToString(v)
		if !isKnownType(t) {
			return nil, r.AqlError("sift_spec_error", fmt.Sprintf("types.%s: unknown type %q", k, t), "sift")
		}
		out[k] = t
	}
	return out, nil
}

// resolveFields reads the `fields` option: a List yields names; a Map yields
// ordered names plus per-field types (a String/Atom value is the type, a Map
// value carries {type:…}). Merged into typeacc. given reports whether fields
// was supplied.
func resolveFields(o siftOpts, typeacc map[string]string, r *native.Registry) (names []string, given bool, err error) {
	v, ok := o.get("fields")
	if !ok {
		return nil, false, nil
	}
	if l, e := native.AsList(v); e == nil && !l.IsNil() {
		for i := 0; i < l.Len(); i++ {
			names = append(names, native.ValToString(l.Get(i)))
		}
		return names, true, nil
	}
	m, _ := native.AsMap(v)
	if m == nil {
		return nil, false, r.AqlError("sift_spec_error", "fields must be a list or a map", "sift")
	}
	for _, k := range m.Keys() {
		names = append(names, k)
		fv, _ := m.Get(k)
		if sub, e := native.AsMap(fv); e == nil && sub != nil {
			if tv, has := sub.Get("type"); has {
				t := native.ValToString(tv)
				if !isKnownType(t) {
					return nil, false, r.AqlError("sift_spec_error", fmt.Sprintf("fields.%s.type: unknown type %q", k, t), "sift")
				}
				typeacc[k] = t
			}
			continue
		}
		if native.IsConcrete(fv) {
			t := native.ValToString(fv)
			if !isKnownType(t) {
				return nil, false, r.AqlError("sift_spec_error", fmt.Sprintf("fields.%s: unknown type %q", k, t), "sift")
			}
			typeacc[k] = t
		}
	}
	return names, true, nil
}

// siftTable builds a Table value from ordered column names, their coercion
// types, and the row maps.
func siftTable(cols []string, types map[string]string, rows []native.Value) native.Value {
	fields := native.NewOrderedMap()
	for _, c := range cols {
		fields.Set(c, typeLiteralFor(types[c]))
	}
	return native.NewValueRaw(native.TList, native.TableData{
		Record: native.RecordTypeInfo{Fields: fields},
		Rows:   rows,
	})
}

// coerceCell coerces a raw cell under types[col] (falling back to def), routing
// a failure through strict.
func coerceCell(col, cell string, types map[string]string, def, strict string, line int, r *native.Registry) (native.Value, bool, error) {
	t := types[col]
	if t == "" {
		t = def
	}
	val, err := coerce(t, cell)
	if err != nil {
		if strict == "skip" {
			return native.NewNone(), true, nil
		}
		return native.Value{}, false, r.AqlError("sift_type_error", fmt.Sprintf("line %d: %v", line, err), "sift")
	}
	return val, false, nil
}

// ---- kv --------------------------------------------------------------------

func famParseKV(src string, o siftOpts, r *native.Registry) (native.Value, error) {
	lines, err := o.preprocess(src)
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	sep, err := o.str("sep", ":")
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	m, err := famKVLines(lines, o, sep, 0, r)
	if err != nil {
		return native.Value{}, err
	}
	return native.NewMap(m), nil
}

// famKVLines parses key/value lines into an ordered map, splitting on sep.
// base is the 1-based line number of lines[0] (blocks passes a per-block
// offset). Keys are normalized (names option) but NOT de-duplicated — the
// repeat option owns duplicate keys.
func famKVLines(lines []string, o siftOpts, sep string, base int, r *native.Registry) (*native.OrderedMap, error) {
	trim, err := o.boolean("trim", true)
	if err != nil {
		return nil, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	unquote, err := o.boolean("unquote", false)
	if err != nil {
		return nil, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	names, err := o.str("names", "keep")
	if err != nil {
		return nil, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	repeat, err := o.str("repeat", "last")
	if err != nil {
		return nil, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	vtype, err := o.str("vtype", "string")
	if err != nil {
		return nil, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	if !isKnownType(vtype) {
		return nil, r.AqlError("sift_spec_error", "unknown vtype: "+vtype, "sift")
	}
	types, err := typesMap(o, r)
	if err != nil {
		return nil, err
	}
	strict, err := o.strict()
	if err != nil {
		return nil, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	out := native.NewOrderedMap()
	for i, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		idx := strings.Index(ln, sep)
		if idx < 0 {
			if strict == "skip" {
				continue
			}
			return nil, r.AqlError("sift_parse_error", fmt.Sprintf("line %d: no separator %q", base+i+1, sep), "sift")
		}
		key, val := ln[:idx], ln[idx+len(sep):]
		if trim {
			key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		}
		if unquote {
			val = siftUnquote(val)
		}
		if names == "normalize" {
			key = normalizeName(key)
		}
		cv, skipped, cerr := coerceCell(key, val, types, vtype, strict, base+i+1, r)
		if cerr != nil {
			return nil, cerr
		}
		if skipped {
			continue
		}
		if err := kvStore(out, key, cv, repeat, r); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func kvStore(out *native.OrderedMap, key string, val native.Value, repeat string, r *native.Registry) error {
	prev, exists := out.Get(key)
	switch repeat {
	case "list":
		if exists {
			l, _ := native.AsMutableList(prev)
			out.Set(key, native.NewList(append(l, val)))
		} else {
			out.Set(key, native.NewList([]native.Value{val}))
		}
	case "first":
		if !exists {
			out.Set(key, val)
		}
	case "error":
		if exists {
			return r.AqlError("sift_parse_error", "duplicate key: "+key, "sift")
		}
		out.Set(key, val)
	case "last":
		out.Set(key, val)
	default:
		return r.AqlError("sift_spec_error", "opts.repeat must be 'last', 'first', 'error', or 'list'", "sift")
	}
	return nil
}

func siftUnquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// ---- blocks ----------------------------------------------------------------

func famParseBlocks(src string, o siftOpts, r *native.Registry) (native.Value, error) {
	lines, err := o.preprocess(src)
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	sep, err := o.str("sep", "blank")
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	var blocks [][]string
	var blockAt []int
	switch sep {
	case "blank":
		cur := []string{}
		at := 1
		for i, ln := range lines {
			if strings.TrimSpace(ln) == "" {
				if len(cur) > 0 {
					blocks = append(blocks, cur)
					blockAt = append(blockAt, at)
					cur = nil
				}
				continue
			}
			if len(cur) == 0 {
				at = i + 1
			}
			cur = append(cur, ln)
		}
		if len(cur) > 0 {
			blocks = append(blocks, cur)
			blockAt = append(blockAt, at)
		}
	case "indent":
		var cur []string
		at := 1
		for i, ln := range lines {
			if strings.TrimSpace(ln) == "" {
				continue
			}
			unindented := ln[0] != ' ' && ln[0] != '\t'
			if unindented && len(cur) > 0 {
				blocks = append(blocks, cur)
				blockAt = append(blockAt, at)
				cur = nil
			}
			if len(cur) == 0 {
				at = i + 1
			}
			cur = append(cur, ln)
		}
		if len(cur) > 0 {
			blocks = append(blocks, cur)
			blockAt = append(blockAt, at)
		}
	default:
		return native.Value{}, r.AqlError("sift_spec_error", "opts.sep must be 'blank' or 'indent'", "sift")
	}

	table, err := o.boolean("table", false)
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	// The block boundary rides `sep`; the inner key/value separator is `kvsep`
	// (default ":") so the two never collide.
	kvsep, err := o.str("kvsep", ":")
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	maps := make([]*native.OrderedMap, len(blocks))
	for i, b := range blocks {
		m, err := famKVLines(b, o, kvsep, blockAt[i]-1, r)
		if err != nil {
			return native.Value{}, err
		}
		maps[i] = m
	}
	if !table {
		elems := make([]native.Value, len(maps))
		for i, m := range maps {
			elems[i] = native.NewMap(m)
		}
		return native.NewList(elems), nil
	}
	// Table: union of keys in first-seen order; absent field → None.
	var cols []string
	seen := map[string]bool{}
	for _, m := range maps {
		for _, k := range m.Keys() {
			if !seen[k] {
				seen[k] = true
				cols = append(cols, k)
			}
		}
	}
	rows := make([]native.Value, len(maps))
	for i, m := range maps {
		row := native.NewOrderedMap()
		for _, c := range cols {
			if v, ok := m.Get(c); ok {
				row.Set(c, v)
			} else {
				row.Set(c, native.NewNone())
			}
		}
		rows[i] = native.NewMap(row)
	}
	return siftTable(cols, map[string]string{}, rows), nil
}

// ---- columns ---------------------------------------------------------------

var siftWSRe = regexp.MustCompile(`\s+`)

func famParseColumns(src string, o siftOpts, r *native.Registry) (native.Value, error) {
	lines, err := o.preprocess(src)
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	header, err := o.boolean("header", true)
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	nameMode, err := o.str("names", "normalize")
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	label, err := o.str("label", "")
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	types, err := typesMap(o, r)
	if err != nil {
		return native.Value{}, err
	}
	fieldNames, given, err := resolveFields(o, types, r)
	if err != nil {
		return native.Value{}, err
	}

	var cols []string
	body := lines
	if given {
		seen := map[string]int{}
		for _, f := range fieldNames {
			cols = append(cols, applyNames(f, "keep", seen))
		}
		if header && len(body) > 0 {
			body = body[1:] // header consumed, names ignored
		}
	} else {
		if !header {
			return native.Value{}, r.AqlError("sift_spec_error", "columns: header:false requires fields", "sift")
		}
		if len(body) == 0 {
			return siftTable(nil, types, nil), nil
		}
		seen := map[string]int{}
		for _, h := range siftFields(body[0]) {
			cols = append(cols, applyNames(h, nameMode, seen))
		}
		body = body[1:]
	}

	rows := make([]native.Value, 0, len(body))
	for i, ln := range body {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		var vals []string
		effCols := cols
		if label != "" {
			toks := siftFields(ln)
			row := native.NewOrderedMap()
			lbl := ""
			if len(toks) > 0 {
				lbl = strings.TrimSuffix(toks[0], ":")
				toks = toks[1:]
			}
			row.Set(label, native.NewString(lbl))
			for j, c := range cols {
				cell := ""
				if j < len(toks) {
					cell = toks[j]
				}
				cv, _, cerr := coerceCell(c, cell, types, "string", "error", i+1, r)
				if cerr != nil {
					return native.Value{}, cerr
				}
				row.Set(c, cv)
			}
			rows = append(rows, native.NewMap(row))
			continue
		}
		vals = siftSplitN(ln, len(effCols))
		row := native.NewOrderedMap()
		for j, c := range effCols {
			cell := ""
			if j < len(vals) {
				cell = vals[j]
			}
			cv, _, cerr := coerceCell(c, cell, types, "string", "error", i+1, r)
			if cerr != nil {
				return native.Value{}, cerr
			}
			row.Set(c, cv)
		}
		rows = append(rows, native.NewMap(row))
	}
	labeledCols := cols
	if label != "" {
		labeledCols = append([]string{label}, cols...)
	}
	return siftTable(labeledCols, types, rows), nil
}

// siftFields splits on whitespace runs, dropping empty leading/trailing fields.
func siftFields(s string) []string {
	return siftWSRe.Split(strings.TrimSpace(s), -1)
}

// siftSplitN splits on whitespace runs into at most n fields, the last greedy.
func siftSplitN(s string, n int) []string {
	s = strings.TrimSpace(s)
	if n <= 0 {
		return siftWSRe.Split(s, -1)
	}
	return siftWSRe.Split(s, n)
}

// ---- dsv -------------------------------------------------------------------

func famParseDSV(src string, o siftOpts, r *native.Registry) (native.Value, error) {
	lines, err := o.preprocess(src)
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	sep, err := o.str("sep", "ws")
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	limit, err := o.integer("limit", 0)
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	types, err := typesMap(o, r)
	if err != nil {
		return native.Value{}, err
	}
	fieldNames, given, err := resolveFields(o, types, r)
	if err != nil {
		return native.Value{}, err
	}

	split := func(ln string) []string {
		if sep == "ws" {
			return siftSplitN(ln, int(limit))
		}
		if limit > 0 {
			return strings.SplitN(ln, sep, int(limit))
		}
		return strings.Split(ln, sep)
	}

	var cols []string
	if given {
		seen := map[string]int{}
		for _, f := range fieldNames {
			cols = append(cols, applyNames(f, "keep", seen))
		}
	}
	rows := make([]native.Value, 0, len(lines))
	for i, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		vals := split(ln)
		if cols == nil {
			for j := range vals {
				cols = append(cols, fmt.Sprintf("c%d", j+1))
			}
		}
		row := native.NewOrderedMap()
		for j, c := range cols {
			cell := ""
			if j < len(vals) {
				cell = vals[j]
			}
			cv, _, cerr := coerceCell(c, cell, types, "string", "error", i+1, r)
			if cerr != nil {
				return native.Value{}, cerr
			}
			row.Set(c, cv)
		}
		rows = append(rows, native.NewMap(row))
	}
	return siftTable(cols, types, rows), nil
}

// ---- fixed -----------------------------------------------------------------

func famParseFixed(src string, o siftOpts, r *native.Registry) (native.Value, error) {
	lines, err := o.preprocess(src)
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	trim, err := o.boolean("trim", true)
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	types, err := typesMap(o, r)
	if err != nil {
		return native.Value{}, err
	}
	spans, err := fixedSpans(o, r)
	if err != nil {
		return native.Value{}, err
	}
	fieldNames, given, err := resolveFields(o, types, r)
	if err != nil {
		return native.Value{}, err
	}
	var cols []string
	if given {
		seen := map[string]int{}
		for _, f := range fieldNames {
			cols = append(cols, applyNames(f, "keep", seen))
		}
	} else {
		for j := range spans {
			cols = append(cols, fmt.Sprintf("c%d", j+1))
		}
	}
	if len(cols) != len(spans) {
		return native.Value{}, r.AqlError("sift_spec_error", "fixed: fields count must match column spans", "sift")
	}
	rows := make([]native.Value, 0, len(lines))
	for i, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		row := native.NewOrderedMap()
		for j, c := range cols {
			cell := fixedSlice(ln, spans[j][0], spans[j][1])
			if trim {
				cell = strings.TrimSpace(cell)
			}
			cv, _, cerr := coerceCell(c, cell, types, "string", "error", i+1, r)
			if cerr != nil {
				return native.Value{}, cerr
			}
			row.Set(c, cv)
		}
		rows = append(rows, native.NewMap(row))
	}
	return siftTable(cols, types, rows), nil
}

func fixedSpans(o siftOpts, r *native.Registry) ([][2]int, error) {
	_, hasCols := o.get("cols")
	_, hasWidths := o.get("widths")
	if hasCols && hasWidths {
		return nil, r.AqlError("sift_spec_error", "fixed: give exactly one of cols or widths", "sift")
	}
	if v, ok := o.get("cols"); ok {
		l, e := native.AsList(v)
		if e != nil || l.IsNil() {
			return nil, r.AqlError("sift_spec_error", "fixed: cols must be a list of [start end] pairs", "sift")
		}
		spans := make([][2]int, l.Len())
		for i := 0; i < l.Len(); i++ {
			pair, e := native.AsList(l.Get(i))
			if e != nil || pair.IsNil() || pair.Len() != 2 {
				return nil, r.AqlError("sift_spec_error", "fixed: each col must be a [start end] pair", "sift")
			}
			s, e1 := pair.Get(0).AsConcreteInteger()
			en, e2 := pair.Get(1).AsConcreteInteger()
			if e1 != nil || e2 != nil {
				return nil, r.AqlError("sift_spec_error", "fixed: col bounds must be integers", "sift")
			}
			spans[i] = [2]int{int(s), int(en)}
		}
		return spans, nil
	}
	if v, ok := o.get("widths"); ok {
		l, e := native.AsList(v)
		if e != nil || l.IsNil() {
			return nil, r.AqlError("sift_spec_error", "fixed: widths must be a list of integers", "sift")
		}
		spans := make([][2]int, l.Len())
		pos := 0
		for i := 0; i < l.Len(); i++ {
			w, e := l.Get(i).AsConcreteInteger()
			if e != nil {
				return nil, r.AqlError("sift_spec_error", "fixed: widths must be integers", "sift")
			}
			spans[i] = [2]int{pos, pos + int(w)}
			pos += int(w)
		}
		return spans, nil
	}
	return nil, r.AqlError("sift_spec_error", "fixed: one of cols or widths is required", "sift")
}

func fixedSlice(ln string, start, end int) string {
	runes := []rune(ln)
	if start < 0 || start > len(runes) {
		return ""
	}
	if end < 0 || end > len(runes) {
		end = len(runes)
	}
	if end < start {
		return ""
	}
	return string(runes[start:end])
}

// ---- pattern ---------------------------------------------------------------

var (
	siftReMu    sync.Mutex
	siftRECache = map[string]*regexp.Regexp{}
)

func siftCompile(src string) (*regexp.Regexp, error) {
	siftReMu.Lock()
	defer siftReMu.Unlock()
	if re, ok := siftRECache[src]; ok {
		return re, nil
	}
	re, err := regexp.Compile(src)
	if err != nil {
		return nil, err
	}
	siftRECache[src] = re
	return re, nil
}

func famParsePattern(src string, o siftOpts, r *native.Registry) (native.Value, error) {
	reSrc, err := o.str("re", "")
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	if reSrc == "" {
		return native.Value{}, r.AqlError("sift_spec_error", "pattern: opts.re is required", "sift")
	}
	if vt, _ := o.str("vtype", "string"); !isKnownType(vt) {
		return native.Value{}, r.AqlError("sift_spec_error", "unknown vtype: "+vt, "sift")
	}
	re, err := siftCompile(reSrc)
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", "pattern: bad regexp: "+err.Error(), "sift")
	}
	groups := re.SubexpNames()
	named := 0
	for _, g := range groups {
		if g != "" {
			named++
		}
	}
	if named == 0 {
		return native.Value{}, r.AqlError("sift_spec_error", "pattern: re must have at least one named group", "sift")
	}
	nameMode, err := o.str("names", "normalize")
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	vtype, err := o.str("vtype", "string")
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	types, err := typesMap(o, r)
	if err != nil {
		return native.Value{}, err
	}
	per, err := o.str("per", "source")
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	strict, err := o.strict()
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}
	lines, err := o.preprocess(src)
	if err != nil {
		return native.Value{}, r.AqlError("sift_spec_error", err.Error(), "sift")
	}

	// Field names: normalized group names, in group order, de-duplicated.
	seen := map[string]int{}
	colName := make([]string, len(groups))
	var cols []string
	for i, g := range groups {
		if g == "" {
			continue
		}
		colName[i] = applyNames(g, nameMode, seen)
		cols = append(cols, colName[i])
	}

	buildRow := func(match []string, line int) (*native.OrderedMap, bool, error) {
		row := native.NewOrderedMap()
		for i, g := range groups {
			if g == "" {
				continue
			}
			// A non-participating optional group has an empty submatch AND a
			// zero index range; regexp reports it as "" at index 2i..2i+1 = -1.
			col := colName[i]
			cell := match[i]
			if cell == "\x00" { // non-participating optional group → None (§5.2)
				row.Set(col, native.NewNone())
				continue
			}
			cv, skipped, cerr := coerceCell(col, cell, types, vtype, strict, line, r)
			if cerr != nil {
				return nil, false, cerr
			}
			if skipped {
				return nil, true, nil
			}
			row.Set(col, cv)
		}
		return row, false, nil
	}

	if per == "source" {
		text := strings.Join(lines, "\n")
		m := reSubmatchNilAware(re, text)
		if m == nil {
			if strict == "skip" {
				return native.NewNone(), nil
			}
			return native.Value{}, r.AqlError("sift_parse_error", "pattern: no match in source", "sift")
		}
		row, skipped, cerr := buildRow(m, 0)
		if cerr != nil {
			return native.Value{}, cerr
		}
		if skipped {
			return native.NewNone(), nil
		}
		return native.NewMap(row), nil
	}
	if per != "line" {
		return native.Value{}, r.AqlError("sift_spec_error", "opts.per must be 'source' or 'line'", "sift")
	}
	rows := make([]native.Value, 0, len(lines))
	for i, ln := range lines {
		m := reSubmatchNilAware(re, ln)
		if m == nil {
			if strict == "skip" {
				continue
			}
			return native.Value{}, r.AqlError("sift_parse_error", fmt.Sprintf("line %d: no match", i+1), "sift")
		}
		row, skipped, cerr := buildRow(m, i+1)
		if cerr != nil {
			return native.Value{}, cerr
		}
		if skipped {
			continue
		}
		rows = append(rows, native.NewMap(row))
	}
	return siftTable(cols, types, rows), nil
}

// reSubmatchNilAware returns the submatch strings, or nil on no match. It marks
// a non-participating group with a sentinel "\x00" (empty index range) so the
// caller can render None; a participating empty match stays "".
func reSubmatchNilAware(re *regexp.Regexp, s string) []string {
	idx := re.FindStringSubmatchIndex(s)
	if idx == nil {
		return nil
	}
	n := len(idx) / 2
	out := make([]string, n)
	for i := 0; i < n; i++ {
		lo, hi := idx[2*i], idx[2*i+1]
		if lo < 0 || hi < 0 {
			out[i] = "\x00" // non-participating → None
		} else {
			out[i] = s[lo:hi]
		}
	}
	return out
}
