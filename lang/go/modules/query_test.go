package modules

import (
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/native"
)

// queryRegistry returns a registry with the aql:query module installed
// and a parse func wired, plus an in-memory "people" table registered in
// the context store.
func queryRegistry(t *testing.T) *native.Registry {
	t.Helper()
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	r.SetParseFunc(parser.Parse)
	if err := InstallQueryExports(r); err != nil {
		t.Fatal(err)
	}
	makePeopleTable(r)
	return r
}

// makePeopleTable registers a "people" table: name, age, city.
func makePeopleTable(r *native.Registry) {
	fields := native.NewOrderedMap()
	fields.Set("name", native.NewTypeLiteral(native.TString))
	fields.Set("age", native.NewTypeLiteral(native.TInteger))
	fields.Set("city", native.NewTypeLiteral(native.TString))
	rec := native.RecordTypeInfo{Fields: fields}

	mkRow := func(name string, age int64, city string) native.Value {
		om := native.NewOrderedMap()
		om.Set("name", native.NewString(name))
		om.Set("age", native.NewInteger(age))
		om.Set("city", native.NewString(city))
		return native.NewMap(om)
	}

	td := native.TableData{
		Record: rec,
		Rows: []native.Value{
			mkRow("Alice", 30, "London"),
			mkRow("Bob", 25, "Paris"),
			mkRow("Carol", 40, "London"),
			mkRow("Dave", 18, "Berlin"),
		},
	}
	r.ContextSet("people", native.NewValueRaw(native.TList, td))
}

// makeVisitsTable registers a "visits" table: who, place.
func makeVisitsTable(r *native.Registry) {
	fields := native.NewOrderedMap()
	fields.Set("who", native.NewTypeLiteral(native.TString))
	fields.Set("place", native.NewTypeLiteral(native.TString))
	rec := native.RecordTypeInfo{Fields: fields}

	mkRow := func(who, place string) native.Value {
		om := native.NewOrderedMap()
		om.Set("who", native.NewString(who))
		om.Set("place", native.NewString(place))
		return native.NewMap(om)
	}

	td := native.TableData{
		Record: rec,
		Rows: []native.Value{
			mkRow("Alice", "Museum"),
			mkRow("Bob", "Park"),
			mkRow("Zoe", "Cafe"), // no matching person
		},
	}
	r.ContextSet("visits", native.NewValueRaw(native.TList, td))
}

func runQuerySrc(t *testing.T, r *native.Registry, src string) ([]native.Value, error) {
	t.Helper()
	values, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return native.NewTop(r).Run(values)
}

// materialize coerces a query result Value to TableData. select
// returns an eagerly materialized table, so the payload is TableData
// directly.
func materialize(t *testing.T, v native.Value) native.TableData {
	t.Helper()
	if td, ok := v.Data.(native.TableData); ok {
		return td
	}
	t.Fatalf("expected materialized TableData, got %T", v.Data)
	return native.TableData{}
}

func rowCount(t *testing.T, src string) int {
	t.Helper()
	r := queryRegistry(t)
	result, err := runQuerySrc(t, r, src)
	if err != nil {
		t.Fatalf("%q: unexpected error: %v", src, err)
	}
	if len(result) != 1 {
		t.Fatalf("%q: expected 1 result, got %d", src, len(result))
	}
	return len(materialize(t, result[0]).Rows)
}

// --- Module structure ---

func TestQueryModuleExports(t *testing.T) {
	r, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := BuildQueryModule(r)
	if err != nil {
		t.Fatal(err)
	}
	exp, ok := desc.Exports["query"]
	if !ok {
		t.Fatal("missing 'query' export map")
	}
	for _, w := range []string{"from", "where", "select", "order", "group", "having", "limit", "offset", "distinct"} {
		if _, ok := exp.Get(w); !ok {
			t.Errorf("missing export query.%s", w)
		}
	}
}

// --- select * ---

func TestQuerySelectStar(t *testing.T) {
	if got := rowCount(t, `query.from people query.select []`); got != 4 {
		t.Errorf("select [] (all rows): expected 4, got %d", got)
	}
}

// --- where ---

func TestQueryWhereFilter(t *testing.T) {
	if got := rowCount(t, `query.from people query.where [age gt 25] query.select []`); got != 2 {
		t.Errorf("where age>25: expected 2 (Alice,Carol), got %d", got)
	}
}

// --- projection + alias ---

func TestQuerySelectColumns(t *testing.T) {
	r := queryRegistry(t)
	result, err := runQuerySrc(t, r, `query.from people query.select [name age]`)
	if err != nil {
		t.Fatal(err)
	}
	td := materialize(t, result[0])
	cols := td.Record.Fields.Keys()
	if len(cols) != 2 || cols[0] != "name" || cols[1] != "age" {
		t.Errorf("expected [name age], got %v", cols)
	}
}

// --- order + limit ---

func TestQueryOrderLimit(t *testing.T) {
	r := queryRegistry(t)
	result, err := runQuerySrc(t, r, `query.from people query.order [age desc] query.limit 2 query.select [name age]`)
	if err != nil {
		t.Fatal(err)
	}
	td := materialize(t, result[0])
	if len(td.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(td.Rows))
	}
	first, _ := native.AsMap(td.Rows[0])
	nameVal, _ := first.Get("name")
	if got, _ := native.AsString(nameVal); got != "Carol" {
		t.Errorf("expected top row Carol (age 40), got %q", got)
	}
}

// --- group + having ---

func TestQueryGroupHaving(t *testing.T) {
	r := queryRegistry(t)
	// Cities with more than one person: London (Alice, Carol).
	src := `query.from people query.group [city] query.having [cnt gt 1] query.select [city [count city cnt]]`
	result, err := runQuerySrc(t, r, src)
	if err != nil {
		t.Fatal(err)
	}
	td := materialize(t, result[0])
	if len(td.Rows) != 1 {
		t.Fatalf("expected 1 group (London), got %d", len(td.Rows))
	}
}

// --- distinct ---

func TestQueryDistinct(t *testing.T) {
	r := queryRegistry(t)
	result, err := runQuerySrc(t, r, `query.from people query.distinct query.select [city]`)
	if err != nil {
		t.Fatal(err)
	}
	td := materialize(t, result[0])
	if len(td.Rows) != 3 {
		t.Errorf("expected 3 distinct cities, got %d", len(td.Rows))
	}
}

// --- join ---

func TestQueryJoinOn(t *testing.T) {
	r := queryRegistry(t)
	makeVisitsTable(r)
	// people JOIN visits ON people.name = visits.who
	src := `query.from people query.join visits query.on [name eq who] query.select [name place]`
	result, err := runQuerySrc(t, r, src)
	if err != nil {
		t.Fatal(err)
	}
	td := materialize(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 joined rows (Alice, Bob), got %d", len(td.Rows))
	}
}

// --- set operation ---

func TestQueryUnion(t *testing.T) {
	r := queryRegistry(t)
	// Londoners UNION Berliners.
	src := `query.from people query.where [city eq 'London'] query.union (query.from people query.where [city eq 'Berlin']) query.select []`
	result, err := runQuerySrc(t, r, src)
	if err != nil {
		t.Fatal(err)
	}
	td := materialize(t, result[0])
	if len(td.Rows) != 3 {
		t.Errorf("expected 3 rows (Alice, Carol, Dave), got %d", len(td.Rows))
	}
}

// --- negative: unknown table ---

func TestQueryUnknownTable(t *testing.T) {
	r := queryRegistry(t)
	_, err := runQuerySrc(t, r, `query.from nonexistent query.select []`)
	if err == nil {
		t.Fatal("expected error for unknown table")
	}
}

// --- negative: select on a non-table ---

func TestQueryFromNonTable(t *testing.T) {
	r := queryRegistry(t)
	r.ContextSet("notable", native.NewInteger(42))
	_, err := runQuerySrc(t, r, `query.from notable query.select []`)
	if err == nil {
		t.Fatal("expected error when source is not a table")
	}
}

// --- destructuring DX: unpack lifts query words to bare names ---

// TestQueryUnpackBareWords is the headline DX use case: after importing
// aql:query, destructure the words so the whole pipeline reads like SQL with
// no `query.` prefix. The bound module wrappers preserve their inner natives'
// QuoteArgs (from's bare table name) and NoEvalArgs (the where/select clause
// lists) because InstallDef rebinds a trivial-delegation wrapper to the inner
// native's real signatures — exactly what dot-access dispatches against.
func TestQueryUnpackBareWords(t *testing.T) {
	r := queryRegistry(t)
	src := `unpack [from where select] query
	        from people where [age gt 25] select [name age]`
	result, err := runQuerySrc(t, r, src)
	if err != nil {
		t.Fatalf("bare-word pipeline failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	td := materialize(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("where age>25: expected 2 rows (Alice, Carol), got %d", len(td.Rows))
	}
	cols := td.Record.Fields.Keys()
	if len(cols) != 2 || cols[0] != "name" || cols[1] != "age" {
		t.Errorf("expected projection [name age], got %v", cols)
	}
}

// TestQueryUnpackClauseCoverage exercises a full bare-word pipeline across the
// clause families (order, group/having, distinct, aggregates) to confirm the
// rebinding preserves each inner native's arg-handling, not just where/select.
func TestQueryUnpackClauseCoverage(t *testing.T) {
	cases := []struct {
		name string
		src  string
		rows int
	}{
		{"order", `unpack [from where order select] query  from people where [city eq 'London'] order [age desc] select []`, 2},
		{"group-having", `unpack [from group having select] query  from people group [city] having [cnt gt 1] select [city [count city cnt]]`, 1},
		{"distinct", `unpack [from distinct select] query  from people distinct select [city]`, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := queryRegistry(t)
			result, err := runQuerySrc(t, r, c.src)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			if got := len(materialize(t, result[0]).Rows); got != c.rows {
				t.Errorf("%s: expected %d rows, got %d", c.name, c.rows, got)
			}
		})
	}
}

// TestQueryUnpackRename confirms a wrapper rebound under a DIFFERENT name
// (def w query.where) still dispatches via the inner native, including its
// NoEvalArgs — the body word names the original inner native to look up.
func TestQueryUnpackRename(t *testing.T) {
	r := queryRegistry(t)
	// fr is from under a new name; the clause words stay bare via unpack.
	src := `def fr query.from  unpack [where select] query  fr people where [age gt 25] select []`
	result, err := runQuerySrc(t, r, src)
	if err != nil {
		t.Fatalf("rename-alias pipeline failed: %v", err)
	}
	if got := len(materialize(t, result[0]).Rows); got != 2 {
		t.Errorf("expected 2 rows, got %d", got)
	}
}
