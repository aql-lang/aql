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
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:audit', 'text', 'design/LANG-ENG-CONTENT-AUDIT.0.md', 'lang/eng content audit — ADR-012 evidence', NULL, 'audit-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:cli-md', 'text', 'CLI.md', 'CLI.md subcommand reference', NULL, 'cli-md-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:completeness-review', 'text', 'design/checker-compiler-completeness-review.0.md', 'Type checker + bytecode compiler — completeness review', NULL, 'completeness-review-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:coverage-parity', 'text', 'design/ENG-COVERAGE-PARITY.0.md', 'eng standalone coverage-parity program', NULL, 'coverage-parity-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:decl-grammar', 'text', 'design/DECLARATIVE-GRAMMAR.0.md', 'declarative grammar artifact for both parser twins', NULL, 'decl-grammar-2026-08', 'primary', '{
  "repository": "boru-lang/boru"
}');
INSERT INTO sources VALUES ('src:readme', 'text', 'README.md', 'boru README', NULL, 'readme-2026-07', 'primary', '{
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
INSERT INTO entities VALUES ('ent:Document:1344160336771235777', 'Document', 'EXPLANATION.md', 'explanation.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:203047846460430642', 'Document', 'design/DECLARATIVE-GRAMMAR.0.md', 'design/declarative-grammar.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:203047846460430642', 'role', 'the shared declarative tabnas grammar artifact (parser/go/grammar.json): contract, loader pair, and the batch-migration state');
INSERT INTO entities VALUES ('ent:Document:3080274854606714513', 'Document', 'ADR.md', 'adr.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:3294415633888265368', 'Document', 'design/checker-compiler-completeness-review.0.md', 'design/checker-compiler-completeness-review.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:3294415633888265368', 'role', 'the 2026-08 checker/compiler completeness review and its §9 implementation record — the living index of the HOF-compilation graduations and the remaining frontier work');
INSERT INTO entities VALUES ('ent:Document:3521225411209893772', 'Document', 'design/LANG-ENG-CONTENT-AUDIT.0.md', 'design/lang-eng-content-audit.0.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:3904106568037504161', 'Document', 'AGENTS.md', 'agents.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:3955423872539901697', 'Document', 'HOWTO.md', 'howto.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:4163489813681141089', 'Document', 'README.md', 'readme.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:481508614007064969', 'Document', 'REFERENCE.md', 'reference.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:4990200910103175455', 'Document', 'CLI.md', 'cli.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:5105101056062860425', 'Document', 'design/ENG-COVERAGE-PARITY.0.md', 'design/eng-coverage-parity.0.md', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Document:5105101056062860425', 'role', 'the standalone 100%/100% coverage program for eng/go and eng/ts: the ratcheting gate floors (make cover-gate-eng, make test-ts), the gap inventories, and the staged plans');
INSERT INTO entities VALUES ('ent:Document:520435226487613788', 'Document', 'design/ notes', 'design/ notes', 'accepted');
INSERT INTO entities VALUES ('ent:Document:5292060467150439417', 'Document', 'NUR.md', 'nur.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:6176355086953937469', 'Document', 'TUTORIAL.md', 'tutorial.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:6369673620858945660', 'Document', 'eng/go/CLAUDE.md', 'eng/go/claude.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:7770110494347118706', 'Document', 'lang/go/CLAUDE.md', 'lang/go/claude.md', 'accepted');
INSERT INTO entities VALUES ('ent:Document:9071667555031388966', 'Document', 'parser/go/CLAUDE.md', 'parser/go/claude.md', 'accepted');
INSERT INTO entities VALUES ('ent:Organization:8832543031059216933', 'Organization', 'boru-lang', 'boru-lang', 'accepted');
INSERT INTO entities VALUES ('ent:Product:1578575807542004434', 'Product', 'eng/go module', 'eng/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:1578575807542004434', 'path', 'eng/go');
INSERT INTO entities VALUES ('ent:Product:1628071779061296422', 'Product', 'calc/go module', 'calc/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:1628071779061296422', 'path', 'calc/go');
INSERT INTO entities VALUES ('ent:Product:2046405728378673079', 'Product', 'kg knowledge-graph pipeline', 'kg knowledge-graph pipeline', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:2046405728378673079', 'path', 'kg');
INSERT INTO entities VALUES ('ent:Product:247893255613018706', 'Product', 'parser/go module', 'parser/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:247893255613018706', 'path', 'parser/go');
INSERT INTO entities VALUES ('ent:Product:4032424380612892464', 'Product', 'boru-lang/boru repository', 'boru-lang/boru repository', 'accepted');
INSERT INTO entity_external_ids VALUES ('ent:Product:4032424380612892464', 'github', 'boru-lang/boru');
INSERT INTO entities VALUES ('ent:Product:4938585304473715098', 'Product', 'basic/go module', 'basic/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:4938585304473715098', 'path', 'basic/go');
INSERT INTO entities VALUES ('ent:Product:4976123614970806523', 'Product', 'boru:cli module', 'boru:cli module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:4976123614970806523', 'language', 'boru');
INSERT INTO entity_attributes VALUES ('ent:Product:4976123614970806523', 'path', 'lang/go/modules/cli.boru');
INSERT INTO entities VALUES ('ent:Product:5136370952190450338', 'Product', 'core/go module', 'core/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:5136370952190450338', 'path', 'core/go');
INSERT INTO entities VALUES ('ent:Product:5397757900009317848', 'Product', 'lang/go module', 'lang/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:5397757900009317848', 'path', 'lang/go');
INSERT INTO entities VALUES ('ent:Product:5770789618934008141', 'Product', 'lang/spec directory', 'lang/spec directory', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:5770789618934008141', 'path', 'lang/spec');
INSERT INTO entities VALUES ('ent:Product:6022551412401652470', 'Product', 'compiler/go module', 'compiler/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:6022551412401652470', 'path', 'compiler/go');
INSERT INTO entities VALUES ('ent:Product:6509502233970515618', 'Product', 'check/go module', 'check/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:6509502233970515618', 'path', 'check/go');
INSERT INTO entities VALUES ('ent:Product:6926990046092332766', 'Product', 'cmd/go module', 'cmd/go module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:6926990046092332766', 'path', 'cmd/go');
INSERT INTO entities VALUES ('ent:Product:8635404738244704660', 'Product', 'utils coreutils subset', 'utils coreutils subset', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:8635404738244704660', 'path', 'utils');
INSERT INTO entities VALUES ('ent:Product:9122085017676103232', 'Product', 'boru binary', 'boru binary', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:9122085017676103232', 'role', 'CLI, REPL, type checker, formatter, LSP, registry client, vault, supervisor');
INSERT INTO entities VALUES ('ent:Product:9152211458201666591', 'Product', 'wpg playground module', 'wpg playground module', 'accepted');
INSERT INTO entity_attributes VALUES ('ent:Product:9152211458201666591', 'path', 'wpg');
INSERT INTO assertions VALUES ('ast:1270339351209541981', 'ent:Product:1578575807542004434', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1270339351209541981', 'src:readme', 'Repository layout', 'eng/go/ | Engine kernel, parser, and kernel spec runner.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1356352958644953400', 'ent:Product:5136370952190450338', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1356352958644953400', 'src:readme', 'Repository layout', 'core/go/ | The interpreter core module: values, types, matching, the step loop. Standalone (apd only); eng builds on it.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1374144917814367558', 'ent:Concept:3854395902791518463', 'has_attribute', 'literal', NULL, '"concatenative"', 'String', NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1374144917814367558', 'src:readme', 'intro', 'Underneath, boru is concatenative: words can equally take their arguments from a value stack', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1638267057731290625', 'ent:Document:203047846460430642', 'supports', 'entity', 'ent:Product:1578575807542004434', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1638267057731290625', 'src:decl-grammar', 'The contract', 'eng/go/parser/grammar.json is the single source of the boru grammar''s STRUCTURE, loaded by both parsers', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:1970337823730676342', 'ent:Document:7770110494347118706', 'supports', 'entity', 'ent:Product:5397757900009317848', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:1970337823730676342', 'src:agents', 'Working in the code', 'Language layer — native words, modules, registry, help/describe, capabilities | lang/go/CLAUDE.md', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2116615243097114079', 'ent:Product:247893255613018706', 'related_to', 'entity', 'ent:Product:5136370952190450338', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2116615243097114079', 'src:readme', 'Repository layout', 'parser/go/ | The parser module: boru source text to kernel values (tabnas/jsonic lexer plus the embedded declarative grammar). Depends on core alone.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2190303907985753377', 'ent:Document:3955423872539901697', 'supports', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2190303907985753377', 'src:readme', 'Documentation', 'How-To Guides | You have a specific task and want a recipe.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2308356135508106004', 'ent:Document:5292060467150439417', 'mentions', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2308356135508106004', 'src:readme', 'Documentation', 'Non-Uniformity Register | You want the recorded deviations from the language''s uniform rules, each pending, resolved, or explicitly allowed.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2313048313946518913', 'ent:Product:8635404738244704660', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2313048313946518913', 'src:agents', 'Repository layout', 'utils/ | A coreutils subset written in boru — real programs that prove the CLI story (argv, exit codes, streams, baked permissions).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2457753349338579625', 'ent:Document:4990200910103175455', 'supports', 'entity', 'ent:Product:9122085017676103232', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2457753349338579625', 'src:readme', 'Documentation', 'CLI Reference | You want to drive the boru binary from the shell.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2510153559955756366', 'ent:Concept:7376417356888575267', 'part_of', 'entity', 'ent:Product:5770789618934008141', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2510153559955756366', 'src:agents', 'Task router', 'The executable language spec (the rows tests run against) | lang/spec/*.tsv', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2650141132074711437', 'ent:Product:9122085017676103232', 'supports', 'entity', 'ent:Concept:5837115061456563631', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2650141132074711437', 'src:agents', 'First: let the tool document itself', 'The boru CLI documents both the language and itself, and that output is generated from the live engine', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2714957710081530623', 'ent:Document:3521225411209893772', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2714957710081530623', 'src:audit', 'title', 'lang/eng content audit', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:2964439567693798749', 'ent:Product:6509502233970515618', 'related_to', 'entity', 'ent:Product:5136370952190450338', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:2964439567693798749', 'src:readme', 'Repository layout', 'check/go/ | The type checker: the analysis pass over core. Depends on core only.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:303023300656590935', 'ent:Document:1344160336771235777', 'supports', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:303023300656590935', 'src:readme', 'Documentation', 'Explanation | You want to understand why boru is the way it is.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:311483063513570585', 'ent:Product:4976123614970806523', 'part_of', 'entity', 'ent:Product:5397757900009317848', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:311483063513570585', 'src:agents', 'Repository layout', 'utils/ | A coreutils subset written in boru — real programs that prove the CLI story (argv, exit codes, streams, baked permissions).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:314582934812456120', 'ent:Document:3904106568037504161', 'mentions', 'entity', 'ent:Concept:5837115061456563631', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:314582934812456120', 'src:agents', 'First: let the tool document itself', 'Before grepping source or guessing a word''s signature, ask the binary.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3211344478610580799', 'ent:Concept:3854395902791518463', 'related_to', 'entity', 'ent:Concept:4587555710592773395', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3211344478610580799', 'src:readme', 'Forward arguments', 'the defining feature of the surface syntax is forward arguments', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:3940080421122549792', 'ent:Document:203047846460430642', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:3940080421122549792', 'src:decl-grammar', 'title', 'DECLARATIVE-GRAMMAR.0 — one grammar artifact for both parser twins', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4109835522011898628', 'ent:Concept:3854395902791518463', 'has_attribute', 'literal', NULL, '"typed, word-based query language"', 'String', NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4109835522011898628', 'src:readme', 'intro', 'boru is a typed, word-based query language.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4129485658791420324', 'ent:Concept:2039420555596601682', 'part_of', 'entity', 'ent:Product:9122085017676103232', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4129485658791420324', 'src:readme', 'Overview', 'a secrets vault', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4135702956650890893', 'ent:Product:4938585304473715098', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4135702956650890893', 'src:readme', 'Repository layout', 'basic/go/ | The base language layer: fundamental words and predefined content types, on eng only (ADR-013).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4330711182908461459', 'ent:Product:6509502233970515618', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4330711182908461459', 'src:readme', 'Repository layout', 'check/go/ | The type checker: the analysis pass over core. Depends on core only.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:4539802379148808939', 'ent:Document:3080274854606714513', 'mentions', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:4539802379148808939', 'src:readme', 'Documentation', 'Architecture Design Record | You want the key architectural decisions and the reasoning behind them.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:459410346719323347', 'ent:Document:520435226487613788', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:459410346719323347', 'src:readme', 'Repository layout', 'Internal design notes and proposals.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5028088745462111124', 'ent:Product:5397757900009317848', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5028088745462111124', 'src:readme', 'Repository layout', 'lang/go/ | The language layer: public lang API and the consolidated native word library.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5158349308628574819', 'ent:Product:1578575807542004434', 'related_to', 'entity', 'ent:Product:5136370952190450338', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5158349308628574819', 'src:readme', 'Repository layout', 'core/go/ | The interpreter core module: values, types, matching, the step loop. Standalone (apd only); eng builds on it.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5247468906425855718', 'ent:Product:1628071779061296422', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5247468906425855718', 'src:readme', 'Repository layout', 'calc/go/ | A small calculator built directly on eng (learning example).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5406505729946615323', 'ent:Document:6369673620858945660', 'supports', 'entity', 'ent:Product:1578575807542004434', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5406505729946615323', 'src:agents', 'Working in the code', 'Engine kernel — types, values, signatures, matching, the step loop, the parser bridge | eng/go/CLAUDE.md', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5434947745222255640', 'ent:Product:9152211458201666591', 'supports', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5434947745222255640', 'src:readme', 'Install', 'A wasm-powered browser playground is bundled in docs/index.html', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5496270037575797833', 'ent:Product:247893255613018706', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5496270037575797833', 'src:readme', 'Repository layout', 'parser/go/ | The parser module: boru source text to kernel values (tabnas/jsonic lexer plus the embedded declarative grammar). Depends on core alone.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5569270398049625658', 'ent:Product:8635404738244704660', 'related_to', 'entity', 'ent:Product:4976123614970806523', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5569270398049625658', 'src:readme', 'Repository layout', 'utils/ | A coreutils subset written in boru (cat, wc, head, grep, ...) — real programs, built with boru build.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:5780911143793158294', 'ent:Product:9152211458201666591', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:5780911143793158294', 'src:readme', 'Repository layout', 'wpg/ | The wasm web playground (wpg/wasm + wpg/serve).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:6024405707462587556', 'ent:Document:481508614007064969', 'supports', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6024405707462587556', 'src:readme', 'Documentation', 'Reference | You need the precise behaviour of a syntax form, type, or word.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:6284570896860914192', 'ent:Product:6022551412401652470', 'related_to', 'entity', 'ent:Product:6509502233970515618', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6284570896860914192', 'src:readme', 'Repository layout', 'compiler/go/ | The compiler: the recorder, lowering, and bytecode emitter over check and core.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:6670089646672814849', 'ent:Product:5397757900009317848', 'related_to', 'entity', 'ent:Product:4938585304473715098', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:6670089646672814849', 'src:agents', 'Repository layout', 'basic/go/ | The base language layer: fundamental words (stack, definition, control, type-generics) + predefined content types. Depends on eng only (ADR-013).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7294700582288177136', 'ent:Product:4032424380612892464', 'created_by', 'entity', 'ent:Organization:8832543031059216933', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7294700582288177136', 'src:readme', 'Install', 'git clone https://github.com/boru-lang/boru', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7310321027254467049', 'ent:Product:6926990046092332766', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7310321027254467049', 'src:readme', 'Repository layout', 'cmd/go/ | The boru CLI / REPL (github.com/boru-lang/boru/cmd/go).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7447279695537175736', 'ent:Product:2046405728378673079', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7447279695537175736', 'src:agents', 'Repository layout', 'kg/ | The project knowledge graph: an evidence-backed boru pipeline and its generated bundle.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:758093515468550172', 'ent:Product:8635404738244704660', 'related_to', 'entity', 'ent:Product:9122085017676103232', NULL, NULL, NULL, NULL, 0.9, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:758093515468550172', 'src:readme', 'Repository layout', 'real programs, built with boru build', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7661116843982840438', 'ent:Product:5770789618934008141', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7661116843982840438', 'src:readme', 'Repository layout', 'lang/spec/ | Engine spec TSV files (the language''s executable spec).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7828550185343127', 'ent:Document:4163489813681141089', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7828550185343127', 'src:agents', 'header', 'New to the repo? Skim README.md first.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:7875690729105221344', 'ent:Concept:3854395902791518463', 'related_to', 'entity', 'ent:Concept:4841193570246608846', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:7875690729105221344', 'src:readme', 'intro', 'words can equally take their arguments from a value stack, which is what makes point-free pipelines compose', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8053419169907804914', 'ent:Product:9122085017676103232', 'part_of', 'entity', 'ent:Product:6926990046092332766', NULL, NULL, NULL, NULL, 0.97, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8053419169907804914', 'src:readme', 'Repository layout', 'cmd/go/ | The boru CLI / REPL', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8358401633369577874', 'ent:Product:4938585304473715098', 'related_to', 'entity', 'ent:Product:1578575807542004434', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8358401633369577874', 'src:readme', 'Repository layout', 'basic/go/ | The base language layer: fundamental words and predefined content types, on eng only (ADR-013).', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8927535347315204632', 'ent:Concept:6094411313845087998', 'part_of', 'entity', 'ent:Concept:2039420555596601682', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8927535347315204632', 'src:cli-md', 'The vault', 'HTTP wire protocol for secret provision', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:897358123486782210', 'ent:Document:3294415633888265368', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:897358123486782210', 'src:completeness-review', 'title', 'Type checker + bytecode compiler — completeness review', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:8974561920121926213', 'ent:Product:6022551412401652470', 'part_of', 'entity', 'ent:Product:4032424380612892464', NULL, NULL, NULL, NULL, 0.98, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:8974561920121926213', 'src:readme', 'Repository layout', 'compiler/go/ | The compiler: the recorder, lowering, and bytecode emitter over check and core.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:914658451831503592', 'ent:Document:6176355086953937469', 'supports', 'entity', 'ent:Concept:3854395902791518463', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:914658451831503592', 'src:readme', 'Documentation', 'Tutorial | You are new to boru and want to learn it step by step.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:9216246464349064781', 'ent:Document:5105101056062860425', 'supports', 'entity', 'ent:Product:1578575807542004434', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:9216246464349064781', 'src:coverage-parity', 'The contract', 'eng proves itself with its OWN suite, on both implementations.', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:999583482530779094', 'ent:Document:5105101056062860425', 'part_of', 'entity', 'ent:Document:520435226487613788', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:999583482530779094', 'src:coverage-parity', 'title', 'ENG-COVERAGE-PARITY.0 — the standalone 100%/100% program for the kernel twins', 'direct_record', 'kg-ingest');
INSERT INTO assertions VALUES ('ast:999962917132771020', 'ent:Document:3521225411209893772', 'supports', 'entity', 'ent:Product:1578575807542004434', NULL, NULL, NULL, NULL, 0.95, 'asserted', NULL, NULL, '2026-07-22T00:00:00Z', NULL);
INSERT INTO assertion_evidence VALUES ('ast:999962917132771020', 'src:audit', '7A', 'the parser emits kernel STRUCTURAL MARKERS; the engine lowers each marker to word dispatches', 'direct_record', 'kg-ingest');
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