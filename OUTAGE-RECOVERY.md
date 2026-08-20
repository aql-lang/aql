# Outage recovery bundle — NUR095 fix + valof/tis pins

**Update 2026-08-20T20:50Z: PR #394 has MERGED** (main `b41e1ec` carries
the full type-node fusion + docs tree, identical in content to the old
PR head `5e0d91a` this branch is based on). The stranded follow-up
commits must therefore land as a **NEW branch off main with a NEW pull
request** — do not stack on or reopen the merged PR.

The session's git-proxy write path failed (reads and GitHub-API writes
worked; `git push` could not authenticate), stranding two finished
commits in the session container. This branch carries their combined
content, transported byte-exact and verified. To reassemble on a fresh
clone of **main**:

1. `git checkout -b <new-branch> origin/main`

2. Take every file under `core/`, `lang/`, and `test/` from THIS branch
   verbatim (they are the final state, transported byte-exact):

   - `core/go/core_ref.go` — ResolveRef resolves type bindings through
     `Defs.Top` (the valof missed-flip-site fix)
   - `core/go/value_classify.go` — `TypeIsFnShape` + fn-shape-aware
     `IsFnTypedCarrier` (the NUR095 fix, core half)
   - `core/go/value_classify_stage5_test.go` — coverage for both
   - `lang/go/native/fnshape_member_interp_test.go` — interpreter-only
     pin for the trailing member-apply spelling
   - `lang/spec/valof.tsv` — §11 type-binding rows
   - `lang/spec/class.tsv` — fn-members section (NUR095 retirement pins)
   - `lang/spec/user-types.tsv` — §5c tis rows for the remaining kinds
   - `test/go/fissiongate/fissiongate_test.go` — the fission-ratchet
     architectural gate

   (`git checkout origin/claude/nur095-transport-z3alhe -- core lang/go/native/fnshape_member_interp_test.go lang/spec test/go/fissiongate`
   — then confirm only the eight files above changed.)

3. Apply `patches/emit_go_nur095.patch` to `compiler/go/emit.go`
   (`git show origin/claude/nur095-transport-z3alhe:patches/emit_go_nur095.patch | git apply -`)
   — the compiler half of the NUR095 fix (fn-shape carriers in the
   residual apply detectors). Verified: patch applied to the merged
   tree's emit.go reproduces the gated final file byte-exactly.

4. If a NUR095 entry exists in `NUR.md`, delete it — on the merged tree
   it does not (the entry was recorded and retired entirely within the
   stranded commits; net zero).

5. Regenerate the knowledge-graph bundle: `make -C kg graph` (rebuilds
   `kg/out/graph.json`, `graph.md`, `graph.sql` byte-identically — the
   generated_at stamp is pinned; the digest picks up the new
   `test/go/fissiongate` package), then `make -C kg verify`.

6. Gates, all previously PASS on this exact content: `make fmt && make
   vet && make lint && make test && make cover-gate`;
   `make cover-gate-core`; `make cover-gate-compiler`;
   `make verify-bytecode`; `cd test/go && go test ./langspec/` (census
   refusals 0).

7. Push the new branch, open a NEW pull request (suggested title:
   "valof flip completion, fission ratchet, NUR095 fix, tis/fn-member
   pins"), and DELETE this transport branch.

Original commit summaries: (1) "valof flip completion, fission ratchet,
NUR095, tis/fn-member pins" — ResolveRef yields the minted node for
type bindings uniformly (the Stage 2 flip's missed consumer site),
pinned per declaration kind; the fissiongate arch test ratchets the
node-kind-predicate call sites; tis rows extended to disjunct/alias/
singleton/fn-shape/subclass/surface kinds. (2) "fix NUR095:
fn-shape-typed carriers are maybe-callable in the recorder" — the
compiled lane left a fn stored through a NAMED class's fnsig-typed
field inert because the recorder's maybe-callable tests knew only
Function conformance; core.TypeIsFnShape + IsFnTypedCarrier close it,
the residual apply detectors treat fn-shape carriers symmetrically, and
the healed matrix is pinned in class.tsv (the trailing spelling is an
interpreter-only Go pin; the compiled lane's refusal there is the sound
fallback).
