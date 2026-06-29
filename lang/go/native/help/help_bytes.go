package help

// Bytes (Scalar/Bytes) is exposed mostly through SIGNATURE OVERLOADS of
// existing words rather than dedicated words: `convert` handles
// String/List<->Bytes and `convert Bytes <bytes>` compacts; `slice` takes
// sub-ranges; `add` concatenates; `make Bytes [spec]` packs a bit-syntax
// frame; `unpack` decodes one. Generic `size`/`cmp`/`eq`/`sort` work via the
// type's behaviors. The dedicated words are `bytes` (sugar for `make Bytes`)
// and the streaming framer `unpack-prefix`. See design/go-modules/BYTES.10.md.
// Heavier binary ops (hashes, HMAC, hex/base encodings, secure random, UUID)
// live in aql:bin-util.
func init() {
	register(&Entry{
		Word:    "bytes",
		Summary: "Pack a Bytes frame from a layout spec — sugar for `make Bytes`.",
		Description: "`bytes [spec]` packs a `Bytes` value from a layout, identical to " +
			"`make Bytes [spec]`. The spec is a `List` of segment `Map`s — plain, " +
			"JSON-representable data this code only READS (ADR-007: no secondary " +
			"parsing). Segment keys: `name` (String, reads a binding) | `value` " +
			"(Integer, a pack constant) — exactly one; `type` (String) one of u8..u64/" +
			"i8..i64/f32/f64/bits/bytes/utf8/pad; `endian` (String be(default)/le); " +
			"`signed` (Boolean override); `size` (Integer or a field-name String) for " +
			"bits/bytes/utf8/pad. Decode with `unpack` / `unpack-prefix`.",
		Examples: []string{
			`bytes [{value:1 type:'u8'} {value:2 type:'u16'}]   # Bytes<01 00 02>`,
			`def n 2  def body (convert Bytes "hi")  bytes [{value:1 type:'u8'} {name:'n' type:'u16'} {name:'body' type:'bytes'}]   # Bytes<01 00 02 68 69>`,
		},
	})
	register(&Entry{
		Word:    "unpack-prefix",
		Summary: "Decode a leading bit-syntax frame from Bytes, returning the rest.",
		Description: "`unpack-prefix b [spec]` matches as much of the Bytes `b` as " +
			"the spec needs and returns `{ok: <fields> rest: <Bytes>}` with the " +
			"bound fields and the leftover bytes — or `{need: n}` when the buffer " +
			"is too short. The streaming-framer counterpart to `unpack` (which " +
			"decodes a whole frame and binds names into scope). The spec is the " +
			"same JSON-representable `List` of segment `Map`s as `make Bytes` " +
			"(keys: name|value, type, endian, signed, size — see `bytes`). " +
			"Hex/binary byte constants are the `+hb/…/` and `+bb/…/` minilang " +
			"kinds (import \"aql:minilang\").",
		Examples: []string{
			`import "aql:minilang"  unpack-prefix (+hb/0100026869/) [{name:'op' type:'u8'} {name:'len' type:'u16'} {name:'body' type:'bytes' size:'len'}]   # {ok:{op:1 len:2 body:Bytes<68 69>} rest:Bytes<>}`,
			`unpack-prefix (convert Bytes [0]) [{name:'op' type:'u8'} {name:'len' type:'u16'}]   # {need: 2}`,
		},
	})
}
