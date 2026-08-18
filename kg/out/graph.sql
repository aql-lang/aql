BEGIN TRANSACTION;
CREATE TABLE bundle_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE input_files (path TEXT PRIMARY KEY, digest TEXT NOT NULL, chars INTEGER NOT NULL);
CREATE TABLE sources (id TEXT PRIMARY KEY, kind TEXT NOT NULL, locator TEXT NOT NULL, title TEXT, retrieved_at TEXT, content_hash TEXT NOT NULL, authority TEXT NOT NULL, metadata_json TEXT);
CREATE TABLE entities (id TEXT PRIMARY KEY, type TEXT NOT NULL, label TEXT NOT NULL, normalized_label TEXT NOT NULL, status TEXT NOT NULL);
CREATE TABLE entity_aliases (entity_id TEXT NOT NULL REFERENCES entities(id), alias TEXT NOT NULL);
CREATE TABLE entity_external_ids (entity_id TEXT NOT NULL REFERENCES entities(id), scheme TEXT NOT NULL, value TEXT NOT NULL);
CREATE TABLE entity_attributes (entity_id TEXT NOT NULL REFERENCES entities(id), key TEXT NOT NULL, value_json TEXT);
CREATE TABLE assertions (id TEXT PRIMARY KEY, subject_id TEXT NOT NULL REFERENCES entities(id), predicate TEXT NOT NULL, object_kind TEXT NOT NULL, object_entity_id TEXT REFERENCES entities(id), object_value_json TEXT, object_datatype TEXT, object_unit TEXT, object_language TEXT, confidence REAL NOT NULL, status TEXT NOT NULL, valid_from TEXT, valid_to TEXT, recorded_at TEXT NOT NULL, rule TEXT);
CREATE TABLE assertion_evidence (assertion_id TEXT NOT NULL REFERENCES assertions(id), source_id TEXT NOT NULL REFERENCES sources(id), locator TEXT NOT NULL, quote TEXT, extraction_method TEXT NOT NULL, extractor TEXT NOT NULL);
CREATE TABLE identity_decisions (id TEXT PRIMARY KEY, left_entity_id TEXT NOT NULL REFERENCES entities(id), right_entity_id TEXT NOT NULL REFERENCES entities(id), decision TEXT NOT NULL, confidence REAL NOT NULL, review_required INTEGER NOT NULL, supporting_json TEXT, conflicting_json TEXT);
CREATE TABLE validation_issues (id TEXT PRIMARY KEY, severity TEXT NOT NULL, rule TEXT NOT NULL, target_kind TEXT NOT NULL, target_id TEXT NOT NULL, message TEXT NOT NULL, suggested_correction TEXT, automatic_correction_safe INTEGER NOT NULL);
CREATE TABLE schema_proposals (id TEXT PRIMARY KEY, term_kind TEXT NOT NULL, term TEXT NOT NULL, definition TEXT NOT NULL, reason TEXT NOT NULL, status TEXT NOT NULL, domain_json TEXT, range_json TEXT, examples_json TEXT, possible_equivalents_json TEXT);
INSERT INTO bundle_meta VALUES ('schema_version', 'boru-kg/1');
INSERT INTO bundle_meta VALUES ('generated_at', '2026-08-07T00:00:00Z');
INSERT INTO bundle_meta VALUES ('input_digest_algorithm', 'fnv64');
INSERT INTO bundle_meta VALUES ('input_digest_combined', '8738556775595062184');
INSERT INTO input_files VALUES ('../AGENTS.md', '442733134828976200', 9714);
INSERT INTO input_files VALUES ('../CLI.md', '2255385427102909098', 79949);
INSERT INTO input_files VALUES ('../README.md', '7037787103551177539', 12216);
INSERT INTO input_files VALUES ('../basic/go/go.mod', '592614718618454325', 457);
INSERT INTO input_files VALUES ('../calc/go/go.mod', '6164707914494689882', 605);
INSERT INTO input_files VALUES ('../check/go/go.mod', '5545877118704640253', 221);
INSERT INTO input_files VALUES ('../cmd/go/go.mod', '1944156304528717652', 4919);
INSERT INTO input_files VALUES ('../compiler/go/go.mod', '142593282390199928', 331);
INSERT INTO input_files VALUES ('../core/go/go.mod', '2316996521694161686', 98);
INSERT INTO input_files VALUES ('../design/ADR-004-REFINEMENT.0.md', '631969750913133884', 19844);
INSERT INTO input_files VALUES ('../design/BASIC-CHECK-CUT.0.md', '1575227922863509534', 8195);
INSERT INTO input_files VALUES ('../design/BORU-INFOVIEW.0.md', '2090869893701264049', 24408);
INSERT INTO input_files VALUES ('../design/BORU-SCRY.0.md', '3285728856019765195', 16574);
INSERT INTO input_files VALUES ('../design/BORU-VIZ.0.md', '1948552775752251328', 25772);
INSERT INTO input_files VALUES ('../design/CANON-ROUNDTRIP.0.md', '1353161920241380593', 7262);
INSERT INTO input_files VALUES ('../design/CONTENT-ADDRESSING.0.md', '2493386413761043225', 18345);
INSERT INTO input_files VALUES ('../design/CORE-TS-COVERAGE.0.md', '7605485402373327537', 10411);
INSERT INTO input_files VALUES ('../design/CORE-TS-DIVERGENCES.1.md', '7903590270407909717', 22542);
INSERT INTO input_files VALUES ('../design/DECLARATIVE-GRAMMAR.0.md', '4337381568175830188', 3240);
INSERT INTO input_files VALUES ('../design/ENG-COVERAGE-PARITY.0.md', '2541301273793164298', 20169);
INSERT INTO input_files VALUES ('../design/FN-VALUE-OPEN-WORK.0.md', '8479443366485179161', 26190);
INSERT INTO input_files VALUES ('../design/FUNCTION-VALUE-SCOPE.0.md', '4913722754517353679', 65012);
INSERT INTO input_files VALUES ('../design/GO-MODULE-GRAPH.0.md', '4124035938153723972', 28792);
INSERT INTO input_files VALUES ('../design/GO-TS-PARITY.0.md', '3044536311677551962', 23460);
INSERT INTO input_files VALUES ('../design/HOT-CODE-LOADING.0.md', '27384681369847472', 19039);
INSERT INTO input_files VALUES ('../design/LANG-ENG-CONTENT-AUDIT.0.md', '9151136880658015899', 42992);
INSERT INTO input_files VALUES ('../design/MODULE-VIEWS.0.md', '570466612363092696', 22324);
INSERT INTO input_files VALUES ('../design/RELOAD-INVALIDATION.0.md', '2757752285559380360', 21723);
INSERT INTO input_files VALUES ('../design/ROOT-MODULE-FEASIBILITY.0.md', '6235317352461048001', 6313);
INSERT INTO input_files VALUES ('../design/STATE-MACHINES.0.md', '7873846898373868814', 88994);
INSERT INTO input_files VALUES ('../design/TS-PARITY-AUDIT.0.md', '2607459948929922084', 6455);
INSERT INTO input_files VALUES ('../design/checker-compiler-completeness-review.0.md', '5862232790816677050', 39943);
INSERT INTO input_files VALUES ('../editors/tree-sitter/bindings/go/go.mod', '8359550297204300245', 134);
INSERT INTO input_files VALUES ('../eng/go/go.mod', '1395838019470277805', 800);
INSERT INTO input_files VALUES ('../go.work', '3757042855308333246', 546);
INSERT INTO input_files VALUES ('../lang/go/go.mod', '4939140721251129377', 2432);
INSERT INTO input_files VALUES ('../parser/go/go.mod', '8990721982208133206', 350);
INSERT INTO input_files VALUES ('../test/go/go.mod', '5812574713479453703', 3027);
INSERT INTO input_files VALUES ('../test/solardemo/go.mod', '8784937342672483810', 59);
INSERT INTO input_files VALUES ('../test/specfix/go.mod', '7601104241745438425', 1242);
INSERT INTO input_files VALUES ('../tools/piecetool/go.mod', '3890078019736541119', 539);
INSERT INTO input_files VALUES ('../wpg/go.mod', '6010678691882061351', 2627);
INSERT INTO input_files VALUES ('<go tree: modules + packages>', '5798490287673095801', 500);
INSERT INTO input_files VALUES ('project/boru-project.jsonic', '3902473583519536666', 46483);
INSERT INTO sources VALUES ('src:adr-004-refinement', 'text', 'design/ADR-004-REFINEMENT.0.md', 'ADR-004 refinement — argument-handling categories', NULL, 'adr-004-refinement-2026-08-15', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:agents', 'text', 'AGENTS.md', 'AGENTS.md agent guide', NULL, 'agents-2026-07', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:audit', 'text', 'design/LANG-ENG-CONTENT-AUDIT.0.md', 'lang/eng content audit — ADR-012 evidence', NULL, 'audit-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:basic-check-cut', 'text', 'design/BASIC-CHECK-CUT.0.md', 'removing basic''s dependency on check', NULL, 'basic-check-cut-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:boru-infoview', 'text', 'design/BORU-INFOVIEW.0.md', 'boru infoview proposal — the stack at the cursor', NULL, 'boru-infoview-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:boru-scry', 'text', 'design/BORU-SCRY.0.md', 'boru:scry introspection-as-data proposal', NULL, 'boru-scry-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:boru-viz', 'text', 'design/BORU-VIZ.0.md', 'boru:viz diagram source generation proposal', NULL, 'boru-viz-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:canon-roundtrip', 'text', 'design/CANON-ROUNDTRIP.0.md', 'CANON-ROUNDTRIP — canon always round-trips', NULL, 'canon-roundtrip-2026-08-15', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:cli-md', 'text', 'CLI.md', 'CLI.md subcommand reference', NULL, 'cli-md-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:completeness-review', 'text', 'design/checker-compiler-completeness-review.0.md', 'Type checker + bytecode compiler — completeness review', NULL, 'completeness-review-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:content-addressing', 'text', 'design/CONTENT-ADDRESSING.0.md', 'content addressing: identity by hash, and what it would actually take', NULL, 'content-addressing-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:core-ts-coverage', 'text', 'design/CORE-TS-COVERAGE.0.md', 'core/ts coverage program', NULL, 'core-ts-coverage-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:core-ts-divergences', 'text', 'design/CORE-TS-DIVERGENCES.1.md', '135 measured core-level Go/TS divergences', NULL, 'core-ts-divergences-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:coverage-parity', 'text', 'design/ENG-COVERAGE-PARITY.0.md', 'eng standalone coverage-parity program', NULL, 'coverage-parity-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:decl-grammar', 'text', 'design/DECLARATIVE-GRAMMAR.0.md', 'declarative grammar artifact for both parser twins', NULL, 'decl-grammar-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:fn-value-open-work', 'text', 'design/FN-VALUE-OPEN-WORK.0.md', 'function values: the open work, re-measured', NULL, 'fn-value-open-work-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:function-value-scope', 'text', 'design/FUNCTION-VALUE-SCOPE.0.md', 'function value scope: where a fn value''s free words resolve', NULL, 'function-value-scope-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:go-module-graph', 'text', 'design/GO-MODULE-GRAPH.0.md', 'Go module graph and per-module coverage, measured', NULL, 'go-module-graph-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:go-tree', 'api', 'workspace tree', 'Go package discovery (filesystem walk)', NULL, 'go-tree', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:go-ts-parity', 'text', 'design/GO-TS-PARITY.0.md', 'full functional parity on core, parser and basic', NULL, 'go-ts-parity-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:go-work', 'text', 'go.work', 'Go workspace file', NULL, 'go-work', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:basic-go', 'text', 'basic/go/go.mod', 'basic/go go.mod', NULL, 'gomod-basic-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:calc-go', 'text', 'calc/go/go.mod', 'calc/go go.mod', NULL, 'gomod-calc-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:check-go', 'text', 'check/go/go.mod', 'check/go go.mod', NULL, 'gomod-check-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:cmd-go', 'text', 'cmd/go/go.mod', 'cmd/go go.mod', NULL, 'gomod-cmd-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:compiler-go', 'text', 'compiler/go/go.mod', 'compiler/go go.mod', NULL, 'gomod-compiler-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:core-go', 'text', 'core/go/go.mod', 'core/go go.mod', NULL, 'gomod-core-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:editors-tree-sitter-bindings-go', 'text', 'editors/tree-sitter/bindings/go/go.mod', 'editors/tree-sitter/bindings/go go.mod', NULL, 'gomod-editors-tree-sitter-bindings-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:eng-go', 'text', 'eng/go/go.mod', 'eng/go go.mod', NULL, 'gomod-eng-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:lang-go', 'text', 'lang/go/go.mod', 'lang/go go.mod', NULL, 'gomod-lang-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:parser-go', 'text', 'parser/go/go.mod', 'parser/go go.mod', NULL, 'gomod-parser-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:test-go', 'text', 'test/go/go.mod', 'test/go go.mod', NULL, 'gomod-test-go', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:test-solardemo', 'text', 'test/solardemo/go.mod', 'test/solardemo go.mod', NULL, 'gomod-test-solardemo', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:test-specfix', 'text', 'test/specfix/go.mod', 'test/specfix go.mod', NULL, 'gomod-test-specfix', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:tools-piecetool', 'text', 'tools/piecetool/go.mod', 'tools/piecetool go.mod', NULL, 'gomod-tools-piecetool', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:gomod:wpg', 'text', 'wpg/go.mod', 'wpg go.mod', NULL, 'gomod-wpg', 'primary', '{
  "derived_from": "code",
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:hot-code-loading', 'text', 'design/HOT-CODE-LOADING.0.md', 'hot code loading: the mechanism report and reload design', NULL, 'hot-code-loading-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:module-views', 'text', 'design/MODULE-VIEWS.0.md', 'module-provided views and widgets proposal', NULL, 'module-views-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:readme', 'text', 'README.md', 'boru README', NULL, 'readme-2026-07', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:reload-invalidation', 'text', 'design/RELOAD-INVALIDATION.0.md', 'reload invalidation: hot reload with transparent compilation at zero hot-path cost', NULL, 'reload-invalidation-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:root-module', 'text', 'design/ROOT-MODULE-FEASIBILITY.0.md', 'root module below core and parser, measured', NULL, 'root-module-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:state-machines', 'text', 'design/STATE-MACHINES.0.md', 'general-purpose state machines: the boru:state module', NULL, 'state-machines-2026-08-14', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:ts-parity-audit', 'text', 'design/TS-PARITY-AUDIT.0.md', 'parser twin parity audit', NULL, 'ts-parity-audit-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO entities VALUES ('ent:Concept:2039420555596601682', 'Concept', 'secrets vault', 'secrets vault', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Concept:2039420555596601682', 'role', 'encrypted API-key / token store, driven by the boru vault subcommand');
INSERT INTO entities VALUES ('ent:Concept:3854395902791518463', 'Concept', 'boru language', 'boru language', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Concept:3854395902791518463', 'paradigm', 'concatenative, typed, word-based');
INSERT INTO entities VALUES ('ent:Concept:4587555710592773395', 'Concept', 'Forward arguments', 'forward arguments', 'accepted');
INSERT INTO entities VALUES ('ent:Concept:4841193570246608846', 'Concept', 'Value stack', 'value stack', 'accepted');
INSERT INTO entities VALUES ('ent:Concept:5837115061456563631', 'Concept', 'boru describe discovery', 'boru describe discovery', 'accepted');
INSERT INTO entities VALUES ('ent:Concept:6094411313845087998', 'Concept', 'vault wire protocol', 'vault wire protocol', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Concept:6094411313845087998', 'role', 'read-only, HashiCorp-style HTTP API for secret provision (boru vault serve), authenticated by capability tokens');
INSERT INTO entities VALUES ('ent:Concept:7376417356888575267', 'Concept', 'Executable language spec', 'executable language spec', 'accepted');
INSERT INTO entities VALUES ('ent:Document:1162242714758522750', 'Document', 'design/BASIC-CHECK-CUT.0.md', 'design/basic-check-cut.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:1162242714758522750', 'role', 'why basic no longer depends on check: the 23 symbols it used, which moved into core as pure primitives, which became a core-owned analysis seam, and the two gates that keep the edge from coming back');
INSERT INTO entities VALUES ('ent:Document:1344160336771235777', 'Document', 'EXPLANATION.md', 'explanation.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:1913611373576952100', 'Document', 'design/STATE-MACHINES.0.md', 'design/state-machines.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:1913611373576952100', 'role', 'the design RFC for general-purpose state machines: a mixed Go+boru boru:state module (definition/bindings/snapshot split, pure step, thirteen-item semantic freeze, in-definition input classification via classes:/classify:, state_* check diagnostics, service and process hosts) and the argued decision to add words, not syntax — revised against Noble''s Forth FSM paper for the tabular lineage the statechart survey had missed');
INSERT INTO entities VALUES ('ent:Document:203047846460430642', 'Document', 'design/DECLARATIVE-GRAMMAR.0.md', 'design/declarative-grammar.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:203047846460430642', 'role', 'the shared declarative tabnas grammar artifact (parser/go/grammar.json): contract, loader pair, and the batch-migration state');
INSERT INTO entities VALUES ('ent:Document:2168448879393025844', 'Document', 'design/BORU-VIZ.0.md', 'design/boru-viz.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:2168448879393025844', 'role', 'the boru:viz proposal: pure diagram-source generation (Mermaid + DOT, D2 later) from arbitrary data structures — code generation only, written in boru, with the shared graph/tree/trace/schema contract its §3 pins for boru:scry and every other producer');
INSERT INTO entities VALUES ('ent:Document:2308799538575712501', 'Document', 'design/CORE-TS-DIVERGENCES.1.md', 'design/core-ts-divergences.1.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:2308799538575712501', 'role', 'the ten classes of measured core/go-vs-core/ts divergence behind core/spec/divergent.tsv, why the 1808-row crossdiff was blind to all of them, and the hit rate that makes the uncovered surface the place to look');
INSERT INTO entities VALUES ('ent:Document:2574876380285708892', 'Document', 'design/ROOT-MODULE-FEASIBILITY.0.md', 'design/root-module-feasibility.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:2574876380285708892', 'role', 'the measured verdict on a shared module below core and parser: Value alone closes over 75.7% of core/go, so the cut as posed is a rename — includes the piecetool -closure method and the priced seam alternatives');
INSERT INTO entities VALUES ('ent:Document:3080274854606714513', 'Document', 'ADR.md', 'adr.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:3294415633888265368', 'Document', 'design/checker-compiler-completeness-review.0.md', 'design/checker-compiler-completeness-review.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:3294415633888265368', 'role', 'the 2026-08 checker/compiler completeness review and its §9 implementation record — the living index of the HOF-compilation graduations and the remaining frontier work');
INSERT INTO entities VALUES ('ent:Document:3521225411209893772', 'Document', 'design/LANG-ENG-CONTENT-AUDIT.0.md', 'design/lang-eng-content-audit.0.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:3534903004749141856', 'Document', 'design/CONTENT-ADDRESSING.0.md', 'design/content-addressing.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:3534903004749141856', 'role', 'the design note for deriving a definition''s identity from its content rather than its name: the three costs that share that root cause (the per-invoke DepsFresh walk, the AOT codec''s symbolic-reference refusals, the pre-1.0 rename tax), the split between an ARTIFACT digest over file bytes — unblocked, already specified by boru-vendor §5 — and a DEFINITION digest over meaning, which needs canonicity, alpha normalisation, macro expansion, referent substitution and cycle components; three options for referent substitution under call-time binding, recommending a (text digest, world digest) compound key; a five-phase sequence; and the rejections — codebase-as-database, hash-based type identity, immutable definitions as a language rule. Measured by design/unison-hash-identity-probe.0.md and scripts/hash-identity-probe.sh');
INSERT INTO entities VALUES ('ent:Document:3904106568037504161', 'Document', 'AGENTS.md', 'agents.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:3955423872539901697', 'Document', 'HOWTO.md', 'howto.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:4163489813681141089', 'Document', 'README.md', 'readme.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:4529681846923033548', 'Document', 'design/MODULE-VIEWS.0.md', 'design/module-views.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:4529681846923033548', 'role', 'the rich-display sequel BORU-VIZ.0.md deferred: how any module exposes views and widgets of its own semantics — producer words over the viz data contract with kind-tag vocabulary, TUI widgets by composition, the view word''s display bundles via open-word extension, and dashboard alignment with DEBUG-MODULE §7');
INSERT INTO entities VALUES ('ent:Document:4790579719562719716', 'Document', 'design/CORE-TS-COVERAGE.0.md', 'design/core-ts-coverage.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:4790579719562719716', 'role', 'the coverage program for core/ts (the sibling of ENG-COVERAGE-PARITY, one layer down): the staged work list, the ratcheting TS_CORE_GATE_LINES floor, and the measurement that core/spec is a cross-engine SPEC and not a coverage instrument');
INSERT INTO entities VALUES ('ent:Document:481508614007064969', 'Document', 'REFERENCE.md', 'reference.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:4880036076125012648', 'Document', 'design/HOT-CODE-LOADING.0.md', 'design/hot-code-loading.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:4880036076125012648', 'role', 'the report on boru''s hot code loading ability — late binding end to end, uncached file modules, GC-generational old code, fork snapshot semantics, the aless re-anchor precedent, the BEAM comparison — and the reload-as-a-protocol design: a reload word with keep-old-on-failure, supervisor-broadcast propagation, the Plugin.migrate state hook, handler-stack hygiene, a persistent Vm.open sub-engine');
INSERT INTO entities VALUES ('ent:Document:4990200910103175455', 'Document', 'CLI.md', 'cli.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:5105101056062860425', 'Document', 'design/ENG-COVERAGE-PARITY.0.md', 'design/eng-coverage-parity.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:5105101056062860425', 'role', 'the standalone 100%/100% coverage program for eng/go and eng/ts: the ratcheting gate floors (make cover-gate-eng, make test-ts), the gap inventories, and the staged plans');
INSERT INTO entities VALUES ('ent:Document:5175176782070740682', 'Document', 'design/BORU-INFOVIEW.0.md', 'design/boru-infoview.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:5175176782070740682', 'role', 'the boru infoview proposal: position-indexed stack display learned from the Lean 4 infoview — carriers at the cursor via a trace-armed Check, actual values via the debugger''s ring and replay, LSP-first delivery (inlay hints, hover, code actions, boru/stackAt), then a binary-served panel rendering scry data through viz');
INSERT INTO entities VALUES ('ent:Document:520435226487613788', 'Document', 'design/ notes', 'design/ notes', 'accepted');
INSERT INTO entities VALUES ('ent:Document:5215900749522722466', 'Document', 'design/GO-MODULE-GRAPH.0.md', 'design/go-module-graph.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:5215900749522722466', 'role', 'the measured Go-side snapshot: the module inventory, the direct-require graph and its twelve-edge transitive reduction, and per-module coverage in both columns — the merged ADR-008 gate and each module''s own standalone suite');
INSERT INTO entities VALUES ('ent:Document:5292060467150439417', 'Document', 'NUR.md', 'nur.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:5313783338663858074', 'Document', 'design/RELOAD-INVALIDATION.0.md', 'design/reload-invalidation.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:5313783338663858074', 'role', 'how hot reload and transparent compilation coexist without slowing compiled code: per-ref valid flags flipped push-style through a reverse dependency index (the HotSpot/Truffle/Julia equilibrium), ref unification at Finalize, world-pinned whole-program units, per-world restamp budgets — plus the confirmed F1 pass-hoisting divergence and its interim whole-program-refusal fix');
INSERT INTO entities VALUES ('ent:Document:5807284485979550128', 'Document', 'design/ADR-004-REFINEMENT.0.md', 'design/adr-004-refinement.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:5807284485979550128', 'role', 'ADR-004''s reasoning note, PROMOTED 2026-08-15 into the record''s four-categories amendment (NUR023, retired): the four argument-handling categories (forward-eligible, mixed-barrier, stack-only, quoting slots), BarrierPos semantics and its five resolution sites, the stack-only closed list with a two-criterion admission test that admits both apply and __casematch, and the composition rationale — measured at 97 intermediate-barrier signatures of 493, distinguished from the 20-word mixed-overload count');
INSERT INTO entities VALUES ('ent:Document:5873755881373321364', 'Document', 'design/TABNAS-DOT-BOUNDARY-REPORT.0.md', 'design/tabnas-dot-boundary-report.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:5873755881373321364', 'role', 'the file-ready upstream defect report for the last live tabnas shim: both ports'' runnable reproduction against the bare dependency, the follow-character table showing the divergence is conditional (0xFF.5 differs, 0xFF.x agrees), and the acceptance criterion that a regression test must cover both');
INSERT INTO entities VALUES ('ent:Document:6176355086953937469', 'Document', 'TUTORIAL.md', 'tutorial.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:6186742803977787158', 'Document', 'design/BORU-SCRY.0.md', 'design/boru-scry.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:6186742803977787158', 'role', 'the boru:scry proposal: the mechanism whereby a boru system gains knowledge of itself as plain data — census, per-word, graph, schema and trace words curated over existing engine seams, and the plan for resolving the overlap with boru:debug');
INSERT INTO entities VALUES ('ent:Document:6369673620858945660', 'Document', 'eng/go/CLAUDE.md', 'eng/go/claude.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:7583878321315113890', 'Document', 'design/TABNAS-UPSTREAM-FIRST.0.md', 'design/tabnas-upstream-first.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:7583878321315113890', 'role', 'ADR-014''s case study: why a tabnas parser defect is fixed upstream and never behind a boru shim — the five shims the 2026-08-10 upgrade retired, the two-workarounds-deep episode that earned the rule, and the boundary against boru''s own grammar-layer divergences');
INSERT INTO entities VALUES ('ent:Document:7594380001231677524', 'Document', 'design/FUNCTION-VALUE-SCOPE.0.md', 'design/function-value-scope.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:7594380001231677524', 'role', 'the defect report and design for free-word resolution in function values: the interpreter resolves them in the RUNNING module while the bytecode compiler resolves them in the DEFINING one, so a cross-module fn value can silently bind a same-named word in the caller and return a wrong number with boru check clean; the mechanism (FnDefInfo.Registry set at export, honoured on the value path and by the VM, dropped by name dispatch and by the native-callback seam), why closure capture cannot carry it, the phased fix, the tree-wide migration audit measuring 0 migration sites and 0 silent-change sites against 224+ repaired (the cost is nil because the ecosystem already worked around it in writing — 12 utils unroll Cli.main, aless ships a wrapper, sort.aql is one file), the finding that every divergence runner in the ecosystem runs compile-preferring mode in its INTERPRETER column and is structurally blind, and the three-clause target semantics for function values');
INSERT INTO entities VALUES ('ent:Document:7656991804093821644', 'Document', 'design/FN-VALUE-OPEN-WORK.0.md', 'design/fn-value-open-work.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:7656991804093821644', 'role', 'the open-work inventory for the function-value line after PRs #366/#375/#378: four remaining items, all unblocked since the 2026-08-17 maintainer rulings (§1.1) — clause 3 (parens do not re-step) ruled BROAD, clause 2 (passing a function requires /r) enabled by the ADR-011 amendment with all four engine sites retiring together (re-opening the NUR038 call-head question, registered as NUR078), break 2 (the compiler''s sound refusal of a 0-arg fn read from a plain container), and the StackForm apply — ruled a new dedicated Apply Op — every figure re-measured against 8732662, correcting three that the earlier notes carry: the clause-3 row count is 30 not 31, the clause-2 bare-name count is 3 not 9, and narrow-vs-broad ARE separable, by FnDefInfo.Name rather than by a rejected value-borne marker; plus the finding that break 2 is an arity-0 extension of tryMemberFnArrivalDispatch rather than Phase 3 work, that its refusal masks a confirmed miscompile and is untracked by any frontier row, and that the apply Op''s sketched target has two holes — a 0-arg anonymous fn is silently not applied, and `apply` itself double-records so a replay applies twice');
INSERT INTO entities VALUES ('ent:Document:7770110494347118706', 'Document', 'lang/go/CLAUDE.md', 'lang/go/claude.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:8751021793288559660', 'Document', 'design/CANON-ROUNDTRIP.0.md', 'design/canon-roundtrip.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:8751021793288559660', 'role', 'ADR-015''s reasoning: why canon is a VALUE round-trip (deq) rather than a textual fixpoint, why no kind is exempt, the 2026-08-15 measurements showing functions render as a debug spelling keyed on the binding name and Store as a pointer-bearing Go struct dump, NUR031 as the equality prerequisite, and the two-port property gate that lands with a shrinking failure ledger');
INSERT INTO entities VALUES ('ent:Document:8799605016341004740', 'Document', 'design/TS-PARITY-AUDIT.0.md', 'design/ts-parity-audit.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:8799605016341004740', 'role', 'the parser twin parity audit: the render defects the stream oracle found, the parser/spec corpus that replaced the self-referential battery, and the open divergences');
INSERT INTO entities VALUES ('ent:Document:8875579216819453768', 'Document', 'design/GO-TS-PARITY.0.md', 'design/go-ts-parity.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:8875579216819453768', 'role', 'the Go/TS parity program for core, parser and basic: the three shared TSV corpora and their two-runner rule, the capability table that forces core/ts before basic/ts, the divergence ledger, and the finding that an uncovered branch in one port is where a divergence hides');
INSERT INTO entities VALUES ('ent:Document:9071667555031388966', 'Document', 'parser/go/CLAUDE.md', 'parser/go/claude.md', 'accepted');
INSERT INTO entities VALUES ('ent:Organization:8832543031059216933', 'Organization', 'boru-lang', 'boru-lang', 'accepted');
INSERT INTO entities VALUES ('ent:Product:2046405728378673079', 'Product', 'kg knowledge-graph pipeline', 'kg knowledge-graph pipeline', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:2046405728378673079', 'path', 'kg');
INSERT INTO entities VALUES ('ent:Product:4032424380612892464', 'Product', 'boru-lang/boru repository', 'boru-lang/boru repository', 'accepted');
INSERT INTO entity_external_ids VALUES ('ent:Product:4032424380612892464', 'github', 'boru-lang/boru');
INSERT INTO entities VALUES ('ent:Product:4976123614970806523', 'Product', 'boru:cli module', 'boru:cli module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:4976123614970806523', 'language', 'boru');
INSERT INTO entity_attributes VALUES ('ent:Product:4976123614970806523', 'path', 'lang/go/modules/cli.boru');
INSERT INTO entities VALUES ('ent:Product:5770789618934008141', 'Product', 'lang/spec directory', 'lang/spec directory', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:5770789618934008141', 'path', 'lang/spec');
INSERT INTO entities VALUES ('ent:Product:8635404738244704660', 'Product', 'utils coreutils subset', 'utils coreutils subset', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:8635404738244704660', 'path', 'utils');
INSERT INTO entities VALUES ('ent:Product:9122085017676103232', 'Product', 'boru binary', 'boru binary', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:9122085017676103232', 'role', 'CLI, REPL, type checker, formatter, LSP, registry client, vault, supervisor');
INSERT INTO entities VALUES ('ent:SoftwareModule:1529704399546216258', 'SoftwareModule', 'wpg/serve package', 'wpg/serve package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:1529704399546216258', 'go_module', 'github.com/boru-lang/boru/wpg');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:1529704399546216258', 'parent_module', 'wpg');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:1529704399546216258', 'path', 'wpg/serve');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:1529704399546216258', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:1633412495372444656', 'SoftwareModule', 'lang/go/modules package', 'lang/go/modules package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:1633412495372444656', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:1633412495372444656', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:1633412495372444656', 'path', 'lang/go/modules');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:1633412495372444656', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:2013670336276694550', 'SoftwareModule', 'core/go module', 'core/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:2013670336276694550', 'go_module', 'github.com/boru-lang/boru/core/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:2013670336276694550', 'path', 'core/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:2013670336276694550', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:2013670336276694550', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:2224184413468547100', 'SoftwareModule', 'lang/go/test package', 'lang/go/test package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:2224184413468547100', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:2224184413468547100', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:2224184413468547100', 'path', 'lang/go/test');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:2224184413468547100', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:3568378724923050340', 'SoftwareModule', 'test/go/specgen package', 'test/go/specgen package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:3568378724923050340', 'go_module', 'github.com/boru-lang/boru/test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:3568378724923050340', 'parent_module', 'test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:3568378724923050340', 'path', 'test/go/specgen');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:3568378724923050340', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:3641482396164462740', 'SoftwareModule', 'test/go/docexamples package', 'test/go/docexamples package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:3641482396164462740', 'go_module', 'github.com/boru-lang/boru/test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:3641482396164462740', 'parent_module', 'test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:3641482396164462740', 'path', 'test/go/docexamples');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:3641482396164462740', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:4116347247488215916', 'SoftwareModule', 'test/go/covergate package', 'test/go/covergate package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4116347247488215916', 'go_module', 'github.com/boru-lang/boru/test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4116347247488215916', 'parent_module', 'test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4116347247488215916', 'path', 'test/go/covergate');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4116347247488215916', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:4125120573695539350', 'SoftwareModule', 'test/go/langspec package', 'test/go/langspec package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4125120573695539350', 'go_module', 'github.com/boru-lang/boru/test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4125120573695539350', 'parent_module', 'test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4125120573695539350', 'path', 'test/go/langspec');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4125120573695539350', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:4192460694199531608', 'SoftwareModule', 'wpg module', 'wpg module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4192460694199531608', 'go_module', 'github.com/boru-lang/boru/wpg');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4192460694199531608', 'path', 'wpg');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4192460694199531608', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4192460694199531608', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:425341189454841366', 'SoftwareModule', 'eng/go module', 'eng/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:425341189454841366', 'go_module', 'github.com/boru-lang/boru/eng/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:425341189454841366', 'path', 'eng/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:425341189454841366', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:425341189454841366', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:4361728672720029650', 'SoftwareModule', 'cmd/go module', 'cmd/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4361728672720029650', 'go_module', 'github.com/boru-lang/boru/cmd/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4361728672720029650', 'path', 'cmd/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4361728672720029650', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4361728672720029650', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:4386785925506277682', 'SoftwareModule', 'test/specfix module', 'test/specfix module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4386785925506277682', 'go_module', 'github.com/boru-lang/boru/test/specfix');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4386785925506277682', 'path', 'test/specfix');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4386785925506277682', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4386785925506277682', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:4559967244660037230', 'SoftwareModule', 'basic/go module', 'basic/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4559967244660037230', 'go_module', 'github.com/boru-lang/boru/basic/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4559967244660037230', 'path', 'basic/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4559967244660037230', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4559967244660037230', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:4589894982256403754', 'SoftwareModule', 'cmd/go/genhelp package', 'cmd/go/genhelp package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4589894982256403754', 'go_module', 'github.com/boru-lang/boru/cmd/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4589894982256403754', 'parent_module', 'cmd/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4589894982256403754', 'path', 'cmd/go/genhelp');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4589894982256403754', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:4598127138166596702', 'SoftwareModule', 'test/solardemo module', 'test/solardemo module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4598127138166596702', 'go_module', 'github.com/boru-lang/boru/test/solardemo');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4598127138166596702', 'path', 'test/solardemo');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4598127138166596702', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4598127138166596702', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:4598450785187489172', 'SoftwareModule', 'lang/go/capabilities package', 'lang/go/capabilities package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4598450785187489172', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4598450785187489172', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4598450785187489172', 'path', 'lang/go/capabilities');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4598450785187489172', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:4629987360876655630', 'SoftwareModule', 'lang/go/native package', 'lang/go/native package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4629987360876655630', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4629987360876655630', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4629987360876655630', 'path', 'lang/go/native');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4629987360876655630', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:4648368824093240216', 'SoftwareModule', 'editors/tree-sitter/bindings/go module', 'editors/tree-sitter/bindings/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4648368824093240216', 'go_module', 'github.com/tree-sitter/tree-sitter-boru');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4648368824093240216', 'path', 'editors/tree-sitter/bindings/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4648368824093240216', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4648368824093240216', 'workspace_member', 'false');
INSERT INTO entities VALUES ('ent:SoftwareModule:4765954660468613608', 'SoftwareModule', 'test/go/cliexamples package', 'test/go/cliexamples package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4765954660468613608', 'go_module', 'github.com/boru-lang/boru/test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4765954660468613608', 'parent_module', 'test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4765954660468613608', 'path', 'test/go/cliexamples');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:4765954660468613608', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:5138375578915662736', 'SoftwareModule', 'test/go module', 'test/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5138375578915662736', 'go_module', 'github.com/boru-lang/boru/test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5138375578915662736', 'path', 'test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5138375578915662736', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5138375578915662736', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:5172256887243902980', 'SoftwareModule', 'test/go/vary package', 'test/go/vary package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5172256887243902980', 'go_module', 'github.com/boru-lang/boru/test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5172256887243902980', 'parent_module', 'test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5172256887243902980', 'path', 'test/go/vary');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5172256887243902980', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:559301050642427014', 'SoftwareModule', 'check/go module', 'check/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:559301050642427014', 'go_module', 'github.com/boru-lang/boru/check/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:559301050642427014', 'path', 'check/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:559301050642427014', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:559301050642427014', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:591370078981669488', 'SoftwareModule', 'test/go/engspec package', 'test/go/engspec package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:591370078981669488', 'go_module', 'github.com/boru-lang/boru/test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:591370078981669488', 'parent_module', 'test/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:591370078981669488', 'path', 'test/go/engspec');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:591370078981669488', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:5919856352625365594', 'SoftwareModule', 'calc/go module', 'calc/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5919856352625365594', 'go_module', 'github.com/boru-lang/boru/calc/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5919856352625365594', 'path', 'calc/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5919856352625365594', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:5919856352625365594', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:6519065918608370232', 'SoftwareModule', 'wpg/wasm package', 'wpg/wasm package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6519065918608370232', 'go_module', 'github.com/boru-lang/boru/wpg');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6519065918608370232', 'parent_module', 'wpg');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6519065918608370232', 'path', 'wpg/wasm');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6519065918608370232', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:6706536563979604982', 'SoftwareModule', 'parser/go module', 'parser/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6706536563979604982', 'go_module', 'github.com/boru-lang/boru/parser/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6706536563979604982', 'path', 'parser/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6706536563979604982', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6706536563979604982', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:6880687338933514154', 'SoftwareModule', 'compiler/go module', 'compiler/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6880687338933514154', 'go_module', 'github.com/boru-lang/boru/compiler/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6880687338933514154', 'path', 'compiler/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6880687338933514154', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:6880687338933514154', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:7327774866203707838', 'SoftwareModule', 'tools/piecetool module', 'tools/piecetool module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:7327774866203707838', 'go_module', 'github.com/boru-lang/boru/tools/piecetool');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:7327774866203707838', 'path', 'tools/piecetool');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:7327774866203707838', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:7327774866203707838', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:8275629451197117420', 'SoftwareModule', 'lang/go module', 'lang/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8275629451197117420', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8275629451197117420', 'path', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8275629451197117420', 'unit', 'go-module');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8275629451197117420', 'workspace_member', 'true');
INSERT INTO entities VALUES ('ent:SoftwareModule:8281032769668059040', 'SoftwareModule', 'lang/go/policy package', 'lang/go/policy package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8281032769668059040', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8281032769668059040', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8281032769668059040', 'path', 'lang/go/policy');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8281032769668059040', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:8290440371688101140', 'SoftwareModule', 'lang/go/stackform package', 'lang/go/stackform package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8290440371688101140', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8290440371688101140', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8290440371688101140', 'path', 'lang/go/stackform');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8290440371688101140', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:8376380866775607244', 'SoftwareModule', 'lang/go/formatter package', 'lang/go/formatter package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8376380866775607244', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8376380866775607244', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8376380866775607244', 'path', 'lang/go/formatter');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8376380866775607244', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:8582068475984941492', 'SoftwareModule', 'lang/go/tuikit package', 'lang/go/tuikit package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8582068475984941492', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8582068475984941492', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8582068475984941492', 'path', 'lang/go/tuikit');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8582068475984941492', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:8650101979302333988', 'SoftwareModule', 'cmd/go/boru package', 'cmd/go/boru package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8650101979302333988', 'go_module', 'github.com/boru-lang/boru/cmd/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8650101979302333988', 'parent_module', 'cmd/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8650101979302333988', 'path', 'cmd/go/boru');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:8650101979302333988', 'unit', 'go-package');
INSERT INTO entities VALUES ('ent:SoftwareModule:927371487649425292', 'SoftwareModule', 'lang/go/debugserve package', 'lang/go/debugserve package', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:927371487649425292', 'go_module', 'github.com/boru-lang/boru/lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:927371487649425292', 'parent_module', 'lang/go');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:927371487649425292', 'path', 'lang/go/debugserve');
INSERT INTO entity_attributes VALUES ('ent:SoftwareModule:927371487649425292', 'unit', 'go-package');
INSERT INTO assertions VALUES ('ast:1048763594130214961', 'ent:SoftwareModule:591370078981669488', 'part_of', 'entity', 'ent:SoftwareModule:5138375578915662736', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1048763594130214961', 'src:go-tree', 'test/go/engspec', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1126534628156470241', 'ent:SoftwareModule:5138375578915662736', 'depends_on', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1126534628156470241', 'src:gomod:test-go', 'require block', 'github.com/boru-lang/boru/parser/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1147348943755751326', 'ent:SoftwareModule:5138375578915662736', 'depends_on', 'entity', 'ent:SoftwareModule:4386785925506277682', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1147348943755751326', 'src:gomod:test-go', 'require block', 'github.com/boru-lang/boru/test/specfix v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1200607522753691950', 'ent:Document:5105101056062860425', 'supports', 'entity', 'ent:SoftwareModule:425341189454841366', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1200607522753691950', 'src:coverage-parity', 'The contract', 'eng proves itself with its OWN suite, on both implementations.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1291087365743607224', 'ent:SoftwareModule:8275629451197117420', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/lang/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1291087365743607224', 'src:gomod:lang-go', 'module directive', 'module github.com/boru-lang/boru/lang/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1374144917814367558', 'ent:Concept:3854395902791518463', 'has_attribute', 'literal', NULL, '"concatenative"', 'String', NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1374144917814367558', 'src:readme', 'intro', 'Underneath, boru is concatenative: words can equally take their arguments from a value stack', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1432642710886220267', 'ent:SoftwareModule:8290440371688101140', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1432642710886220267', 'src:go-tree', 'lang/go/stackform', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1529088506745454642', 'ent:SoftwareModule:6880687338933514154', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1529088506745454642', 'src:go-work', 'use block', './compiler/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1536707264993433580', 'ent:SoftwareModule:4559967244660037230', 'depends_on', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1536707264993433580', 'src:gomod:basic-go', 'require block', 'github.com/boru-lang/boru/parser/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1615036799797522109', 'ent:Document:2168448879393025844', 'supports', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1615036799797522109', 'src:boru-viz', 'Architecture — what is new', '`lang/go/modules/viz.go` — `BuildVizModule` scaffold', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1731838044320190440', 'ent:Product:9122085017676103232', 'part_of', 'entity', 'ent:SoftwareModule:4361728672720029650', NULL, NULL, NULL, NULL, 0.97, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1731838044320190440', 'src:readme', 'Repository layout', 'cmd/go/ | The boru CLI / REPL', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1778398845817128450', 'ent:SoftwareModule:4629987360876655630', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1778398845817128450', 'src:go-tree', 'lang/go/native', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1787176291360062660', 'ent:Document:5807284485979550128', 'supports', 'entity', 'ent:Concept:4587555710592773395', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1787176291360062660', 'src:adr-004-refinement', 'Why it exists', 'ADR-004 describes a default but not a system.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1795605286754417159', 'ent:SoftwareModule:4386785925506277682', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/test/specfix"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1795605286754417159', 'src:gomod:test-specfix', 'module directive', 'module github.com/boru-lang/boru/test/specfix', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1803749198783100155', 'ent:SoftwareModule:5919856352625365594', 'depends_on', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1803749198783100155', 'src:gomod:calc-go', 'require block', 'github.com/boru-lang/boru/core/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:1870579735210236434', 'ent:Document:2168448879393025844', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1870579735210236434', 'src:boru-viz', 'title', 'boru:viz — diagram source generation from arbitrary data structures', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1910960561653111690', 'ent:SoftwareModule:4765954660468613608', 'part_of', 'entity', 'ent:SoftwareModule:5138375578915662736', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1910960561653111690', 'src:go-tree', 'test/go/cliexamples', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:2099674608751247787', 'ent:SoftwareModule:5138375578915662736', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2099674608751247787', 'src:go-work', 'use block', './test/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:2132946225724718620', 'ent:SoftwareModule:7327774866203707838', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/tools/piecetool"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2132946225724718620', 'src:gomod:tools-piecetool', 'module directive', 'module github.com/boru-lang/boru/tools/piecetool', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:2174589708589527462', 'ent:SoftwareModule:5138375578915662736', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/test/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2174589708589527462', 'src:gomod:test-go', 'module directive', 'module github.com/boru-lang/boru/test/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:2190303907985753377', 'ent:Document:3955423872539901697', 'supports', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2190303907985753377', 'src:readme', 'Documentation', 'How-To Guides | You have a specific task and want a recipe.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2250649441720436972', 'ent:SoftwareModule:4598450785187489172', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2250649441720436972', 'src:go-tree', 'lang/go/capabilities', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:2308356135508106004', 'ent:Document:5292060467150439417', 'mentions', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2308356135508106004', 'src:readme', 'Documentation', 'Non-Uniformity Register | You want the recorded deviations from the language''s uniform rules, each pending, resolved, or explicitly allowed.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2313048313946518913', 'ent:Product:8635404738244704660', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2313048313946518913', 'src:agents', 'Repository layout', 'utils/ | A coreutils subset written in boru — real programs that prove the CLI story (argv, exit codes, streams, baked permissions).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2321418037316607481', 'ent:SoftwareModule:5919856352625365594', 'depends_on', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2321418037316607481', 'src:gomod:calc-go', 'require block', 'github.com/boru-lang/boru/parser/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:2457753349338579625', 'ent:Document:4990200910103175455', 'supports', 'entity', 'ent:Product:9122085017676103232', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2457753349338579625', 'src:readme', 'Documentation', 'CLI Reference | You want to drive the boru binary from the shell.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2510153559955756366', 'ent:Concept:7376417356888575267', 'part_of', 'entity', 'ent:Product:5770789618934008141', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2510153559955756366', 'src:agents', 'Task router', 'The executable language spec (the rows tests run against) | lang/spec/*.tsv', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2522351206637396085', 'ent:SoftwareModule:6880687338933514154', 'depends_on', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2522351206637396085', 'src:gomod:compiler-go', 'require block', 'github.com/boru-lang/boru/core/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:2546454704586227304', 'ent:SoftwareModule:5138375578915662736', 'depends_on', 'entity', 'ent:SoftwareModule:4559967244660037230', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2546454704586227304', 'src:gomod:test-go', 'require block', 'github.com/boru-lang/boru/basic/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:2608375789644467266', 'ent:Document:5807284485979550128', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2608375789644467266', 'src:adr-004-refinement', 'title', 'ADR-004 refinement — argument-handling categories', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2618788706676923636', 'ent:Document:2574876380285708892', 'supports', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2618788706676923636', 'src:root-module', 'The finding', '**The finding: `Value` alone closes over 75.7% of `core/go`.**', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2650141132074711437', 'ent:Product:9122085017676103232', 'supports', 'entity', 'ent:Concept:5837115061456563631', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2650141132074711437', 'src:agents', 'First: let the tool document itself', 'The boru CLI documents both the language and itself, and that output is generated from the live engine', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2714957710081530623', 'ent:Document:3521225411209893772', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2714957710081530623', 'src:audit', 'title', 'Lang/Eng Content Audit — and the Types-Module Proposal', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2717924021842468202', 'ent:Document:5313783338663858074', 'supports', 'entity', 'ent:SoftwareModule:6880687338933514154', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2717924021842468202', 'src:reload-invalidation', '5. The design', 'replace the per-invoke DepSnap walk with a per-ref valid', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2979279625592738698', 'ent:SoftwareModule:1529704399546216258', 'part_of', 'entity', 'ent:SoftwareModule:4192460694199531608', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2979279625592738698', 'src:go-tree', 'wpg/serve', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3002920817860690706', 'ent:SoftwareModule:8275629451197117420', 'depends_on', 'entity', 'ent:SoftwareModule:4559967244660037230', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3002920817860690706', 'src:gomod:lang-go', 'require block', 'github.com/boru-lang/boru/basic/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3008209088769258064', 'ent:Document:6186742803977787158', 'supports', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3008209088769258064', 'src:boru-scry', 'Architecture', 'File layout: `lang/go/modules/scry.go`, `docs_scry.go`,', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:303023300656590935', 'ent:Document:1344160336771235777', 'supports', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:303023300656590935', 'src:readme', 'Documentation', 'Explanation | You want to understand why boru is the way it is.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:311117795035296475', 'ent:SoftwareModule:8275629451197117420', 'depends_on', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:311117795035296475', 'src:gomod:lang-go', 'require block', 'github.com/boru-lang/boru/parser/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:314582934812456120', 'ent:Document:3904106568037504161', 'mentions', 'entity', 'ent:Concept:5837115061456563631', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:314582934812456120', 'src:agents', 'First: let the tool document itself', 'Before grepping source or guessing a word''s signature, ask the binary.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3155371871007005577', 'ent:Document:8875579216819453768', 'supports', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3155371871007005577', 'src:go-ts-parity', 'The measurement', 'the parity gap is the uncovered surface', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3178522596510652426', 'ent:SoftwareModule:6706536563979604982', 'depends_on', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3178522596510652426', 'src:gomod:parser-go', 'require block', 'github.com/boru-lang/boru/core/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3211344478610580799', 'ent:Concept:3854395902791518463', 'related_to', 'entity', 'ent:Concept:4587555710592773395', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3211344478610580799', 'src:readme', 'Forward arguments', 'the defining feature of the surface syntax is forward arguments', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3217052068632183309', 'ent:Document:5175176782070740682', 'supports', 'entity', 'ent:SoftwareModule:4361728672720029650', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3217052068632183309', 'src:boru-infoview', 'Architecture — what is new', '`cmd/go/internal/lsp/` — inlay hints, hover, code actions, `boru/stackAt`', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3261162321000951212', 'ent:SoftwareModule:4386785925506277682', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3261162321000951212', 'src:go-work', 'use block', './test/specfix', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3331627459435955194', 'ent:SoftwareModule:8275629451197117420', 'depends_on', 'entity', 'ent:SoftwareModule:6880687338933514154', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3331627459435955194', 'src:gomod:lang-go', 'require block', 'github.com/boru-lang/boru/compiler/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3384030660507785166', 'ent:Document:1162242714758522750', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3384030660507785166', 'src:basic-check-cut', 'title', 'BASIC-CHECK-CUT.0 — removing `basic`''s dependency on `check`', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3410841662781299382', 'ent:Document:4790579719562719716', 'supports', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3410841662781299382', 'src:core-ts-coverage', 'The corpus is not the instrument', 'keep growing `core/spec` as a cross-engine spec, and stop treating it', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3427363101233560350', 'ent:Document:8751021793288559660', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3427363101233560350', 'src:canon-roundtrip', 'title', 'CANON-ROUNDTRIP — canon always round-trips', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3438862149494160814', 'ent:SoftwareModule:4589894982256403754', 'part_of', 'entity', 'ent:SoftwareModule:4361728672720029650', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3438862149494160814', 'src:go-tree', 'cmd/go/genhelp', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:351832070028345115', 'ent:SoftwareModule:4125120573695539350', 'part_of', 'entity', 'ent:SoftwareModule:5138375578915662736', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:351832070028345115', 'src:go-tree', 'test/go/langspec', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3612971925750437545', 'ent:SoftwareModule:8582068475984941492', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3612971925750437545', 'src:go-tree', 'lang/go/tuikit', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3656929081728712135', 'ent:SoftwareModule:7327774866203707838', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3656929081728712135', 'src:go-work', 'use block', './tools/piecetool', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3727255686445494517', 'ent:SoftwareModule:3568378724923050340', 'part_of', 'entity', 'ent:SoftwareModule:5138375578915662736', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3727255686445494517', 'src:go-tree', 'test/go/specgen', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3762011129750730370', 'ent:Document:5215900749522722466', 'related_to', 'entity', 'ent:Product:2046405728378673079', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3762011129750730370', 'src:go-module-graph', 'Status', 'the same ground truth `kg/gomod.boru` reads to build the knowledge graph''s module view', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3820981091835606929', 'ent:SoftwareModule:4361728672720029650', 'depends_on', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3820981091835606929', 'src:gomod:cmd-go', 'require block', 'github.com/boru-lang/boru/parser/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:3821304020166363356', 'ent:Document:1162242714758522750', 'supports', 'entity', 'ent:SoftwareModule:4559967244660037230', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3821304020166363356', 'src:basic-check-cut', 'Status', '**Status:** COMPLETE (2026-08-08)', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3940080421122549792', 'ent:Document:203047846460430642', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3940080421122549792', 'src:decl-grammar', 'title', 'DECLARATIVE-GRAMMAR.0 — one grammar artifact for both parser twins', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3957627250371362911', 'ent:Document:5215900749522722466', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3957627250371362911', 'src:go-module-graph', 'title', 'Go module graph and per-module coverage', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4109835522011898628', 'ent:Concept:3854395902791518463', 'has_attribute', 'literal', NULL, '"typed, word-based query language"', 'String', NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4109835522011898628', 'src:readme', 'intro', 'boru is a typed, word-based query language.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4129485658791420324', 'ent:Concept:2039420555596601682', 'part_of', 'entity', 'ent:Product:9122085017676103232', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4129485658791420324', 'src:readme', 'Overview', 'a secrets vault', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4193417245179262389', 'ent:SoftwareModule:425341189454841366', 'depends_on', 'entity', 'ent:SoftwareModule:6880687338933514154', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4193417245179262389', 'src:gomod:eng-go', 'require block', 'github.com/boru-lang/boru/compiler/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4235718707717393125', 'ent:SoftwareModule:4648368824093240216', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4235718707717393125', 'src:go-tree', 'editors/tree-sitter/bindings/go/go.mod', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4247002761012787571', 'ent:Document:4529681846923033548', 'supports', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4247002761012787571', 'src:module-views', 'Context', 'the renderer''s kind set is **closed**', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4299279342547359938', 'ent:Document:5175176782070740682', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4299279342547359938', 'src:boru-infoview', 'title', 'boru infoview — the stack at the cursor', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4367944145334365650', 'ent:SoftwareModule:4559967244660037230', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4367944145334365650', 'src:go-work', 'use block', './basic/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4503779949059601989', 'ent:SoftwareModule:425341189454841366', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/eng/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4503779949059601989', 'src:gomod:eng-go', 'module directive', 'module github.com/boru-lang/boru/eng/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4539802379148808939', 'ent:Document:3080274854606714513', 'mentions', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4539802379148808939', 'src:readme', 'Documentation', 'Architecture Design Record | You want the key architectural decisions and the reasoning behind them.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:459410346719323347', 'ent:Document:520435226487613788', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:459410346719323347', 'src:readme', 'Repository layout', 'Internal design notes and proposals.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4605160037939659790', 'ent:Document:4880036076125012648', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4605160037939659790', 'src:hot-code-loading', 'title', 'A report on boru''s **hot code loading** ability', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4649598326942754342', 'ent:SoftwareModule:425341189454841366', 'depends_on', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4649598326942754342', 'src:gomod:eng-go', 'require block', 'github.com/boru-lang/boru/parser/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4722992453068743762', 'ent:SoftwareModule:6880687338933514154', 'depends_on', 'entity', 'ent:SoftwareModule:559301050642427014', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4722992453068743762', 'src:gomod:compiler-go', 'require block', 'github.com/boru-lang/boru/check/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4765339225869748057', 'ent:SoftwareModule:4386785925506277682', 'depends_on', 'entity', 'ent:SoftwareModule:559301050642427014', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4765339225869748057', 'src:gomod:test-specfix', 'require block', 'github.com/boru-lang/boru/check/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4820340354877574923', 'ent:SoftwareModule:425341189454841366', 'depends_on', 'entity', 'ent:SoftwareModule:4386785925506277682', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4820340354877574923', 'src:gomod:eng-go', 'require block', 'github.com/boru-lang/boru/test/specfix v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4877857793539633559', 'ent:SoftwareModule:4361728672720029650', 'depends_on', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4877857793539633559', 'src:gomod:cmd-go', 'require block', 'github.com/boru-lang/boru/lang/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4898603144641696747', 'ent:Document:1913611373576952100', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4898603144641696747', 'src:state-machines', 'title', 'Design for general-purpose **state machines** in boru — a builtin', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4932254227310747047', 'ent:SoftwareModule:6880687338933514154', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/compiler/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4932254227310747047', 'src:gomod:compiler-go', 'module directive', 'module github.com/boru-lang/boru/compiler/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:4950098273738144333', 'ent:SoftwareModule:4598127138166596702', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/test/solardemo"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4950098273738144333', 'src:gomod:test-solardemo', 'module directive', 'module github.com/boru-lang/boru/test/solardemo', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5127918679616522004', 'ent:Document:8799605016341004740', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5127918679616522004', 'src:ts-parity-audit', 'title', 'TS-PARITY-AUDIT.0 — the parser battery does not agree with parser/go', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5214003688923155039', 'ent:SoftwareModule:6706536563979604982', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5214003688923155039', 'src:go-work', 'use block', './parser/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5245330609796646290', 'ent:SoftwareModule:425341189454841366', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5245330609796646290', 'src:go-work', 'use block', './eng/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5256016689838784468', 'ent:Document:1913611373576952100', 'supports', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5256016689838784468', 'src:state-machines', 'Scope decisions (agreed)', 'State machines ship as the `boru:state` module', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:534808747601329006', 'ent:Document:6369673620858945660', 'supports', 'entity', 'ent:SoftwareModule:425341189454841366', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:534808747601329006', 'src:agents', 'Working in the code', 'Engine kernel — types, values, signatures, matching, the step loop, the parser bridge | eng/go/CLAUDE.md', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:535388861626259779', 'ent:Document:8799605016341004740', 'supports', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:535388861626259779', 'src:ts-parity-audit', 'the differential', 'After the fixes the differential is **0 divergences across 1,765 rows**.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5359122693730754691', 'ent:SoftwareModule:6519065918608370232', 'part_of', 'entity', 'ent:SoftwareModule:4192460694199531608', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5359122693730754691', 'src:go-tree', 'wpg/wasm', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5532600402363181981', 'ent:Document:7770110494347118706', 'supports', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5532600402363181981', 'src:agents', 'Working in the code', 'Language layer — native words, modules, registry, help/describe, capabilities | lang/go/CLAUDE.md', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5555847556064706927', 'ent:SoftwareModule:4192460694199531608', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5555847556064706927', 'src:go-work', 'use block', './wpg', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5569270398049625658', 'ent:Product:8635404738244704660', 'related_to', 'entity', 'ent:Product:4976123614970806523', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5569270398049625658', 'src:readme', 'Repository layout', 'utils/ | A coreutils subset written in boru (cat, wc, head, grep, …) — real programs, built with boru build.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5589670101945647091', 'ent:SoftwareModule:4648368824093240216', 'has_attribute', 'literal', NULL, '"github.com/tree-sitter/tree-sitter-boru"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5589670101945647091', 'src:gomod:editors-tree-sitter-bindings-go', 'module directive', 'module github.com/tree-sitter/tree-sitter-boru', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5661573044671554336', 'ent:SoftwareModule:425341189454841366', 'depends_on', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5661573044671554336', 'src:gomod:eng-go', 'require block', 'github.com/boru-lang/boru/core/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:569417614246256590', 'ent:SoftwareModule:5172256887243902980', 'part_of', 'entity', 'ent:SoftwareModule:5138375578915662736', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:569417614246256590', 'src:go-tree', 'test/go/vary', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5752370331192826008', 'ent:SoftwareModule:559301050642427014', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5752370331192826008', 'src:go-work', 'use block', './check/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5778588895625917877', 'ent:SoftwareModule:425341189454841366', 'depends_on', 'entity', 'ent:SoftwareModule:559301050642427014', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5778588895625917877', 'src:gomod:eng-go', 'require block', 'github.com/boru-lang/boru/check/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5799645053343114985', 'ent:SoftwareModule:5919856352625365594', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/calc/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5799645053343114985', 'src:gomod:calc-go', 'module directive', 'module github.com/boru-lang/boru/calc/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:5861093039722048761', 'ent:Document:2574876380285708892', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5861093039722048761', 'src:root-module', 'title', 'ROOT-MODULE-FEASIBILITY.0 — measuring a shared module below core and parser', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5872086631131530189', 'ent:SoftwareModule:2013670336276694550', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5872086631131530189', 'src:go-work', 'use block', './core/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:6024405707462587556', 'ent:Document:481508614007064969', 'supports', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6024405707462587556', 'src:readme', 'Documentation', 'Reference | You need the precise behaviour of a syntax form, type, or word.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:6112159793123999904', 'ent:Document:4529681846923033548', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6112159793123999904', 'src:module-views', 'title', 'Design for **module-provided views and widgets** in boru', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:6197320912878839009', 'ent:Document:3534903004749141856', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6197320912878839009', 'src:content-addressing', 'title', 'CONTENT-ADDRESSING.0 — identity by hash, and what it would actually take', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:6294168041362264130', 'ent:SoftwareModule:2013670336276694550', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/core/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6294168041362264130', 'src:gomod:core-go', 'module directive', 'module github.com/boru-lang/boru/core/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:6299681106539820881', 'ent:Document:6186742803977787158', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6299681106539820881', 'src:boru-scry', 'title', 'boru:scry — a boru system''s knowledge of itself, as plain data', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:6506454126246230063', 'ent:SoftwareModule:559301050642427014', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/check/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6506454126246230063', 'src:gomod:check-go', 'module directive', 'module github.com/boru-lang/boru/check/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:6658327612052881629', 'ent:SoftwareModule:8275629451197117420', 'depends_on', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6658327612052881629', 'src:gomod:lang-go', 'require block', 'github.com/boru-lang/boru/core/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:668078731239663174', 'ent:Document:7594380001231677524', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:668078731239663174', 'src:function-value-scope', 'title', 'Where a function value''s **free words** resolve', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:6783118704822344398', 'ent:SoftwareModule:4386785925506277682', 'depends_on', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6783118704822344398', 'src:gomod:test-specfix', 'require block', 'github.com/boru-lang/boru/parser/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:6799788586482649785', 'ent:Document:4790579719562719716', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6799788586482649785', 'src:core-ts-coverage', 'title', 'CORE-TS-COVERAGE.0 — taking core/ts from 62% to 100%', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:6860032272018226827', 'ent:Document:5175176782070740682', 'supports', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6860032272018226827', 'src:boru-infoview', 'Architecture — what is new', '`lang/go/checktrace.go` — the `CheckTrace` position→stack oracle', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:6939544502052218836', 'ent:SoftwareModule:4386785925506277682', 'depends_on', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6939544502052218836', 'src:gomod:test-specfix', 'require block', 'github.com/boru-lang/boru/core/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:6971877089875160938', 'ent:SoftwareModule:559301050642427014', 'depends_on', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6971877089875160938', 'src:gomod:check-go', 'require block', 'github.com/boru-lang/boru/core/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:699504737122703801', 'ent:Document:4880036076125012648', 'supports', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:699504737122703801', 'src:hot-code-loading', '2.3', 'Re-import **is** reload, today, for', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7011485005147552963', 'ent:SoftwareModule:4192460694199531608', 'supports', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7011485005147552963', 'src:readme', 'Install', 'A wasm-powered browser playground is bundled in docs/index.html', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7019101754043141304', 'ent:Document:2308799538575712501', 'supports', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7019101754043141304', 'src:core-ts-divergences', 'Why none of these were visible', 'An uncovered branch in one port is where a divergence hides', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7044130325114909254', 'ent:SoftwareModule:4192460694199531608', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/wpg"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7044130325114909254', 'src:gomod:wpg', 'module directive', 'module github.com/boru-lang/boru/wpg', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:7207675757765927631', 'ent:Document:3521225411209893772', 'supports', 'entity', 'ent:SoftwareModule:425341189454841366', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7207675757765927631', 'src:audit', '7A', 'the parser emits kernel STRUCTURAL MARKERS; the engine lowers each marker to word dispatches', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7212088808110577326', 'ent:SoftwareModule:8275629451197117420', 'depends_on', 'entity', 'ent:SoftwareModule:425341189454841366', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7212088808110577326', 'src:gomod:lang-go', 'require block', 'github.com/boru-lang/boru/eng/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:7294700582288177136', 'ent:Product:4032424380612892464', 'created_by', 'entity', 'ent:Organization:8832543031059216933', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7294700582288177136', 'src:readme', 'Install', 'git clone https://github.com/boru-lang/boru', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7447279695537175736', 'ent:Product:2046405728378673079', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7447279695537175736', 'src:agents', 'Repository layout', 'kg/ | The project knowledge graph: an evidence-backed boru pipeline and its generated bundle.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:747512178358509988', 'ent:SoftwareModule:5919856352625365594', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:747512178358509988', 'src:go-work', 'use block', './calc/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:758093515468550172', 'ent:Product:8635404738244704660', 'related_to', 'entity', 'ent:Product:9122085017676103232', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:758093515468550172', 'src:readme', 'Repository layout', 'real programs, built with boru build', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7616275222305495199', 'ent:SoftwareModule:8650101979302333988', 'part_of', 'entity', 'ent:SoftwareModule:4361728672720029650', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7616275222305495199', 'src:go-tree', 'cmd/go/boru', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:7661116843982840438', 'ent:Product:5770789618934008141', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7661116843982840438', 'src:readme', 'Repository layout', 'lang/spec/ | Engine spec TSV files (the language''s executable spec).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:767395016779850813', 'ent:Document:7594380001231677524', 'supports', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:767395016779850813', 'src:function-value-scope', '1. Summary', 'The mechanism for the correct behaviour already exists and is already', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7825661200694944199', 'ent:SoftwareModule:4361728672720029650', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7825661200694944199', 'src:go-work', 'use block', './cmd/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:7828550185343127', 'ent:Document:4163489813681141089', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7828550185343127', 'src:agents', 'header', 'New to the repo? Skim README.md first.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7875690729105221344', 'ent:Concept:3854395902791518463', 'related_to', 'entity', 'ent:Concept:4841193570246608846', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7875690729105221344', 'src:readme', 'intro', 'words can equally take their arguments from a value stack, which is what makes point-free pipelines compose', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7916954283464366890', 'ent:SoftwareModule:4559967244660037230', 'depends_on', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7916954283464366890', 'src:gomod:basic-go', 'require block', 'github.com/boru-lang/boru/core/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8143463681370129906', 'ent:SoftwareModule:4192460694199531608', 'depends_on', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8143463681370129906', 'src:gomod:wpg', 'require block', 'github.com/boru-lang/boru/lang/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8189213360333637896', 'ent:Product:4976123614970806523', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8189213360333637896', 'src:agents', 'Repository layout', 'utils/ | A coreutils subset written in boru — real programs that prove the CLI story (argv, exit codes, streams, baked permissions).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8296105166081105589', 'ent:SoftwareModule:4386785925506277682', 'depends_on', 'entity', 'ent:SoftwareModule:6880687338933514154', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8296105166081105589', 'src:gomod:test-specfix', 'require block', 'github.com/boru-lang/boru/compiler/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8305882503258666818', 'ent:SoftwareModule:4361728672720029650', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/cmd/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8305882503258666818', 'src:gomod:cmd-go', 'module directive', 'module github.com/boru-lang/boru/cmd/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8334090252473743925', 'ent:Document:2308799538575712501', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8334090252473743925', 'src:core-ts-divergences', 'title', 'CORE-TS-DIVERGENCES.1 — 135 measured core-level divergences, and where they hid', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8360168369078766404', 'ent:Document:8875579216819453768', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8360168369078766404', 'src:go-ts-parity', 'title', 'GO-TS-PARITY.0 — full functional parity on core, parser and basic', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8427787091950146539', 'ent:SoftwareModule:4116347247488215916', 'part_of', 'entity', 'ent:SoftwareModule:5138375578915662736', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8427787091950146539', 'src:go-tree', 'test/go/covergate', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8559088752918568803', 'ent:SoftwareModule:5138375578915662736', 'depends_on', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8559088752918568803', 'src:gomod:test-go', 'require block', 'github.com/boru-lang/boru/lang/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:859379005577864411', 'ent:SoftwareModule:5138375578915662736', 'depends_on', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:859379005577864411', 'src:gomod:test-go', 'require block', 'github.com/boru-lang/boru/core/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8606476400104716036', 'ent:Document:5313783338663858074', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8606476400104716036', 'src:reload-invalidation', 'title', 'How **hot code reloading** and **transparent compilation** coexist without', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8638473926594381359', 'ent:SoftwareModule:1633412495372444656', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8638473926594381359', 'src:go-tree', 'lang/go/modules', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8686166953339451140', 'ent:SoftwareModule:8281032769668059040', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8686166953339451140', 'src:go-tree', 'lang/go/policy', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8731733153397530894', 'ent:SoftwareModule:4598127138166596702', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8731733153397530894', 'src:go-work', 'use block', './test/solardemo', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8820902757097961904', 'ent:SoftwareModule:3641482396164462740', 'part_of', 'entity', 'ent:SoftwareModule:5138375578915662736', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8820902757097961904', 'src:go-tree', 'test/go/docexamples', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8905043332608038803', 'ent:SoftwareModule:927371487649425292', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8905043332608038803', 'src:go-tree', 'lang/go/debugserve', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8927535347315204632', 'ent:Concept:6094411313845087998', 'part_of', 'entity', 'ent:Concept:2039420555596601682', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8927535347315204632', 'src:cli-md', 'The vault', 'HTTP wire protocol for secret provision', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8933845283517661039', 'ent:SoftwareModule:8376380866775607244', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8933845283517661039', 'src:go-tree', 'lang/go/formatter', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8946871445019255259', 'ent:Document:8751021793288559660', 'supports', 'entity', 'ent:SoftwareModule:2013670336276694550', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8946871445019255259', 'src:canon-roundtrip', 'The contract', '`canon v` renders a value as boru source text which, re-parsed,', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8955705768046061429', 'ent:Document:203047846460430642', 'supports', 'entity', 'ent:SoftwareModule:6706536563979604982', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8955705768046061429', 'src:decl-grammar', 'The contract', 'parser/go/grammar.json is the single source of the boru grammar''s STRUCTURE, loaded by both parsers', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:897358123486782210', 'ent:Document:3294415633888265368', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:897358123486782210', 'src:completeness-review', 'title', 'Type checker + bytecode compiler — completeness review', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8982543179606202117', 'ent:SoftwareModule:8275629451197117420', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8982543179606202117', 'src:go-work', 'use block', './lang/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:8989313594377163134', 'ent:SoftwareModule:2224184413468547100', 'part_of', 'entity', 'ent:SoftwareModule:8275629451197117420', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8989313594377163134', 'src:go-tree', 'lang/go/test', NULL, 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:9059850742711593788', 'ent:SoftwareModule:6706536563979604982', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/parser/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:9059850742711593788', 'src:gomod:parser-go', 'module directive', 'module github.com/boru-lang/boru/parser/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:9138469699059997665', 'ent:SoftwareModule:4559967244660037230', 'has_attribute', 'literal', NULL, '"github.com/boru-lang/boru/basic/go"', 'String', 'go-module-path', NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:9138469699059997665', 'src:gomod:basic-go', 'module directive', 'module github.com/boru-lang/boru/basic/go', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:914658451831503592', 'ent:Document:6176355086953937469', 'supports', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:914658451831503592', 'src:readme', 'Documentation', 'Tutorial | You are new to boru and want to learn it step by step.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:965013337224509220', 'ent:SoftwareModule:8275629451197117420', 'depends_on', 'entity', 'ent:SoftwareModule:559301050642427014', NULL, NULL, NULL, NULL, 1, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:965013337224509220', 'src:gomod:lang-go', 'require block', 'github.com/boru-lang/boru/check/go v0.0.0', 'rule', 'kg-gomod');
INSERT INTO assertions VALUES ('ast:999583482530779094', 'ent:Document:5105101056062860425', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-08-07T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:999583482530779094', 'src:coverage-parity', 'title', 'ENG-COVERAGE-PARITY.0 — the standalone 100%/100% program for the kernel twins', 'direct_record', 'kg-ingest');
COMMIT;