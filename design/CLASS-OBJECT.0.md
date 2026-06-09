# Class / Object Split — Container Symmetry and the `class` Word

Status: **decisions accepted 2026-06-09** (language owner, via design
review; revised same day — no aliases, paren-free definition forms,
flat instances); **implementation pending**. ADR-005 records the
decision summary; this document owns the design and plan.

## 1. Decisions

1. **Container symmetry.** The 2×2: `List : Array :: Map : Object` —
   immutable value vs mutable container, indexed vs keyed. `Object`
   becomes a core, *constructible*, *enumerable*, *mutable* keyed
   container — the keyed sibling of `Array`. **Store stays a separate
   surface type** (scoped copy-on-write context semantics).
2. **`class` defines, `refine` extends.**
   `def Foo class {…}` defines a root class (no parens — the same
   nested-collection shape as `def name fn […]`).
   `def Bar refine Foo {…}` defines a subclass. `refine` keeps its
   general meaning — *refine an existing type* — and a class is just
   one more refinable type; the scalar forms (`refine Integer`
   newtypes, predicate subsets) are untouched.
3. **No deprecated aliases.** `refine Object {…}` is **removed**, not
   aliased: it raises a loud error with a hint pointing at
   `class {…}`. Pre-1.0 clean break, carried in the README upgrade
   notes.
4. **Classes have no prototypes — just instances.** A class is a
   schema (fields + defaults + parent class). `make` resolves the
   full default set **eagerly, flat into the instance** by walking
   the class chain at construction time. Instances carry one flat
   field map: no `Prototype` link, no delegation at `get`. (The
   context/Store scope chain is separate machinery and unaffected.)
5. **Sealed instances.** Writing an undeclared field on a class
   instance raises `[aql/sealed_field]` loudly. Open dynamic data
   belongs on plain Object.
6. **No prototype surface, no prototype dispatch.** Polymorphism
   stays with signature dispatch through the type lattice — one
   dispatch mechanism (the ADR-004 principle applied to dispatch).

## 2. Proposed surface, type by type

The contract column-wise: **immutable values return the updated copy
from `set`; mutable containers mutate in place and return nothing.**

### 2.1 Map — immutable, keyed (largely current behaviour)

```
def m {a:1, b:2}              # literal; computed key in a literal: {[k]: v}
m get a                       # returns 1      — bare key is literal (JS .key)
def k 'b'
m get (k)                     # returns 2      — parens = computed key (JS [k])
def m2 (m set c 3)            # returns {a:1 b:2 c:3} — NEW map (landed 2026-06-09)
m                             # returns {a:1 b:2}     — receiver untouched
{} set a 1 set b 2            # returns {a:1 b:2}     — copy-returning chains
"aql:struct-util" import end
StructUtil.items m            # returns [['a' 1] ['b' 2]]
m size                        # returns 2
m {a:1, b:2} deq              # returns true   — structural; eq is identity
```

### 2.2 List — immutable, indexed (one proposed addition)

```
def xs [1 2 3]
xs get 0                      # returns 1
xs push 4                     # returns [1 2 3 4]   — copy-returning (current)
"aql:array-util" import end
ArrayUtil.insert-at 1 99 xs   # returns [1 99 2 3]  — copy-returning (current)
ArrayUtil.remove-at 0 xs      # returns [2 3]       — copy-returning (current)

def ys (xs set 0 99)          # PROPOSED: returns [99 2 3], xs unchanged —
xs                            # returns [1 2 3]
                              # completes the column rule (today List set is
                              # a signature error). Open question §6.1, now
                              # recommended: yes, for symmetry with Map.
```

### 2.3 Object — mutable, keyed, open, fully enumerable (Phase B)

```
def o make Object {}          # empty open object — today an error (retires B5)
def o2 make Object {a:1}      # seeded from a map
o2 set b 2                    # in place; returns NOTHING — read o2 back
def k 'dyn'
o2 set (k) 3                  # computed key, in place
o2 get b                      # returns 2
o2 get (k)                    # returns 3
StructUtil.items o2           # returns [['a' 1] ['b' 2] ['dyn' 3]] — ALL
                              # fields enumerate (no invisible-dynamic-field
                              # class of bug)
o2 size                       # returns 3
def alias o2                  # bindings share the container (mutable column):
alias set a 9 end o2 get a    # returns 9
```

### 2.4 Array — mutable, indexed (constructor proposed)

```
def a make Array [1 2 3]      # PROPOSED constructor — today Array is
                              # host-side-only (§6.2)
a set 0 99                    # in place; returns NOTHING — bounds-checked
a get 0                       # returns 99
a size                        # returns 3
a set 5 1                     # ERROR — index out of bounds (current set rule)
```

### 2.5 Classes and instances

```
def Point class {x:0, y:0}            # root class: schema + defaults
def Point3 refine Point {z:0}         # subclass: inherits x,y; adds z

def p make Point3 {x:1}               # instance — defaults resolved FLAT at
                                      # make-time: p's own fields are
                                      # {x:1 y:0 z:0}; no prototype link
typeof p                              # returns Point3
p is Point                            # returns true  — nominal subtyping (lattice)
p get x                               # returns 1
p set y 5                             # in place (mutable column); returns nothing
p set w 9                             # ERROR [aql/sealed_field]: 'w' is not a
                                      #   field of Point3 (fields: x y z)
                                      #   hint: declare it in the class, or use
                                      #   a plain Object for open data
StructUtil.items p                    # returns [['x' 1] ['y' 5] ['z' 0]] — flat,
                                      # fully enumerable

# Methods stay free functions dispatched on the nominal type:
def norm fn [[p:Point] [Float] [((p.x dup mul) add (p.y dup mul)) MathUtil.sqrt]]
def norm fn [[p:Point3] [Float] [ … ]]   # overload: subclass-specific dispatch

# The removed form fails loudly (no alias):
def Box refine Object {v:0}
# ERROR [aql/refine_error]: refine Object is no longer the class form
#   hint: def Box class {v:0}
```

### 2.6 Crossing the columns (proposed, small)

```
convert Map o2                # Object → Map   — freeze: immutable snapshot
convert Object m              # Map → Object   — thaw: fresh mutable container
```

One pair of `convert` overloads completes the story (today `convert`
already handles scalar conversions); deep vs shallow: **shallow**, to
match the rest of the copy semantics.

## 3. What this changes in the engine

- **`class {schema}`** mints a nominal lattice type carrying
  `ObjectTypeInfo`-style schema + defaults + parent — the machinery
  `refine Object` uses today, under the new word. `refine X` where X
  is a class type routes to the same minting with a parent.
- **`make <Class> {…}`** resolves defaults eagerly: walk the class
  parent chain once, lay every field into a single flat `Fields` map,
  overlay the provided values, reject unknown keys (sealing applies
  from construction onward). `buildBasePrototype` and the instance
  `Prototype` walk in `GetField` are **deleted** for class instances —
  reads become a single map lookup. (`contextstack.go` keeps its own
  chain; it never depended on instance prototypes.)
- **`set` on a class instance** checks the field against the schema:
  declared → in-place write; undeclared → `[aql/sealed_field]` with
  the field list in the message.
- **Plain Object** instances are the same flat structure with no
  schema and no seal: any key writes, everything enumerates.
- **Enumeration** (`StructUtil.items`/`size`/`walk`) reads the flat
  field map — the dynamic-fields-invisible bug ceases to exist
  structurally rather than being patched.
- **`def Foo class {…}` parses with no parens** — the same
  nested-function-word collection that already makes
  `def name fn […]` work; `class` needs the same treatment in the
  forward collector as `fn`.

## 4. Interactions with open proposals

- **B5 (`make Object {}`)** — resolved by design in Phase B (becomes
  a value, not an error). The ERRORS.0.md §4 hint proposal stays
  superseded.
- **T9.1 side gap (non-enumerable dynamic fields)** — dies
  structurally with flat instances + open Objects.
- **`raise` (ERRORS.0.md §2)** — `sealed_field` and `refine_error`
  arrive through the normal native error path; no dependency.

## 5. Implementation phases

| Phase | Content | Breaking? |
|-------|---------|-----------|
| A | `class` word + `refine <Class>` subclassing + flat eager-default instances + sealing + **removal of `refine Object`** (loud `refine_error` with hint) | **yes** — one clean break, README upgrade note; all `refine Object` call sites rewrite to `class` |
| B | `make Object {…}` / `make Object {}` constructs plain open Objects; full enumeration; docs lead with the 2×2 table | no (turns an error into a value) |
| C | Column completion: List copy-returning `set`; `make Array […]`; `convert` freeze/thaw pair | no (additive) |

Phase A bundles sealing with the rewrite on purpose: every
`refine Object` site must be touched anyway, so the sealed contract
arrives exactly where code is already being edited — no separate
breaking wave. Standard discipline per phase: positive + negative
spec rows (sealed write, unknown-key `make`, removed-form error,
enumeration coverage), fnmodel golden regeneration, `describe` /
REFERENCE / TUTORIAL / README-upgrade-notes in the same change.

## 6. Open questions

1. **List `set`** — proposed yes (§2.2) to complete the column rule;
   confirm before Phase C.
2. **`make Array […]`** — proposed yes (§2.4); confirm before Phase C.
3. **`convert` freeze/thaw** (§2.6) — shallow proposed; confirm.
4. **Class introspection** — what `describe Point3` renders (schema,
   parent, defaults). Decide during Phase A from the existing
   ObjectTypeInfo rendering.
