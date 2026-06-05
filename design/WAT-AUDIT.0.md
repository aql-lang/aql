# WAT Audit — Surprising Behaviours of the Reference Implementation

Status: **observational**. This is an audit, not a proposal. It records
behaviours of the `aql` reference implementation (as built from this
tree) that are likely to surprise a user, contradict the prose
documentation, or both. Nothing here changes engine behaviour; the
companion documentation pass only makes the prose match what the binary
actually does.

Every entry below was reproduced against `cmd/go/bin/aql` built at the
audit commit. Commands are shown exactly as run. `=>` in this document
means "evaluates to" (a human annotation — see Exhibit U for why that
matters), not literal syntax.

## How to read this

Each exhibit is tagged with a disposition:

| Tag | Meaning |
| --- | --- |
| **doc** | The implementation is defensible; the *docs* were wrong. Fixed in the documentation pass. |
| **bug** | A clear implementation defect (wrong answer, nonsensical error, or crash). Left unfixed by request; recorded here. |
| **design** | A deliberate-looking choice with sharp edges. Needs an owner to confirm or revise. Documented as-is. |

The documentation pass that accompanies this audit touches only **doc**
items plus accuracy notes for **design** items. No **bug** was fixed.

---

## A. The flagship infix example does not exist — `doc`

The README, EXPLANATION, and TUTORIAL all sell infix reading with
`"hello" upper`. There is no `upper` word in the base environment.

```
$ aql -e '"hello" upper'
error: [aql/undefined_word]: undefined word: upper
```

The real word is `StringUtil.upper`, behind `"aql:string-util" import
end`. More broadly, `aql describe` documents **159** words; **90** of
them report `undefined word` when invoked, because `describe` lists
module words (`abs`, `changecase`, `split`, `trim`, …) under bare names
with no import hint. `describe abs` shows docs for a word that
`aql do -- '-1 abs'` cannot call.

## B. The CLI cannot evaluate a leading negative number — `bug`

```
$ aql do '-7 0 add'
flag provided but not defined: -7 0 add
$ aql do -- '-7 0 add'
-7
```

A leading `-7` is parsed as an unknown flag. The `--` separator works
but is undocumented.

## C. Forward collection breaks composition — `design`

```
$ aql do '3 4 add 2 mul'        => 18      # naive reading: 14
$ aql do -- '1 lt 2 lt 3'       => 1 false 2
```

`add` collects the `2` that visually belongs to `mul`, computing
`4+2=6`, leaving `3 6` for `mul`. Comparisons don't chain; they leave
debris. The "reads naturally" claim holds only for a single isolated
operation.

## D. "Keep each step on its own line" is false; the REPL resets the stack — `doc`

EXPLANATION says you can avoid Exhibit C by keeping each step on its own
line. Newlines are **not** barriers in a file:

```
$ printf '3 4 add\n2 mul\n' > t.aql && aql t.aql   => 18
```

The advice appears to work in the REPL only because the REPL **wipes the
stack between lines**:

```
$ printf '3 4 add\n2 mul\n' | aql repl
7
error: no matching signature for mul   (stack: 2)
```

So the same two lines give `18` in a file and `7` + a crash in the REPL.

## E. String truthiness: only `""` and exactly `"false"` are falsy — `design`

```
$ aql do -- 'if "false" ["T"] ["F"]'   => F
$ aql do -- 'if "FALSE" ["T"] ["F"]'   => T
$ aql do -- 'if "0"     ["T"] ["F"]'   => T
$ aql do -- 'if "no"    ["T"] ["F"]'   => T
```

Source: `eng/go/core_helpers.go:494` (`case "false", "":`). A non-empty
string is truthy unless it is exactly the lowercase word `"false"`.
`if []` is a separate inconsistency: it errors (`condition produced no
value`) rather than being falsy like `""`.

## F. `and` / `or` return operands, not booleans — `design`

```
$ aql do -- '1 2 and'      => 2
$ aql do -- 'true 5 and'   => 5
$ aql do -- '0 9 or'       => 9
```

`describe and` says "Logical AND of two booleans." The implementation is
Python-style operand-returning short-circuit over any type.

## G. `add` of a boolean and an int is a string; the checker approves — `design`

```
$ aql do -- 'true 1 add'     => true1
$ aql check -e 'true 1 add'  => check: 0 error(s), 0 warning(s)
```

Boolean stringifies and concatenates with the number. The static
checker reports no problem.

## H. Equal values that are not substitutable — `design`

```
$ aql do '1 1.0 eq'    => true
$ aql do '1 1.0 cmp'   => 0
$ aql do '1 2 div'     => 0      # integer division
$ aql do '1.0 2 div'   => 0.5    # real division
```

`1` and `1.0` are equal by both `eq` and `cmp`, yet not interchangeable.
`1 2 div` silently truncates with no remainder warning.

## I. "Any word, any position" is selectively false — `doc`

```
$ aql do '1 dup'    => 1 1
$ aql do 'dup 1'    => error: no matching signature for dup
$ aql do 'reverse [1 2 3]'  => [3 2 1]   # but this forward-collects fine
```

`reverse`/`add` collect forward; `dup`/`swap` don't. The universal
prefix/infix/suffix claim does not hold for stack-shuffling words.

## J. `aql fmt -w` is documented everywhere and always errors — `doc` (+ `bug`-adjacent)

```
$ aql fmt -w script.aql
error: open -w: no such file or directory
$ aql fmt -h
error: open -h: no such file or directory
```

`cmd/go/internal/fmt/fmt.go` treats every argument as a filename
(`files = args`). There is no flag parsing: no `-w`, no `-h`, no
stdout/diff mode. Formatting is always in place. `aql fmt` with **no**
args reformats every `.aql` file in the working tree — convenient and
dangerous. CLI.md (×3) and README all show the non-existent `-w` flag.

## K. Integer overflow has two contradictory silent behaviours — `bug`

```
$ aql do '9223372036854775807 1 add'   => 9223372036854776000.0
$ aql do '2 63 pow'                     => -9223372036854775808
```

`add` promotes to float (and loses precision); `pow` wraps two's
complement to negative. Neither flags anything.

## L. The type named `Decimal` is binary float64 — `doc`/`design`

```
$ aql do '0.1 0.2 add'            => 0.30000000000000004
$ aql do -- '0.1 0.2 add 0.3 eq'  => false
```

`typeof 0.1` is `Decimal`, a name that implies base-10 exactness. It is
IEEE-754 binary float with the standard `0.1 + 0.2 ≠ 0.3` behaviour.

## M. Core literals and builtins are unprotected mutable bindings — `design`

```
$ aql do -- 'def true fn [[] [Number] [42]] true'                      => 42
$ aql do -- 'def add fn [[x:Number y:Number] [Number] [x sub y]] 5 3 add'  => -2
```

`def true` shadows the boolean literal; `def add` redefines addition. No
warning.

## N. "Errors are values, not exceptions" — only inside `do [...]` — `doc`

EXPLANATION: *"When `1 div 0` fails, it doesn't unwind the stack — it
produces an Error value that sits on the stack like any other."*

```
$ aql do -- '1 div 0 dup'    => error: division by zero    # unwinds; never reaches dup
$ aql do -- 'do [1 div 0]'   => error(division by zero)    # value — but only here
```

Errors become values only when a `do [...]` (or the `error` handler)
captures them. Bare operations unwind.

## O. Schrödinger's quotation — `[...]` evaluates or defers by receiver — `design`

```
$ aql do -- '[1 2 add]'     => [3]      # eager
$ aql do -- 'do [1 2 add]'  => 3        # deferred
$ aql do -- '[1 0 div]'     => error: division by zero      # eager → throws
$ aql do -- 'do [1 0 div]'  => error(division by zero)      # deferred → caught
```

The same `[code]` syntax is eagerly-evaluated data in most positions and
a deferred block when consumed by a word like `do`/`each`. There is no
lexical way to tell which. (Note: `eval` does not exist; the evaluator
is the overloaded word `do`.)

## P. `lt`/`gt` compare across any types and never error — `design`

```
$ aql do -- '1 "a" lt'   => true
$ aql do -- '"a" 1 lt'   => false
$ aql do -- 'true 1 lt'  => true
$ aql do '[3 "a" 1 true] sort'   => [true 1 3 'a']
```

A universal total order `bool < number < string` means cross-type
comparison silently succeeds; heterogeneous lists sort without error.

## Q/W. `convert Integer` takes a string but refuses a number — `bug`

```
$ aql do '"4" convert Integer'   => 4
$ aql do '3.9 convert Integer'   => error: convert: cannot convert "3.9" to number
$ aql do '3.0 convert Integer'   => error: convert: cannot convert "3.0" to number
```

Text-to-int works; number-to-int fails — even for whole-valued floats.
The error message stringifies the float (`"3.0"`) and then claims it is
not a number.

## R. Missing data is `None` in some places and a crash in others — `design`

```
$ aql do '{a: 1} . b'          => None        # missing key: silent
$ aql do '[10 20 30] 5 get'    => None        # out of range: silent
$ aql do -- '[10 20 30] -1 get'  => None        # negative index: silent (no end-indexing)
$ aql do '{a: 1} . a . b'      => error: no matching signature for get   # field on a scalar
```

Silent `None` from bad indices means off-by-one and accidental-negative
bugs vanish until a downstream stage chokes.

## S. `"${...}"` interpolation works only in backtick strings — `doc`/`design`

```
$ aql do -- '"sum is ${1 add 2}"'   => sum is ${1 add 2}   # double quotes: literal
$ aql do -- '`sum is ${1 add 2}`'   => sum is 3            # backticks: interpolated
```

A `${}` in a double-quoted string is emitted verbatim with no
diagnostic.

## T. Duplicate map-literal keys collapse silently — `design`

```
$ aql do '{a: 1, a: 2}'   => {a:2}
```

Last write wins; no duplicate-key warning.

## U. The `=>` in every doc example is the anonymous-function arrow — `doc`

The docs annotate results as `expr => value` on ~360 lines. `=>` is also
a real operator: the `afn` (lambda) arrow (`eng` grammar maps `=>` →
`afn`).

```
$ aql do -- 'def square fn [[x:Number][Number][x mul x]] 4 square => 16'    => fn (Integer)
$ aql do -- 'def square fn [[x:Number][Number][x mul x]] 4 square => 9999'  => fn (Integer)
```

Pasting a whole doc line (annotation included) builds a function and
silently discards the computed value — the false claim `=> 9999` raises
no error. The doc-test harness (`test/go/docexamples`) splits on `=>`
and evaluates only the left side, so CI never sees this; a human pasting
into the REPL does. The documentation pass adds a notation note.

## V. `:Number` record fields launder strings into numbers — `design`/`bug`

```
$ aql do -- 'def Point refine Record [x:Number y:Number] end make Point ["1" "2"]'  => {x:1 y:2}
$ aql do -- 'def Point refine Record [x:Number y:Number] end make Point [3 "4"]'    => {x:3 y:4}
$ aql do -- 'def P refine Record [x:Number] end make P [true]'   => error: cannot convert "true" to number
```

A `:Number` field coerces by stringify-then-reparse rather than
rejecting non-numbers. `make P [true]` exposes the mechanism. Note this
contradicts Exhibit Q/W: `make` coerces freely while `convert` refuses.

## X. Moving `if` one position turns a 3-step countdown into a 1 GB stack blowup — `design`/`bug`

```
# prefix if — works:
$ aql do -- 'def cd fn [[n:Integer][Integer][if (n 0 eq) [0] [n 1 sub cd]]] 3 cd'  => 0
# suffix if — same logic, if written last:
$ aql do -- 'def cd fn [[n:Integer][Integer][n 0 eq [0] [n 1 sub cd] if]] 3 cd'
   => runtime: goroutine stack exceeds 1000000000-byte limit
```

In suffix position, forward collection mis-binds `if`'s arguments so the
base case never fires.

## Y. Stack overflow is reported with two wrong names — `bug`

```
$ aql do -- 'def runaway fn [[n:Integer][Integer][n 1 add runaway]] 0 runaway'
   => error: [aql/syntax_error]: unmatched opening parenthesis
```

Genuine infinite recursion is reported as a `syntax_error` about
parentheses (which are balanced). The other overflow path (Exhibit X)
reports `runtime: goroutine stack exceeds 1GB`. Neither says "recursion
too deep."

## Z. Two distinct "nothing" values that are not equal — `design`

```
$ aql do -- '[1,,2] 1 get'   => null
$ aql do 'none'              => None({})
$ aql do -- 'null none eq'     => false
```

`null` (list gap-filler) and `none` (`None({})`) are different bottom
values and compare unequal.

## AA. A double comma fabricates a `null` element — `design`/`bug`

```
$ aql do -- '[1, 2, 3]'   => [1 2 3]      # commas optional
$ aql do -- '1 , 2'       => 1 2          # bare comma is a no-op word
$ aql do -- '[1,,2]'      => [1 null 2]   # double comma invents an element
```

Commas are decorative no-ops everywhere except an empty slot between
two, which materialises a `null` and lengthens the list.

## AB. A typo'd float yields a signature error about `get` — `design`

```
$ aql do '1.2.3'   => error: [aql/signature_error]: no matching signature for get
```

`1.2.3` lexes as `1.2` then `.` (field access) then `3`. The diagnostic
points at an accessor the user never wrote.

## AC. Strings are not indexable and have no built-in length; maps are not `each`-able — `design`

```
$ aql do '"hello" 0 get'   => error: no matching signature for get
$ aql do '"hello" len'     => error: undefined word: len
$ aql do '{a:1 b:2} each [dup mul]'   => error: no matching signature for each
```

String index and length (`len`/`length`) are absent from the base
vocabulary (length is an unimported module word); `each` iterates lists
but not maps.

## AD. Two fields of the same record cannot be added inline — `design`

```
$ aql do -- '{first: 1 second: 2} . first add . second'
   => error: [aql/signature_error]: no matching signature for add
```

Forward collection has `add` reach past the `. second` accessor word.
`record.first + record.second` requires parentheses or stack juggling.

---

## Disposition summary

| Exhibit | Subject | Tag |
| --- | --- | --- |
| A | `upper` / `describe` list 90 undefined-bare words | doc |
| B | leading negative number → flag error | bug |
| C | forward collection breaks composition | design |
| D | newline ≠ barrier; REPL resets stack | doc |
| E | only `""`/`"false"` are falsy | design |
| F | `and`/`or` return operands | design |
| G | `bool add int` → String; checker passes | design |
| H | `1 eq 1.0` but not substitutable | design |
| I | "any position" false for `dup`/`swap` | doc |
| J | `fmt -w` / `-h` never implemented | doc |
| K | two int-overflow behaviours | bug |
| L | `Decimal` is binary float | doc/design |
| M | redefine `true`/builtins silently | design |
| N | errors-as-values only in `do [...]` | doc |
| O | `[...]` eager vs deferred by receiver | design |
| P | cross-type `lt` never errors | design |
| Q/W | `convert Integer` rejects numbers, accepts text | bug |
| R | missing data: `None` vs crash | design |
| S | `${}` only in backticks | doc/design |
| T | duplicate map keys collapse | design |
| U | `=>` is the `afn` arrow | doc |
| V | `:Number` fields launder strings | design/bug |
| X | suffix `if` → unbounded recursion | design/bug |
| Y | stack overflow → wrong error names | bug |
| Z | `null` ≠ `none` | design |
| AA | `[1,,2]` fabricates `null` | design/bug |
| AB | `1.2.3` → `get` signature error | design |
| AC | strings not indexable; no `len`; maps not `each` | design |
| AD | `rec.a add rec.b` fails | design |

## What the documentation pass changed

Only **doc**-tagged items (and accuracy notes for **design** items where
the docs already discuss the topic) were edited:

- `upper` examples → `StringUtil.upper` with the import shown (A).
- `aql fmt -w` → `aql fmt` everywhere; the in-place / no-arg / no-stdout
  reality stated (J).
- The "keep each step on its own line" advice corrected; the REPL
  stack-reset behaviour documented (D).
- The "any word, any position" claim qualified for stack words (I).
- The "Errors as values" section reworded to require `do [...]` (N).
- A **Notation** note added wherever `=>` is used, stating it is an
  annotation and that bare `=>` is the lambda arrow (U).
- Accuracy notes added for truthiness (E), `and`/`or` (F),
  `Decimal`/float (L), `convert` limits (Q/W), `null` vs `none` (Z), and
  string indexing (AC) where the relevant section already exists.

No engine behaviour was changed. Every **bug** above remains live.
