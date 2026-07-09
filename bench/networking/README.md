# Networking performance test — AQL vs. Go, TypeScript, Python, Ruby

A like-for-like **TCP echo round-trip** microbenchmark comparing the
`aql:net` module against equivalent hand-written Go, Node/TypeScript, Python,
and Ruby. Every implementation is checked in so the test is repeatable.

## What it measures

One persistent TCP connection carrying a newline-framed request/response
protocol (`"hello world\n"` echoed back). Each runtime performs **20,000
timed round-trips** plus one warm-up exchange, then reports throughput
(`req/s`) and mean per-round-trip latency. Round-trip (ping-pong) latency is
the fairest cross-language networking microbenchmark: it isolates the
per-request cost of the send/receive path rather than raw bulk throughput.

Every implementation is server + client in one process over loopback TCP with
`TCP_NODELAY` on both ends — Go's default (which the `aql:net` sockets inherit,
being `net.Conn` values) and set explicitly in the Node / Python / Ruby ports
to match. Each uses its own port so runs don't collide.

The AQL version deliberately uses the same primitive words the networking
examples in `design/examples/apps/` are built on: `Net.serve-raw`,
`Net.connect-raw`, `Net.send-bytes`, `Net.recv-until`.

## Files

| File | Runtime |
|------|---------|
| `echo_aql.aql`  | AQL (`aql:net`) — server + client in one process |
| `echo_go.go`    | Plain Go (`net` + `bufio`) |
| `echo_ts.ts`    | Plain TypeScript (Node `net`) |
| `echo_py.py`    | Plain Python (`socket` + `threading`) |
| `echo_rb.rb`    | Plain Ruby (`socket` + `Thread`) |
| `loop_only.aql` | AQL no-network control: same per-iteration work, no sockets, to isolate interpreter-loop overhead from socket-word cost |

## Running

```bash
# from the repo root (build the CLI once: cd cmd/go && make build)
cmd/go/bin/aql run -install network bench/networking/echo_aql.aql   # bytecode compiler (default)
cmd/go/bin/aql run -no-compile -install network bench/networking/echo_aql.aql
go run  bench/networking/echo_go.go
npx ts-node bench/networking/echo_ts.ts    # or: bun run bench/networking/echo_ts.ts
python3 bench/networking/echo_py.py
ruby    bench/networking/echo_rb.rb

cmd/go/bin/aql run bench/networking/loop_only.aql   # control (no network capability needed)
```

Take the best of ~3 runs per runtime (loopback microbenchmarks are noisy).

`bench/networking/` is intentionally outside every Go module tree, so it is
never touched by `make fmt/vet/lint/test/cover-gate`.

## Results

Best of 3 runs each (Go 1.24.7, Node 22, Python 3.11, Ruby 3.3, loopback
Linux, single box):

| Runtime | req/s | µs / round-trip | vs. Go |
|---|--:|--:|--:|
| Go (`net` + `bufio`)                    | ~109,000 | ~9.1   | 1× (baseline) |
| **AQL — bytecode compiler (default)**   | **~55,000** | **~18** | **~2.0× slower** |
| TypeScript (Node `net`)                 | ~48,000  | ~21    | ~2.3× slower |
| Python (`socket`)                       | ~14,600  | ~69    | ~7.5× slower |
| Ruby (`socket`)                         | ~13,500  | ~74    | ~8.1× slower |
| AQL — interpreter (`-no-compile`)       | ~620     | ~1,610 | ~176× slower |

The **compiled** AQL server runs its per-connection handler on the bytecode VM
and lands within ~2× of Go — **ahead of Node, and ~4× ahead of Python and
Ruby**. Only hand-written Go is faster. That is a **~90× jump over AQL's own
tree-walking interpreter** (~620 → ~55,000 req/s), the payoff of compiling the
`serve-raw` handler body to a unit (see *Resolution* below). The handler
compiles whether its connection param is typed `Socket` or `Any` (an earlier
revision required `Socket`; two compiler fixes removed that — see below).

An earlier revision of this file reported ~940 req/s for the "compiler
(default)" row: at that time function-body compilation did not exist, so the
handler ran interpreted regardless of the compile flag. That row is superseded
by the callback-compilation work.

### Where AQL's time goes (the interpreter path)

This analysis was done on the **interpreted** handler (the `-no-compile` /
pre-compilation ~620 req/s row), and is why compiling the handler mattered.
The `loop_only.aql` control does the same per-iteration `Bytes` build +
`convert`/`join` work with no sockets: **~68 ms for 20,000 iterations
(~3.4 µs/iter)**. The interpreted round-trip cost ~1,065 µs/iter, so **~99.7%
of it was in the networking word layer per call** — socket handle lookup +
signature matching, the per-`recv` read-deadline syscall, the read mutex, and
the forked-registry output syncing on the server goroutine — not the loop body.
Compiling the handler body to a VM unit removes the interpreter's per-word
dispatch overhead and is what lifts AQL to ~18 µs/RT (~55,000 req/s).

### Caveats

- Latency-bound (ping-pong). A pipelined/bulk-throughput test would narrow the
  gaps somewhat but not change the ordering.
- Best-of-3 on a single loopback box; treat the figures as order-of-magnitude,
  not precise. Compiled AQL (~2× Go, ahead of Node, ~4× ahead of Python/Ruby)
  vs the interpreter (~176× Go) is the headline the numbers support.

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

## Resolution — the handler now compiles (~97× faster)

The highest-leverage fix above **landed**: the callback-compilation seam
(`design/CALLBACK-COMPILATION.0.md`) compiles a `serve-raw` connection handler
body to its own bytecode unit and runs it on the VM (`RunUnit`) per connection,
instead of `Registry.CallAQL` on the interpreter. The echo handler body — `for`,
`def`, `convert`, `join`, and the `Net.recv-until` / `Net.send-bytes` socket
words over the connection — compiles in full, and this benchmark now measures
**~58,000 req/s compiled vs ~600 interpreted** (the table above).

**Param typing (no longer required).** The socket words are module overloads
(`Net.recv-until : [Socket Bytes]` / `[Socket Bytes Map]`). An explicitly-`Any`
handler param now binds a **gradual** carrier, so the socket-word dispatch
resolves optimistically and the whole handler compiles — `[sock:Any]` and
`[sock:Socket]` both reach the VM (the benchmark uses `Socket` only for clarity).
Historically `[sock:Any]` was a hard blocker: the strict `Any` receiver could not
pick an overload, `def line (Net.recv-until sock nl)` was misread as *redefining*
a word `line` with the locked builtin signature `[Socket Bytes Map]`, and that
`locked_signature` error refused whole-program compilation. Two compiler fixes
removed it — see *Compiler fixes* below.

This corrected an earlier hypothesis that the handler was blocked by a deep
"module fn-value dispatch over a dynamic receiver" carrier-inference wall. It is
not: module dispatch over a `Socket` compiles fine (`Net.close sock`,
`Net.recv-until sock (convert Bytes "\n")` both compile); the only blocker was
the strict modelling of the `Any` param.

### Compiler fixes

1. **Gradual `Any` handler params (root).** The stored-handler compile path
   (`fnValueInputs`, `eng/go/emit.go`) built strict `Any` param carriers; it now
   uses `ParamInputCarrier`, so an `Any` param is gradual exactly as on the
   ordinary user-fn compile path. A body word over it poly-matches at runtime
   instead of failing `no_signature` against the strict `Any` top.
2. **Failed dispatch is not a word extension (defense-in-depth).** A value whose
   producing call genuinely fails to dispatch is left as a `FailedDispatch`
   Function carrying the native's locked signatures; `defWordExtension`
   (`lang/go/native/native_definition.go`) now declines to treat it as an
   open-words merge, so a def-bound failed dispatch inside a loop reports the real
   dispatch diagnostic instead of a spurious `locked_signature`.

Both landed gate-green (`verify-bytecode` differential + `-race`, `cover-gate`
100%).

### `diag/` — the localization harness

`go_server.go` / `go_client.go` (split echo), `go_server_nonagle.go`
(Nagle-on control), `aql_server.aql` / `aql_client.aql` (split echo),
`aql_server_min.aql` (minimal handler body), and `fork_compute.aql` (times the
pure compute loop *inside* a handler fork). Each takes a port argument or a
fixed port noted in the file; start a server in the background, then run the
matching client.
