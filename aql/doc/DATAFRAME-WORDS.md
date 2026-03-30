# Dataframe Operations Word Design for AQL

## Context

AQL has a full SQL-like query system (`select/from/where/order/group/join/etc.`) that builds `QueryBuilder` objects and executes via SQLite. This design provides a **complementary** set of dataframe-style words that operate directly on table values on the stack in AQL's concatenative style. These are separate from the select-style words and intended for interactive data exploration and transformation pipelines.

Tables in AQL are `TList` values with `TableData` payload (schema + rows). Records are schema definitions (`RecordTypeInfo` with ordered field map). The new words operate on these types directly.

---

## Word Design

Sample data for all examples:
```
set people ("file/people.csv" read)
# name  age   city    sales
# Ana   29    Dublin  100
# Ben   34    Cork    200
# Cara  29    Dublin  150
# Dan   NA    Galway  120
# Ben   34    Cork    200
```

### 1. View Data

#### `head` (NEW)
First N rows of a table. Default 5.
```
Signatures:
  [table integer] -> [table]
  [table]         -> [table]

Examples:
  people head 3          # first 3 rows
  people head            # first 5 rows (entire table if <= 5)
```

#### `tail` (NEW)
Last N rows. Default 5.
```
Signatures:
  [table integer] -> [table]
  [table]         -> [table]

Examples:
  people tail 2          # last 2 rows (Dan, Ben)
```

#### `shape` (NEW)
Returns row count and column count.
```
Signature:
  [table] -> [integer integer]

Example:
  people shape           # 5 4
```

#### `cols` (NEW)
Column names as a list of atoms.
```
Signature:
  [table] -> [list]

Example:
  people cols            # [name, age, city, sales]
```

#### `nrow` (NEW)
Row count of a table.
```
Signature:
  [table] -> [integer]

Example:
  people nrow            # 5
```

#### `ncol` (NEW)
Column count.
```
Signature:
  [table] -> [integer]

Example:
  people ncol            # 4
```

#### `describe` (NEW)
Summary statistics for numeric columns. Returns a table with rows: count, mean, min, max, sum for each numeric column.
```
Signature:
  [table] -> [table]

Example:
  people describe
  # stat   age    sales
  # count  4      5
  # mean   31.5   154
  # min    29     100
  # max    34     200
  # sum    126    770
```

---

### 2. Select Columns

#### `col` (NEW)
Extract a single column as a list of values.
```
Signatures:
  [table atom]   -> [list]
  [table string] -> [list]

Examples:
  people col name        # [Ana, Ben, Cara, Dan, Ben]
  people col sales       # [100, 200, 150, 120, 200]
```

#### `pick` (NEW)
Keep only the specified columns.
```
Signature:
  [table list] -> [table]

Example:
  people pick [name city]
  # name  city
  # Ana   Dublin
  # Ben   Cork
  # ...
```

#### `omit` (NEW)
Drop specified columns.
```
Signatures:
  [table list] -> [table]
  [table atom] -> [table]

Examples:
  people omit [age]      # table without age column
  people omit age        # same, single column shorthand
```

---

### 3. Filter Rows

#### `sift` (NEW)
Filter rows by condition. Accepts a map pattern (field → value/predicate) or a quoted function body.
```
Signatures:
  [table map]  -> [table]     # pattern match
  [table list] -> [table]     # quoted predicate per row

Examples:
  # By exact value
  people sift {city:"Dublin"}
  # name  age  city    sales
  # Ana   29   Dublin  100
  # Cara  29   Dublin  150

  # By comparison (using paren expressions in maps)
  people sift {age:(gt 30)}
  # Ben  34  Cork  200
  # Ben  34  Cork  200

  # By missing value
  people sift {age:none}
  # Dan  NA  Galway  120

  # By predicate function (row map is on stack)
  people sift [var [[r] r.sales gt 100]]
  # Ben   34  Cork    200
  # Cara  29  Dublin  150
  # Dan   NA  Galway  120
  # Ben   34  Cork    200
```

---

### 4. Add / Modify Columns

#### `mutate` (NEW)
Add or modify columns. Takes a map where keys are column names and values are either literal values or quoted code that receives each row.
```
Signature:
  [table map] -> [table]

Examples:
  # Add a computed column
  people mutate {tax:(sales mul 0.1)}
  # name  age  city    sales  tax
  # Ana   29   Dublin  100    10
  # ...

  # Modify existing column
  people mutate {age:(age add 1)}
  # ages become 30, 35, 30, NA, 35
```

#### `rename` (NEW)
Rename columns. Map of old name → new name.
```
Signature:
  [table map] -> [table]

Example:
  people rename {sales:revenue}
  # column "sales" becomes "revenue"
```

---

### 5. Sort Data

#### `sortby` (NEW)
Sort table by one or more columns. Atoms sort ascending; `[col desc]` pairs for descending.
```
Signatures:
  [table atom] -> [table]          # single column ascending
  [table list] -> [table]          # multiple columns, with optional direction

Examples:
  people sortby age
  # sorted by age ascending (NA last)

  people sortby [sales desc]
  # Ben   34  Cork    200
  # Ben   34  Cork    200
  # Cara  29  Dublin  150
  # Dan   NA  Galway  120
  # Ana   29  Dublin  100

  people sortby [[city asc] [sales desc]]
  # multi-column sort
```

---

### 6. Aggregation

#### `sum` (NEW)
Sum of a numeric list or a table column.
```
Signatures:
  [list]       -> [number]     # sum elements of a list
  [table atom] -> [number]     # sum of named column

Examples:
  people col sales sum         # 770  (via list signature)
  people sum sales             # 770  (via table signature)
```

#### `mean` (NEW)
Arithmetic mean. Skips none/NA values.
```
Signatures:
  [list]       -> [number]
  [table atom] -> [number]

Examples:
  people col age mean          # 31.5
  people mean age              # 31.5
```

#### `count` (NEW signature on existing query-internal word)
Count rows, or count non-missing values in a column.
```
Signatures:
  [table]      -> [integer]    # total rows
  [table atom] -> [integer]    # non-NA count for column

Examples:
  people count                 # 5
  people count age             # 4  (Dan's age is NA)
```

#### `min` / `max` (EXTEND existing words)
Add table-column signatures.
```
New signatures:
  [table atom] -> [number]

Examples:
  people min age               # 29
  people max sales             # 200
```

---

### 7. Group By + Aggregate

#### `groupby` (NEW)
Group rows by column(s) and aggregate. Takes column(s) to group by and a map of aggregation specs.
```
Signatures:
  [table atom map]  -> [table]    # group by single column
  [table list map]  -> [table]    # group by multiple columns

Examples:
  people groupby city {total_sales:(sum sales)}
  # city    total_sales
  # Cork    400
  # Dublin  250
  # Galway  120

  people groupby city {n:(count)}
  # city    n
  # Cork    2
  # Dublin  2
  # Galway  1

  people groupby city {total:(sum sales) avg_age:(mean age)}
  # city    total  avg_age
  # Cork    400    34
  # Dublin  250    29
  # Galway  120    NA
```

---

### 8. Join / Merge

#### `merge` (NEW)
Join two tables. Default is left join on shared column names.
```
Signatures:
  [table table map]  -> [table]    # with options {on:col, how:left|inner|...}
  [table table atom] -> [table]    # join on named column

Examples:
  set regions {Dublin:Leinster, Cork:Munster, Galway:Connacht}
  # Assume regions is a table with city, region columns

  people merge regions city
  # name  age  city    sales  region
  # Ana   29   Dublin  100    Leinster
  # ...

  people merge regions {on:city how:left}
```

---

### 9. Combine Tables

#### `stack` (NEW)
Vertically concatenate tables with matching schemas.
```
Signature:
  [table table] -> [table]

Example:
  set extras ({name:"Eva" age:31 city:"Limerick" sales:180})
  people stack extras
  # original 5 rows + Eva row = 6 rows
```

---

### 10. Missing Data

#### `dropna` (NEW)
Drop rows with any missing values, or missing in specific column(s).
```
Signatures:
  [table]      -> [table]     # drop rows with any NA
  [table atom] -> [table]     # drop rows where column is NA
  [table list] -> [table]     # drop rows where any listed column is NA

Examples:
  people dropna              # removes Dan row
  people dropna age          # removes Dan row (age is NA)
```

#### `fillna` (NEW)
Fill missing values in specified columns.
```
Signature:
  [table map] -> [table]     # map of column → fill value

Example:
  people fillna {age:0}
  # Dan's age becomes 0
```

---

### 11. Duplicates

#### `dedup` (NEW)
Remove duplicate rows (keeps first occurrence).
```
Signatures:
  [table]      -> [table]     # deduplicate on all columns
  [table list] -> [table]     # deduplicate on specified columns

Examples:
  people dedup
  # removes second Ben row

  people dedup [name city]
  # deduplicate based on name+city combination
```

#### `dupes` (NEW)
Return only the duplicate rows.
```
Signatures:
  [table]      -> [table]
  [table list] -> [table]

Example:
  people dupes
  # Ben  34  Cork  200   (the duplicate row)
```

---

### 12. Reshape

#### `melt` (NEW)
Convert wide format to long format (unpivot).
```
Signature:
  [table list map] -> [table]   # id_cols, {var:varname val:valname}

Example:
  # Given wide table: name, Jan, Feb
  wide melt [name] {var:month val:sales}
  # name  month  sales
  # Ana   Jan    10
  # Ben   Jan    20
  # Ana   Feb    15
  # Ben   Feb    25
```

#### `pivot` (NEW)
Convert long format to wide format.
```
Signature:
  [table atom atom atom] -> [table]   # index_col, var_col, val_col

Example:
  long pivot name month sales
  # name  Jan  Feb
  # Ana   10   15
  # Ben   20   25
```

---

### 13. Apply / Transform

#### `apply` (NEW)
Apply a function to each value in a column, producing a new column.
```
Signatures:
  [table atom list]      -> [table]   # column name, quoted code body
  [table atom atom list] -> [table]   # column, new_col_name, quoted code body

Examples:
  people apply age [add 1]
  # ages become 30, 35, 30, NA, 35

  people apply name name_lower [lower]
  # adds column name_lower with lowercased names

  people apply sales adult [ge 18]
  # adds boolean column "adult"
```

---

### 14. Row Access

#### `row` (NEW)
Access a specific row by index, returned as a map.
```
Signature:
  [table integer] -> [map]

Example:
  people row 0
  # {name:Ana, age:29, city:Dublin, sales:100}
```

#### `slice` (EXTEND existing word)
Add table signature for row slicing.
```
New signature:
  [table integer integer] -> [table]

Example:
  people slice 0 2
  # first 2 rows (Ana, Ben)
```

---

### 15. String/Date Column Operations

String and date operations on columns are handled via `apply`:

```
# Lowercase names
people apply name [lower]

# Filter names containing "a" (case insensitive)
people sift {name:(contains "a")}

# Extract year from date column
dates apply date year [slice 0 4]
# Or with a dedicated approach if date types are added later
```

---

## Word Summary Table

| Word | Status | Signatures | Category |
|------|--------|-----------|----------|
| `head` | NEW | `[table int] -> [table]`, `[table] -> [table]` | View |
| `tail` | NEW | `[table int] -> [table]`, `[table] -> [table]` | View |
| `shape` | NEW | `[table] -> [int int]` | View |
| `cols` | NEW | `[table] -> [list]` | View |
| `nrow` | NEW | `[table] -> [int]` | View |
| `ncol` | NEW | `[table] -> [int]` | View |
| `describe` | NEW | `[table] -> [table]` | View |
| `col` | NEW | `[table atom] -> [list]` | Columns |
| `pick` | NEW | `[table list] -> [table]` | Columns |
| `omit` | NEW | `[table list] -> [table]`, `[table atom] -> [table]` | Columns |
| `sift` | NEW | `[table map] -> [table]`, `[table list] -> [table]` | Filter |
| `mutate` | NEW | `[table map] -> [table]` | Modify |
| `rename` | NEW | `[table map] -> [table]` | Modify |
| `sortby` | NEW | `[table atom] -> [table]`, `[table list] -> [table]` | Sort |
| `sum` | NEW | `[list] -> [number]`, `[table atom] -> [number]` | Aggregate |
| `mean` | NEW | `[list] -> [number]`, `[table atom] -> [number]` | Aggregate |
| `count` | NEW | `[table] -> [int]`, `[table atom] -> [int]` | Aggregate |
| `min` | EXTEND | `[table atom] -> [number]` | Aggregate |
| `max` | EXTEND | `[table atom] -> [number]` | Aggregate |
| `groupby` | NEW | `[table atom map] -> [table]`, `[table list map] -> [table]` | Group |
| `merge` | NEW | `[table table atom] -> [table]`, `[table table map] -> [table]` | Join |
| `stack` | NEW | `[table table] -> [table]` | Combine |
| `dropna` | NEW | `[table] -> [table]`, `[table atom] -> [table]` | Missing |
| `fillna` | NEW | `[table map] -> [table]` | Missing |
| `dedup` | NEW | `[table] -> [table]`, `[table list] -> [table]` | Duplicates |
| `dupes` | NEW | `[table] -> [table]` | Duplicates |
| `melt` | NEW | `[table list map] -> [table]` | Reshape |
| `pivot` | NEW | `[table atom atom atom] -> [table]` | Reshape |
| `apply` | NEW | `[table atom list] -> [table]` | Transform |
| `row` | NEW | `[table int] -> [map]` | Access |
| `slice` | EXTEND | `[table int int] -> [table]` | Access |

**Total: 28 new words + 3 extended words = 31 word operations**

---

## Composable Workflow Examples

### Filter, aggregate, and sort
```aql
# Total sales > 100 by city, sorted descending
people sift {sales:(gt 100)} groupby city {total:(sum sales)} sortby [total desc]
# city    total
# Cork    400
# Dublin  150
# Galway  120
```

### Multi-step pipeline
```aql
set people ("file/people.csv" read)

# Clean: fill missing ages, remove dupes, add tax column
people fillna {age:0} dedup mutate {tax:(sales mul 0.1)}

# Inspect
people head 3
people describe
people shape
```

### Column extraction and aggregation
```aql
people col sales mean          # 154
people col age mean            # 31.5
people col city dedup          # [Dublin, Cork, Galway] as list
```

### Join and reshape
```aql
set regions ("file/regions.csv" read)
people merge regions city pick [name city region]
```

---

## Design Decisions

1. **Execution model**: Hybrid — simple operations (head, tail, col, pick, omit, row, shape, etc.) work in-memory on `TableData.Rows`. Heavy operations (groupby, merge, sortby, dedup, sift with complex conditions) leverage SQLite when beneficial via `Registry.SQLite.StoreTempTable()`.

2. **Filter syntax (sift)**: Both approaches supported:
   - Map patterns for equality/simple: `people sift {city:"Dublin"}` and `people sift {age:(gt 30)}`
   - List predicates for complex conditions: `people sift [age 30 gt]` (RPN-style)

3. **Row computation context (mutate/apply)**: Implicit field names — field names from the current row are temporarily defined as words during body evaluation. `people mutate {tax:(sales mul 0.1)}` — `sales` resolves to the current row's sales value. This is analogous to how `var` binds names.

---

## Implementation Notes

### Files to create
Each word (or word family) follows the existing pattern of one file per word:
- `aql/internal/engine/builtin_table_head.go`
- `aql/internal/engine/builtin_table_tail.go`
- `aql/internal/engine/builtin_table_shape.go`
- `aql/internal/engine/builtin_table_cols.go`
- `aql/internal/engine/builtin_table_nrow.go`
- `aql/internal/engine/builtin_table_describe.go`
- `aql/internal/engine/builtin_table_col.go`
- `aql/internal/engine/builtin_table_pick.go`
- `aql/internal/engine/builtin_table_omit.go`
- `aql/internal/engine/builtin_table_sift.go`
- `aql/internal/engine/builtin_table_mutate.go`
- `aql/internal/engine/builtin_table_rename.go`
- `aql/internal/engine/builtin_table_sortby.go`
- `aql/internal/engine/builtin_table_aggregate.go` (sum, mean, count, min, max)
- `aql/internal/engine/builtin_table_groupby.go`
- `aql/internal/engine/builtin_table_merge.go`
- `aql/internal/engine/builtin_table_stack.go`
- `aql/internal/engine/builtin_table_missing.go` (dropna, fillna)
- `aql/internal/engine/builtin_table_dedup.go` (dedup, dupes)
- `aql/internal/engine/builtin_table_reshape.go` (melt, pivot)
- `aql/internal/engine/builtin_table_apply.go`
- `aql/internal/engine/builtin_table_row.go`

### Files to modify
- `aql/internal/engine/registry.go` - register all new words in `NewRegistry()`
- `aql/internal/engine/builtin_string_slice.go` - extend slice with table signature
- `aql/internal/engine/builtin_math_*.go` - extend min/max with table-column signatures

### Key existing code to reuse
- `TableData` struct (`format.go:260`) - for table manipulation
- `RecordTypeInfo` (`value.go:84`) - for schema handling
- `NewOrderedMap()` / `OrderedMap` - for building result maps
- `newValue(TList, TableData{...})` - for creating table return values
- `r.Register()` pattern from `registry.go:128`
- Table → SQLite loading via `Registry.SQLite.StoreTempTable()` for complex operations

### Verification
- Add test file: `aql/test/dataframe_test.go` with test cases for each word
- Use existing CSV test data or create `test/testdata/people.csv`
- Run: `cd aql && go test ./... -run TestDataframe -v`
- Verify composition: test multi-word pipelines
- Verify type safety: test error cases (wrong types, missing columns)
