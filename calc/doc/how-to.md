# How-to — recipes for hosting eng

These are short, goal-directed recipes. Each one points back at the
exact location in calc's source where the pattern is used in
context. If you want to learn the model end-to-end, start with
[tutorial.md](tutorial.md) instead.

## How to register a binary word that promotes Integer→Decimal

Use one `[TNumber, TNumber]` signature, dispatch through
`eng.AsNumber` (which works for both), then choose the return
constructor at the end.

```go
r.RegisterNativeFunc(eng.NativeFunc{
    Name:        "add",
    ForwardArgs: true,
    Signatures: []eng.NativeSig{{
        Args: []*eng.Type{eng.TNumber, eng.TNumber},
        Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
            a, _ := eng.AsNumber(args[0])  // float64 regardless of input subtype
            b, _ := eng.AsNumber(args[1])
            res := b + a                   // b op a — see explanation.md
            // collapse back to Integer when both inputs were Integer and the
            // result is whole
            if args[0].Parent.Matches(eng.TInteger) &&
                args[1].Parent.Matches(eng.TInteger) &&
                res == math.Trunc(res) {
                return []eng.Value{eng.NewInteger(int64(res))}, nil
            }
            return []eng.Value{eng.NewDecimal(res)}, nil
        },
        Returns: []*eng.Type{eng.TNumber},
    }},
})
```

See [`words.go::numHandler`](../words.go) for the closure factory
that lets `add`/`sub`/`mul`/`mod`/`pow` all share this skeleton.

## How to register a 0-arg constant word

A "constant" is a NativeFunc whose handler ignores `args` and
returns one value. No `ForwardArgs`, no `Args` — the dispatcher
just runs the handler when the word appears.

```go
r.RegisterNativeFunc(eng.NativeFunc{
    Name: "pi",
    Signatures: []eng.NativeSig{{
        Handler: func(_ []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
            return []eng.Value{eng.NewDecimal(math.Pi)}, nil
        },
        Returns: []*eng.Type{eng.TNumber},
    }},
})
```

See [`words.go::registerConstants`](../words.go) — calc registers
`pi` and `e` this way.

## How to register a stack-manipulator (FullStack)

For ops that don't have a fixed arg count — `clear`, `depth`,
calc's `show` — set `FullStack: true`. The handler receives the
entire stack and returns the new stack:

```go
r.RegisterNativeFunc(eng.NativeFunc{
    Name: "clear",
    Signatures: []eng.NativeSig{{
        FullStack: true,
        Handler: func(_ []eng.Value, _ map[string]eng.Value, stk []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
            return []eng.Value{}, nil  // empty replacement = clear
        },
        Returns: []*eng.Type{},
    }},
})
```

The third parameter — `stk` — is the live carrier stack. Mutating
the returned slice is fine because it replaces the engine's
internal stack wholesale. `show` in [words.go](../words.go) uses
this to peek at the stack without consuming it: return
`append([]eng.Value{}, stk...)`.

## How to report stack underflow

For fixed-arity stack ops (`dup` needs 1, `swap` needs 2,
`over` needs 2), check `len(stk)` at the top of the handler and
return a plain Go `error`:

```go
Handler: func(_ []eng.Value, _ map[string]eng.Value, stk []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
    if len(stk) < 2 {
        return nil, fmt.Errorf("swap: needs 2 items, stack has %d", len(stk))
    }
    out := append([]eng.Value{}, stk...)
    i := len(out) - 1
    out[i], out[i-1] = out[i-1], out[i]
    return out, nil
},
```

For stack-only words declared with `Args: []*eng.Type{TAny, TAny}`
the dispatcher reports `signature_error` before your handler ever
runs — the explicit `len` check is only needed for `FullStack`
handlers.

## How to direct word output to an arbitrary writer

calc's `print` and `show` write to a writer the host configures
at construction time rather than `os.Stdout`. The pattern is
"close over `io.Writer` at registration time":

```go
func RegisterWords(r *eng.Registry, out io.Writer) {
    if out == nil {
        out = io.Discard
    }
    r.RegisterNativeFunc(eng.NativeFunc{
        Name: "print",
        Signatures: []eng.NativeSig{{
            Args: []*eng.Type{eng.TAny},
            Handler: func(args []eng.Value, _ map[string]eng.Value, _ []eng.Value, _ *eng.Registry) ([]eng.Value, error) {
                fmt.Fprintln(out, args[0].String())
                return nil, nil
            },
        }},
    })
}
```

See [`words.go::registerDisplay`](../words.go). Tests pass a
`*bytes.Buffer`; the CLI passes `os.Stdout`; the REPL passes the
same writer it uses for its prompt.

## How to share a stack across multiple `Run` calls

`eng.Engine.Run` is one-shot: it takes a token slice and returns
whatever lands on the stack at the end. To make a REPL where
`1 2` followed by `add` produces `3`, prepend the residual stack
as literals before parsing the next line:

```go
func (c *Calc) Eval(src string) ([]eng.Value, error) {
    values, err := parser.Parse(src)
    if err != nil {
        return nil, fmt.Errorf("parse: %w", err)
    }
    seed := make([]eng.Value, 0, len(c.stack)+len(values))
    seed = append(seed, c.stack...)        // ← carry-over
    seed = append(seed, values...)
    result, err := eng.NewTop(c.Registry).Run(seed)
    if err != nil {
        return nil, err  // c.stack untouched on error
    }
    c.stack = result
    return result, nil
}
```

See [`calc.go::Eval`](../calc.go). The pattern works because
literal values are no-ops at the start of a program — they sit on
the stack and any subsequent word consumes them in source order.

## How to make failed evaluations not corrupt state

Only commit the new stack on success. The snippet above does this
by reassigning `c.stack = result` *after* the error check.
[`calc_test.go::TestFailedEvalLeavesStackIntact`](../calc_test.go)
pins this behaviour:

```go
c.Eval("1 2")     // c.stack == [1, 2]
c.Eval("0 div")   // returns error; c.stack still [1, 2]
```

## How to add a REPL meta-command

Reserve a prefix that can't appear in valid source — calc uses
`:`. Inside the read loop, before parsing, check for the prefix
and dispatch to a Go function:

```go
if strings.HasPrefix(line, ":") {
    switch strings.TrimPrefix(line, ":") {
    case "quit", "q":
        return
    case "stack", "s":
        fmt.Fprintln(out, FormatStack(c.Stack()))
    // …
    }
    continue
}
```

See [`repl.go::runMeta`](../repl.go). Meta-commands run as plain
Go, so they have full access to the host: read the def stack
(`r.Defs.Names()`), inspect type bindings (`r.Types.Names()`),
write history files, anything.

## How to test a word without running the CLI

Construct the host in-process with a `*bytes.Buffer` as the
output writer and run a one-liner through `Eval`:

```go
func TestAdd(t *testing.T) {
    buf := &bytes.Buffer{}
    c, _ := calc.New(buf)
    stk, err := c.Eval("add 2 3")
    if err != nil {
        t.Fatalf("Eval: %v", err)
    }
    if got, _ := eng.AsInteger(stk[0]); got != 5 {
        t.Errorf("want 5, got %d", got)
    }
}
```

The whole suite in [`calc_test.go`](../calc_test.go) is structured
this way. The buffer also lets you assert on what `print` / `show`
wrote without going near `os.Stdout`.

## How to enforce "this host has no kernel-level core words"

Don't call any `eng.RegisterCore*` function. After the eng→lang
migration there *are* no such functions to call — `eng.New(r)`
and `eng.NewTop(r)` give you a bare engine. The only words on
your registry are the ones you register yourself with
`r.RegisterNativeFunc`. Use [`calc/words.go`](../words.go) as
the reference for the minimal vocabulary: 21 words, every one of
them defined by the host.

## How to run the CLI

```bash
cd calc
make build                 # → bin/calc
./bin/calc -e "10 sub 3"   # 7
./bin/calc                 # interactive REPL
./bin/calc 2 pi mul        # positional args joined with spaces
```

## How to check coverage

```bash
cd calc
make cover         # per-package totals + 10 least-covered functions
make cover-html    # opens go tool cover -html=…
```

The repo-root `Makefile` also has `make cover` that aggregates
across every module.
