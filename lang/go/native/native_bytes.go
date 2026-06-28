package native

import (
	"bytes"
	"fmt"
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
// reference TBytes see a non-nil pointer at slice-init time. A
// registration error is recorded (never panics) for DefaultRegistry to
// surface — see ADR-005 and typeinit.go.
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
// The full value is available via to-text / BinUtil.hex-encode.
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
// sorts below every concrete value (two literals bubble to the Rank
// fallback via ErrNoComparer).
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

// bytesNatives are the core Bytes words (no import needed). The crypto /
// encoding / hashing / random / uuid surface lives in aql:bin-util
// (design/go-modules/BIN-UTIL.10.md); generic length/equality/ordering/
// printing come from the behaviors above.
var bytesNatives = []NativeFunc{
	{
		Name: "utf8",
		Signatures: []NativeSig{
			{Args: []*Type{TString}, Handler: utf8Handler, Returns: []*Type{TBytes}, BarrierPos: -1},
		},
	},
	{
		Name: "to-text",
		Signatures: []NativeSig{
			{Args: []*Type{TBytes}, Handler: toTextHandler, Returns: []*Type{TString}, BarrierPos: -1},
		},
	},
	{
		Name: "bytes",
		Signatures: []NativeSig{
			{Args: []*Type{TList}, Handler: bytesOfListHandler, Returns: []*Type{TBytes}, BarrierPos: -1},
		},
	},
	{
		Name: "byte-ints",
		Signatures: []NativeSig{
			{Args: []*Type{TBytes}, Handler: byteIntsHandler, Returns: []*Type{TList}, BarrierPos: -1},
		},
	},
	{
		Name: "byte-slice",
		Signatures: []NativeSig{
			{Args: []*Type{TBytes, TInteger, TInteger}, Handler: byteSliceHandler, Returns: []*Type{TBytes}, BarrierPos: -1},
		},
	},
	{
		Name: "byte-cat",
		Signatures: []NativeSig{
			{Args: []*Type{TList}, Handler: byteCatHandler, Returns: []*Type{TBytes}, BarrierPos: -1},
		},
	},
	{
		Name: "compact",
		Signatures: []NativeSig{
			{Args: []*Type{TBytes}, Handler: compactHandler, Returns: []*Type{TBytes}, BarrierPos: -1},
		},
	},
}

// utf8: [String] -> Bytes. []byte(s) allocates a fresh slice, so the
// result owns its storage.
func utf8Handler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	s, err := args[0].AsConcreteString()
	if err != nil {
		return nil, r.AqlError("bytes_error", "utf8: expected a String", "utf8")
	}
	return []Value{newBytes([]byte(s))}, nil
}

// to-text: [Bytes] -> String. Rejects invalid UTF-8 with bad-encoding so
// binary data never silently corrupts through a text round-trip.
func toTextHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b, ok := asBytes(args[0])
	if !ok {
		return nil, r.AqlError("bytes_error", "to-text: expected Bytes", "to-text")
	}
	if !utf8.Valid(b) {
		return nil, r.AqlError("bad-encoding", "to-text: bytes are not valid UTF-8", "to-text")
	}
	return []Value{NewString(string(b))}, nil
}

// bytes: [List<Integer 0-255>] -> Bytes.
func bytesOfListHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	lst, err := RequireConcreteList(args[0], "bytes")
	if err != nil {
		return nil, err
	}
	out := make([]byte, lst.Len())
	for i := 0; i < lst.Len(); i++ {
		n, ierr := lst.Get(i).AsConcreteInteger()
		if ierr != nil {
			return nil, r.AqlError("expected-byte", fmt.Sprintf("bytes: element %d is not an Integer", i), "bytes")
		}
		if n < 0 || n > 255 {
			return nil, r.AqlError("expected-byte", fmt.Sprintf("bytes: element %d = %d is out of range 0-255", i, n), "bytes")
		}
		out[i] = byte(n)
	}
	return []Value{newBytes(out)}, nil
}

// byte-ints: [Bytes] -> List<Integer 0-255>.
func byteIntsHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b, ok := asBytes(args[0])
	if !ok {
		return nil, r.AqlError("bytes_error", "byte-ints: expected Bytes", "byte-ints")
	}
	out := make([]Value, len(b))
	for i, c := range b {
		out[i] = NewInteger(int64(c))
	}
	return []Value{NewList(out)}, nil
}

// byte-slice: [Bytes start len] -> Bytes. Returns a zero-copy sub-slice
// VIEW sharing the parent's backing array (use compact to drop the
// retention of a large buffer). Out-of-range raises index-range.
func byteSliceHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b, ok := asBytes(args[0])
	if !ok {
		return nil, r.AqlError("bytes_error", "byte-slice: expected Bytes", "byte-slice")
	}
	start64, _ := args[1].AsConcreteInteger()
	n64, _ := args[2].AsConcreteInteger()
	start, n := int(start64), int(n64)
	if start < 0 || n < 0 || start+n > len(b) {
		return nil, r.AqlError("index-range",
			fmt.Sprintf("byte-slice: range [%d,%d) out of bounds for %d bytes", start, start+n, len(b)), "byte-slice")
	}
	// Three-index slice caps len==cap so the view cannot read past its window.
	return []Value{newBytes(b[start : start+n : start+n])}, nil
}

// byte-cat: [List<Bytes>] -> Bytes. One fresh build of the joined slice.
func byteCatHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	lst, err := RequireConcreteList(args[0], "byte-cat")
	if err != nil {
		return nil, err
	}
	parts := make([][]byte, lst.Len())
	total := 0
	for i := 0; i < lst.Len(); i++ {
		b, ok := asBytes(lst.Get(i))
		if !ok {
			return nil, r.AqlError("bytes_error", fmt.Sprintf("byte-cat: element %d is not Bytes", i), "byte-cat")
		}
		parts[i] = b
		total += len(b)
	}
	out := make([]byte, 0, total)
	for _, p := range parts {
		out = append(out, p...)
	}
	return []Value{newBytes(out)}, nil
}

// compact: [Bytes] -> Bytes. Forces a minimal fresh copy, dropping any
// shared backing array a byte-slice view was pinning (BYTES.10.md §4.3).
func compactHandler(args []Value, _ map[string]Value, _ []Value, r *Registry) ([]Value, error) {
	b, ok := asBytes(args[0])
	if !ok {
		return nil, r.AqlError("bytes_error", "compact: expected Bytes", "compact")
	}
	return []Value{newBytes(append([]byte(nil), b...))}, nil
}
