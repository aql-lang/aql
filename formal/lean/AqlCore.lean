/-
  AqlCore.lean — a mechanized prototype of AQL's core ideas.

  This is the milestone-6 seed from `design/FORMAL-VERIFICATION.0.md`:
  a *deep embedding* of a tractable fragment of the AQL abstract machine
  (`FORMAL-SPEC.md` §4 + §6), built to put AQL's two most distinctive
  claims under a proof checker rather than a test runner:

    1. forward collection — a word takes its arguments from BOTH the
       tokens to its right and the values already on the stack; and
    2. source-spelling equivalence — `y x op`, `y op x`, and `op x y`
       all compute the same result (FORMAL-SPEC §6.4).

  Everything below type-checks under Lean 4.15.0 with no Mathlib
  dependency (only the bundled core/Std). The `#eval` lines and `main`
  also let the file *run*, so the same artifact both proves and computes.

  Scope, honestly stated. This fragment covers integer literals, binary
  words (add/sub/mul), the `end` barrier, the stack, and forward
  collection up to arity two. It deliberately omits: user functions,
  modules, effects, concurrency, floats/overflow, and refinement
  predicates. Those are the later milestones; this is the substrate.
-/

namespace AqlCore

/-! ## 1. Syntax — the abstract machine's tape (FORMAL-SPEC §4) -/

/-- Binary words. `sub` is asymmetric, which is what makes the
    spelling-equivalence theorem non-trivial. -/
inductive Op
  | add | sub | mul
  deriving DecidableEq, Repr

/-- Tape items: a literal, a word call, or the `end` barrier (§6.6). -/
inductive Item
  | lit  : Int → Item
  | word : Op → Item
  | endb : Item            -- the `end` / `;` barrier
  deriving DecidableEq, Repr

abbrev Tape  := List Item
/-- Data stack, head = top (matches FORMAL-SPEC's `S · v` with top at the right,
    read here left-to-right with the top at the head). -/
abbrev Stack := List Int

/-- Word denotation on the collected argument vector `(args[0], args[1])`.
    `sub` is `args[1] - args[0]`, which is exactly what reproduces the
    engine's `10 sub 3 = 7` with `args[0]=3, args[1]=10`. -/
def applyOp : Op → Int → Int → Int
  | Op.add, x, y => x + y
  | Op.sub, x, y => y - x
  | Op.mul, x, y => x * y

/-! ## 2. Forward collection (FORMAL-SPEC §6.4)

Forward collection scans the tokens to the right of a word and gathers
up to `n` leading literal values, STOPPING at the first non-literal
(another word, or the `end` barrier) or end of tape. This single
function is the formal heart of AQL's surface syntax. -/

def takeForward : Tape → Nat → (List Int × Tape)
  | t, 0 => ([], t)
  | (Item.lit n :: rest), (k+1) =>
      let r := takeForward rest k
      (n :: r.1, r.2)
  | t, _+1 => ([], t)      -- head is a word/`end`/empty: collection stops

/-! ## 3. Evaluation (FORMAL-SPEC §6)

Small-step intent, packaged as a fuel-bounded function so it is total and
computable. At a word we collect forward first (`args[0]`, then
`args[1]`), then fill any remaining slots from the stack, top first —
precisely the §6.4 rule. The same `applyOp op a b` is reached regardless
of how the two arguments were sourced. -/

def eval : Nat → Tape → Stack → Option Stack
  | 0,    _,                     _ => none                 -- out of fuel
  | _+1,  [],                    s => some s               -- done
  | f+1,  (Item.lit n  :: rest), s => eval f rest (n :: s) -- push literal
  | f+1,  (Item.endb   :: rest), s => eval f rest s        -- §6.6: inert when hit directly
  | f+1,  (Item.word op :: rest), s =>
      match takeForward rest 2, s with
      -- two forward args: stack untouched
      | ([a0, a1], rest'),          _              => eval f rest' (applyOp op a0 a1 :: s)
      -- one forward, one from the stack top
      | ([a0],     rest'), (b :: s')               => eval f rest' (applyOp op a0 b  :: s')
      -- none forward, two from the stack (top first)
      | ([],       rest'), (a0 :: b :: s')         => eval f rest' (applyOp op a0 b  :: s')
      -- anything else under-supplies the word: stuck (a signature error)
      | _,                 _                       => none

/-- Run a closed tape and read the top of the final stack. -/
def run (t : Tape) : Option Int :=
  match eval (t.length + 8) t [] with
  | some (r :: _) => some r
  | _             => none

/-! ## 4. Executable sanity (cross-validation against the real engine)

These mirror `aql do '...'`. The real `aql` binary returns 7 for all
three `sub` spellings and a signature error for `10 sub end 3`; the model
agrees (see `#eval`s below and the theorems in §5/§6). -/

-- `sub 3 10`  (prefix / all-forward)
#eval run [Item.word Op.sub, Item.lit 3, Item.lit 10]    -- some 7
-- `10 sub 3`  (infix / one forward, one stack)
#eval run [Item.lit 10, Item.word Op.sub, Item.lit 3]    -- some 7
-- `10 3 sub`  (postfix / all stack)
#eval run [Item.lit 10, Item.lit 3, Item.word Op.sub]    -- some 7
-- `10 sub end 3`  (barrier blocks the forward `3` ⇒ under-supplied)
#eval run [Item.lit 10, Item.word Op.sub, Item.endb, Item.lit 3]  -- none


/-! ## 5. Headline theorem — source-spelling equivalence

The three spellings of any binary word over any two operands collect the
SAME argument vector and therefore compute the same value. In the spec
and the TSV suite this is enumerated by example; here it is one theorem,
universally quantified over the word and both operands. -/

theorem spelling_equiv (op : Op) (x y : Int) :
    -- postfix `y x op`
    run [Item.lit y, Item.lit x, Item.word op] = some (applyOp op x y)
  ∧ -- infix `y op x`
    run [Item.lit y, Item.word op, Item.lit x] = some (applyOp op x y)
  ∧ -- prefix `op x y`
    run [Item.word op, Item.lit x, Item.lit y] = some (applyOp op x y) := by
  refine ⟨?_, ?_, ?_⟩ <;> rfl

/-- Corollary in the engine's own words: all three spellings agree with
    each other (not merely with the model's normal form). -/
theorem spelling_agree (op : Op) (x y : Int) :
    run [Item.lit y, Item.lit x, Item.word op]
      = run [Item.lit y, Item.word op, Item.lit x]
  ∧ run [Item.lit y, Item.word op, Item.lit x]
      = run [Item.word op, Item.lit x, Item.lit y] := by
  obtain ⟨h1, h2, h3⟩ := spelling_equiv op x y
  exact ⟨h1.trans h2.symm, h2.trans h3.symm⟩


/-! ## 6. The `end` barrier changes meaning (FORMAL-SPEC §6.6)

A barrier between a word and a following literal stops forward
collection, so the word can no longer reach that operand. The engine
reports a signature error; the model gets stuck (`none`). This is a
*negative* theorem — it asserts what must be rejected. -/

theorem barrier_blocks_forward :
    run [Item.lit 10, Item.word Op.sub, Item.lit 3] = some 7
  ∧ run [Item.lit 10, Item.word Op.sub, Item.endb, Item.lit 3] = none := by
  constructor <;> rfl


/-! ## 7. Determinism (FORMAL-SPEC §6, modulo §7.4 concurrency)

`run` is a function, so evaluation is deterministic by construction. We
record it explicitly because for the real engine — with forward
collection and first-match dispatch — determinism is not visually
obvious. -/

theorem run_deterministic (t : Tape) (a b : Int) :
    run t = some a → run t = some b → a = b := by
  intro ha hb
  have : some a = some b := ha.symm.trans hb
  exact Option.some.inj this


/-! ## 8. A slice of Half A — the type lattice and subtyping
       (FORMAL-SPEC §5.1)

The `⊑` relation of the spec, as the reflexive-transitive closure of the
parent edges, with reflexivity and transitivity proved and a worked
`Integer ⊑ Any`. This is the seed of the type-soundness work: the same
relation a verified checker must decide. -/

inductive Ty
  | any | scalar | number | integer | float | boolean
  deriving DecidableEq, Repr

/-- The immediate parent edge of the hierarchy (FORMAL-SPEC §5.1). -/
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

/-- Everything is a subtype of `Any` (the spec's top). -/
example : Sub Ty.integer Ty.any :=
  Sub.step rfl (Sub.step rfl (Sub.step rfl Sub.refl))

/-- `Integer ⊑ Number`, and hence by transitivity `Integer ⊑ Any`. -/
example : Sub Ty.integer Ty.any :=
  sub_trans (Sub.step rfl Sub.refl)                 -- Integer ⊑ Number
            (sub_trans (Sub.step rfl Sub.refl)      -- Number  ⊑ Scalar
                       (Sub.step rfl Sub.refl))     -- Scalar  ⊑ Any


/-! ## 9. Runnable summary -/

def check (label : String) (got expected : Option Int) : IO Unit :=
  let mark := if got == expected then "ok" else "XX"
  IO.println s!"  [{mark}] {label}: {repr got} (want {repr expected})"

def main : IO Unit := do
  IO.println "AQL core model — executable cross-checks (cf. `aql do`):"
  check "sub 3 10   (prefix) " (run [Item.word Op.sub, Item.lit 3, Item.lit 10]) (some 7)
  check "10 sub 3   (infix)  " (run [Item.lit 10, Item.word Op.sub, Item.lit 3]) (some 7)
  check "10 3 sub   (postfix)" (run [Item.lit 10, Item.lit 3, Item.word Op.sub]) (some 7)
  check "add 1 2             " (run [Item.word Op.add, Item.lit 1, Item.lit 2]) (some 3)
  check "mul 4 5             " (run [Item.word Op.mul, Item.lit 4, Item.lit 5]) (some 20)
  check "10 sub end 3 (barrier)" (run [Item.lit 10, Item.word Op.sub, Item.endb, Item.lit 3]) none
  IO.println "Proven theorems: spelling_equiv, spelling_agree, barrier_blocks_forward,"
  IO.println "                 run_deterministic, sub_trans (+ Integer ⊑ Any)."

end AqlCore

/-- Top-level entry point so `lean --run AqlCore.lean` executes the checks. -/
def main : IO Unit := AqlCore.main
