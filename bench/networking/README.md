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
