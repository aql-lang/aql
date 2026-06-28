package help

// Core Bytes words (Scalar/Bytes). The heavier binary surface — crypto,
// HMAC, CRC, hex/base encodings, secure random, UUID — lives in the
// aql:bin-util module. Generic `size`, `cmp`/`lt`/`sort`, `eq`, and
// printing work on Bytes via its type behaviors. See
// design/go-modules/BYTES.10.md.
func init() {
	register(&Entry{
		Word:    "utf8",
		Summary: "Encode a String as UTF-8 Bytes.",
		Description: "`utf8 s` returns the UTF-8 octets of the String `s` as a " +
			"Bytes value. Total (never fails); the inverse `to-text` decodes back " +
			"and rejects invalid UTF-8.",
		Examples: []string{`utf8 "hi"        # Bytes<68 69>`},
	})

	register(&Entry{
		Word:    "to-text",
		Summary: "Decode Bytes as a UTF-8 String.",
		Description: "`to-text b` decodes the Bytes `b` as UTF-8 and returns a " +
			"String. Raises [aql/bad-encoding] if the bytes are not valid UTF-8, " +
			"so binary data never corrupts silently. Inverse of `utf8`.",
		Examples: []string{`to-text (utf8 "hi")    # 'hi'`},
	})

	register(&Entry{
		Word:    "bytes",
		Summary: "Build Bytes from a List of integers (0–255).",
		Description: "`bytes lst` packs a List of Integers, each in 0–255, into a " +
			"Bytes value. An out-of-range or non-integer element raises " +
			"[aql/expected-byte]. Inverse of `byte-ints`.",
		Examples: []string{`bytes [104 105]        # Bytes<68 69>`},
	})

	register(&Entry{
		Word:    "byte-ints",
		Summary: "Unpack Bytes to a List of integers (0–255).",
		Description: "`byte-ints b` returns the octets of `b` as a List of " +
			"Integers in 0–255. Inverse of `bytes`.",
		Examples: []string{`byte-ints (utf8 "hi")  # [104 105]`},
	})

	register(&Entry{
		Word:    "byte-slice",
		Summary: "Take a sub-range of Bytes (zero-copy view).",
		Description: "`byte-slice b start len` returns the `len` octets of `b` " +
			"starting at `start`, as a zero-copy view sharing the backing array. " +
			"An out-of-range window raises [aql/index-range]. Use `compact` to " +
			"drop the retained backing array of a small slice over a large buffer.",
		Examples: []string{`to-text (byte-slice (utf8 "hello") 1 3)   # 'ell'`},
	})

	register(&Entry{
		Word:    "byte-cat",
		Summary: "Concatenate a List of Bytes into one Bytes.",
		Description: "`byte-cat parts` joins a List of Bytes into a single Bytes " +
			"value (one fresh allocation). A non-Bytes element raises " +
			"[aql/bytes_error].",
		Examples: []string{`to-text (byte-cat [(utf8 "ab") (utf8 "cd")])   # 'abcd'`},
	})

	register(&Entry{
		Word:    "compact",
		Summary: "Copy Bytes to fresh minimal storage.",
		Description: "`compact b` returns a Bytes with its own minimally-sized " +
			"backing array. Bytes is otherwise shared zero-copy; `compact` is the " +
			"escape hatch for when a small `byte-slice` view would otherwise pin a " +
			"large buffer alive.",
		Examples: []string{`compact (byte-slice big 0 4)   # owns a 4-byte buffer`},
	})
}
