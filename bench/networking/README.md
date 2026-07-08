# Networking performance test — AQL vs. plain Go vs. plain TypeScript

A like-for-like **TCP echo round-trip** microbenchmark comparing the
`aql:net` module against equivalent hand-written Go and Node/TypeScript.

## What it measures

One persistent TCP connection carrying a newline-framed request/response
protocol (`"hello world\n"` echoed back). Each runtime performs **20,000
timed round-trips** plus one warm-up exchange, then reports throughput
(`req/s`) and mean per-round-trip latency. Round-trip (ping-pong) latency is
the fairest cross-language networking microbenchmark: it isolates the
per-request cost of the send/receive path rather than raw bulk throughput.

All three ends run over loopback TCP with `TCP_NODELAY` enabled — it is Go's
default (which the `aql:net` sockets inherit, being `net.Conn` values) and is
set explicitly in the Node version to match.

The AQL version deliberately uses the same primitive words the networking
examples in `design/examples/apps/` are built on: `Net.serve-raw`,
`Net.connect-raw`, `Net.send-bytes`, `Net.recv-until`.

## Files

| File | Runtime |
|------|---------|
| `echo_aql.aql`  | AQL (`aql:net`) — server + client in one process |
| `echo_go.go`    | Plain Go (`net` + `bufio`) |
| `echo_ts.ts`    | Plain TypeScript (Node `net`) |
| `loop_only.aql` | AQL no-network control: same per-iteration work, no sockets, to isolate interpreter-loop overhead from socket-word cost |

## Running

```bash
# from the repo root (build the CLI once: cd cmd/go && make build)
cmd/go/bin/aql run -install network bench/networking/echo_aql.aql   # bytecode compiler (default)
cmd/go/bin/aql run -no-compile -install network bench/networking/echo_aql.aql
go run bench/networking/echo_go.go
ts-node bench/networking/echo_ts.ts        # or: bun run bench/networking/echo_ts.ts

cmd/go/bin/aql run bench/networking/loop_only.aql   # control (no network capability needed)
```

`bench/networking/` is intentionally outside every Go module tree, so it is
never touched by `make fmt/vet/lint/test/cover-gate`.

## Results

Representative figures (≈3 runs each; Go 1.24, Node 22, loopback Linux):

| Runtime | req/s | µs / round-trip | vs. AQL |
|---|--:|--:|--:|
| Go (`net` + `bufio`)               | ~118,000 | ~8.5   | ~125× faster |
| TypeScript (Node `net`)            | ~53,000  | ~19    | ~55× faster  |
| AQL — bytecode compiler (default)  | ~940     | ~1,065 | 1× (baseline) |
| AQL — interpreter (`-no-compile`)  | ~680     | ~1,470 | 0.7×          |

So Go ≈ 2.2× Node, and both lead AQL by one-to-two orders of magnitude on
this workload. AQL's bytecode compiler buys ~1.4× over its tree-walking
interpreter but does not close the gap.

### Where AQL's time goes

The `loop_only.aql` control does the same per-iteration `Bytes` build +
`convert`/`join` work with no sockets: **~68 ms for 20,000 iterations
(~3.4 µs/iter)**. The full round-trip costs ~1,065 µs/iter, so **~99.7% of
the cost is in the networking word layer per call**, not in AQL's evaluation
of the loop body. The read path is already buffered (`bufio`), so it is
per-word overhead — socket handle lookup + signature matching, the per-`recv`
read-deadline syscall, the read mutex, and the forked-registry output syncing
on the server goroutine — rather than byte-by-byte I/O.

### Caveats

- Latency-bound (ping-pong). A pipelined/bulk-throughput test would narrow the
  gap somewhat but not change the ordering.
- AQL is an interpreted, strongly-typed query language; a ~50–125× gap versus
  compiled Go / JIT'd V8 on a tight network loop is expected and is not the
  workload AQL is optimized for.

## Root-cause analysis

The initial guess (per-call socket syscalls) was wrong. Localizing the cost by
crossing AQL against Go on each end (`diag/`) shows the overhead is almost
entirely **server-side**, and is an interpreter-vs-compiler artifact, not a
network artifact:

| Experiment | req/s | µs / RT |
|---|--:|--:|
| Go server + Go client (baseline)            | ~118,000 | ~8.5  |
| **AQL client** + Go server                  | ~20,800  | ~48   |
| **AQL server** + Go client (full handler)   | ~870     | ~1,150 |
| AQL server, *minimal* handler + Go client   | ~11,000  | ~91   |
| Go server with **Nagle ON** + Go client     | ~21,600  | ~46   |

Findings, each isolated:

1. **The AQL client path is fine** (~48 µs/RT, ~5–6× Go). The timed client loop
   runs at the top level, which the bytecode compiler *does* compile.
2. **The bottleneck is the `serve-raw` connection handler.** Swapping only which
   end is AQL moves the cost by ~24×.
3. **Not Nagle / TCP options.** A Go echo server with `SetNoDelay(false)` stays
   fast — loopback ACKs arrive before Nagle can coalesce single-frame writes.
4. **The handler body runs on the tree-walking interpreter, not the bytecode
   compiler.** The identical `convert`/`join`/`def` loop costs **76 ms compiled
   at top level, 1423 ms under `-no-compile`, and 1457 ms inside a `serve-raw`
   fork** (`loop_only.aql` vs `diag/fork_compute.aql`). The in-fork figure
   matches the interpreter figure, not the compiled one — a ~19× per-word
   penalty paid on every request.

The mechanism is in `eng/go/registry.go`: `Registry.CallAQL` evaluates every
function body via `sub := NewTop(r); sub.Run(tokens)` — the interpreter. The
bytecode compiler applies to the top-level program only, so any AQL function
body (and therefore every per-connection `serve-raw` handler) is interpreted.
`-force-compile` at the top level does not change this.

**In one line:** AQL's networking isn't slow because of the network — it is slow
because server request handlers are AQL *function bodies*, and function bodies
currently execute on the interpreter rather than the compiler, at ~19× the
per-word cost. Compiling fn bodies / handler loops would be the highest-leverage
fix; the socket words themselves are not the constraint.

### `diag/` — the localization harness

`go_server.go` / `go_client.go` (split echo), `go_server_nonagle.go`
(Nagle-on control), `aql_server.aql` / `aql_client.aql` (split echo),
`aql_server_min.aql` (minimal handler body), and `fork_compute.aql` (times the
pure compute loop *inside* a handler fork). Each takes a port argument or a
fixed port noted in the file; start a server in the background, then run the
matching client.
