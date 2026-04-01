# AQL Type Hierarchy

Every AQL value carries a hierarchical type path (e.g. `Scalar/String/Proper`).
A child type matches its parent: `Scalar/String/Proper` matches `Scalar/String`
matches `Scalar`. A parent does NOT match a child.

## Type Tree

```
Any                          -- matches everything
None                         -- bottom type, matches nothing

Scalar
  String
    Proper                   -- non-empty string
    Empty                    -- empty string ""
  Number
    Integer                  -- int64; literal subtypes e.g. Integer/42
    Decimal                  -- float64
  Boolean                    -- true / false
  Atom                       -- bare unquoted word used as data

Node
  List                       -- ordered sequence of values
    Args                     -- argument list (internal)
  Map                        -- ordered key-value pairs
    Options                  -- map with defaults/constraints
    Inspect                  -- inspection result (word or type)

Word
  Function                   -- callable function reference
  __IN                       -- internal native function
  __FW                       -- forward reference (deferred call)
  __OP                       -- open paren marker
  __PE                       -- paren expression
  __FN                       -- function definition (def/fn body)
  __UF                       -- undef spec
  __RC                       -- return type check marker
  __DJ                       -- disjunction (union type)
  __MK                       -- mark (loop/control anchor)
  __MV                       -- move (jump to mark)
  __MD                       -- module descriptor

Object                       -- mutable typed instances
  Store                      -- mutable key-value store with prototype chain
    System                   -- system configuration store
  Array                      -- mutable ordered array
  Error                      -- error value
  Table                      -- list of typed records (SQL-backed)
  Record                     -- typed field schema
  Fetch                      -- HTTP fetch operation
    Request                  -- fetch request
    Response                 -- fetch response
  Resource                   -- external resource
    Entity                   -- resource entity
  [User-defined]             -- created via `object` word

Type                         -- metatypes (types of types)
  ScalarType                 -- metatype for Scalar subtypes
  NodeType                   -- metatype for Node subtypes
```

## Mutability

- **Nodes** (Map, List) are immutable values.
- **Objects** (Store, Record instances, user-defined) are mutable instances.
- **Scalars** are immutable values.

## Store

`Object/Store` is a special mutable key-value store. Unlike regular Objects
which have typed fields, Stores hold arbitrary key-value pairs. Key resolution
walks the prototype chain, enabling scope-like lookup when contexts are nested.

The execution context is a Store. The `context` word pushes the current
context Store onto the stack for use with `get` and `set`.

## ID Prefixes

Every value carries a unique ID: `<prefix>_<12 hex chars>`.

| Prefix | Category |
|--------|----------|
| `S_`   | Scalar (strings, numbers, booleans, atoms) |
| `N_`   | Node (lists, maps, errors) |
| `W_`   | Word (functions, control markers) |
| `T_`   | Type/Object (type literals, objects, stores, Any, None) |

## Type Matching

- `Any` matches everything.
- A child matches a parent: `Scalar/String/Proper` matches `Scalar/String`.
- A parent does NOT match a child: `Scalar/String` does not match `Scalar/String/Proper`.
- Signature matching uses `Matches()` for type compatibility.

## Short Name Expansion

Type names are auto-expanded when creating types:

| Short Name | Expands To |
|-----------|------------|
| `String`  | `Scalar/String` |
| `Number`  | `Scalar/Number` |
| `Integer` | `Scalar/Number/Integer` |
| `Decimal` | `Scalar/Number/Decimal` |
| `Boolean` | `Scalar/Boolean` |
| `Atom`    | `Scalar/Atom` |
| `List`    | `Node/List` |
| `Map`     | `Node/Map` |
| `Store`   | `Object/Store` |
| `Table`   | `Object/Table` |
| `Record`  | `Object/Record` |
| `Resource` | `Object/Resource` |
| `Entity`  | `Object/Resource/Entity` |
| `Function` | `Word/Function` |

## Metatypes

The `Type` hierarchy classifies types themselves:

- `Type/ScalarType` — metatype for Scalar subtypes (depth > 1)
- `Type/NodeType` — metatype for Node subtypes (depth > 1)
- `Type` — metatype for everything else (roots, Object types)
