package help

// Bytes (Scalar/Bytes) is a plain binary Scalar. Its value operations are
// SIGNATURE OVERLOADS of existing words: `convert` handles String/List<->Bytes
// and `convert Bytes <bytes>` compacts; `slice` takes sub-ranges; `add`
// concatenates; generic `size`/`cmp`/`eq`/`sort` work via the type's behaviors.
// Binary FRAME layouts are a separate concern: define a frame TYPE with
// `def Packet (refine Bytes [layout])`, then `make Packet {fields}` packs and
// `unpack`/`unpack-prefix` decode (ADR-007 — the layout is plain Node data on
// the type, not a parsed spec). See design/go-modules/BYTES.10.md. Heavier
// binary ops (hashes, HMAC, hex/base encodings, secure random, UUID) live in
// aql:bin-util.
func init() {
	register(&Entry{
		Word:    "unpack-prefix",
		Summary: "Decode a leading frame from Bytes against a frame type, returning the rest.",
		Description: "`unpack-prefix <FrameType> b` matches as much of the Bytes `b` as " +
			"the frame type's layout needs and returns `{ok: <fields> rest: <Bytes>}` " +
			"with the decoded fields and the leftover bytes — or `{need: n}` when the " +
			"buffer is too short. The streaming-framer counterpart to `unpack` (which " +
			"decodes a whole frame to a field Map). The frame type is defined with " +
			"`def P (refine Bytes [layout])`, where the layout is a `List` of segment " +
			"`Map`s — keys `name`|`value`, `type` (u8..u64/i8..i64/f32/f64/bits/bytes/" +
			"utf8/pad), `endian` (be/le), `signed`, `size` (Integer or a field-name " +
			"String). Hex/binary byte constants are the `+hb/…/` / `+bb/…/` minilang " +
			"kinds (import \"aql:minilang\").",
		Examples: []string{
			`def Msg (refine Bytes [{name:'op' type:'u8'} {name:'len' type:'u16'} {name:'body' type:'bytes' size:'len'}])  unpack-prefix Msg (convert Bytes [1 0 2 104 105])   # {ok:{op:1 len:2 body:Bytes<68 69>} rest:Bytes<>}`,
			`def Hdr (refine Bytes [{name:'op' type:'u8'} {name:'len' type:'u16'}])  unpack-prefix Hdr (convert Bytes [0])   # {need: 2}`,
		},
	})
}
