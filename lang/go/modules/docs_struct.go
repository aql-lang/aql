package modules

func init() {
	registerDocs("aql:struct-util", map[string]string{
		"clone":     "Deep-copy a value.",
		"getpath":   "Read the value at a dotted path.",
		"setpath":   "Write a value at a path, returning a new structure.",
		"inject":    "Resolve backtick `$`-path references against a store.",
		"merge":     "Deep, index-wise merge (later argument wins).",
		"walk":      "Collect leaf path/value pairs.",
		"items":     "Map entries as [key value] pairs.",
		"transform": "Shape data via a spec.",
		"validate":  "Check data against a shape spec.",
		"selector":  "Query-select children matching a spec.",
		"jsonify":   "Serialise a value to JSON text (class instances carry $class; user $-keys escape to $$).",
		"parse":     "Decode jsonic/JSON text to data (the complement of jsonify) — data context, loud parse_error on malformed input.",
		"reify":     "Hydrate a class instance from JSON text or a Node — explicit class or tor-union target.",
		"nodify":    "Project a value to its Node/Scalar form.",
	})

	// Results from verified lang/spec/module-struct.tsv rows. These words
	// take a PATH or a SHAPE as data, so what a caller needs to see is the
	// notation — which the [String Map] signature cannot carry.
	registerExamples("aql:struct-util", map[string][]string{
		"getpath": {
			`StructUtil.getpath "a.b" {a:{b:42}}              ;# 42 — dotted path into nested data`,
			`StructUtil.getpath "name" {name:'Alice'}         ;# 'Alice'`,
		},
		"setpath": {
			`StructUtil.setpath {a:1} "b" 99                  ;# {a:1 b:99}`,
			`StructUtil.setpath {a:{b:1} c:2} "a.b" 99        ;# {a:{b:99} c:2}`,
			`StructUtil.setpath {a:1} "x.y" 5                 ;# {a:1 x:{y:5}} — missing levels are created`,
			`StructUtil.setpath {xs:[10 20 30]} "xs.1" 9      ;# {xs:[10 9 30]} — a numeric step indexes a list`,
		},
		"merge": {
			`StructUtil.merge {a:1 b:2} {b:3 c:4}             ;# {a:1 b:3 c:4} — later wins`,
			`StructUtil.merge {a:{x:1 y:2}} {a:{y:3 z:4}}     ;# {a:{x:1 y:3 z:4}} — DEEP, not a top-level overwrite`,
		},
		"clone": {`StructUtil.clone {a:1 b:2}                       ;# {a:1 b:2} — a deep copy, sharing nothing`},
		"items": {`StructUtil.items {a:1 b:2}                       ;# [['a' 1] ['b' 2]] — key/value pairs`},
		"walk":  {`StructUtil.walk {a:1 b:{c:2}}                    ;# [{path:'a' value:1} {path:'b.c' value:2}]`},
		"inject": {
			`StructUtil.inject {greeting:'` + "`name`" + `'} {name:'Alice'}   ;# {greeting:'Alice'} — backticks interpolate from the second map`,
		},
		"selector": {
			`StructUtil.selector {color:'red'} {a:{color:'red'} b:{color:'blue'}}`,
			`;# [{$KEY:'a' color:'red'}] — the matching children, each tagged with its key`,
		},
		"validate":  {`StructUtil.validate {name:"$STRING"} {name:'Alice'}   ;# {name:'$STRING'} — shape check; raises on a mismatch`},
		"transform": {`StructUtil.transform {x:99} {a:'1'}              ;# {x:99} — apply a transform spec to data`},
		"jsonify": {
			`StructUtil.jsonify 42                            ;# '42'`,
			`StructUtil.jsonify 'hi'                          ;# '"hi"' — note the quotes are IN the String`,
			`StructUtil.jsonify true                          ;# 'true'`,
		},
		"nodify": {`StructUtil.nodify {a:1 b:2}                      ;# {a:1 b:2} — normalise to plain nodes`},
		"reify": {
			`def Point class {x:1, y:2}`,
			`def p (StructUtil.reify Point {x:9})             ;# a Point instance, not a bare Map`,
			`p typeof                                         ;# Point`,
		},
		"parse": {`StructUtil.parse '{"a":1}'                      ;# {a:1} — jsonic-tolerant parse to data`},
	})
}
