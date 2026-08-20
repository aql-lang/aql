# Outage recovery bundle — NUR095 fix + valof/tis pins

The session's git-proxy write path failed (reads and GitHub-API writes
work; `git push` cannot authenticate), stranding two finished commits
locally. This branch, based on the PR #394 head `5e0d91a`, carries
their combined content. To reassemble the real branch state on a fresh
clone of `5e0d91a`:

1. Take every file under `core/`, `lang/`, and `test/` from THIS branch
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

2. Apply `patches/emit_go_nur095.patch` to `compiler/go/emit.go`
   (`git apply patches/emit_go_nur095.patch`) — the compiler half of
   the NUR095 fix (fn-shape carriers in the residual apply detectors).

3. Delete the NUR095 entry from `NUR.md` if present — on `5e0d91a` it
   does not exist yet, so there is nothing to do on a clean reassembly
   (the entry was added and retired entirely within the stranded
   commits; net zero).

4. Regenerate the knowledge-graph bundle: `make -C kg graph` (rebuilds
   `kg/out/graph.json`, `graph.md`, `graph.sql` byte-identically — the
   generated_at stamp is pinned; the digest picks up the new
   `test/go/fissiongate` package), then `make -C kg verify`.

5. Gates, all previously PASS on this exact content: `make fmt && make
   vet && make lint && make test && make cover-gate`;
   `make cover-gate-core`; `make cover-gate-compiler`;
   `make verify-bytecode`; `cd test/go && go test ./langspec/` (census
   refusals 0).

Original commit messages preserved in the git history of the stranded
container; summary: (1) "valof flip completion, fission ratchet,
NUR095, tis/fn-member pins"; (2) "fix NUR095: fn-shape-typed carriers
are maybe-callable in the recorder". Delete this branch once the real
branch push lands.
