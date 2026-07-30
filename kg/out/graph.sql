BEGIN TRANSACTION;
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
INSERT INTO sources VALUES ('src:agents', 'text', 'AGENTS.md', 'AGENTS.md agent guide', NULL, 'agents-2026-07', 'primary', '{
  "repository": "aql-lang/aql"
}');
INSERT INTO sources VALUES ('src:readme', 'text', 'README.md', 'AQL README', NULL, 'readme-2026-07', 'primary', '{
  "repository": "aql-lang/aql"
}');
INSERT INTO entities VALUES ('ent:Concept:2039420555596601682', 'Concept', 'secrets vault', 'secrets vault', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Concept:2039420555596601682', 'role', 'encrypted API-key / token store, driven by the aql vault subcommand');
INSERT INTO entities VALUES ('ent:Concept:4587555710592773395', 'Concept', 'Forward arguments', 'forward arguments', 'accepted');
INSERT INTO entities VALUES ('ent:Concept:4841193570246608846', 'Concept', 'Value stack', 'value stack', 'accepted');
INSERT INTO entities VALUES ('ent:Concept:6434995827834555649', 'Concept', 'aql describe discovery', 'aql describe discovery', 'accepted');
INSERT INTO entities VALUES ('ent:Concept:7376417356888575267', 'Concept', 'Executable language spec', 'executable language spec', 'accepted');
INSERT INTO entities VALUES ('ent:Concept:8101927001878214763', 'Concept', 'AQL language', 'aql language', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Concept:8101927001878214763', 'paradigm', 'concatenative, typed, word-based');
INSERT INTO entities VALUES ('ent:Document:1344160336771235777', 'Document', 'EXPLANATION.md', 'explanation.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:3080274854606714513', 'Document', 'ADR.md', 'adr.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:3904106568037504161', 'Document', 'AGENTS.md', 'agents.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:3955423872539901697', 'Document', 'HOWTO.md', 'howto.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:4163489813681141089', 'Document', 'README.md', 'readme.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:481508614007064969', 'Document', 'REFERENCE.md', 'reference.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:4990200910103175455', 'Document', 'CLI.md', 'cli.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:520435226487613788', 'Document', 'design/ notes', 'design/ notes', 'accepted');
INSERT INTO entities VALUES ('ent:Document:5292060467150439417', 'Document', 'NUR.md', 'nur.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:6176355086953937469', 'Document', 'TUTORIAL.md', 'tutorial.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:6369673620858945660', 'Document', 'eng/go/CLAUDE.md', 'eng/go/claude.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:7770110494347118706', 'Document', 'lang/go/CLAUDE.md', 'lang/go/claude.md', 'accepted');
INSERT INTO entities VALUES ('ent:Organization:1921695231834521033', 'Organization', 'aql-lang', 'aql-lang', 'accepted');
INSERT INTO entities VALUES ('ent:Product:1225291689777530860', 'Product', 'aql binary', 'aql binary', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:1225291689777530860', 'role', 'CLI, REPL, type checker, formatter, LSP, registry client, vault, supervisor');
INSERT INTO entities VALUES ('ent:Product:1578575807542004434', 'Product', 'eng/go module', 'eng/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:1578575807542004434', 'path', 'eng/go');
INSERT INTO entities VALUES ('ent:Product:1628071779061296422', 'Product', 'calc/go module', 'calc/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:1628071779061296422', 'path', 'calc/go');
INSERT INTO entities VALUES ('ent:Product:2046405728378673079', 'Product', 'kg knowledge-graph pipeline', 'kg knowledge-graph pipeline', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:2046405728378673079', 'path', 'kg');
INSERT INTO entities VALUES ('ent:Product:3515219182955540947', 'Product', 'aql:cli module', 'aql:cli module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:3515219182955540947', 'language', 'AQL');
INSERT INTO entity_attributes VALUES ('ent:Product:3515219182955540947', 'path', 'lang/go/modules/cli.aql');
INSERT INTO entities VALUES ('ent:Product:5397757900009317848', 'Product', 'lang/go module', 'lang/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:5397757900009317848', 'path', 'lang/go');
INSERT INTO entities VALUES ('ent:Product:5770789618934008141', 'Product', 'lang/spec directory', 'lang/spec directory', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:5770789618934008141', 'path', 'lang/spec');
INSERT INTO entities VALUES ('ent:Product:6289010343240002096', 'Product', 'aql-lang/aql repository', 'aql-lang/aql repository', 'accepted');
INSERT INTO entity_external_ids VALUES ('ent:Product:6289010343240002096', 'github', 'aql-lang/aql');
INSERT INTO entities VALUES ('ent:Product:6926990046092332766', 'Product', 'cmd/go module', 'cmd/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:6926990046092332766', 'path', 'cmd/go');
INSERT INTO entities VALUES ('ent:Product:8635404738244704660', 'Product', 'utils coreutils subset', 'utils coreutils subset', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:8635404738244704660', 'path', 'utils');
INSERT INTO entities VALUES ('ent:Product:9152211458201666591', 'Product', 'wpg playground module', 'wpg playground module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:9152211458201666591', 'path', 'wpg');
INSERT INTO assertions VALUES ('ast:1051514145981152920', 'ent:Document:4163489813681141089', 'part_of', 'entity', 'ent:Product:6289010343240002096', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1051514145981152920', 'src:agents', 'header', 'New to the repo? Skim README.md first.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1171165446568948711', 'ent:Product:2046405728378673079', 'part_of', 'entity', 'ent:Product:6289010343240002096', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1171165446568948711', 'src:agents', 'Repository layout', 'kg/ | The project knowledge graph: an evidence-backed AQL pipeline and its generated bundle.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1203342690817164800', 'ent:Concept:8101927001878214763', 'related_to', 'entity', 'ent:Concept:4587555710592773395', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1203342690817164800', 'src:readme', 'Forward arguments', 'the defining feature of the surface syntax is forward arguments', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1479893618357868202', 'ent:Product:6289010343240002096', 'created_by', 'entity', 'ent:Organization:1921695231834521033', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1479893618357868202', 'src:readme', 'Install', 'git clone https://github.com/aql-lang/aql', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1850760637092848881', 'ent:Product:9152211458201666591', 'supports', 'entity', 'ent:Concept:8101927001878214763', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1850760637092848881', 'src:readme', 'Install', 'A wasm-powered browser playground is bundled in docs/index.html', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1970337823730676342', 'ent:Document:7770110494347118706', 'supports', 'entity', 'ent:Product:5397757900009317848', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1970337823730676342', 'src:agents', 'Working in the code', 'Language layer — native words, modules, registry, help/describe, capabilities | lang/go/CLAUDE.md', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2046274359691114498', 'ent:Product:6926990046092332766', 'part_of', 'entity', 'ent:Product:6289010343240002096', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2046274359691114498', 'src:readme', 'Repository layout', 'cmd/go/ | The aql CLI / REPL (github.com/aql-lang/aql/cmd/go).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2249958554827628895', 'ent:Concept:8101927001878214763', 'related_to', 'entity', 'ent:Concept:4841193570246608846', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2249958554827628895', 'src:readme', 'intro', 'words can equally take their arguments from a value stack, which is what makes point-free pipelines compose', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2451444146340209174', 'ent:Document:4990200910103175455', 'supports', 'entity', 'ent:Product:1225291689777530860', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2451444146340209174', 'src:readme', 'Documentation', 'CLI Reference | You want to drive the aql binary from the shell.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2510153559955756366', 'ent:Concept:7376417356888575267', 'part_of', 'entity', 'ent:Product:5770789618934008141', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2510153559955756366', 'src:agents', 'Task router', 'The executable language spec (the rows tests run against) | lang/spec/*.tsv', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2985288027710857565', 'ent:Product:5770789618934008141', 'part_of', 'entity', 'ent:Product:6289010343240002096', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2985288027710857565', 'src:readme', 'Repository layout', 'lang/spec/ | Engine spec TSV files (the language''s executable spec).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3068916487690115997', 'ent:Document:5292060467150439417', 'mentions', 'entity', 'ent:Concept:8101927001878214763', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3068916487690115997', 'src:readme', 'Documentation', 'Non-Uniformity Register | You want the recorded deviations from the language''s uniform rules, each pending, resolved, or explicitly allowed.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3771181527847705674', 'ent:Document:1344160336771235777', 'supports', 'entity', 'ent:Concept:8101927001878214763', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3771181527847705674', 'src:readme', 'Documentation', 'Explanation | You want to understand why AQL is the way it is.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:426160728788970211', 'ent:Product:3515219182955540947', 'part_of', 'entity', 'ent:Product:5397757900009317848', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:426160728788970211', 'src:agents', 'Repository layout', 'utils/ | A coreutils subset written in AQL — real programs that prove the CLI story (argv, exit codes, streams, baked permissions).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4324334117755500109', 'ent:Product:1628071779061296422', 'part_of', 'entity', 'ent:Product:6289010343240002096', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4324334117755500109', 'src:readme', 'Repository layout', 'calc/go/ | A small calculator built directly on eng (learning example).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4471816819522755262', 'ent:Document:3080274854606714513', 'mentions', 'entity', 'ent:Concept:8101927001878214763', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4471816819522755262', 'src:readme', 'Documentation', 'Architecture Design Record | You want the key architectural decisions and the reasoning behind them.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5275633100085741168', 'ent:Product:1225291689777530860', 'supports', 'entity', 'ent:Concept:6434995827834555649', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5275633100085741168', 'src:agents', 'First: let the tool document itself', 'The aql CLI documents both the language and itself, and that output is generated from the live engine', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5406505729946615323', 'ent:Document:6369673620858945660', 'supports', 'entity', 'ent:Product:1578575807542004434', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5406505729946615323', 'src:agents', 'Working in the code', 'Engine kernel — types, values, signatures, matching, the step loop, the parser bridge | eng/go/CLAUDE.md', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5635609475312982017', 'ent:Document:6176355086953937469', 'supports', 'entity', 'ent:Concept:8101927001878214763', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5635609475312982017', 'src:readme', 'Documentation', 'Tutorial | You are new to AQL and want to learn it step by step.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5654774896833246723', 'ent:Product:8635404738244704660', 'related_to', 'entity', 'ent:Product:1225291689777530860', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5654774896833246723', 'src:readme', 'Repository layout', 'real programs, built with aql build', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:6158382806169937646', 'ent:Product:8635404738244704660', 'part_of', 'entity', 'ent:Product:6289010343240002096', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6158382806169937646', 'src:agents', 'Repository layout', 'utils/ | A coreutils subset written in AQL — real programs that prove the CLI story (argv, exit codes, streams, baked permissions).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:6238259023438491688', 'ent:Document:3955423872539901697', 'supports', 'entity', 'ent:Concept:8101927001878214763', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6238259023438491688', 'src:readme', 'Documentation', 'How-To Guides | You have a specific task and want a recipe.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:6774669354954439029', 'ent:Concept:8101927001878214763', 'has_attribute', 'literal', NULL, '"typed, word-based query language"', 'String', NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6774669354954439029', 'src:readme', 'intro', 'AQL is a typed, word-based query language.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7033649832942744685', 'ent:Document:481508614007064969', 'supports', 'entity', 'ent:Concept:8101927001878214763', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7033649832942744685', 'src:readme', 'Documentation', 'Reference | You need the precise behaviour of a syntax form, type, or word.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7078456856804224733', 'ent:Product:1225291689777530860', 'part_of', 'entity', 'ent:Product:6926990046092332766', NULL, NULL, NULL, NULL, 0.97, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7078456856804224733', 'src:readme', 'Repository layout', 'cmd/go/ | The aql CLI / REPL', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7771136335190801661', 'ent:Product:9152211458201666591', 'part_of', 'entity', 'ent:Product:6289010343240002096', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7771136335190801661', 'src:readme', 'Repository layout', 'wpg/ | The wasm web playground (wpg/wasm + wpg/serve).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7834498903675958327', 'ent:Product:5397757900009317848', 'part_of', 'entity', 'ent:Product:6289010343240002096', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7834498903675958327', 'src:readme', 'Repository layout', 'lang/go/ | The language layer: public lang API and the consolidated native word library.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7961542002030072644', 'ent:Document:3904106568037504161', 'mentions', 'entity', 'ent:Concept:6434995827834555649', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7961542002030072644', 'src:agents', 'First: let the tool document itself', 'Before grepping source or guessing a word''s signature, ask the binary.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8318307539480741368', 'ent:Product:8635404738244704660', 'related_to', 'entity', 'ent:Product:3515219182955540947', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8318307539480741368', 'src:readme', 'Repository layout', 'utils/ | A coreutils subset written in AQL (cat, wc, head, grep, ...) — real programs, built with aql build.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8361181589655620459', 'ent:Concept:2039420555596601682', 'part_of', 'entity', 'ent:Product:1225291689777530860', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8361181589655620459', 'src:readme', 'Overview', 'a secrets vault', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8953029635026218565', 'ent:Concept:8101927001878214763', 'has_attribute', 'literal', NULL, '"concatenative"', 'String', NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8953029635026218565', 'src:readme', 'intro', 'Underneath, AQL is concatenative: words can equally take their arguments from a value stack', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8992180734200862056', 'ent:Document:520435226487613788', 'part_of', 'entity', 'ent:Product:6289010343240002096', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8992180734200862056', 'src:readme', 'Repository layout', 'Internal design notes and proposals.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:9025109992695699694', 'ent:Product:1578575807542004434', 'part_of', 'entity', 'ent:Product:6289010343240002096', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:9025109992695699694', 'src:readme', 'Repository layout', 'eng/go/ | Engine kernel, parser, and kernel spec runner.', 'direct_record', 'kg-ingest');
INSERT INTO schema_proposals VALUES ('schema-proposal:7752279147991378124', 'entity_type', 'SoftwareModule', 'A buildable code module or package within a software repository.', 'Repository modules are currently typed Product, which blurs shipped artifacts and source-tree modules.', 'requires_approval', '[
  "SoftwareModule"
]', '[
  "Product",
  "Concept"
]', '[
  "cmd/go",
  "lang/go",
  "eng/go"
]', '[
  "Product"
]');
COMMIT;