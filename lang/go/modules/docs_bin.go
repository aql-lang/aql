package modules

func init() {
	registerDocs("aql:bin-util", map[string]string{
		"band":          "Bitwise AND of two integers.",
		"base64-decode": "Decode a standard-alphabet base64 string to its bytes.",
		"base64-encode": "Encode a string's bytes as standard-alphabet base64.",
		"bitlen":        "Minimum number of bits to represent the value.",
		"bnot":          "Bitwise NOT (complement) of an integer.",
		"bor":           "Bitwise OR of two integers.",
		"bsl":           "Shift left by n bits.",
		"bsr":           "Arithmetic (sign-extending) shift right by n bits.",
		"busr":          "Unsigned (logical) shift right by n bits.",
		"bxor":          "Bitwise XOR of two integers.",
		"chr":           "Character from a code point.",
		"clear":         "Clear (zero) the bit at the given index.",
		"clz":           "Count leading zero bits (64-bit).",
		"ctz":           "Count trailing zero bits (64-bit).",
		"extract":       "Extract the bitfield in range [lo, hi).",
		"fnv32":         "32-bit FNV-1a hash of a string.",
		"fnv64":         "64-bit FNV-1a hash of a string.",
		"hex-decode":    "Decode a hexadecimal string to its bytes.",
		"hex-encode":    "Encode a string's bytes as lowercase hexadecimal.",
		"insert":        "Replace the bitfield in range [lo, hi) with a value.",
		"mask":          "Low-N-bit mask.",
		"ord":           "Code point of a character.",
		"parity":        "True if the integer has an odd number of set bits.",
		"popcount":      "Count of set bits in an integer.",
		"reverse":       "Reverse all 64 bits of the value.",
		"rotl":          "Rotate left by n bits.",
		"rotr":          "Rotate right by n bits.",
		"set":           "Set (turn on) the bit at the given index.",
		"swap":          "Reverse the byte order (8 bytes) of the value.",
		"test":          "True if the bit at the given index is set.",
		"toggle":        "Flip the bit at the given index.",
	})

	// Most of this module reads fine as generated permutations
	// (`BinUtil.band 12 10`), so only the words with a non-obvious
	// convention are authored: the bit-index family (which reads in swap
	// form), the field ops, and the codecs. Results from verified
	// lang/spec/module-bin.tsv rows.
	registerExamples("aql:bin-util", map[string][]string{
		"set":    {`0 BinUtil.set 3                                  ;# 8 — set bit 3 of 0; VALUE first, bit second`},
		"clear":  {`15 BinUtil.clear 0                               ;# 14`},
		"toggle": {`5 BinUtil.toggle 1                               ;# 7`},
		"test": {
			`5 BinUtil.test 0                                 ;# true  (5 = 0b101)`,
			`5 BinUtil.test 1                                 ;# false`,
		},
		"extract":       {`255 BinUtil.extract 0 4                          ;# 15 — 4 bits starting at 0`},
		"insert":        {`255 BinUtil.insert 0 4 0                         ;# 240 — write 0 into bits [0,4)`},
		"mask":          {`BinUtil.mask 4                                   ;# 15 — the low 4 bits set`},
		"bitlen":        {`BinUtil.bitlen 255                               ;# 8 — bits needed to represent it`},
		"rotl":          {`255 BinUtil.rotl 8                               ;# 65280 — rotate, not shift`},
		"rotr":          {`65280 BinUtil.rotr 8                             ;# 255`},
		"swap":          {`BinUtil.swap 1                                   ;# 72057594037927936 — byte order reversed`},
		"reverse":       {`BinUtil.reverse 1                                ;# -9223372036854775808 — BIT order reversed`},
		"clz":           {`BinUtil.clz 1                                    ;# 63 — leading zeros in 64 bits`},
		"ctz":           {`BinUtil.ctz 8                                    ;# 3 — trailing zeros`},
		"ord":           {`BinUtil.ord 'A'                                  ;# 65`},
		"chr":           {`BinUtil.chr 65                                   ;# 'A'`},
		"base64-encode": {`BinUtil.base64-encode 'hello'                    ;# 'aGVsbG8='`},
		"base64-decode": {
			`BinUtil.base64-decode 'aGVsbG8='                 ;# 'hello'`,
			`BinUtil.base64-decode '!!notbase64'              ;# raises decode_error`,
		},
		"hex-encode": {`BinUtil.hex-encode 'hi'                          ;# '6869'`},
		"hex-decode": {
			`BinUtil.hex-decode '6869'                        ;# 'hi'`,
			`BinUtil.hex-decode '6'                           ;# raises decode_error — odd length`,
		},
		"fnv32": {`BinUtil.fnv32 'hello'                            ;# 1335831723 — NOT a cryptographic hash`},
		"fnv64": {`BinUtil.fnv64 'hello'                            ;# 2607821981565500683`},
	})
}
