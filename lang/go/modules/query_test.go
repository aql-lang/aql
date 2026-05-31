package modules

import (
	"strings"
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

// materialize forces a query result Value to concrete TableData. The
// query is lazy (a MaterializerPayload), so it runs here; an already
// concrete TableData is returned directly.
func materialize(t *testing.T, v native.Value) native.TableData {
	t.Helper()
	if mp, ok := v.Data.(native.MaterializerPayload); ok {
		td, err := mp.M.Materialize()
		if err != nil {
			t.Fatalf("materialize: %v", err)
		}
		return td
	}
	if td, ok := v.Data.(native.TableData); ok {
		return td
	}
	t.Fatalf("expected a lazy query or TableData, got %T", v.Data)
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
	if got := rowCount(t, `query.select [] query.from people`); got != 4 {
		t.Errorf("select [] (all rows): expected 4, got %d", got)
	}
}

// --- where ---

func TestQueryWhereFilter(t *testing.T) {
	if got := rowCount(t, `query.select [] query.from people query.where [age gt 25]`); got != 2 {
		t.Errorf("where age>25: expected 2 (Alice,Carol), got %d", got)
	}
}

// --- projection + alias ---

func TestQuerySelectColumns(t *testing.T) {
	r := queryRegistry(t)
	result, err := runQuerySrc(t, r, `query.select [name age] query.from people`)
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
	result, err := runQuerySrc(t, r, `query.select [name age] query.from people query.order [age desc] query.limit 2`)
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
	src := `query.select [city [count city cnt]] query.from people query.group [city] query.having [cnt gt 1]`
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
	result, err := runQuerySrc(t, r, `query.select [city] query.from people query.distinct`)
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
	src := `query.select [name place] query.from people query.join visits query.on [name eq who]`
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
	src := `query.select [] query.from people query.where [city eq 'London'] query.union (query.select [] query.from people query.where [city eq 'Berlin'])`
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
	_, err := runQuerySrc(t, r, `query.select [] query.from nonexistent`)
	if err == nil {
		t.Fatal("expected error for unknown table")
	}
}

// --- negative: select on a non-table ---

func TestQueryFromNonTable(t *testing.T) {
	r := queryRegistry(t)
	r.ContextSet("notable", native.NewInteger(42))
	_, err := runQuerySrc(t, r, `query.select [] query.from notable`)
	if err == nil {
		t.Fatal("expected error when source is not a table")
	}
}

// --- lazy materialization ---

// TestQueryLazyPrintsAsTable confirms a select-first query is lazy: the
// value carries a query, and it renders as a formatted table when its
// String() is taken (the print path), with no explicit run word.
func TestQueryLazyPrintsAsTable(t *testing.T) {
	r := queryRegistry(t)
	result, err := runQuerySrc(t, r, `query.select [name] query.from people query.where [age gt 35]`)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result[0].Data.(native.MaterializerPayload); !ok {
		t.Fatalf("expected a lazy MaterializerPayload, got %T", result[0].Data)
	}
	rendered := result[0].String()
	if !strings.Contains(rendered, "Carol") {
		t.Errorf("lazy query did not render its row: %q", rendered)
	}
}

// TestQueryNoFromErrors confirms `from` is required: a select with no
// from errors when it is materialized.
func TestQueryNoFromErrors(t *testing.T) {
	r := queryRegistry(t)
	result, err := runQuerySrc(t, r, `query.select [name]`)
	if err != nil {
		t.Fatalf("seeding select should not error eagerly: %v", err)
	}
	mp, ok := result[0].Data.(native.MaterializerPayload)
	if !ok {
		t.Fatalf("expected a lazy query, got %T", result[0].Data)
	}
	if _, mErr := mp.M.Materialize(); mErr == nil {
		t.Fatal("expected a 'no FROM' error when materializing a sourceless select")
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
	src := `unpack [select from where] query
	        select [name age] from people where [age gt 25]`
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
		{"order", `unpack [select from where order] query  select [] from people where [city eq 'London'] order [age desc]`, 2},
		{"group-having", `unpack [select from group having] query  select [city [count city cnt]] from people group [city] having [cnt gt 1]`, 1},
		{"distinct", `unpack [select from distinct] query  select [city] from people distinct`, 3},
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
	src := `def fr query.from  unpack [select where] query  select [] fr people where [age gt 25]`
	result, err := runQuerySrc(t, r, src)
	if err != nil {
		t.Fatalf("rename-alias pipeline failed: %v", err)
	}
	if got := len(materialize(t, result[0]).Rows); got != 2 {
		t.Errorf("expected 2 rows, got %d", got)
	}
}
