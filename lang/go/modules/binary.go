package modules

import (
	"fmt"
	"hash/fnv"
	"math/bits"

	"github.com/aql-lang/aql/lang/go/native"
)

// sign63Mask clears the sign bit of a 64-bit hash so the result is a
// non-negative AQL Integer (0 … 2^63-1). Probabilistic structures index
// with `hash mod m`, and a negative dividend would produce a negative
// index — so the hash words deliberately return non-negative values.
const sign63Mask = 0x7FFFFFFFFFFFFFFF

// BuildBinaryModule creates the "aql:bin" native module — rotates,
// bit-counting, single-bit operators, and slice/construct routines.
// The core bitwise operators (band, bor, bxor, bnot, bsl, bsr, busr)
// are AQL built-ins; this module covers the second tier.
//
// After import, words are accessed via dot notation: bin.popcount,
// bin.rotl, bin.test, etc. The `b` prefix is dropped on module words
// because the `bin.` qualifier disambiguates.
//
// See lang/doc/design/BINARY-OPERATIONS.0.md.
func BuildBinaryModule(parent *native.Registry) (native.ModuleDesc, error) {
	subReg, err := native.DefaultRegistry()
	if err != nil {
		return native.ModuleDesc{}, err
	}

	for _, n := range binaryModuleNatives {
		subReg.RegisterNativeFunc(n)
	}

	exports := native.NewOrderedMap()

	// Unary Integer -> Integer.
	for _, name := range []string{"popcount", "clz", "ctz", "bitlen", "mask", "reverse", "swap"} {
		exports.Set(name, makeBinUnaryFnDef(name, subReg, native.TInteger))
	}

	// Unary Integer -> Boolean.
	exports.Set("parity", makeBinUnaryFnDef("parity", subReg, native.TBoolean))

	// Binary Integer Integer -> Integer.
	for _, name := range []string{"rotl", "rotr", "set", "clear", "toggle"} {
		exports.Set(name, makeBinBinaryFnDef(name, subReg, native.TInteger))
	}

	// Binary Integer Integer -> Boolean.
	exports.Set("test", makeBinBinaryFnDef("test", subReg, native.TBoolean))

	// Ternary Integer Integer Integer -> Integer.
	exports.Set("extract", makeBinTernaryFnDef("extract", subReg))

	// Quaternary Integer Integer Integer Integer -> Integer.
	exports.Set("insert", makeBinQuaternaryFnDef("insert", subReg))

	// Character codes: String -> Integer (`ord`) and Integer -> String
	// (`chr`). These replace the O(95) printable-ASCII alphabet trick that
	// every char-code-needing AQL library otherwise has to roll by hand.
	// See §9.8 in the DX report.
	exports.Set("ord", makeBinFnDef1("ord", subReg, native.TString, native.TInteger))
	exports.Set("chr", makeBinFnDef1("chr", subReg, native.TInteger, native.TString))

	// Non-cryptographic string hashes: String -> Integer. FNV-1a, the
	// standard library's hash/fnv. `fnv32` returns the full 32-bit hash
	// (always a non-negative Integer); `fnv64` returns the 64-bit hash
	// with its sign bit cleared (non-negative, see sign63Mask) so it is
	// directly usable as `hash mod m` in bloom/sketch/dedup libraries.
	// See §9.9 in the DX report.
	exports.Set("fnv32", makeBinFnDef1("fnv32", subReg, native.TString, native.TInteger))
	exports.Set("fnv64", makeBinFnDef1("fnv64", subReg, native.TString, native.TInteger))

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"bin": exports},
	}
	return desc, nil
}

func makeBinUnaryFnDef(wordName string, subReg *native.Registry, returnType *native.Type) native.Value {
	fnDef := native.FnDefInfo{
		Name: wordName,
		Sigs: []native.FnSig{
			{
				Params:  []native.FnParam{{Type: native.TInteger}},
				Returns: []*native.Type{returnType},
				Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
			},
		},
		Registry: subReg,
	}
	return native.NewFnDef(fnDef)
}

// makeBinFnDef1 builds a one-parameter module wrapper whose param and
// return types may differ (e.g. ord: String -> Integer). makeBinUnaryFnDef
// is the Integer -> returnType special case.
func makeBinFnDef1(wordName string, subReg *native.Registry, paramType, returnType *native.Type) native.Value {
	fnDef := native.FnDefInfo{
		Name: wordName,
		Sigs: []native.FnSig{
			{
				Params:  []native.FnParam{{Type: paramType}},
				Returns: []*native.Type{returnType},
				Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
			},
		},
		Registry: subReg,
	}
	return native.NewFnDef(fnDef)
}

func makeBinBinaryFnDef(wordName string, subReg *native.Registry, returnType *native.Type) native.Value {
	fnDef := native.FnDefInfo{
		Name: wordName,
		Sigs: []native.FnSig{
			{
				Params: []native.FnParam{
					{Type: native.TInteger},
					{Type: native.TInteger},
				},
				Returns: []*native.Type{returnType},
				Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
			},
		},
		Registry: subReg,
	}
	return native.NewFnDef(fnDef)
}

func makeBinTernaryFnDef(wordName string, subReg *native.Registry) native.Value {
	fnDef := native.FnDefInfo{
		Name: wordName,
		Sigs: []native.FnSig{
			{
				Params: []native.FnParam{
					{Type: native.TInteger},
					{Type: native.TInteger},
					{Type: native.TInteger},
				},
				Returns: []*native.Type{native.TInteger},
				Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
			},
		},
		Registry: subReg,
	}
	return native.NewFnDef(fnDef)
}

func makeBinQuaternaryFnDef(wordName string, subReg *native.Registry) native.Value {
	fnDef := native.FnDefInfo{
		Name: wordName,
		Sigs: []native.FnSig{
			{
				Params: []native.FnParam{
					{Type: native.TInteger},
					{Type: native.TInteger},
					{Type: native.TInteger},
					{Type: native.TInteger},
				},
				Returns: []*native.Type{native.TInteger},
				Body:    []native.Value{native.NewWord(wordName)}, BarrierPos: -1,
			},
		},
		Registry: subReg,
	}
	return native.NewFnDef(fnDef)
}

// ---- handlers ----

func intArg(v native.Value) (int64, error) {
	return v.AsConcreteInteger()
}

// binaryModuleNatives holds the NativeFunc registrations for the
// module's words. Note the swap convention for binary ops:
// `value rotl count` → args[1]=value, args[0]=count.
var binaryModuleNatives = []native.NativeFunc{
	// --- bit counting (unary) ---
	{
		Name: "popcount",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				x, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(int64(bits.OnesCount64(uint64(x))))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "clz",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				x, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(int64(bits.LeadingZeros64(uint64(x))))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "ctz",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				x, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(int64(bits.TrailingZeros64(uint64(x))))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "parity",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				x, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewBoolean(bits.OnesCount64(uint64(x))%2 == 1)}, nil
			},
			Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
		}},
	},
	{
		Name: "bitlen",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				x, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(int64(bits.Len64(uint64(x))))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},

	// --- slice / construct (unary) ---
	{
		Name: "mask",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				n, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				if n <= 0 {
					return []native.Value{native.NewInteger(0)}, nil
				}
				if n >= 64 {
					return []native.Value{native.NewInteger(-1)}, nil
				}
				return []native.Value{native.NewInteger(int64((uint64(1) << uint(n)) - 1))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "reverse",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				x, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(int64(bits.Reverse64(uint64(x))))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "swap",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				x, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(int64(bits.ReverseBytes64(uint64(x))))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},

	// --- rotates (binary) ---
	{
		Name: "rotl",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger, native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				n, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				x, err := intArg(args[1])
				if err != nil {
					return nil, err
				}
				// bits.RotateLeft64 reduces n mod 64 internally and
				// accepts negative shifts (rotates the other way).
				return []native.Value{native.NewInteger(int64(bits.RotateLeft64(uint64(x), int(n%64))))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "rotr",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger, native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				n, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				x, err := intArg(args[1])
				if err != nil {
					return nil, err
				}
				return []native.Value{native.NewInteger(int64(bits.RotateLeft64(uint64(x), -int(n%64))))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},

	// --- single-bit ops (binary) ---
	{
		Name: "test",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger, native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				n, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				x, err := intArg(args[1])
				if err != nil {
					return nil, err
				}
				if n < 0 || n >= 64 {
					return nil, r.AqlError("range_error",
						fmt.Sprintf("bin.test: bit index out of range [0, 64): %d", n), "test")
				}
				return []native.Value{native.NewBoolean((uint64(x)>>uint(n))&1 != 0)}, nil
			},
			Returns: []*native.Type{native.TBoolean}, BarrierPos: -1,
		}},
	},
	{
		Name: "set",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger, native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				n, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				x, err := intArg(args[1])
				if err != nil {
					return nil, err
				}
				if n < 0 || n >= 64 {
					return nil, r.AqlError("range_error",
						fmt.Sprintf("bin.set: bit index out of range [0, 64): %d", n), "set")
				}
				return []native.Value{native.NewInteger(int64(uint64(x) | (uint64(1) << uint(n))))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "clear",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger, native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				n, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				x, err := intArg(args[1])
				if err != nil {
					return nil, err
				}
				if n < 0 || n >= 64 {
					return nil, r.AqlError("range_error",
						fmt.Sprintf("bin.clear: bit index out of range [0, 64): %d", n), "clear")
				}
				return []native.Value{native.NewInteger(int64(uint64(x) &^ (uint64(1) << uint(n))))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	{
		Name: "toggle",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger, native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				n, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				x, err := intArg(args[1])
				if err != nil {
					return nil, err
				}
				if n < 0 || n >= 64 {
					return nil, r.AqlError("range_error",
						fmt.Sprintf("bin.toggle: bit index out of range [0, 64): %d", n), "toggle")
				}
				return []native.Value{native.NewInteger(int64(uint64(x) ^ (uint64(1) << uint(n))))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},

	// --- slice / construct (ternary, quaternary) ---
	//
	// `bin.extract value lo hi` → bits [lo, hi) of value.
	// Per §1.4 dispatch, `x op a b` lands as args[0]=a, args[1]=b,
	// args[2]=x — so args[0]=lo, args[1]=hi, args[2]=value.
	{
		Name: "extract",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger, native.TInteger, native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				lo, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				hi, err := intArg(args[1])
				if err != nil {
					return nil, err
				}
				x, err := intArg(args[2])
				if err != nil {
					return nil, err
				}
				if lo < 0 || hi > 64 || lo > hi {
					return nil, r.AqlError("range_error",
						fmt.Sprintf("bin.extract: invalid bit range [%d, %d)", lo, hi), "extract")
				}
				width := uint(hi - lo)
				if width == 0 {
					return []native.Value{native.NewInteger(0)}, nil
				}
				var mask uint64
				if width >= 64 {
					mask = ^uint64(0)
				} else {
					mask = (uint64(1) << width) - 1
				}
				return []native.Value{native.NewInteger(int64((uint64(x) >> uint(lo)) & mask))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	// `bin.insert value lo hi bits` → value with bits at [lo, hi)
	// replaced by the low (hi - lo) bits of `bits`.
	// Per §1.4 dispatch: args[0]=lo, args[1]=hi, args[2]=bits, args[3]=value.
	{
		Name: "insert",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger, native.TInteger, native.TInteger, native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				lo, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				hi, err := intArg(args[1])
				if err != nil {
					return nil, err
				}
				bits_, err := intArg(args[2])
				if err != nil {
					return nil, err
				}
				x, err := intArg(args[3])
				if err != nil {
					return nil, err
				}
				if lo < 0 || hi > 64 || lo > hi {
					return nil, r.AqlError("range_error",
						fmt.Sprintf("bin.insert: invalid bit range [%d, %d)", lo, hi), "insert")
				}
				width := uint(hi - lo)
				if width == 0 {
					return []native.Value{native.NewInteger(x)}, nil
				}
				var fieldMask uint64
				if width >= 64 {
					fieldMask = ^uint64(0)
				} else {
					fieldMask = (uint64(1) << width) - 1
				}
				shifted := (uint64(bits_) & fieldMask) << uint(lo)
				clear := uint64(x) &^ (fieldMask << uint(lo))
				return []native.Value{native.NewInteger(int64(clear | shifted))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	// --- character codes ---
	// `bin.ord s` → the Unicode codepoint of the first rune of s.
	{
		Name: "ord",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TString},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				s, err := args[0].AsConcreteString()
				if err != nil {
					return nil, err
				}
				rs := []rune(s)
				if len(rs) == 0 {
					return nil, r.AqlError("range_error", "bin.ord: empty string has no codepoint", "ord")
				}
				return []native.Value{native.NewInteger(int64(rs[0]))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	// `bin.chr n` → the single-rune string for codepoint n.
	{
		Name: "chr",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TInteger},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, r *native.Registry) ([]native.Value, error) {
				n, err := intArg(args[0])
				if err != nil {
					return nil, err
				}
				if n < 0 || n > 0x10FFFF {
					return nil, r.AqlError("range_error",
						fmt.Sprintf("bin.chr: codepoint %d out of range [0, 0x10FFFF]", n), "chr")
				}
				return []native.Value{native.NewString(string(rune(n)))}, nil
			},
			Returns: []*native.Type{native.TString}, BarrierPos: -1,
		}},
	},
	// --- non-cryptographic string hashes (FNV-1a) ---
	// `bin.fnv32 s` → the 32-bit FNV-1a hash of s (non-negative).
	{
		Name: "fnv32",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TString},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				s, err := args[0].AsConcreteString()
				if err != nil {
					return nil, err
				}
				h := fnv.New32a()
				_, _ = h.Write([]byte(s))
				return []native.Value{native.NewInteger(int64(h.Sum32()))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
	// `bin.fnv64 s` → the 64-bit FNV-1a hash of s, sign bit cleared so the
	// result is a non-negative Integer usable directly as `hash mod m`.
	{
		Name: "fnv64",

		Signatures: []native.NativeSig{{
			Args: []*native.Type{native.TString},
			Handler: func(args []native.Value, _ map[string]native.Value, _ []native.Value, _ *native.Registry) ([]native.Value, error) {
				s, err := args[0].AsConcreteString()
				if err != nil {
					return nil, err
				}
				h := fnv.New64a()
				_, _ = h.Write([]byte(s))
				return []native.Value{native.NewInteger(int64(h.Sum64() & sign63Mask))}, nil
			},
			Returns: []*native.Type{native.TInteger}, BarrierPos: -1,
		}},
	},
}
