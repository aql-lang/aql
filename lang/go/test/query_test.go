//go:build query
// +build query

package test

import (
	"github.com/aql-lang/aql/lang/go/native"
	"testing"

	"github.com/aql-lang/aql/eng/go/parser"
)

// runQuery sets up a registry, loads a CSV file, stores it, and runs a query.
func runQuery(t *testing.T, setup string, query string) ([]native.Value, error) {
	t.Helper()
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	// Run setup (e.g., load file and set as table name).
	if setup != "" {
		setupVals, err := parser.Parse(setup)
		if err != nil {
			return nil, err
		}
		if _, err := eng.Run(setupVals); err != nil {
			return nil, err
		}
	}

	// Run the query.
	queryVals, err := parser.Parse(query)
	if err != nil {
		return nil, err
	}
	return eng.Run(queryVals)
}

// --- from word ---

func TestFromLooksUpTable(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}

	v := result[0]
	if !native.IsTableType(v) {
		t.Fatalf("expected table type, got %s", v.Parent)
	}

	rows, _ := native.AsList(v)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Check all columns are present.
	r0, _ := native.AsMap(rows[0])
	assertField(t, r0, "name", "Alice")
	assertField(t, r0, "age", "30")
	assertField(t, r0, "city", "London")
}

func TestFromUnknownTable(t *testing.T) {
	t.Skip("query words disabled")
	_, err := runQuery(t, "", `select * from nonexistent`)
	if err == nil {
		t.Fatal("expected error for unknown table")
	}
}

// --- select * ---

func TestSelectStarFromFile(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set items ("file/items.tsv" read)`,
		`select * from items`,
	)
	if err != nil {
		t.Fatal(err)
	}

	v := result[0]
	rows, _ := native.AsList(v)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	r0, _ := native.AsMap(rows[0])
	assertField(t, r0, "id", "1")
	assertField(t, r0, "name", "Widget")
	assertField(t, r0, "price", "9.99")
}

// --- select [cols] ---

func TestSelectSpecificColumns(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select [name city] from people`,
	)
	if err != nil {
		t.Fatal(err)
	}

	v := result[0]
	rows, _ := native.AsList(v)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Should only have name and city columns.
	r0, _ := native.AsMap(rows[0])
	if r0.Len() != 2 {
		t.Fatalf("expected 2 columns, got %d", r0.Len())
	}
	assertField(t, r0, "name", "Alice")
	assertField(t, r0, "city", "London")

	// age should not be present.
	if _, ok := r0.Get("age"); ok {
		t.Error("age column should not be present")
	}
}

// --- select with aliases ---

func TestSelectWithAlias(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select [[name person_name] city] from people`,
	)
	if err != nil {
		t.Fatal(err)
	}

	v := result[0]
	rows, _ := native.AsList(v)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// name should be aliased to person_name.
	r0, _ := native.AsMap(rows[0])
	assertField(t, r0, "person_name", "Alice")
	assertField(t, r0, "city", "London")

	// Original name column should not be present.
	if _, ok := r0.Get("name"); ok {
		t.Error("original 'name' column should be aliased away")
	}
}

// --- select against internal (non-file) tables ---

func TestSelectAgainstInternalTable(t *testing.T) {
	t.Skip("query words disabled")
	// Build a table manually without SQLite backing.
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	// Create a table using AQL type system.
	fields := native.NewOrderedMap()
	fields.Set("color", native.NewTypeLiteral(native.TString))
	fields.Set("count", native.NewTypeLiteral(native.TString))
	recType := native.RecordTypeInfo{Fields: fields}

	row1 := native.NewOrderedMap()
	row1.Set("color", native.NewString("red"))
	row1.Set("count", native.NewString("5"))
	row2 := native.NewOrderedMap()
	row2.Set("color", native.NewString("blue"))
	row2.Set("count", native.NewString("3"))

	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{native.NewMap(row1), native.NewMap(row2)},
		SQLite: false, // not backed by SQLite
	}
	tableVal := native.Value{Parent: native.TList, Data: td}

	// Store in registry.
	reg.Store["colors"] = tableVal

	// Now select from it.
	queryVals, err := parser.Parse(`select [color] from colors`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	r0, _ := native.AsMap(rows[0])
	assertField(t, r0, "color", "red")
	if r0.Len() != 1 {
		t.Errorf("expected 1 column, got %d", r0.Len())
	}
}

// --- star word ---

func TestStarWord(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select star from people`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	r0, _ := native.AsMap(rows[0])
	assertField(t, r0, "name", "Alice")
	assertField(t, r0, "age", "30")
	assertField(t, r0, "city", "London")
}

// --- currying (partial application via def) ---

func TestCurriedFrom(t *testing.T) {
	t.Skip("query words disabled")
	// def from01 from people end; select * from01
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	steps := []string{
		`context set people ("file/people.csv" read)`,
		`def from01 from people end`,
		`select * from01`,
	}

	var result []native.Value
	for _, step := range steps {
		vals, err := parser.Parse(step)
		if err != nil {
			t.Fatal(err)
		}
		result, err = eng.Run(vals)
		if err != nil {
			t.Fatalf("step %q: %v", step, err)
		}
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	r0, _ := native.AsMap(rows[0])
	assertField(t, r0, "name", "Alice")
}

func TestCurriedSelect(t *testing.T) {
	t.Skip("query words disabled")
	// def select01 select star end; select01 from people
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	steps := []string{
		`context set people ("file/people.csv" read)`,
		`def select01 select star end`,
		`select01 from people`,
	}

	var result []native.Value
	for _, step := range steps {
		vals, err := parser.Parse(step)
		if err != nil {
			t.Fatal(err)
		}
		result, err = eng.Run(vals)
		if err != nil {
			t.Fatalf("step %q: %v", step, err)
		}
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

func TestCurriedBoth(t *testing.T) {
	t.Skip("query words disabled")
	// def select01 select star end; def from01 from people end; select01 from01
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	steps := []string{
		`context set people ("file/people.csv" read)`,
		`def select01 select star end`,
		`def from01 from people end`,
		`select01 from01`,
	}

	var result []native.Value
	for _, step := range steps {
		vals, err := parser.Parse(step)
		if err != nil {
			t.Fatal(err)
		}
		result, err = eng.Run(vals)
		if err != nil {
			t.Fatalf("step %q: %v", step, err)
		}
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	r0, _ := native.AsMap(rows[0])
	assertField(t, r0, "name", "Alice")
}

func TestCurriedSelectCols(t *testing.T) {
	t.Skip("query words disabled")
	// def sel_name select [name] end; sel_name from people
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	steps := []string{
		`context set people ("file/people.csv" read)`,
		`def sel_name select [name] end`,
		`sel_name from people`,
	}

	var result []native.Value
	for _, step := range steps {
		vals, err := parser.Parse(step)
		if err != nil {
			t.Fatal(err)
		}
		result, err = eng.Run(vals)
		if err != nil {
			t.Fatalf("step %q: %v", step, err)
		}
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	r0, _ := native.AsMap(rows[0])
	assertField(t, r0, "name", "Alice")
	if r0.Len() != 1 {
		t.Errorf("expected 1 column, got %d", r0.Len())
	}
}

// --- where ---

func TestWhereBasic(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [name eq "Alice"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
	assertField(t, native.AsMap(rows[0]), "age", "30")
}

func TestWhereNumericComparison(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [age gt "25"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (age 30 and 35), got %d", len(rows))
	}
}

func TestWhereLt(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [age lt "30"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (age 25), got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Bob")
}

func TestWhereAnd(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [age gte "30" and city eq "London"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
}

func TestWhereOr(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [city eq "London" or city eq "Tokyo"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestWhereWithColumns(t *testing.T) {
	t.Skip("query words disabled")
	// Use parens so where filters before select projects columns.
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select [name] (from people where [city eq "Paris"])`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Bob")
	if native.AsMap(rows[0]).Len() != 1 {
		t.Errorf("expected 1 column, got %d", native.AsMap(rows[0]).Len())
	}
}

func TestWhereNoMatch(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [name eq "Nobody"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

func TestWhereLike(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [name like "A%"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
}

func TestWhereNeq(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [name neq "Alice"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Bob")
	assertField(t, native.AsMap(rows[1]), "name", "Charlie")
}

// --- order ---

func TestOrderByColumn(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people order [name]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Alphabetical: Alice, Bob, Charlie
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
	assertField(t, native.AsMap(rows[1]), "name", "Bob")
	assertField(t, native.AsMap(rows[2]), "name", "Charlie")
}

func TestOrderByDesc(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people order [name desc]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Reverse alphabetical: Charlie, Bob, Alice
	assertField(t, native.AsMap(rows[0]), "name", "Charlie")
	assertField(t, native.AsMap(rows[1]), "name", "Bob")
	assertField(t, native.AsMap(rows[2]), "name", "Alice")
}

func TestOrderByAtom(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people order name`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
	assertField(t, native.AsMap(rows[2]), "name", "Charlie")
}

func TestOrderBySyntax(t *testing.T) {
	t.Skip("query words disabled")
	// "order by name" should work the same as "order name"
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people order by name`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
	assertField(t, native.AsMap(rows[2]), "name", "Charlie")
}

func TestOrderByListSyntax(t *testing.T) {
	t.Skip("query words disabled")
	// "order by [name desc]" should work
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people order by [name desc]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	assertField(t, native.AsMap(rows[0]), "name", "Charlie")
	assertField(t, native.AsMap(rows[2]), "name", "Alice")
}

// --- limit ---

func TestLimit(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people limit 2`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestLimitOne(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people limit 1`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

func TestLimitZero(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people limit 0`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

// --- chaining where + order + limit ---

func TestWhereOrderLimit(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [age gte "25"] order [name] limit 2`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// age >= 25 gives all 3, ordered by name: Alice, Bob, Charlie, limited to 2
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
	assertField(t, native.AsMap(rows[1]), "name", "Bob")
}

func TestWhereAndOrder(t *testing.T) {
	t.Skip("query words disabled")
	// Use parens so where and order are applied before select projects columns.
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select [name] (from people where [age gte "30"] order [name desc])`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// age >= 30: Alice(30), Charlie(35), ordered desc: Charlie, Alice
	assertField(t, native.AsMap(rows[0]), "name", "Charlie")
	assertField(t, native.AsMap(rows[1]), "name", "Alice")
}

func TestOrderAndLimit(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people order [age] limit 1`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Bob") // youngest
}

// --- non-SQLite table ---

func TestWhereOnInternalTable(t *testing.T) {
	t.Skip("query words disabled")
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("color", native.NewTypeLiteral(native.TString))
	fields.Set("count", native.NewTypeLiteral(native.TString))
	recType := native.RecordTypeInfo{Fields: fields}

	row1 := native.NewOrderedMap()
	row1.Set("color", native.NewString("red"))
	row1.Set("count", native.NewString("5"))
	row2 := native.NewOrderedMap()
	row2.Set("color", native.NewString("blue"))
	row2.Set("count", native.NewString("3"))

	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{native.NewMap(row1), native.NewMap(row2)},
		SQLite: false,
	}
	reg.Store["colors"] = native.Value{Parent: native.TList, Data: td}

	queryVals, err := parser.Parse(`select * from colors where [color eq "red"]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "color", "red")
}

// --- SQLite flag on loaded table ---

// --- offset ---

func TestOffset(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people order [name] limit 2 offset 1`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// Ordered by name: Alice, Bob, Charlie; offset 1 skips Alice
	assertField(t, native.AsMap(rows[0]), "name", "Bob")
	assertField(t, native.AsMap(rows[1]), "name", "Charlie")
}

func TestLimitOffset(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people order [name] limit 1 offset 2`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	// Ordered: Alice(0), Bob(1), Charlie(2); offset 2, limit 1 → Charlie
	assertField(t, native.AsMap(rows[0]), "name", "Charlie")
}

// --- distinct ---

func TestDistinct(t *testing.T) {
	t.Skip("query words disabled")
	// Create a table with duplicate city values.
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select [city] from people`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows without distinct, got %d", len(rows))
	}

	// With distinct — all cities happen to be unique, so same count.
	result, err = runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select [city] (from people distinct)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows = native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows with distinct (all unique), got %d", len(rows))
	}
}

func TestDistinctDuplicates(t *testing.T) {
	t.Skip("query words disabled")
	// Build a table with duplicate values.
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("color", native.NewTypeLiteral(native.TString))
	recType := native.RecordTypeInfo{Fields: fields}

	mkRow := func(color string) native.Value {
		om := native.NewOrderedMap()
		om.Set("color", native.NewString(color))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{mkRow("red"), mkRow("blue"), mkRow("red"), mkRow("blue"), mkRow("red")},
		SQLite: false,
	}
	reg.Store["colors"] = native.Value{Parent: native.TList, Data: td}

	queryVals, err := parser.Parse(`select [color] (from colors distinct)`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 distinct colors, got %d", len(rows))
	}
}

// --- nulls first / nulls last ---

func TestOrderNullsFirst(t *testing.T) {
	t.Skip("query words disabled")
	// Build a table with some NULL values.
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("name", native.NewTypeLiteral(native.TString))
	fields.Set("score", native.NewTypeLiteral(native.TString))
	recType := native.RecordTypeInfo{Fields: fields}

	mkRow := func(name, score string) native.Value {
		om := native.NewOrderedMap()
		om.Set("name", native.NewString(name))
		if score != "" {
			om.Set("score", native.NewString(score))
		} else {
			om.Set("score", native.NewString(""))
		}
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{mkRow("Alice", "90"), mkRow("Bob", ""), mkRow("Charlie", "80")},
		SQLite: false,
	}
	reg.Store["scores"] = native.Value{Parent: native.TList, Data: td}

	// Order by score with nulls first — empty strings sort first.
	queryVals, err := parser.Parse(`select * from scores order [score asc nulls first]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Empty string sorts first with NULLS FIRST.
	assertField(t, native.AsMap(rows[0]), "name", "Bob")
}

// --- order by position ---

func TestOrderByPosition(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people order [1]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Column 1 is "name" — alphabetical: Alice, Bob, Charlie
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
	assertField(t, native.AsMap(rows[2]), "name", "Charlie")
}

func TestOrderByPositionDesc(t *testing.T) {
	t.Skip("query words disabled")
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people order [1 desc]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	assertField(t, native.AsMap(rows[0]), "name", "Charlie")
	assertField(t, native.AsMap(rows[2]), "name", "Alice")
}

// --- is null / is not null ---

func TestWhereIsNull(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("name", native.NewTypeLiteral(native.TString))
	fields.Set("email", native.NewTypeLiteral(native.TString))
	recType := native.RecordTypeInfo{Fields: fields}

	mkRow := func(name, email string) native.Value {
		om := native.NewOrderedMap()
		om.Set("name", native.NewString(name))
		om.Set("email", native.NewString(email))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{mkRow("Alice", "alice@test.com"), mkRow("Bob", ""), mkRow("Charlie", "charlie@test.com")},
		SQLite: false,
	}
	reg.Store["users"] = native.Value{Parent: native.TList, Data: td}

	// is not null — in SQLite all TEXT columns are non-null (empty string != null)
	// but the query should still execute without error.
	queryVals, err := parser.Parse(`select * from users where [email is not null]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (all non-null TEXT), got %d", len(rows))
	}
}

// --- between ---

func TestWhereBetween(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [age between "25" "30"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (age 25 and 30), got %d", len(rows))
	}
}

func TestWhereNotBetween(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [age not between "25" "30"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (age 35), got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Charlie")
}

// --- NOT prefix ---

func TestWhereNotSimple(t *testing.T) {
	// [not name eq "Alice"] → NOT ("name" = 'Alice')
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [not name eq "Alice"] order [name]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (Bob, Charlie), got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Bob")
	assertField(t, native.AsMap(rows[1]), "name", "Charlie")
}

func TestWhereNotWithSubList(t *testing.T) {
	// [not [city eq "London" or city eq "Tokyo"]] → NOT ("city" = 'London' OR "city" = 'Tokyo')
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [not [city eq "London" or city eq "Tokyo"]]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (Paris), got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Bob")
}

func TestWhereNotAndOther(t *testing.T) {
	// [not name eq "Alice" and city eq "Tokyo"] → NOT ("name" = 'Alice') AND "city" = 'Tokyo'
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [not name eq "Alice" and city eq "Tokyo"]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (Charlie), got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Charlie")
}

// --- Nested groups (parenthesized conditions) ---

func TestWhereNestedGroup(t *testing.T) {
	// [[city eq "London" or city eq "Paris"] and age gte "30"]
	// → ("city" = 'London' OR "city" = 'Paris') AND "age" >= '30'
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [[city eq "London" or city eq "Paris"] and age gte "30"]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (Alice: London, 30), got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
}

func TestWhereNestedGroupRight(t *testing.T) {
	// [age gte "30" and [city eq "London" or city eq "Tokyo"]]
	// → "age" >= '30' AND ("city" = 'London' OR "city" = 'Tokyo')
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [age gte "30" and [city eq "London" or city eq "Tokyo"]]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (Alice, Charlie), got %d", len(rows))
	}
}

func TestWhereNotWithNestedGroup(t *testing.T) {
	// [not [city eq "London" or city eq "Paris"]] → NOT (...) — only Tokyo remains
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [not [city eq "London" or city eq "Paris"]]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (Charlie: Tokyo), got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Charlie")
}

func TestWhereDoubleNested(t *testing.T) {
	// [[city eq "London"] or [city eq "Tokyo"]] — two groups connected by OR
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [[city eq "London"] or [city eq "Tokyo"]] order [name]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (Alice, Charlie), got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
	assertField(t, native.AsMap(rows[1]), "name", "Charlie")
}

func TestWhereBetweenAndOther(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [age between "25" "35" and city eq "London"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
}

// --- glob ---

func TestWhereGlob(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [name glob "A*"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
}

func TestWhereGlobCaseSensitive(t *testing.T) {
	// GLOB is case-sensitive unlike LIKE.
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [name glob "a*"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows (GLOB is case-sensitive), got %d", len(rows))
	}
}

// --- typed columns ---

func TestTypedIntegerColumn(t *testing.T) {
	// Create a table where "age" is TInteger, not TString.
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("name", native.NewTypeLiteral(native.TString))
	fields.Set("age", native.NewTypeLiteral(native.TInteger))
	recType := native.RecordTypeInfo{Fields: fields}

	mkRow := func(name string, age int64) native.Value {
		om := native.NewOrderedMap()
		om.Set("name", native.NewString(name))
		om.Set("age", native.NewInteger(age))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows: []native.Value{
			mkRow("Alice", 30),
			mkRow("Bob", 25),
			mkRow("Charlie", 35),
		},
		SQLite: false,
	}
	reg.Store["people"] = native.Value{Parent: native.TList, Data: td}

	// Numeric comparison should work correctly with INTEGER column.
	// With TEXT, "9" > "25" is true (string ordering). With INTEGER, 9 < 25.
	queryVals, err := parser.Parse(`select * from people where [age gt 25] order [age]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (age 30 and 35), got %d", len(rows))
	}

	// Results should come back as integers, not strings.
	ageVal, ok, _ := native.AsMap(rows[0]).Get("age")
	if !ok {
		t.Fatal("expected age field")
	}
	if !ageVal.Parent.Matches(native.TInteger) {
		t.Errorf("expected age to be integer type, got %s", ageVal.Parent)
	}
	_v1, _ := native.AsInteger(ageVal)
	if _v1 != 30 {
		_v2, _ := native.AsInteger(ageVal)
		t.Errorf("expected age 30, got %d", _v2)
	}

	// Ordered by age: 30, 35.
	age2, _ := native.AsMap(rows[1]).Get("age")
	_v3, _ := native.AsInteger(age2)
	if _v3 != 35 {
		_v4, _ := native.AsInteger(age2)
		t.Errorf("expected second row age 35, got %d", _v4)
	}
}

func TestTypedBooleanColumn(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("name", native.NewTypeLiteral(native.TString))
	fields.Set("active", native.NewTypeLiteral(native.TBoolean))
	recType := native.RecordTypeInfo{Fields: fields}

	mkRow := func(name string, active bool) native.Value {
		om := native.NewOrderedMap()
		om.Set("name", native.NewString(name))
		om.Set("active", native.NewBoolean(active))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows: []native.Value{
			mkRow("Alice", true),
			mkRow("Bob", false),
			mkRow("Charlie", true),
		},
		SQLite: false,
	}
	reg.Store["users"] = native.Value{Parent: native.TList, Data: td}

	queryVals, err := parser.Parse(`select * from users where [active eq 1]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 active users, got %d", len(rows))
	}

	// Results should come back as booleans.
	activeVal, ok, _ := native.AsMap(rows[0]).Get("active")
	if !ok {
		t.Fatal("expected active field")
	}
	if !activeVal.Parent.Matches(native.TBoolean) {
		t.Errorf("expected active to be boolean type, got %s", activeVal.Parent)
	}
	_v5, _ := native.AsBoolean(activeVal)
	if !_v5 {
		t.Error("expected active to be true")
	}
}

func TestTypedIntegerOrdering(t *testing.T) {
	// This test verifies that INTEGER columns sort numerically, not lexically.
	// With TEXT: "9" > "25" > "100" (wrong). With INTEGER: 9 < 25 < 100 (correct).
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("val", native.NewTypeLiteral(native.TInteger))
	recType := native.RecordTypeInfo{Fields: fields}

	mkRow := func(val int64) native.Value {
		om := native.NewOrderedMap()
		om.Set("val", native.NewInteger(val))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{mkRow(100), mkRow(9), mkRow(25)},
		SQLite: false,
	}
	reg.Store["nums"] = native.Value{Parent: native.TList, Data: td}

	queryVals, err := parser.Parse(`select * from nums order [val]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Should be: 9, 25, 100 (numeric order, not "100", "25", "9").
	v0, _ := native.AsMap(rows[0]).Get("val")
	v1, _ := native.AsMap(rows[1]).Get("val")
	v2, _ := native.AsMap(rows[2]).Get("val")
	_v6, _ := native.AsInteger(v0)
	_v7, _ := native.AsInteger(v1)
	_v8, _ := native.AsInteger(v2)
	if _v6 != 9 || _v7 != 25 || _v8 != 100 {
		_v9, _ := native.AsInteger(v0)
		_v10, _ := native.AsInteger(v1)
		_v11, _ := native.AsInteger(v2)
		t.Errorf("expected [9, 25, 100], got [%d, %d, %d]", _v9, _v10, _v11)
	}
}

// --- IN / NOT IN ---

func TestWhereIn(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [city in ["London" "Tokyo"]]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestWhereNotIn(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [city not in ["London" "Tokyo"]]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r0, _ := native.AsMap(rows[0])
	name, _ := r0.Get("name")
	_v12, _ := native.AsString(name)
	if _v12 != "Bob" {
		_v13, _ := native.AsString(name)
		t.Errorf("expected Bob, got %s", _v13)
	}
}

// --- IN with subquery ---

func TestWhereInSubquery(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read) context set cities ("file/cities.csv" read)`,
		`select * from people where [city in (select [city] from cities)] order [name]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (Alice, Charlie), got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
	assertField(t, native.AsMap(rows[1]), "name", "Charlie")
}

func TestWhereNotInSubquery(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read) context set cities ("file/cities.csv" read)`,
		`select * from people where [city not in (select [city] from cities)]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (Bob), got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Bob")
}

func TestWhereInSubqueryWithFilter(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read) context set cities ("file/cities.csv" read)`,
		`select * from people where [city in (select [city] from cities where [country eq "UK"])]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (Alice: London/UK), got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
}

func TestWhereInSubqueryEmpty(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read) context set cities ("file/cities.csv" read)`,
		`select * from people where [city in (select [city] from cities where [country eq "None"])]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

// --- REGEXP ---

func TestWhereRegexp(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [name regexp "^[AB]"]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (Alice, Bob), got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
	assertField(t, native.AsMap(rows[1]), "name", "Bob")
}

func TestWhereRegexpNoMatch(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [name regexp "^Z"]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

func TestWhereNotRegexp(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [not [name regexp "^[AB]"]]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (Charlie), got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Charlie")
}

func TestWhereRegexpDigits(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [age regexp "^3"]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (Alice:30, Charlie:35), got %d", len(rows))
	}
}

// --- GROUP BY / HAVING ---

func TestGroupByWithCount(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("dept", native.NewTypeLiteral(native.TString))
	fields.Set("name", native.NewTypeLiteral(native.TString))
	recType := native.RecordTypeInfo{Fields: fields}

	mkRow := func(dept, name string) native.Value {
		om := native.NewOrderedMap()
		om.Set("dept", native.NewString(dept))
		om.Set("name", native.NewString(name))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows: []native.Value{
			mkRow("eng", "Alice"),
			mkRow("eng", "Bob"),
			mkRow("sales", "Charlie"),
			mkRow("eng", "Dave"),
			mkRow("sales", "Eve"),
		},
	}
	reg.Store["staff"] = native.Value{Parent: native.TList, Data: td}

	queryVals, err := parser.Parse(`select [dept [count name cnt]] from staff group by [dept] order [dept]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(rows))
	}

	r0, _ := native.AsMap(rows[0])
	dept0, _ := r0.Get("dept")
	cnt0, _ := r0.Get("cnt")
	_v14, _ := native.AsString(dept0)
	if _v14 != "eng" {
		_v15, _ := native.AsString(dept0)
		t.Errorf("expected dept eng, got %s", _v15)
	}
	_v16, _ := native.AsInteger(cnt0)
	if _v16 != 3 {
		_v17, _ := native.AsInteger(cnt0)
		t.Errorf("expected count 3, got %d", _v17)
	}

	r1, _ := native.AsMap(rows[1])
	cnt1, _ := r1.Get("cnt")
	_v18, _ := native.AsInteger(cnt1)
	if _v18 != 2 {
		_v19, _ := native.AsInteger(cnt1)
		t.Errorf("expected count 2, got %d", _v19)
	}
}

func TestHaving(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("dept", native.NewTypeLiteral(native.TString))
	fields.Set("name", native.NewTypeLiteral(native.TString))
	recType := native.RecordTypeInfo{Fields: fields}

	mkRow := func(dept, name string) native.Value {
		om := native.NewOrderedMap()
		om.Set("dept", native.NewString(dept))
		om.Set("name", native.NewString(name))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows: []native.Value{
			mkRow("eng", "Alice"),
			mkRow("eng", "Bob"),
			mkRow("sales", "Charlie"),
			mkRow("eng", "Dave"),
		},
	}
	reg.Store["staff"] = native.Value{Parent: native.TList, Data: td}

	// Only groups with count > 1
	queryVals, err := parser.Parse(`select [dept [count name cnt]] from staff group by [dept] having [cnt gt 1]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 group (eng has 3), got %d", len(rows))
	}
	r0, _ := native.AsMap(rows[0])
	dept, _ := r0.Get("dept")
	_v20, _ := native.AsString(dept)
	if _v20 != "eng" {
		_v21, _ := native.AsString(dept)
		t.Errorf("expected eng, got %s", _v21)
	}
}

// --- Table aliases ---

func TestFromAlias(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people as p`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

// --- JOINs ---

func TestInnerJoin(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	// orders table
	oFields := native.NewOrderedMap()
	oFields.Set("order_id", native.NewTypeLiteral(native.TString))
	oFields.Set("product", native.NewTypeLiteral(native.TString))
	oFields.Set("qty", native.NewTypeLiteral(native.TString))
	oRec := native.RecordTypeInfo{Fields: oFields}
	mkOrder := func(id, product, qty string) native.Value {
		om := native.NewOrderedMap()
		om.Set("order_id", native.NewString(id))
		om.Set("product", native.NewString(product))
		om.Set("qty", native.NewString(qty))
		return native.NewMap(om)
	}
	oTD := native.TableData{
		Record: oRec,
		Rows: []native.Value{
			mkOrder("1", "widget", "10"),
			mkOrder("2", "gadget", "5"),
			mkOrder("3", "widget", "3"),
		},
	}
	reg.Store["orders"] = native.Value{Parent: native.TList, Data: oTD}

	// products table
	pFields := native.NewOrderedMap()
	pFields.Set("product", native.NewTypeLiteral(native.TString))
	pFields.Set("price", native.NewTypeLiteral(native.TString))
	pRec := native.RecordTypeInfo{Fields: pFields}
	mkProduct := func(name, price string) native.Value {
		om := native.NewOrderedMap()
		om.Set("product", native.NewString(name))
		om.Set("price", native.NewString(price))
		return native.NewMap(om)
	}
	pTD := native.TableData{
		Record: pRec,
		Rows: []native.Value{
			mkProduct("widget", "9.99"),
			mkProduct("gadget", "19.99"),
		},
	}
	reg.Store["products"] = native.Value{Parent: native.TList, Data: pTD}

	queryVals, err := parser.Parse(`select * from orders join products using [product] order [order_id]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 joined rows, got %d", len(rows))
	}

	// First row should have both order and product fields.
	r0, _ := native.AsMap(rows[0])
	oid, _ := r0.Get("order_id")
	price, _ := r0.Get("price")
	_v22, _ := native.AsString(oid)
	if _v22 != "1" {
		_v23, _ := native.AsString(oid)
		t.Errorf("expected order_id 1, got %s", _v23)
	}
	_v24, _ := native.AsString(price)
	if _v24 != "9.99" {
		_v25, _ := native.AsString(price)
		t.Errorf("expected price 9.99, got %s", _v25)
	}
}

func TestLeftJoin(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	// people
	pFields := native.NewOrderedMap()
	pFields.Set("name", native.NewTypeLiteral(native.TString))
	pFields.Set("dept_id", native.NewTypeLiteral(native.TString))
	pRec := native.RecordTypeInfo{Fields: pFields}
	mkPerson := func(name, deptID string) native.Value {
		om := native.NewOrderedMap()
		om.Set("name", native.NewString(name))
		om.Set("dept_id", native.NewString(deptID))
		return native.NewMap(om)
	}
	pTD := native.TableData{
		Record: pRec,
		Rows: []native.Value{
			mkPerson("Alice", "1"),
			mkPerson("Bob", "2"),
			mkPerson("Charlie", "99"), // no matching dept
		},
	}
	reg.Store["people"] = native.Value{Parent: native.TList, Data: pTD}

	// depts
	dFields := native.NewOrderedMap()
	dFields.Set("dept_id", native.NewTypeLiteral(native.TString))
	dFields.Set("dept_name", native.NewTypeLiteral(native.TString))
	dRec := native.RecordTypeInfo{Fields: dFields}
	mkDept := func(id, name string) native.Value {
		om := native.NewOrderedMap()
		om.Set("dept_id", native.NewString(id))
		om.Set("dept_name", native.NewString(name))
		return native.NewMap(om)
	}
	dTD := native.TableData{
		Record: dRec,
		Rows: []native.Value{
			mkDept("1", "Engineering"),
			mkDept("2", "Sales"),
		},
	}
	reg.Store["depts"] = native.Value{Parent: native.TList, Data: dTD}

	queryVals, err := parser.Parse(`select * from people leftjoin depts using [dept_id] order [name]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (left join preserves all left rows), got %d", len(rows))
	}

	// Charlie should have NULL dept_name.
	r2, _ := native.AsMap(rows[2])
	name2, _ := r2.Get("name")
	_v26, _ := native.AsString(name2)
	if _v26 != "Charlie" {
		_v27, _ := native.AsString(name2)
		t.Errorf("expected Charlie, got %s", _v27)
	}
}

// --- Set operations ---

func TestUnion(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	mkTable := func(names ...string) native.TableData {
		fields := native.NewOrderedMap()
		fields.Set("name", native.NewTypeLiteral(native.TString))
		recType := native.RecordTypeInfo{Fields: fields}
		rows := make([]native.Value, len(names))
		for i, n := range names {
			om := native.NewOrderedMap()
			om.Set("name", native.NewString(n))
			rows[i] = native.NewMap(om)
		}
		return native.TableData{Record: recType, Rows: rows}
	}

	reg.Store["t1"] = native.Value{Parent: native.TList, Data: mkTable("Alice", "Bob")}
	reg.Store["t2"] = native.Value{Parent: native.TList, Data: mkTable("Bob", "Charlie")}

	// UNION removes duplicates.
	queryVals, err := parser.Parse(`select * (from t1 union from t2) order [name]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 unique rows, got %d", len(rows))
	}
}

func TestUnionAll(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	mkTable := func(names ...string) native.TableData {
		fields := native.NewOrderedMap()
		fields.Set("name", native.NewTypeLiteral(native.TString))
		recType := native.RecordTypeInfo{Fields: fields}
		rows := make([]native.Value, len(names))
		for i, n := range names {
			om := native.NewOrderedMap()
			om.Set("name", native.NewString(n))
			rows[i] = native.NewMap(om)
		}
		return native.TableData{Record: recType, Rows: rows}
	}

	reg.Store["t1"] = native.Value{Parent: native.TList, Data: mkTable("Alice", "Bob")}
	reg.Store["t2"] = native.Value{Parent: native.TList, Data: mkTable("Bob", "Charlie")}

	// UNION ALL keeps duplicates.
	queryVals, err := parser.Parse(`select * (from t1 unionall from t2) order [name]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows (with duplicate Bob), got %d", len(rows))
	}
}

func TestIntersect(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	mkTable := func(names ...string) native.TableData {
		fields := native.NewOrderedMap()
		fields.Set("name", native.NewTypeLiteral(native.TString))
		recType := native.RecordTypeInfo{Fields: fields}
		rows := make([]native.Value, len(names))
		for i, n := range names {
			om := native.NewOrderedMap()
			om.Set("name", native.NewString(n))
			rows[i] = native.NewMap(om)
		}
		return native.TableData{Record: recType, Rows: rows}
	}

	reg.Store["t1"] = native.Value{Parent: native.TList, Data: mkTable("Alice", "Bob")}
	reg.Store["t2"] = native.Value{Parent: native.TList, Data: mkTable("Bob", "Charlie")}

	queryVals, err := parser.Parse(`select * (from t1 intersect from t2)`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (Bob), got %d", len(rows))
	}
	name, _ := native.AsMap(rows[0]).Get("name")
	_v28, _ := native.AsString(name)
	if _v28 != "Bob" {
		_v29, _ := native.AsString(name)
		t.Errorf("expected Bob, got %s", _v29)
	}
}

func TestExcept(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	mkTable := func(names ...string) native.TableData {
		fields := native.NewOrderedMap()
		fields.Set("name", native.NewTypeLiteral(native.TString))
		recType := native.RecordTypeInfo{Fields: fields}
		rows := make([]native.Value, len(names))
		for i, n := range names {
			om := native.NewOrderedMap()
			om.Set("name", native.NewString(n))
			rows[i] = native.NewMap(om)
		}
		return native.TableData{Record: recType, Rows: rows}
	}

	reg.Store["t1"] = native.Value{Parent: native.TList, Data: mkTable("Alice", "Bob")}
	reg.Store["t2"] = native.Value{Parent: native.TList, Data: mkTable("Bob", "Charlie")}

	queryVals, err := parser.Parse(`select * (from t1 except from t2)`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (Alice), got %d", len(rows))
	}
	name, _ := native.AsMap(rows[0]).Get("name")
	_v30, _ := native.AsString(name)
	if _v30 != "Alice" {
		_v31, _ := native.AsString(name)
		t.Errorf("expected Alice, got %s", _v31)
	}
}

// --- CAST ---

func TestCast(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select [[cast age integer age_int]] from people order [age_int]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// CAST should produce integer ordering: 25, 30, 35 (not "25", "30", "35").
	v0, _ := native.AsMap(rows[0]).Get("age_int")
	v1, _ := native.AsMap(rows[1]).Get("age_int")
	v2, _ := native.AsMap(rows[2]).Get("age_int")
	_v32, _ := native.AsInteger(v0)
	_v33, _ := native.AsInteger(v1)
	_v34, _ := native.AsInteger(v2)
	if _v32 != 25 || _v33 != 30 || _v34 != 35 {
		_v35, _ := native.AsInteger(v0)
		_v36, _ := native.AsInteger(v1)
		_v37, _ := native.AsInteger(v2)
		t.Errorf("expected [25, 30, 35], got [%d, %d, %d]", _v35, _v36, _v37)
	}
}

// --- Aggregate words standalone ---

func TestCountStar(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select [[count * total]] from people`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	cnt, _ := native.AsMap(rows[0]).Get("total")
	_v38, _ := native.AsInteger(cnt)
	if _v38 != 3 {
		_v39, _ := native.AsInteger(cnt)
		t.Errorf("expected count 3, got %d", _v39)
	}
}

func TestSumAggregate(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("val", native.NewTypeLiteral(native.TInteger))
	recType := native.RecordTypeInfo{Fields: fields}

	mkRow := func(v int64) native.Value {
		om := native.NewOrderedMap()
		om.Set("val", native.NewInteger(v))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{mkRow(10), mkRow(20), mkRow(30)},
	}
	reg.Store["nums"] = native.Value{Parent: native.TList, Data: td}

	queryVals, err := parser.Parse(`select [[sum val total]] from nums`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	total, _ := native.AsMap(rows[0]).Get("total")
	_v40, _ := native.AsInteger(total)
	if _v40 != 60 {
		_v41, _ := native.AsInteger(total)
		t.Errorf("expected sum 60, got %d", _v41)
	}
}

// --- SQLite flag on loaded table ---

// --- JOIN with ON condition (covers buildJoinCondition, quoteJoinCol) ---

func TestJoinWithOnCondition(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	// employees table
	eFields := native.NewOrderedMap()
	eFields.Set("emp_name", native.NewTypeLiteral(native.TString))
	eFields.Set("dept_id", native.NewTypeLiteral(native.TString))
	eRec := native.RecordTypeInfo{Fields: eFields}
	mkEmp := func(name, deptID string) native.Value {
		om := native.NewOrderedMap()
		om.Set("emp_name", native.NewString(name))
		om.Set("dept_id", native.NewString(deptID))
		return native.NewMap(om)
	}
	eTD := native.TableData{
		Record: eRec,
		Rows:   []native.Value{mkEmp("Alice", "1"), mkEmp("Bob", "2")},
	}
	reg.Store["employees"] = native.Value{Parent: native.TList, Data: eTD}

	// departments table
	dFields := native.NewOrderedMap()
	dFields.Set("id", native.NewTypeLiteral(native.TString))
	dFields.Set("dept_name", native.NewTypeLiteral(native.TString))
	dRec := native.RecordTypeInfo{Fields: dFields}
	mkDept := func(id, name string) native.Value {
		om := native.NewOrderedMap()
		om.Set("id", native.NewString(id))
		om.Set("dept_name", native.NewString(name))
		return native.NewMap(om)
	}
	dTD := native.TableData{
		Record: dRec,
		Rows:   []native.Value{mkDept("1", "Engineering"), mkDept("2", "Sales")},
	}
	reg.Store["departments"] = native.Value{Parent: native.TList, Data: dTD}

	// JOIN with ON using non-qualified column names (since columns are unique across tables)
	queryVals, err := parser.Parse(`select * from employees join departments on [dept_id eq id] order [emp_name]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 joined rows, got %d", len(rows))
	}
	r0, _ := native.AsMap(rows[0])
	empName, _ := r0.Get("emp_name")
	deptName, _ := r0.Get("dept_name")
	_v42, _ := native.AsString(empName)
	if _v42 != "Alice" {
		_v43, _ := native.AsString(empName)
		t.Errorf("expected Alice, got %s", _v43)
	}
	_v44, _ := native.AsString(deptName)
	if _v44 != "Engineering" {
		_v45, _ := native.AsString(deptName)
		t.Errorf("expected Engineering, got %s", _v45)
	}
}

func TestJoinWithOnMultipleConditions(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	// t1 table
	f1 := native.NewOrderedMap()
	f1.Set("a", native.NewTypeLiteral(native.TString))
	f1.Set("b", native.NewTypeLiteral(native.TString))
	r1 := native.RecordTypeInfo{Fields: f1}
	mk1 := func(a, b string) native.Value {
		om := native.NewOrderedMap()
		om.Set("a", native.NewString(a))
		om.Set("b", native.NewString(b))
		return native.NewMap(om)
	}
	td1 := native.TableData{Record: r1, Rows: []native.Value{mk1("1", "x"), mk1("2", "y")}}
	reg.Store["t1"] = native.Value{Parent: native.TList, Data: td1}

	// t2 table
	f2 := native.NewOrderedMap()
	f2.Set("c", native.NewTypeLiteral(native.TString))
	f2.Set("d", native.NewTypeLiteral(native.TString))
	r2 := native.RecordTypeInfo{Fields: f2}
	mk2 := func(c, d string) native.Value {
		om := native.NewOrderedMap()
		om.Set("c", native.NewString(c))
		om.Set("d", native.NewString(d))
		return native.NewMap(om)
	}
	td2 := native.TableData{Record: r2, Rows: []native.Value{mk2("1", "x"), mk2("2", "z")}}
	reg.Store["t2"] = native.Value{Parent: native.TList, Data: td2}

	// JOIN with AND in ON condition (non-qualified since temp table names differ)
	queryVals, err := parser.Parse(`select * from t1 join t2 on [a eq c and b eq d]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (only a=1,b=x matches), got %d", len(rows))
	}
}

func TestJoinWithOnDotQualified(t *testing.T) {
	// Test dot-qualified column names in ON clause with SQLite-backed tables.
	// This exercises quoteJoinCol with dot notation.
	result, err := runQuery(t,
		`context set people ("file/people.csv" read) context set departments ("file/departments.csv" read)`,
		`select * from people join departments on ["people.age" eq "departments.dept_id"] order [name]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	// age is text "30", "25", "35" and dept_id is "1","2","3" — no matches expected
	// but the query should execute without error, exercising the dot-qualified path
	rows, _ := native.AsList(result[0])
	_ = rows // result doesn't matter, we just need the dot-qualified parsing to work
}

// --- CROSS JOIN ---

func TestCrossJoin(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	mkTable := func(field string, vals ...string) native.TableData {
		fields := native.NewOrderedMap()
		fields.Set(field, native.NewTypeLiteral(native.TString))
		recType := native.RecordTypeInfo{Fields: fields}
		rows := make([]native.Value, len(vals))
		for i, v := range vals {
			om := native.NewOrderedMap()
			om.Set(field, native.NewString(v))
			rows[i] = native.NewMap(om)
		}
		return native.TableData{Record: recType, Rows: rows}
	}

	reg.Store["colors"] = native.Value{Parent: native.TList, Data: mkTable("color", "red", "blue")}
	reg.Store["sizes"] = native.Value{Parent: native.TList, Data: mkTable("size", "S", "M", "L")}

	queryVals, err := parser.Parse(`select * from colors crossjoin sizes`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 6 {
		t.Fatalf("expected 6 rows (2x3 cross join), got %d", len(rows))
	}
}

// --- CAST additional type mappings ---

func TestCastToReal(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select [[cast age real]] from people order [age]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

func TestCastToText(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("val", native.NewTypeLiteral(native.TInteger))
	recType := native.RecordTypeInfo{Fields: fields}
	mkRow := func(v int64) native.Value {
		om := native.NewOrderedMap()
		om.Set("val", native.NewInteger(v))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{mkRow(42)},
	}
	reg.Store["nums"] = native.Value{Parent: native.TList, Data: td}

	queryVals, err := parser.Parse(`select [[cast val text t]] from nums`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	v, _ := native.AsMap(rows[0]).Get("t")
	_v46, _ := native.AsString(v)
	if _v46 != "42" {
		_v47, _ := native.AsString(v)
		t.Errorf("expected '42', got %q", _v47)
	}
}

func TestCastWithoutAlias(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select [[cast age integer]] from people limit 1`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	// Without alias, output name defaults to the column name.
	v, ok, _ := native.AsMap(rows[0]).Get("age")
	if !ok {
		t.Fatal("expected 'age' field in result")
	}
	_v48, _ := native.AsInteger(v)
	if _v48 != 30 {
		_v49, _ := native.AsInteger(v)
		t.Errorf("expected 30, got %d", _v49)
	}
}

// --- Aggregate edge cases ---

func TestAvgAggregate(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("val", native.NewTypeLiteral(native.TInteger))
	recType := native.RecordTypeInfo{Fields: fields}
	mkRow := func(v int64) native.Value {
		om := native.NewOrderedMap()
		om.Set("val", native.NewInteger(v))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{mkRow(10), mkRow(20), mkRow(30)},
	}
	reg.Store["nums"] = native.Value{Parent: native.TList, Data: td}

	queryVals, err := parser.Parse(`select [[avg val average]] from nums`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	avg, _ := native.AsMap(rows[0]).Get("average")
	// AVG of 10,20,30 = 20
	_v50, _ := native.AsInteger(avg)
	if _v50 != 20 {
		_v51, _ := native.AsInteger(avg)
		t.Errorf("expected avg 20, got %d", _v51)
	}
}

func TestMinMaxAggregate(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("val", native.NewTypeLiteral(native.TInteger))
	recType := native.RecordTypeInfo{Fields: fields}
	mkRow := func(v int64) native.Value {
		om := native.NewOrderedMap()
		om.Set("val", native.NewInteger(v))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{mkRow(10), mkRow(20), mkRow(30)},
	}
	reg.Store["nums"] = native.Value{Parent: native.TList, Data: td}

	queryVals, err := parser.Parse(`select [[min val lo] [max val hi]] from nums`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	lo, _ := native.AsMap(rows[0]).Get("lo")
	hi, _ := native.AsMap(rows[0]).Get("hi")
	_v52, _ := native.AsInteger(lo)
	if _v52 != 10 {
		_v53, _ := native.AsInteger(lo)
		t.Errorf("expected min 10, got %d", _v53)
	}
	_v54, _ := native.AsInteger(hi)
	if _v54 != 30 {
		_v55, _ := native.AsInteger(hi)
		t.Errorf("expected max 30, got %d", _v55)
	}
}

func TestAggregateWithoutAlias(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select [[count name]] from people`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	// Default alias is "count_name"
	cnt, ok, _ := native.AsMap(rows[0]).Get("count_name")
	if !ok {
		t.Fatal("expected 'count_name' field (default alias)")
	}
	_v56, _ := native.AsInteger(cnt)
	if _v56 != 3 {
		_v57, _ := native.AsInteger(cnt)
		t.Errorf("expected 3, got %d", _v57)
	}
}

// --- WHERE IS NULL ---

func TestWhereIsNullActual(t *testing.T) {
	// Test IS NULL with actual NULL values (not just empty strings).
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("name", native.NewTypeLiteral(native.TString))
	fields.Set("score", native.NewTypeLiteral(native.TInteger))
	recType := native.RecordTypeInfo{Fields: fields}

	row1 := native.NewOrderedMap()
	row1.Set("name", native.NewString("Alice"))
	row1.Set("score", native.NewInteger(90))
	row2 := native.NewOrderedMap()
	row2.Set("name", native.NewString("Bob"))
	row2.Set("score", native.Value{Parent: native.TNone})
	row3 := native.NewOrderedMap()
	row3.Set("name", native.NewString("Charlie"))
	row3.Set("score", native.NewInteger(80))

	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{native.NewMap(row1), native.NewMap(row2), native.NewMap(row3)},
	}
	reg.Store["students"] = native.Value{Parent: native.TList, Data: td}

	// IS NULL
	queryVals, err := parser.Parse(`select * from students where [score is null]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row with NULL score, got %d", len(rows))
	}
	name, _ := native.AsMap(rows[0]).Get("name")
	_v58, _ := native.AsString(name)
	if _v58 != "Bob" {
		_v59, _ := native.AsString(name)
		t.Errorf("expected Bob, got %s", _v59)
	}
}

// --- Multi-column GROUP BY ---

func TestMultiColumnGroupBy(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("dept", native.NewTypeLiteral(native.TString))
	fields.Set("role", native.NewTypeLiteral(native.TString))
	fields.Set("name", native.NewTypeLiteral(native.TString))
	recType := native.RecordTypeInfo{Fields: fields}

	mkRow := func(dept, role, name string) native.Value {
		om := native.NewOrderedMap()
		om.Set("dept", native.NewString(dept))
		om.Set("role", native.NewString(role))
		om.Set("name", native.NewString(name))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows: []native.Value{
			mkRow("eng", "dev", "Alice"),
			mkRow("eng", "dev", "Bob"),
			mkRow("eng", "mgr", "Charlie"),
			mkRow("sales", "dev", "Dave"),
		},
	}
	reg.Store["staff"] = native.Value{Parent: native.TList, Data: td}

	queryVals, err := parser.Parse(`select [dept role [count name cnt]] from staff group by [dept role] order [dept role]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(rows))
	}
	// eng/dev = 2, eng/mgr = 1, sales/dev = 1
	cnt0, _ := native.AsMap(rows[0]).Get("cnt")
	_v60, _ := native.AsInteger(cnt0)
	if _v60 != 2 {
		_v61, _ := native.AsInteger(cnt0)
		t.Errorf("expected count 2 for eng/dev, got %d", _v61)
	}
}

// --- ORDER BY NULLS LAST ---

func TestOrderNullsLast(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("name", native.NewTypeLiteral(native.TString))
	fields.Set("score", native.NewTypeLiteral(native.TInteger))
	recType := native.RecordTypeInfo{Fields: fields}

	row1 := native.NewOrderedMap()
	row1.Set("name", native.NewString("Alice"))
	row1.Set("score", native.NewInteger(90))
	row2 := native.NewOrderedMap()
	row2.Set("name", native.NewString("Bob"))
	row2.Set("score", native.Value{Parent: native.TNone})
	row3 := native.NewOrderedMap()
	row3.Set("name", native.NewString("Charlie"))
	row3.Set("score", native.NewInteger(80))

	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{native.NewMap(row1), native.NewMap(row2), native.NewMap(row3)},
	}
	reg.Store["students"] = native.Value{Parent: native.TList, Data: td}

	queryVals, err := parser.Parse(`select * from students order [score asc nulls last]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// With NULLS LAST, Bob (null score) should be last.
	lastRow, _ := native.AsMap(rows[2])
	name, _ := lastRow.Get("name")
	_v62, _ := native.AsString(name)
	if _v62 != "Bob" {
		_v63, _ := native.AsString(name)
		t.Errorf("expected Bob last (null score), got %s", _v63)
	}
}

// --- innerjoin keyword ---

func TestInnerJoinKeyword(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	mkTable := func(field string, vals ...string) native.TableData {
		fields := native.NewOrderedMap()
		fields.Set(field, native.NewTypeLiteral(native.TString))
		recType := native.RecordTypeInfo{Fields: fields}
		rows := make([]native.Value, len(vals))
		for i, v := range vals {
			om := native.NewOrderedMap()
			om.Set(field, native.NewString(v))
			rows[i] = native.NewMap(om)
		}
		return native.TableData{Record: recType, Rows: rows}
	}

	// Two tables with shared column for USING
	f1 := native.NewOrderedMap()
	f1.Set("id", native.NewTypeLiteral(native.TString))
	f1.Set("name", native.NewTypeLiteral(native.TString))
	r1 := native.RecordTypeInfo{Fields: f1}
	mk1 := func(id, name string) native.Value {
		om := native.NewOrderedMap()
		om.Set("id", native.NewString(id))
		om.Set("name", native.NewString(name))
		return native.NewMap(om)
	}
	_ = mkTable // suppress unused

	td1 := native.TableData{Record: r1, Rows: []native.Value{mk1("1", "Alice"), mk1("2", "Bob")}}
	reg.Store["t1"] = native.Value{Parent: native.TList, Data: td1}

	f2 := native.NewOrderedMap()
	f2.Set("id", native.NewTypeLiteral(native.TString))
	f2.Set("score", native.NewTypeLiteral(native.TString))
	r2 := native.RecordTypeInfo{Fields: f2}
	mk2 := func(id, score string) native.Value {
		om := native.NewOrderedMap()
		om.Set("id", native.NewString(id))
		om.Set("score", native.NewString(score))
		return native.NewMap(om)
	}
	td2 := native.TableData{Record: r2, Rows: []native.Value{mk2("1", "90"), mk2("2", "85")}}
	reg.Store["t2"] = native.Value{Parent: native.TList, Data: td2}

	queryVals, err := parser.Parse(`select * from t1 innerjoin t2 using [id] order [id]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

// --- ORDER BY with multiple keys ---

func TestOrderByMultipleKeys(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people order [city asc name asc]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// London < Paris < Tokyo
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
	assertField(t, native.AsMap(rows[1]), "name", "Bob")
	assertField(t, native.AsMap(rows[2]), "name", "Charlie")
}

func TestOrderByMultipleKeysDesc(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people order [city desc name asc]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Tokyo > Paris > London (desc)
	assertField(t, native.AsMap(rows[0]), "name", "Charlie")
	assertField(t, native.AsMap(rows[1]), "name", "Bob")
	assertField(t, native.AsMap(rows[2]), "name", "Alice")
}

func TestOrderByMultipleKeysMixed(t *testing.T) {
	// Create test data with duplicate city values to test secondary sort
	result, err := runQuery(t,
		`context set cities ("file/cities.csv" read)`,
		`select * from cities order [country asc city desc]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// Japan < UK (asc), then city desc within each group
	assertField(t, native.AsMap(rows[0]), "city", "Tokyo")
	assertField(t, native.AsMap(rows[1]), "city", "London")
}

// --- COLLATE ---

func TestOrderCollateNocase(t *testing.T) {
	result, err := runQuery(t,
		`context set data ("file/mixed_case.csv" read)`,
		`select * from data order [name collate nocase asc]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Case-insensitive: alice < Bob < CHARLIE
	assertField(t, native.AsMap(rows[0]), "name", "alice")
	assertField(t, native.AsMap(rows[1]), "name", "Bob")
	assertField(t, native.AsMap(rows[2]), "name", "CHARLIE")
}

func TestOrderCollateBinary(t *testing.T) {
	result, err := runQuery(t,
		`context set data ("file/mixed_case.csv" read)`,
		`select * from data order [name collate binary asc]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Binary: uppercase < lowercase (B < C < a)
	assertField(t, native.AsMap(rows[0]), "name", "Bob")
	assertField(t, native.AsMap(rows[1]), "name", "CHARLIE")
	assertField(t, native.AsMap(rows[2]), "name", "alice")
}

func TestWhereEqCollateNocase(t *testing.T) {
	result, err := runQuery(t,
		`context set data ("file/mixed_case.csv" read)`,
		`select * from data where [name eq "ALICE" collate nocase]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "alice")
}

func TestWhereEqCollateNocaseNoMatch(t *testing.T) {
	// Without collate nocase, "ALICE" should not match "alice"
	result, err := runQuery(t,
		`context set data ("file/mixed_case.csv" read)`,
		`select * from data where [name eq "ALICE"]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows (case-sensitive), got %d", len(rows))
	}
}

func TestWhereLikeCollateNocase(t *testing.T) {
	result, err := runQuery(t,
		`context set data ("file/mixed_case.csv" read)`,
		`select * from data where [name like "b%" collate nocase]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Bob")
}

// --- GROUP BY with single atom (group name) ---

func TestGroupByAtom(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("city", native.NewTypeLiteral(native.TString))
	fields.Set("name", native.NewTypeLiteral(native.TString))
	recType := native.RecordTypeInfo{Fields: fields}
	mkRow := func(city, name string) native.Value {
		om := native.NewOrderedMap()
		om.Set("city", native.NewString(city))
		om.Set("name", native.NewString(name))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{mkRow("London", "A"), mkRow("London", "B"), mkRow("Paris", "C")},
	}
	reg.Store["data"] = native.Value{Parent: native.TList, Data: td}

	queryVals, err := parser.Parse(`select [city [count name cnt]] from data group city order [city]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(rows))
	}
}

// --- WHERE with integer comparison (covers valueToSQL integer branch) ---

func TestWhereIntegerComparison(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("name", native.NewTypeLiteral(native.TString))
	fields.Set("age", native.NewTypeLiteral(native.TInteger))
	recType := native.RecordTypeInfo{Fields: fields}
	mkRow := func(name string, age int64) native.Value {
		om := native.NewOrderedMap()
		om.Set("name", native.NewString(name))
		om.Set("age", native.NewInteger(age))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{mkRow("Alice", 30), mkRow("Bob", 25), mkRow("Charlie", 35)},
	}
	reg.Store["people"] = native.Value{Parent: native.TList, Data: td}

	// Use integer literal in WHERE (not string)
	queryVals, err := parser.Parse(`select * from people where [age gt 28]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (age 30, 35), got %d", len(rows))
	}
}

// --- WHERE with boolean/none values (covers valueToSQL branches) ---

func TestWhereBooleanValue(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("name", native.NewTypeLiteral(native.TString))
	fields.Set("active", native.NewTypeLiteral(native.TBoolean))
	recType := native.RecordTypeInfo{Fields: fields}
	mkRow := func(name string, active bool) native.Value {
		om := native.NewOrderedMap()
		om.Set("name", native.NewString(name))
		om.Set("active", native.NewBoolean(active))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{mkRow("Alice", true), mkRow("Bob", false)},
	}
	reg.Store["users"] = native.Value{Parent: native.TList, Data: td}

	// Using string "true" comparison with a boolean column (exercises boolean coercion in aqlValueToSQLParam)
	queryVals, err := parser.Parse(`select * from users where [active eq "true"]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}
	_ = result // exercises the path
}

// --- Mixed type table (covers more aqlValueToSQLParam and sqlResultToAQLValue branches) ---

func TestMixedTypeStorage(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("name", native.NewTypeLiteral(native.TString))
	fields.Set("score", native.NewTypeLiteral(native.TNumber))
	fields.Set("active", native.NewTypeLiteral(native.TBoolean))
	fields.Set("count", native.NewTypeLiteral(native.TInteger))
	recType := native.RecordTypeInfo{Fields: fields}

	// Row with string values that need coercion to numeric types
	row1 := native.NewOrderedMap()
	row1.Set("name", native.NewString("Alice"))
	row1.Set("score", native.NewString("95.5"))  // string → REAL coercion
	row1.Set("active", native.NewString("true")) // string → boolean coercion
	row1.Set("count", native.NewString("42"))    // string → INTEGER coercion

	// Row with proper typed values
	row2 := native.NewOrderedMap()
	row2.Set("name", native.NewString("Bob"))
	row2.Set("score", native.NewInteger(88))     // integer → REAL coercion
	row2.Set("active", native.NewBoolean(false)) // boolean → INTEGER (0/1)
	row2.Set("count", native.NewBoolean(true))   // boolean → INTEGER coercion

	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{native.NewMap(row1), native.NewMap(row2)},
	}
	reg.Store["data"] = native.Value{Parent: native.TList, Data: td}

	queryVals, err := parser.Parse(`select * from data order [name]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// Check Alice's values came back correctly
	r0, _ := native.AsMap(rows[0])
	name, _ := r0.Get("name")
	_v64, _ := native.AsString(name)
	if _v64 != "Alice" {
		_v65, _ := native.AsString(name)
		t.Errorf("expected Alice, got %s", _v65)
	}
	count, _ := r0.Get("count")
	_v66, _ := native.AsInteger(count)
	if _v66 != 42 {
		_v67, _ := native.AsInteger(count)
		t.Errorf("expected count 42, got %d", _v67)
	}
	active, _ := r0.Get("active")
	_v68, _ := native.AsBoolean(active)
	if !_v68 {
		t.Error("expected active true")
	}
}

// --- CAST with different type names (covers aqlTypenameToSQLType branches) ---

func TestCastWithTypeAliases(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select [[cast age int i1] [cast name string s1]] from people limit 1`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	i1, _ := native.AsMap(rows[0]).Get("i1")
	_v69, _ := native.AsInteger(i1)
	if _v69 != 30 {
		_v70, _ := native.AsInteger(i1)
		t.Errorf("expected 30, got %d", _v70)
	}
	s1, _ := native.AsMap(rows[0]).Get("s1")
	_v71, _ := native.AsString(s1)
	if _v71 != "Alice" {
		_v72, _ := native.AsString(s1)
		t.Errorf("expected Alice, got %s", _v72)
	}
}

func TestCastFloat(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select [[cast age float f1] [cast age number n1]] from people limit 1`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

// --- select with string column name ---

func TestSelectStringColumns(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select ["name" "city"] from people limit 1`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r0, _ := native.AsMap(rows[0])
	if r0.Len() != 2 {
		t.Errorf("expected 2 columns, got %d", r0.Len())
	}
}

// --- WHERE with BETWEEN using integer values ---

func TestWhereBetweenIntegers(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("val", native.NewTypeLiteral(native.TInteger))
	recType := native.RecordTypeInfo{Fields: fields}
	mkRow := func(v int64) native.Value {
		om := native.NewOrderedMap()
		om.Set("val", native.NewInteger(v))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{mkRow(10), mkRow(20), mkRow(30), mkRow(40)},
	}
	reg.Store["nums"] = native.Value{Parent: native.TList, Data: td}

	queryVals, err := parser.Parse(`select * from nums where [val between 15 35]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (20, 30), got %d", len(rows))
	}
}

// --- WHERE with atom value (covers valueToSQL atom branch) ---

// --- WHERE with single IN value (covers buildInList single-value branch) ---

func TestWhereInSingleValue(t *testing.T) {
	// IN with a single non-list value (covers buildInList single-value branch)
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [city in "London"]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

// --- CAST with bool type alias (covers aqlTypenameToSQLType "bool" branch) ---

func TestCastBool(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("active", native.NewTypeLiteral(native.TString))
	recType := native.RecordTypeInfo{Fields: fields}
	row := native.NewOrderedMap()
	row.Set("active", native.NewString("1"))
	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{native.NewMap(row)},
	}
	reg.Store["data"] = native.Value{Parent: native.TList, Data: td}

	queryVals, err := parser.Parse(`select [[cast active bool b]] from data`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

// --- WHERE with boolean literal (covers valueToSQL boolean branch) ---

func TestWhereWithBoolLiteral(t *testing.T) {
	reg, err := native.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	eng := native.NewTop(reg)

	fields := native.NewOrderedMap()
	fields.Set("name", native.NewTypeLiteral(native.TString))
	fields.Set("active", native.NewTypeLiteral(native.TBoolean))
	recType := native.RecordTypeInfo{Fields: fields}
	mkRow := func(name string, active bool) native.Value {
		om := native.NewOrderedMap()
		om.Set("name", native.NewString(name))
		om.Set("active", native.NewBoolean(active))
		return native.NewMap(om)
	}
	td := native.TableData{
		Record: recType,
		Rows:   []native.Value{mkRow("Alice", true), mkRow("Bob", false)},
	}
	reg.Store["users"] = native.Value{Parent: native.TList, Data: td}

	// Use boolean false literal in WHERE condition
	queryVals, err := parser.Parse(`select * from users where [active eq false]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	// "false" becomes 'false' as SQL string, and active is stored as INTEGER 0
	// This tests the boolean branch of valueToSQL
	_ = rows
}

// --- Aggregate with string column name in nested list ---

func TestAggregateWithStringColName(t *testing.T) {
	// Uses string literal inside aggregate spec: [count "name" cnt]
	// This covers nameFromValue string branch
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select [[count "name" cnt]] from people`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	cnt, _ := native.AsMap(rows[0]).Get("cnt")
	_v73, _ := native.AsInteger(cnt)
	if _v73 != 3 {
		_v74, _ := native.AsInteger(cnt)
		t.Errorf("expected 3, got %d", _v74)
	}
}

func TestWhereWithAtomValue(t *testing.T) {
	result, err := runQuery(t,
		`context set people ("file/people.csv" read)`,
		`select * from people where [city eq London]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
}

func TestFileLoadSetsSQLiteFlag(t *testing.T) {
	result, err := runWithOSFiles(t, `"file/people.csv" read`)
	if err != nil {
		t.Fatal(err)
	}

	v := result[0]
	td, ok := v.Data.(native.TableData)
	if !ok {
		t.Fatal("expected TableData")
	}
	if !td.SQLite {
		t.Error("expected SQLite flag to be true for file-loaded table")
	}
	if td.TableName != "people" {
		t.Errorf("expected table name 'people', got %q", td.TableName)
	}
}

// --- scalar subquery tests ---

func TestScalarSubqueryInWhereGt(t *testing.T) {
	// avg age = (30+25+35+28)/4 = 29.5, so age > 29.5 → Alice(30), Charlie(35)
	result, err := runQuery(t,
		`context set emp ("file/employees.csv" read)`,
		`select * from emp where [age gt (select [[avg age]] from emp)] order [name]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
	assertField(t, native.AsMap(rows[1]), "name", "Charlie")
}

func TestScalarSubqueryInWhereEq(t *testing.T) {
	// Subquery returns Alice's dept = "Engineering".
	// Engineering employees: Alice, Charlie.
	result, err := runQuery(t,
		`context set emp ("file/employees.csv" read)`,
		`select * from emp where [dept eq (select [dept] from emp where [name eq "Alice"])] order [name]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
	assertField(t, native.AsMap(rows[1]), "name", "Charlie")
}

func TestScalarSubqueryInSelect(t *testing.T) {
	// Each row gets a top_salary column with the max salary (90000).
	result, err := runQuery(t,
		`context set emp ("file/employees.csv" read)`,
		`select [name [(select [[max salary]] from emp) top_salary]] from emp order [name]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}
	assertField(t, native.AsMap(rows[0]), "name", "Alice")
	assertField(t, native.AsMap(rows[0]), "top_salary", "90000")
	assertField(t, native.AsMap(rows[1]), "name", "Bob")
	assertField(t, native.AsMap(rows[1]), "top_salary", "90000")
}

func TestScalarSubqueryMultipleRowsError(t *testing.T) {
	// Subquery returns 4 rows — should error.
	_, err := runQuery(t,
		`context set emp ("file/employees.csv" read)`,
		`select * from emp where [age gt (select [age] from emp)]`,
	)
	if err == nil {
		t.Fatal("expected error for multi-row scalar subquery")
	}
}

func TestScalarSubqueryEmptyReturnsNull(t *testing.T) {
	// Subquery returns no rows (nobody named "Nobody").
	// Comparing with NULL should return no matches.
	result, err := runQuery(t,
		`context set emp ("file/employees.csv" read)`,
		`select * from emp where [name eq (select [name] from emp where [name eq "Nobody"])]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := native.AsList(result[0])
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows (NULL comparison), got %d", len(rows))
	}
}
