package modules

func init() {
	registerDocs("aql:bin-util", map[string]string{
		"band":     "Bitwise AND of two integers.",
		"bitlen":   "Minimum number of bits to represent the value.",
		"bnot":     "Bitwise NOT (complement) of an integer.",
		"bor":      "Bitwise OR of two integers.",
		"bsl":      "Shift left by n bits.",
		"bsr":      "Arithmetic (sign-extending) shift right by n bits.",
		"busr":     "Unsigned (logical) shift right by n bits.",
		"bxor":     "Bitwise XOR of two integers.",
		"chr":      "Character from a code point.",
		"clear":    "Clear (zero) the bit at the given index.",
		"clz":      "Count leading zero bits (64-bit).",
		"ctz":      "Count trailing zero bits (64-bit).",
		"extract":  "Extract the bitfield in range [lo, hi).",
		"fnv32":    "32-bit FNV-1a hash of a string.",
		"fnv64":    "64-bit FNV-1a hash of a string.",
		"insert":   "Replace the bitfield in range [lo, hi) with a value.",
		"mask":     "Low-N-bit mask.",
		"ord":      "Code point of a character.",
		"parity":   "True if the integer has an odd number of set bits.",
		"popcount": "Count of set bits in an integer.",
		"reverse":  "Reverse all 64 bits of the value.",
		"rotl":     "Rotate left by n bits.",
		"rotr":     "Rotate right by n bits.",
		"set":      "Set (turn on) the bit at the given index.",
		"swap":     "Reverse the byte order (8 bytes) of the value.",
		"test":     "True if the bit at the given index is set.",
		"toggle":   "Flip the bit at the given index.",
	})
}
