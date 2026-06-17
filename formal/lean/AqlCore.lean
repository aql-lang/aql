/-
  AqlCore.lean — a mechanized prototype of AQL's core semantics (deepened).

  Milestone-6 seed from `design/FORMAL-VERIFICATION.0.md`: a *deep
  embedding* of a tractable fragment of the AQL abstract machine
  (`FORMAL-SPEC.md` §4 + §6). This iteration deepens the model to put
  AQL's distinctive **forward collection** under a proof checker in a
  more faithful form:

    * two value types — `Int` and `Bool` (so types are non-trivial);
    * variable-arity words — unary (`not`) and binary (`add`/`sub`/`mul`
      and the comparisons `gt`/`lt`/`eq`), so collection is arity-driven;
    * the full §6.4 collection rule — gather up to `arity` leading
      forward literals, then fill the remaining slots from the stack top
      first, and push any *leftover* forward literals back onto the stack
      (the behaviour the engine shows with `add 1 2 7 => 3 7`);
    * the `end` barrier (§6.6).

  Everything type-checks under Lean 4.15.0 with no Mathlib (bundled
  core/Std only). The model's outputs are cross-checked against the real
  `aql` engine by the differential harness in `harness/` — that is the
  "tracer bullet" that connects this model to the implementation.

  Honest scope. The engine is *coercive* where this model is *strict*:
  `aql do 'add true 2'` yields `"2true"` and `'not 5'` yields `false`,
  whereas this model treats a type-mismatched argument vector as a stuck
  signature error (`none`). The harness therefore exercises only the
  homogeneous-type fragment on which model and engine agree; the coercion
  rules are deliberately out of scope (a later milestone). Also omitted:
  user functions, modules, effects, concurrency, floats/overflow, and
  refinement predicates.
-/

namespace AqlCore

/-! ## 1. Values and types (FORMAL-SPEC §5.1) -/

inductive Val
  | int  : Int → Val
  | bool : Bool → Val
  deriving DecidableEq, Repr

/-! ## 2. Words — each with an arity and a (strict) denotation

Binary words receive their argument vector as `[args[0], args[1]]` and
compute with the engine's convention (`sub = args[1] - args[0]`, so that
`10 sub 3 = 7` with `args[0]=3, args[1]=10`; comparisons mirror it). -/

inductive Word
  | add | sub | mul          -- (Int, Int) -> Int
  | gtw | ltw | eqw          -- (Int, Int) -> Bool
  | notw                     -- (Bool) -> Bool   [arity 1]
  deriving DecidableEq, Repr

def arity : Word → Nat
  | Word.notw => 1
  | _         => 2

/-- Strict denotation: `none` on an arity/type mismatch (a signature error). -/
def denote : Word → List Val → Option Val
  | Word.add, [Val.int x, Val.int y] => some (Val.int (x + y))
  | Word.sub, [Val.int x, Val.int y] => some (Val.int (y - x))
  | Word.mul, [Val.int x, Val.int y] => some (Val.int (x * y))
  | Word.gtw, [Val.int x, Val.int y] => some (Val.bool (decide (y > x)))
  | Word.ltw, [Val.int x, Val.int y] => some (Val.bool (decide (y < x)))
  | Word.eqw, [Val.int x, Val.int y] => some (Val.bool (decide (x = y)))
  | Word.notw, [Val.bool b]          => some (Val.bool (!b))
  | _, _                             => none

/-! ## 3. Tape (FORMAL-SPEC §4) -/

inductive Item
  | lit  : Val → Item
  | word : Word → Item
  | endb : Item              -- the `end` / `;` barrier
  deriving Repr

abbrev Tape  := List Item
abbrev Stack := List Val     -- head = top of stack

/-- Readable literal constructors for examples and generated harness code. -/
def ilit (n : Int)  : Item := Item.lit (Val.int n)
def blit (b : Bool) : Item := Item.lit (Val.bool b)

/-! ## 4. Forward collection (FORMAL-SPEC §6.4)

Gather up to `n` leading *literal* values to the right, stopping at the
first non-literal (a word or the `end` barrier) or end of tape. -/

def takeForward : Tape → Nat → (List Val × Tape)
  | t, 0 => ([], t)
  | (Item.lit v :: rest), (k+1) =>
      let r := takeForward rest k
      (v :: r.1, r.2)
  | t, _+1 => ([], t)        -- head is a word / `end` / empty: stop

/-- Pull `k` values off the stack, top first. `none` on underflow. -/
def takeStack : Nat → Stack → Option (Stack × Stack)
  | 0,   s        => some ([], s)
  | _+1, []       => none
  | k+1, v :: s   =>
      match takeStack k s with
      | some (t, s') => some (v :: t, s')
      | none         => none

/-! ## 5. Evaluation (FORMAL-SPEC §6)

Small-step intent as a fuel-bounded total function. At a word: collect
forward, fill the rest from the stack top first (`args = fwd ++ stack`,
exactly §6.4's "remaining slots from the stack, top first"), dispatch,
push the result. Leftover forward literals continue as ordinary pushes. -/

def eval : Nat → Tape → Stack → Option Stack
  | 0,    _,                       _ => none                  -- out of fuel
  | _+1,  [],                      s => some s                -- done
  | f+1,  (Item.lit v   :: rest),  s => eval f rest (v :: s)  -- push literal
  | f+1,  (Item.endb    :: rest),  s => eval f rest s         -- §6.6: inert when hit directly
  | f+1,  (Item.word w  :: rest),  s =>
      let (fwd, rest') := takeForward rest (arity w)
      match takeStack (arity w - fwd.length) s with
      | some (taken, s') =>
          match denote w (fwd ++ taken) with
          | some v => eval f rest' (v :: s')
          | none   => none                                   -- signature error
      | none => none                                         -- stack underflow

/-- Run a closed tape; return the full final stack (top at head). -/
def run (t : Tape) : Option Stack :=
  eval (t.length + 8) t []

/-! ## 6. Rendering — match the engine's printout (stack bottom→top) -/

def renderVal : Val → String
  | Val.int n  => toString n
  | Val.bool b => if b then "true" else "false"

/-- Engine prints the stack bottom-to-top, space separated; errors as a
    sentinel. The harness compares this string to `aql do`'s output. -/
def render : Option Stack → String
  | none   => "ERROR"
  | some s => String.intercalate " " (s.reverse.map renderVal)


/-! ## 7. Executable sanity (mirrors `aql do '...'`) -/

#eval render (run [Item.word Word.sub, ilit 3, ilit 10])              -- "7"
#eval render (run [ilit 10, Item.word Word.sub, ilit 3])             -- "7"
#eval render (run [ilit 10, ilit 3, Item.word Word.sub])            -- "7"
#eval render (run [Item.word Word.add, ilit 1, ilit 2, ilit 7])     -- "3 7"  (leftover)
#eval render (run [Item.word Word.gtw, ilit 5, ilit 3])            -- "false"
#eval render (run [Item.word Word.notw, blit true])               -- "false"
#eval render (run [ilit 10, Item.word Word.sub, Item.endb, ilit 3]) -- "ERROR" (barrier)


/-! ## 8. Headline theorem — source-spelling equivalence

For any *binary* word and any two operands, the three spellings
`y x op`, `y op x`, `op x y` collect the same argument vector and so
compute the same result. (The `notw` case is excluded by `hw`, since a
unary word's spellings are genuinely different.) -/

theorem spelling_equiv (w : Word) (hw : arity w = 2) (x y : Int) :
    run [ilit y, ilit x, Item.word w]
      = run [ilit y, Item.word w, ilit x]
  ∧ run [ilit y, Item.word w, ilit x]
      = run [Item.word w, ilit x, ilit y] := by
  cases w <;> first
    | exact ⟨rfl, rfl⟩
    | exact absurd hw (by decide)

/-- Concrete instances, in the engine's own terms. -/
theorem sub_spellings :
    run [ilit 10, ilit 3, Item.word Word.sub] = some [Val.int 7]
  ∧ run [ilit 10, Item.word Word.sub, ilit 3] = some [Val.int 7]
  ∧ run [Item.word Word.sub, ilit 3, ilit 10] = some [Val.int 7] := by
  refine ⟨?_, ?_, ?_⟩ <;> rfl


/-! ## 9. Arity-driven forward collection

Variable arity through the same machinery: a unary word collects one
argument (forward or from the stack), and a binary word caps its forward
collection at two, pushing any leftover literal back — the engine's
`add 1 2 7 => 3 7`. -/

theorem unary_spellings :
    run [Item.word Word.notw, blit true] = some [Val.bool false]   -- `not true`
  ∧ run [blit true, Item.word Word.notw] = some [Val.bool false] := by  -- `true not`
  constructor <;> rfl

theorem binary_caps_and_leaves_leftover :
    -- `add 1 2 7`: add takes 1,2 → 3; 7 stays. Stack top→head: [7, 3].
    run [Item.word Word.add, ilit 1, ilit 2, ilit 7] = some [Val.int 7, Val.int 3] := by
  rfl


/-! ## 10. The `end` barrier changes meaning (FORMAL-SPEC §6.6)

A barrier between a word and a following literal stops forward
collection, so the word can no longer reach that operand and is
under-supplied (engine: signature error; model: stuck). A *negative*
theorem — it pins down what must be rejected. -/

theorem barrier_blocks_forward :
    run [ilit 10, Item.word Word.sub, ilit 3] = some [Val.int 7]
  ∧ run [ilit 10, Item.word Word.sub, Item.endb, ilit 3] = none := by
  constructor <;> rfl


/-! ## 11. Determinism (FORMAL-SPEC §6, modulo §7.4 concurrency) -/

theorem run_deterministic (t : Tape) (a b : Stack) :
    run t = some a → run t = some b → a = b := by
  intro ha hb
  exact Option.some.inj (ha.symm.trans hb)


/-! ## 12. A slice of Half A — the type lattice and subtyping (§5.1) -/

inductive Ty
  | any | scalar | number | integer | float | boolean
  deriving DecidableEq, Repr

def parent : Ty → Option Ty
  | Ty.integer => some Ty.number
  | Ty.float   => some Ty.number
  | Ty.number  => some Ty.scalar
  | Ty.boolean => some Ty.scalar
  | Ty.scalar  => some Ty.any
  | Ty.any     => none

/-- `Sub a b` ≙ `a ⊑ b`: reflexive-transitive closure of `parent`. -/
inductive Sub : Ty → Ty → Prop
  | refl : Sub t t
  | step : parent a = some b → Sub b c → Sub a c

theorem sub_trans {a b c : Ty} : Sub a b → Sub b c → Sub a c := by
  intro hab hbc
  induction hab with
  | refl => exact hbc
  | step hpar _ ih => exact Sub.step hpar (ih hbc)

/-- The dynamic type of a value, and `Integer`/`Boolean ⊑ Scalar ⊑ Any`. -/
def typeOf : Val → Ty
  | Val.int _  => Ty.integer
  | Val.bool _ => Ty.boolean

example : Sub Ty.integer Ty.any :=
  sub_trans (Sub.step rfl Sub.refl)
            (sub_trans (Sub.step rfl Sub.refl) (Sub.step rfl Sub.refl))

example (b : Bool) : Sub (typeOf (Val.bool b)) Ty.scalar :=
  Sub.step rfl Sub.refl


/-! ## 13. Runnable summary -/

def check (label got expected : String) : IO Unit :=
  let mark := if got == expected then "ok" else "XX"
  IO.println s!"  [{mark}] {label}: {got}  (want {expected})"

def main : IO Unit := do
  IO.println "AQL core model — executable cross-checks (cf. `aql do`):"
  check "sub 3 10    (prefix) " (render (run [Item.word Word.sub, ilit 3, ilit 10])) "7"
  check "10 sub 3    (infix)  " (render (run [ilit 10, Item.word Word.sub, ilit 3])) "7"
  check "10 3 sub    (postfix)" (render (run [ilit 10, ilit 3, Item.word Word.sub])) "7"
  check "add 1 2 7   (leftover)" (render (run [Item.word Word.add, ilit 1, ilit 2, ilit 7])) "3 7"
  check "gt 5 3               " (render (run [Item.word Word.gtw, ilit 5, ilit 3])) "false"
  check "not true            " (render (run [Item.word Word.notw, blit true])) "false"
  check "10 sub end 3 (barrier)" (render (run [ilit 10, Item.word Word.sub, Item.endb, ilit 3])) "ERROR"
  IO.println "Proven: spelling_equiv, sub_spellings, unary_spellings,"
  IO.println "        binary_caps_and_leaves_leftover, barrier_blocks_forward,"
  IO.println "        run_deterministic, sub_trans (+ lattice examples)."

end AqlCore
