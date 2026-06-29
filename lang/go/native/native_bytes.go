package native

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	eng "github.com/aql-lang/aql/eng/go"
)

// TBytes is the Scalar/Bytes type — an immutable byte string and the
// foundation for all binary-adjacent functionality (design/go-modules/
// BYTES.10.md). FixedID 1009 comes from the lang/go/native Scalar band
// (1000-1999, alongside the Scalar/Time family); it is pinned in
// lang/go/test/fixedid_stability_test.go. Registered as an external
// builtin in the var initialiser so package-level signature slices that
// reference TBytes see a non-nil pointer at slice-init time.
var TBytes = registerBytesType()

func registerBytesType() *eng.Type {
	t, err := eng.Builtin.RegisterExternalBuiltin("Scalar/Bytes", 1009, bytesBehavior{})
	if err != nil {
		recordTypeInitErr(fmt.Errorf("native_bytes: register Scalar/Bytes: %w", err))
	}
	return t
}

// init teaches the Go bridge how to convert []byte <-> Bytes. eng owns
// the bridge but not the type, so the conversion is installed here. The
// from-side copies (copy-on-ingest): the caller may reuse its slice.
// Done in init() rather than inside registerBytesType so the closures'
// reference to newBytes (-> TBytes) does not form a var-initialisation
// cycle — same reason miscNatives is built in init(). See native_misc.go.
func init() {
	eng.RegisterBytesBridge(
		func(b []byte) Value { return newBytes(append([]byte(nil), b...)) },
		func(v Value) ([]byte, bool) { return asBytes(v) },
	)
}

// newBytes wraps an OWNED []byte as an immutable Bytes value. The caller
// must not retain or mutate the slice afterwards: Bytes shares its
// backing array zero-copy on clone/fork/send (no DeepCloner), which is
// safe precisely because no word mutates a Bytes in place (BYTES.10.md
// §4). Construction at a trust boundary (the Go bridge, a future socket
// recv) must hand newBytes a fresh copy.
func newBytes(b []byte) Value { return eng.NewExtension(TBytes, b) }

// asBytes extracts the backing slice of a Bytes value.
func asBytes(v Value) ([]byte, bool) {
	if v.Parent == nil || !v.Parent.ConformsTo(TBytes) {
		return nil, false
	}
	ep, ok := v.Data.(ExtensionPayload)
	if !ok {
		return nil, false
	}
	b, ok := ep.Body.([]byte)
	return b, ok
}

// bytesBehavior renders Bytes as hex, compares byte-lexicographically,
// and reports its byte length. Match/Equal fall back to the kernel
// default for non-Bytes operands.
type bytesBehavior struct{}

func (bytesBehavior) Match(v Value, t *Type) bool { return eng.DefaultBehavior.Match(v, t) }

// Format renders Bytes as length-capped hex, e.g. Bytes<68 65 6c 6c 6f>.
func (bytesBehavior) Format(v Value) string {
	b, ok := asBytes(v)
	if !ok {
		return "Bytes<?>"
	}
	const capN = 32
	var sb strings.Builder
	sb.WriteString("Bytes<")
	show := len(b)
	if show > capN {
		show = capN
	}
	for i := 0; i < show; i++ {
		if i > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%02x", b[i])
	}
	if show < len(b) {
		fmt.Fprintf(&sb, " … (%d)", len(b))
	}
	sb.WriteByte('>')
	return sb.String()
}

func (bytesBehavior) Equal(a, b Value) bool {
	ab, aok := asBytes(a)
	bb, bok := asBytes(b)
	if !aok || !bok {
		return eng.DefaultBehavior.Equal(a, b)
	}
	return bytes.Equal(ab, bb)
}

// Compare orders Bytes byte-lexicographically (the Comparer capability),
// opening with the type-literal-first rule so a bare `Bytes` literal
// sorts below every concrete value (two literals bubble to Rank).
func (bytesBehavior) Compare(a, b Value) (int, error) {
	aLit, bLit := IsBareTypeNode(a), IsBareTypeNode(b)
	switch {
	case aLit && bLit:
		return 0, eng.ErrNoComparer
	case aLit:
		return -1, nil
	case bLit:
		return 1, nil
	}
	ab, aok := asBytes(a)
	bb, bok := asBytes(b)
	if !aok || !bok {
		return 0, eng.ErrNoComparer
	}
	return bytes.Compare(ab, bb), nil
}

// Size reports the byte length (the Sizer capability) so the generic
// `size` word works on Bytes with no dedicated word.
func (bytesBehavior) Size(v Value) int {
	if b, ok := asBytes(v); ok {
		return len(b)
	}
	return 0
}

// frameBehavior is the Behavior carried by a frame type — a Bytes subtype
// minted by `refine Bytes [layout]` (see installIdeals). It embeds
// bytesBehavior so a frame value renders/compares/sizes exactly like the
// Bytes it is, and adds the parsed `layout`: the segment list that
// `make`/`unpack`/`unpack-prefix` read to pack and decode. The layout lives
// on the TYPE (not in a value or a side table) so it travels with the type
// binding and there is no secondary parse at the call site (ADR-007). When
// `def P` wraps this type with a bareRefineUnifier, the wrapper preserves
// this Behavior as its `prev`; frameBehaviorOf walks the chain to recover it.
type frameBehavior struct {
	bytesBehavior
	layout []bitSeg // the parsed, validated frame layout
	raw    Value    // the original layout List<Map> (for inspection/convert)
}

// frameBehaviorOf walks t's Behavior chain (down through any kernel wrapper
// installed by `def`/`refine`) and returns the frameBehavior if t is a frame
// type. eng.PrevBehavior follows behaviorWrapper.prev.
func frameBehaviorOf(t *Type) (*frameBehavior, bool) {
	if t == nil {
		return nil, false
	}
	var b TypeBehavior = t.Behavior
	for b != nil {
		if fb, ok := b.(*frameBehavior); ok {
			return fb, true
		}
		nb, ok := eng.PrevBehavior(b)
		if !ok {
			break
		}
		b = nb
	}
	return nil, false
}

// bytesNatives registers the Bytes operations as SIGNATURE OVERLOADS of
// existing words (no new core words except unpack-prefix). Conversions go
// through `convert`, sub-ranges through `slice`, concatenation through
// `add`; the bit-syntax builds with `make Bytes`, decodes with `unpack`,
// and streams with `unpack-prefix`. RegisterNativeFunc APPENDS these sigs
// to the existing words (eng/go/registry.go upsertFnDef); a more-specific
// sig wins dispatch, so the Bytes overloads take precedence over the
// generic `[Scalar Scalar]` / `[Scalar Any]` forms.
var bytesNatives = []NativeFunc{
	{
		Name: "convert",
		Signatures: []NativeSig{
			// String <-> Bytes (UTF-8), List <-> Bytes (0-255 ints), and
			// Bytes -> Bytes (compact copy). Target type is the literal arg0.
			{Args: []*Type{TBytes, TString}, TypeArgs: map[int]bool{0: true}, Handler: convertStringToBytes, ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*Type{TString, TBytes}, TypeArgs: map[int]bool{0: true}, Handler: convertBytesToString, ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*Type{TBytes, TList}, TypeArgs: map[int]bool{0: true}, Handler: convertListToBytes, ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*Type{TList, TBytes}, TypeArgs: map[int]bool{0: true}, Handler: convertBytesToList, ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1},
			{Args: []*Type{TBytes, TBytes}, TypeArgs: map[int]bool{0: true}, Handler: convertBytesToBytes, ReturnsFn: ReturnsFreshInstance(0), BarrierPos: -1},
		},
	},
	{
		Name: "slice",
		Signatures: []NativeSig{
			// `slice start end b` (end-exclusive, zero-copy view); data last.
			{Args: []*Type{TInteger, TInteger, TBytes}, Handler: bytesSliceStartEnd, Returns: []*Type{TBytes}, BarrierPos: -1},
			{Args: []*Type{TInteger, TBytes}, Handler: bytesSliceStart, Returns: []*Type{TBytes}, BarrierPos: -1},
			{Args: []*Type{TBytes}, Handler: bytesSliceAll, Returns: []*Type{TBytes}, BarrierPos: -1},
		},
	},
	{
		Name: "add",
		Signatures: []NativeSig{
			// `a add b` concatenates two byte strings (mirrors String add).
			{Args: []*Type{TBytes, TBytes}, Handler: addBytesHandler, Returns: []*Type{TBytes}, BarrierPos: -1},
		},
	},
	{
		Name: "make",
		Signatures: []NativeSig{
			// `make <FrameType> {fields}` packs a frame. The target is a
			// Bytes-subtype defined with `def P (refine Bytes [layout])`;
			// the layout rides on the type, so dispatch is on the type, not
			// an inline spec. More specific than make's `[TScalar TAny]`.
			{Args: []*Type{TBytes, TMap}, TypeArgs: map[int]bool{0: true}, Handler: makeFrameHandler, Returns: []*Type{TBytes}, BarrierPos: -1},
		},
	},
	{
		Name: "unpack",
		Signatures: []NativeSig{
			// `unpack <FrameType> b` decodes a frame, returning a field Map.
			{Args: []*Type{TBytes, TBytes}, TypeArgs: map[int]bool{0: true}, Handler: unpackFrameHandler, Returns: []*Type{TMap}, BarrierPos: -1},
		},
	},
	{
		Name: "unpack-prefix",
		Signatures: []NativeSig{
			// `unpack-prefix <FrameType> b` -> {ok rest} | {need n}; the streaming framer.
			{Args: []*Type{TBytes, TBytes}, TypeArgs: map[int]bool{0: true}, Handler: unpackPrefixFrameHandler, Returns: []*Type{TMap}, BarrierPos: -1},
		},
	},
}

// ---- convert overloads -----------------------------------------------------

func convertStringToBytes(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	s, err := args[1].AsConcreteString()
	if err != nil {
		return nil, r.AqlError("bytes_error", "convert Bytes: expected a String", "convert")
	}
	return []Value{newBytes([]byte(s))}, nil
}

func convertBytesToString(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b, ok := asBytes(args[1])
	if !ok {
		return nil, r.AqlError("bytes_error", "convert String: expected Bytes", "convert")
	}
	if !utf8.Valid(b) {
		return nil, r.AqlError("bad-encoding", "convert String: bytes are not valid UTF-8", "convert")
	}
	return []Value{NewString(string(b))}, nil
}

func convertListToBytes(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	lst, err := RequireConcreteList(args[1], "convert")
	if err != nil {
		return nil, err
	}
	out := make([]byte, lst.Len())
	for i := 0; i < lst.Len(); i++ {
		n, ierr := lst.Get(i).AsConcreteInteger()
		if ierr != nil {
			return nil, r.AqlError("expected-byte", fmt.Sprintf("convert Bytes: element %d is not an Integer", i), "convert")
		}
		if n < 0 || n > 255 {
			return nil, r.AqlError("expected-byte", fmt.Sprintf("convert Bytes: element %d = %d is out of range 0-255", i, n), "convert")
		}
		out[i] = byte(n)
	}
	return []Value{newBytes(out)}, nil
}

func convertBytesToList(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b, ok := asBytes(args[1])
	if !ok {
		return nil, r.AqlError("bytes_error", "convert List: expected Bytes", "convert")
	}
	out := make([]Value, len(b))
	for i, c := range b {
		out[i] = NewInteger(int64(c))
	}
	return []Value{NewList(out)}, nil
}

// convertBytesToBytes is `convert Bytes <bytes>` — a compacting copy that
// drops any large backing array a zero-copy slice view was pinning.
func convertBytesToBytes(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b, ok := asBytes(args[1])
	if !ok {
		return nil, r.AqlError("bytes_error", "convert Bytes: expected Bytes", "convert")
	}
	return []Value{newBytes(append([]byte(nil), b...))}, nil
}

// ---- slice overloads -------------------------------------------------------

// clampRange normalises a [start,end) window to byte bounds (slice never
// errors on out-of-range; it clamps, matching the String/List slice).
func clampRange(start, end, n int) (int, int) {
	if start < 0 {
		start = 0
	}
	if end > n {
		end = n
	}
	if start > n {
		start = n
	}
	if end < start {
		end = start
	}
	return start, end
}

func bytesSliceStartEnd(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b, _ := asBytes(args[2])
	s64, _ := args[0].AsConcreteInteger()
	e64, _ := args[1].AsConcreteInteger()
	s, e := clampRange(int(s64), int(e64), len(b))
	return []Value{newBytes(b[s:e:e])}, nil
}

func bytesSliceStart(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b, _ := asBytes(args[1])
	s64, _ := args[0].AsConcreteInteger()
	s, e := clampRange(int(s64), len(b), len(b))
	return []Value{newBytes(b[s:e:e])}, nil
}

func bytesSliceAll(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b, _ := asBytes(args[0])
	return []Value{newBytes(b[0:len(b):len(b)])}, nil
}

// ---- add overload ----------------------------------------------------------

// addBytesHandler concatenates two byte strings. Like addConcatHandler it
// joins args[1] ++ args[0], so the swap form `a add b` yields a ++ b.
func addBytesHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b0, _ := asBytes(args[0])
	b1, _ := asBytes(args[1])
	out := make([]byte, 0, len(b1)+len(b0))
	out = append(out, b1...)
	out = append(out, b0...)
	return []Value{newBytes(out)}, nil
}

// ---- bit-syntax: segment spec ---------------------------------------------

// bitSeg is one parsed segment of a pack/unpack spec.
type bitSeg struct {
	name   string // bind/scope name; "" when the segment is a literal
	isLit  bool   // key was a numeric literal (pack: write it; unpack: guard)
	litVal int64
	typ    string // u8..u64,i8..i64,f32,f64,bits,bytes,utf8,pad
	width  int    // byte width for fixed ints/floats (1/2/4/8)
	signed bool
	little bool // endianness (default big = network order)
	// size for bytes/utf8/bits/pad
	hasSize  bool
	sizeLit  int
	sizeName string // size given as a previously-bound name
}

var intWidths = map[string]struct {
	width  int
	signed bool
}{
	"u8": {1, false}, "u16": {2, false}, "u32": {4, false}, "u64": {8, false},
	"i8": {1, true}, "i16": {2, true}, "i32": {4, true}, "i64": {8, true},
}

// readBitSegments reads the spec, a `List` of segment `Map`s. Each Map is a
// fully-structured, JSON-representable descriptor that this code only READS —
// there is NO token-level / sub-language parsing (ADR-007: no secondary
// parsing; every AQL structure is plain Node data a macro could construct).
// Segment Map keys:
//   - name (String) | value (Integer) — exactly one. `name` binds (unpack) or
//     reads from scope (pack); `value` is a pack constant / unpack match-guard.
//   - type (String, required) — u8..u64, i8..i64, f32, f64, bits, bytes,
//     utf8, pad.
//   - endian (String, optional) — "be" (default, network order) or "le".
//   - signed (Boolean, optional) — overrides the u/i prefix default.
//   - size (Integer | String, optional) — a literal bit/byte count, or a
//     String naming a previously-bound field; required for bits/pad, optional
//     for bytes/utf8 (omitted = "the rest").
func readBitSegments(spec Value, r *Registry, word string) ([]bitSeg, error) {
	lst, err := RequireConcreteList(spec, word)
	if err != nil {
		return nil, err
	}
	var segs []bitSeg
	for i := 0; i < lst.Len(); i++ {
		seg, perr := readOneSeg(lst.Get(i), i, r, word)
		if perr != nil {
			return nil, perr
		}
		segs = append(segs, seg)
	}
	for i := range segs {
		if (segs[i].typ == "bits" || segs[i].typ == "pad") && !segs[i].hasSize {
			return nil, r.AqlError("bytes_error", fmt.Sprintf("%s: %s segment needs a `size` (bit count)", word, segs[i].typ), word)
		}
	}
	return segs, nil
}

func readOneSeg(el Value, i int, r *Registry, word string) (bitSeg, error) {
	var seg bitSeg
	segErr := func(detail string) error {
		return r.AqlError("bytes_error", fmt.Sprintf("%s: segment %d %s", word, i, detail), word)
	}
	m, merr := RequireConcreteMap(el, word)
	if merr != nil {
		return seg, segErr("is not a Map")
	}

	// Bind vs literal: exactly one of `name` / `value`.
	nameV, hasName := m.Get("name")
	valV, hasValue := m.Get("value")
	switch {
	case hasName && hasValue:
		return seg, segErr("has both `name` and `value`")
	case hasName:
		s, e := nameV.AsConcreteString()
		if e != nil {
			return seg, segErr("`name` must be a String")
		}
		seg.name = s
	case hasValue:
		n, e := valV.AsConcreteInteger()
		if e != nil {
			return seg, segErr("`value` must be an Integer")
		}
		seg.isLit = true
		seg.litVal = n
	default:
		return seg, segErr("needs a `name` or `value`")
	}

	// type (required).
	typeV, hasType := m.Get("type")
	if !hasType {
		return seg, segErr("has no `type`")
	}
	seg.typ, merr = typeV.AsConcreteString()
	if merr != nil {
		return seg, segErr("`type` must be a String")
	}
	if iw, ok := intWidths[seg.typ]; ok {
		seg.width = iw.width
		seg.signed = iw.signed
	} else {
		switch seg.typ {
		case "f32":
			seg.width = 4
		case "f64":
			seg.width = 8
		case "bits", "bytes", "utf8", "pad":
			// size comes from the `size` key
		default:
			return seg, segErr(fmt.Sprintf("has unknown type %q", seg.typ))
		}
	}

	// endian (optional; default big = network order).
	if endV, ok := m.Get("endian"); ok {
		es, e := endV.AsConcreteString()
		if e != nil {
			return seg, segErr("`endian` must be a String")
		}
		switch es {
		case "be":
			seg.little = false
		case "le":
			seg.little = true
		default:
			return seg, segErr(fmt.Sprintf("has unknown endian %q (want \"be\" or \"le\")", es))
		}
	}

	// signed (optional; overrides the u/i prefix, ints only).
	if sgV, ok := m.Get("signed"); ok {
		b, e := sgV.AsConcreteBoolean()
		if e != nil {
			return seg, segErr("`signed` must be a Boolean")
		}
		if _, isInt := intWidths[seg.typ]; !isInt {
			return seg, segErr(fmt.Sprintf("`signed` is not valid on %s", seg.typ))
		}
		seg.signed = b
	}

	// size (optional; Integer literal or String field-name reference).
	if szV, ok := m.Get("size"); ok {
		seg.hasSize = true
		if n, e := szV.AsConcreteInteger(); e == nil {
			seg.sizeLit = int(n)
		} else if s, e := szV.AsConcreteString(); e == nil {
			seg.sizeName = s
		} else {
			return seg, segErr("`size` must be an Integer or a field-name String")
		}
	}
	return seg, nil
}

// segSize resolves a bits/pad/bytes/utf8 size for PACKING: a literal, a field
// named in the fields map, or -1 ("the rest", unused on pack).
func segSize(seg bitSeg, fields ReadMap, r *Registry, word string) (int, error) {
	if !seg.hasSize {
		return -1, nil
	}
	if seg.sizeName == "" {
		return seg.sizeLit, nil
	}
	v, ok := fields.Get(seg.sizeName)
	if !ok {
		return 0, r.AqlError("bytes_error", fmt.Sprintf("%s: size field %q is not provided", word, seg.sizeName), word)
	}
	n, err := v.AsConcreteInteger()
	if err != nil {
		return 0, r.AqlError("bytes_error", fmt.Sprintf("%s: size field %q is not an Integer", word, seg.sizeName), word)
	}
	return int(n), nil
}

// resolveSize resolves a decode-time size: a literal, "rest" (-1), or a name
// referring to one of THIS frame's already-decoded integer fields (`seen`) —
// the `size:'len'` case, e.g. a body sized by an earlier `len` field.
func resolveSize(seg bitSeg, seen map[string]int64, r *Registry, word string) (int, error) {
	if !seg.hasSize {
		return -1, nil
	}
	if seg.sizeName == "" {
		return seg.sizeLit, nil
	}
	if n, ok := seen[seg.sizeName]; ok {
		return int(n), nil
	}
	return 0, r.AqlError("bytes_error", fmt.Sprintf("%s: size field %q is not an earlier field", word, seg.sizeName), word)
}

// ---- bit writer / reader (MSB-first) --------------------------------------

type bitWriter struct {
	out    []byte
	acc    uint64 // pending sub-byte bits (MSB-first), 0..7 of them
	accCnt int
}

func (w *bitWriter) aligned() bool { return w.accCnt == 0 }

func (w *bitWriter) writeBits(val uint64, n int) {
	for i := n - 1; i >= 0; i-- {
		w.acc = (w.acc << 1) | ((val >> uint(i)) & 1)
		w.accCnt++
		if w.accCnt == 8 {
			w.out = append(w.out, byte(w.acc))
			w.acc, w.accCnt = 0, 0
		}
	}
}

func (w *bitWriter) writeBytes(b []byte) { w.out = append(w.out, b...) }

type bitReader struct {
	b      []byte
	pos    int // byte index
	bitPos int // 0..7 within b[pos]
}

func (rd *bitReader) aligned() bool { return rd.bitPos == 0 }

func (rd *bitReader) remaining() int {
	if rd.pos >= len(rd.b) {
		return 0
	}
	return len(rd.b) - rd.pos
}

func (rd *bitReader) readBits(n int) (uint64, bool) {
	var v uint64
	for i := 0; i < n; i++ {
		if rd.pos >= len(rd.b) {
			return 0, false
		}
		bit := (uint64(rd.b[rd.pos]) >> uint(7-rd.bitPos)) & 1
		v = (v << 1) | bit
		rd.bitPos++
		if rd.bitPos == 8 {
			rd.bitPos = 0
			rd.pos++
		}
	}
	return v, true
}

func putUint(val uint64, width int, little bool) []byte {
	b := make([]byte, width)
	if little {
		for i := 0; i < width; i++ {
			b[i] = byte(val >> uint(8*i))
		}
	} else {
		for i := 0; i < width; i++ {
			b[width-1-i] = byte(val >> uint(8*i))
		}
	}
	return b
}

func getUint(b []byte, little bool) uint64 {
	var v uint64
	if little {
		for i := len(b) - 1; i >= 0; i-- {
			v = (v << 8) | uint64(b[i])
		}
	} else {
		for i := 0; i < len(b); i++ {
			v = (v << 8) | uint64(b[i])
		}
	}
	return v
}

func signExtend(v uint64, width int) int64 {
	bits := uint(width * 8)
	if bits < 64 && v&(1<<(bits-1)) != 0 {
		return int64(v) - (1 << bits)
	}
	return int64(v)
}

// ---- pack: make <FrameType> {fields} --------------------------------------

// makeFrameHandler packs `make <FrameType> {fields}`. The frame type carries
// the layout (frameBehavior); fields supplies each named segment's value. The
// result is a Bytes value tagged as the frame type (so `x is Packet` holds).
func makeFrameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	t := CanonicalType(r, &args[0])
	fb, ok := frameBehaviorOf(t)
	if !ok {
		return nil, r.AqlErrorHint("type_error",
			fmt.Sprintf("make: %s is not a frame type", args[0].String()),
			"make",
			"define one with `def P (refine Bytes [ {name:'x' type:'u8'} … ])`")
	}
	fields, err := RequireConcreteMap(args[1], "make")
	if err != nil {
		return nil, err
	}
	w := &bitWriter{}
	for _, seg := range fb.layout {
		if err := packSeg(w, seg, fields, r, "make"); err != nil {
			return nil, err
		}
	}
	if !w.aligned() {
		return nil, r.AqlError("unaligned", "make: bit segments do not sum to whole bytes", "make")
	}
	return []Value{ReparentValue(newBytes(w.out), t)}, nil
}

// packValue fetches a segment's source value for packing: a literal constant
// (the `value` key) or the named field from the supplied fields map.
func packValue(seg bitSeg, fields ReadMap) (Value, bool) {
	if seg.isLit {
		return NewInteger(seg.litVal), true
	}
	return fields.Get(seg.name)
}

func packSeg(w *bitWriter, seg bitSeg, fields ReadMap, r *Registry, word string) error {
	missing := func() error {
		return r.AqlError("bytes_error", fmt.Sprintf("%s: field %q is not provided", word, seg.name), word)
	}
	switch {
	case seg.typ == "pad":
		n, err := segSize(seg, fields, r, word)
		if err != nil {
			return err
		}
		w.writeBits(0, n)
		return nil
	case seg.typ == "bits":
		n, err := segSize(seg, fields, r, word)
		if err != nil {
			return err
		}
		v, ok := packValue(seg, fields)
		if !ok {
			return missing()
		}
		iv, _ := v.AsConcreteInteger()
		w.writeBits(uint64(iv), n)
		return nil
	}
	if !w.aligned() {
		return r.AqlError("unaligned", fmt.Sprintf("%s: %s segment is not byte-aligned", word, seg.typ), word)
	}
	switch seg.typ {
	case "bytes":
		v, ok := packValue(seg, fields)
		if !ok {
			return missing()
		}
		b, bok := asBytes(v)
		if !bok {
			return r.AqlError("bytes_error", fmt.Sprintf("%s: field %q is not Bytes", word, seg.name), word)
		}
		w.writeBytes(b)
	case "utf8":
		v, ok := packValue(seg, fields)
		if !ok {
			return missing()
		}
		s, serr := v.AsConcreteString()
		if serr != nil {
			return r.AqlError("bytes_error", fmt.Sprintf("%s: field %q is not a String", word, seg.name), word)
		}
		w.writeBytes([]byte(s))
	case "f32":
		v, ok := packValue(seg, fields)
		if !ok {
			return missing()
		}
		f, _ := AsFloat(v)
		w.writeBytes(putUint(uint64(math.Float32bits(float32(f))), 4, seg.little))
	case "f64":
		v, ok := packValue(seg, fields)
		if !ok {
			return missing()
		}
		f, _ := AsFloat(v)
		w.writeBytes(putUint(math.Float64bits(f), 8, seg.little))
	default: // fixed ints
		v, ok := packValue(seg, fields)
		if !ok {
			return missing()
		}
		n, _ := v.AsConcreteInteger()
		w.writeBytes(putUint(uint64(n), seg.width, seg.little))
	}
	return nil
}

// ---- unpack: decode a frame to a field Map, and the streaming variant -----

// unpackFrameHandler decodes `unpack <FrameType> b`, returning a field Map.
func unpackFrameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fb, ok := frameBehaviorOf(CanonicalType(r, &args[0]))
	if !ok {
		return nil, frameTypeErr(r, args[0], "unpack")
	}
	b, _ := asBytes(args[1])
	m, _, err := decodeSegs(b, fb.layout, r, "unpack", false)
	if err != nil {
		if _, short := err.(needMore); short {
			return nil, r.AqlError("no_match", "unpack: buffer too short for the frame", "unpack")
		}
		return nil, err
	}
	return []Value{NewMap(m)}, nil
}

// unpackPrefixFrameHandler decodes a leading frame from b: {ok rest} | {need n}.
func unpackPrefixFrameHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	fb, ok := frameBehaviorOf(CanonicalType(r, &args[0]))
	if !ok {
		return nil, frameTypeErr(r, args[0], "unpack-prefix")
	}
	b, _ := asBytes(args[1])
	m, rest, err := decodeSegs(b, fb.layout, r, "unpack-prefix", false)
	if err != nil {
		if ne, ok := err.(needMore); ok {
			out := NewOrderedMap()
			out.Set("need", NewInteger(int64(ne.n)))
			return []Value{NewMap(out)}, nil
		}
		return nil, err
	}
	out := NewOrderedMap()
	out.Set("ok", NewMap(m))
	out.Set("rest", newBytes(rest))
	return []Value{NewMap(out)}, nil
}

func frameTypeErr(r *Registry, t Value, word string) error {
	return r.AqlErrorHint("type_error",
		fmt.Sprintf("%s: %s is not a frame type", word, t.String()),
		word,
		"define one with `def P (refine Bytes [ {name:'x' type:'u8'} … ])`")
}

// needMore signals a short buffer to unpack-prefix (turned into {need:n}).
type needMore struct{ n int }

func (needMore) Error() string { return "unpack-prefix: need more bytes" }

// decodeSegs walks the segments over b. When bind is true it installs each
// name into scope (unpack); otherwise it collects an ordered map (the
// unpack-prefix {ok} payload) and returns the leftover bytes. A short
// buffer returns a needMore error.
func decodeSegs(b []byte, segs []bitSeg, r *Registry, word string, bind bool) (*OrderedMap, []byte, error) {
	rd := &bitReader{b: b}
	out := NewOrderedMap()
	// seen records integer fields decoded so far so a size like `bytes(len)`
	// resolves against this frame's own earlier segments — needed for the
	// non-binding unpack-prefix where names are not installed into scope.
	seen := map[string]int64{}
	emit := func(name string, v Value) {
		if bind {
			InstallDef(r, name, v)
		} else {
			out.Set(name, v)
		}
	}
	for _, seg := range segs {
		switch seg.typ {
		case "pad":
			n, err := resolveSize(seg, seen, r, word)
			if err != nil {
				return nil, nil, err
			}
			if _, ok := rd.readBits(n); !ok {
				return nil, nil, shortErr(word, bind)
			}
			continue
		case "bits":
			n, err := resolveSize(seg, seen, r, word)
			if err != nil {
				return nil, nil, err
			}
			v, ok := rd.readBits(n)
			if !ok {
				return nil, nil, shortErr(word, bind)
			}
			if seg.isLit {
				if int64(v) != seg.litVal {
					return nil, nil, r.AqlError("no_match", fmt.Sprintf("%s: guard bits != %d", word, seg.litVal), word)
				}
				continue
			}
			seen[seg.name] = int64(v)
			emit(seg.name, NewInteger(int64(v)))
			continue
		}
		if !rd.aligned() {
			return nil, nil, r.AqlError("unaligned", fmt.Sprintf("%s: %s segment is not byte-aligned", word, seg.typ), word)
		}
		switch seg.typ {
		case "bytes", "utf8":
			n, err := resolveSize(seg, seen, r, word)
			if err != nil {
				return nil, nil, err
			}
			if n < 0 {
				n = rd.remaining() // trailing "rest"
			}
			if rd.remaining() < n {
				return nil, nil, shortErr2(word, bind, n-rd.remaining())
			}
			chunk := rd.b[rd.pos : rd.pos+n : rd.pos+n]
			rd.pos += n
			if seg.typ == "utf8" {
				if !utf8.Valid(chunk) {
					return nil, nil, r.AqlError("bad-encoding", word+": utf8 segment is not valid UTF-8", word)
				}
				emit(seg.name, NewString(string(chunk)))
			} else {
				emit(seg.name, newBytes(chunk))
			}
		case "f32", "f64":
			if rd.remaining() < seg.width {
				return nil, nil, shortErr2(word, bind, seg.width-rd.remaining())
			}
			raw := rd.b[rd.pos : rd.pos+seg.width]
			rd.pos += seg.width
			var f float64
			if seg.width == 4 {
				f = float64(math.Float32frombits(uint32(getUint(raw, seg.little))))
			} else {
				f = math.Float64frombits(getUint(raw, seg.little))
			}
			emit(seg.name, NewFloat(f))
		default: // fixed ints
			if rd.remaining() < seg.width {
				return nil, nil, shortErr2(word, bind, seg.width-rd.remaining())
			}
			raw := rd.b[rd.pos : rd.pos+seg.width]
			rd.pos += seg.width
			u := getUint(raw, seg.little)
			var n int64
			if seg.signed {
				n = signExtend(u, seg.width)
			} else {
				n = int64(u)
			}
			if seg.isLit {
				if n != seg.litVal {
					return nil, nil, r.AqlError("no_match", fmt.Sprintf("%s: guard %s != %d", word, seg.typ, seg.litVal), word)
				}
				continue
			}
			seen[seg.name] = n
			emit(seg.name, NewInteger(n))
		}
	}
	if !rd.aligned() {
		return nil, nil, r.AqlError("unaligned", word+": trailing sub-byte bits", word)
	}
	return out, rd.b[rd.pos:], nil
}

func shortErr(word string, bind bool) error {
	if bind {
		return fmt.Errorf("[aql/no_match] %s: buffer too short", word)
	}
	return needMore{n: 1}
}

func shortErr2(word string, bind bool, need int) error {
	if bind {
		return fmt.Errorf("[aql/no_match] %s: buffer too short", word)
	}
	if need < 1 {
		need = 1
	}
	return needMore{n: need}
}
