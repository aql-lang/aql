package modules

func init() {
	registerDocs("boru:emitlang", map[string]string{
		"emit_json": "Built-in JSON emitter: `emit json <value>` → compact JSON (opts {pretty:true} " +
			"or {indent:N} indent). The natural target for Maps, Lists, scalars and Errors.",
		"emit_jsonic": "Built-in jsonic emitter: `emit jsonic <value>` → relaxed JSON with bare " +
			"identifier keys (opts {pretty:true} / {indent:N}).",
		"emit_yaml": "Built-in YAML emitter: `emit yaml <value>` → block-style YAML (opts {indent:N}, " +
			"default 2). Maps emit `key:`, lists emit `- ` items; scalars are minimally quoted.",
		"emit_csv": "Built-in CSV emitter: `emit csv <table>` → RFC-4180 CSV (opts {separation:sep} " +
			"overrides the separator). The natural target for a Table; also accepts a list of records.",
		"emit_tsv": "Built-in TSV emitter: `emit tsv <table>` → tab-separated rows, RFC-4180 quoted.",
		"emit_toml": "Built-in TOML emitter: `emit toml <map>` → `key = value` lines and `[table]` " +
			"headers for nested Maps. A bare scalar or top-level List is emit_unsupported.",
		"emit_ini": "Built-in INI emitter: `emit ini <map>` → `[section]` blocks with `key=value` " +
			"lines; a flat `{k:scalar}` emits bare lines. Deeper nesting is emit_unsupported.",
		"emit_xml": "Built-in XML emitter: `emit xml <xml>` → `<tag attr=…>children</tag>` " +
			"(opts {pretty:true} indents). A non-Xml top-level value is emit_unsupported.",
		"emit_auto": "Emit a value using its natural format: Map/List/scalar/Error → json, Table → " +
			"csv, Xml → xml. The backing word for a bare `emit <data>` (no kind). " +
			"A value with no natural format raises emit_no_natural.",
		"register": "TOMBSTONE — raises emit_registry_frozen. The emit kind namespace is fixed " +
			"(built-in kinds only); pass a custom emitter as a Function value instead: " +
			"def mye (fn [[value:Any opts:Map] [String] [...]])  emit mye <data> — Go hosts " +
			"build one with NewEmitLangFn.",
		"kinds": "List the (fixed) emitter-kind atoms (emit_ stripped; emit_auto excluded).",
	})

	// The exported words are the per-kind entry points, but almost nobody
	// calls them directly: `emit` dispatches on the kind atom, and that
	// spelling is what the signatures cannot show. Results are from
	// verified lang/spec/module-emitlang.tsv rows.
	registerExamples("boru:emitlang", map[string][]string{
		"kinds": {`EmitLang.kinds                                   ;# the kinds emit accepts`},
		"emit_json": {
			`emit {a:1 b:[2 3]}                               ;# '{"a":1,"b":[2,3]}' — json is the default`,
			`emit json {pretty:true} {a:1}                    ;# '{\n  "a": 1\n}'`,
			`emit json {indent:4} {a:1}`,
			`EmitLang.emit_json {a:1} {} end                  ;# '{"a":1}' — the direct form, options then end`,
		},
		"emit_jsonic": {`emit jsonic {a:1 b:"hi"}                         ;# '{a:1,b:"hi"}' — bare keys where legal`},
		"emit_yaml": {
			`emit yaml {a:1}                                  ;# 'a: 1'`,
			`emit yaml [1 2]                                  ;# '- 1\n- 2'`,
		},
		"emit_toml": {`emit toml {a:1}`},
		"emit_ini":  {`emit ini {sec:{a:1}}`},
		"emit_csv":  {`emit csv [{a:1 b:2} {a:3 b:4}]                   ;# header row from the first record`},
		"emit_tsv":  {`emit tsv [{a:1 b:2}]`},
		"emit_xml":  {`emit xml <r><a>1</a></r>                        ;# an Xml ELEMENT, not a Map`},
		"emit_auto": {`emit auto value                                  ;# pick the kind from the value's shape`},
		"register": {
			`EmitLang.register mykind ([v:Any opts:Map] => [ … ])`,
			`;# then "emit mykind value" reaches it`,
		},
	})
}
