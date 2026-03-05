package test

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

// runQuery sets up a registry, loads a CSV file, stores it, and runs a query.
func runQuery(t *testing.T, setup string, query string) ([]engine.Value, error) {
	t.Helper()
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

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
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 value, got %d", len(result))
	}

	v := result[0]
	if !v.IsTableType() {
		t.Fatalf("expected table type, got %s", v.VType)
	}

	rows := v.AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Check all columns are present.
	r0 := rows[0].AsMap()
	assertField(t, r0, "name", "Alice")
	assertField(t, r0, "age", "30")
	assertField(t, r0, "city", "London")
}

func TestFromUnknownTable(t *testing.T) {
	_, err := runQuery(t, "", `select * from nonexistent`)
	if err == nil {
		t.Fatal("expected error for unknown table")
	}
}

// --- select * ---

func TestSelectStarFromFile(t *testing.T) {
	result, err := runQuery(t,
		`set items ("file/items.tsv" read)`,
		`select * from items`,
	)
	if err != nil {
		t.Fatal(err)
	}

	v := result[0]
	rows := v.AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	r0 := rows[0].AsMap()
	assertField(t, r0, "id", "1")
	assertField(t, r0, "name", "Widget")
	assertField(t, r0, "price", "9.99")
}

// --- select [cols] ---

func TestSelectSpecificColumns(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select [name city] from people`,
	)
	if err != nil {
		t.Fatal(err)
	}

	v := result[0]
	rows := v.AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Should only have name and city columns.
	r0 := rows[0].AsMap()
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
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select [[name person_name] city] from people`,
	)
	if err != nil {
		t.Fatal(err)
	}

	v := result[0]
	rows := v.AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// name should be aliased to person_name.
	r0 := rows[0].AsMap()
	assertField(t, r0, "person_name", "Alice")
	assertField(t, r0, "city", "London")

	// Original name column should not be present.
	if _, ok := r0.Get("name"); ok {
		t.Error("original 'name' column should be aliased away")
	}
}

// --- select against internal (non-file) tables ---

func TestSelectAgainstInternalTable(t *testing.T) {
	// Build a table manually without SQLite backing.
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	// Create a table using AQL type system.
	fields := engine.NewOrderedMap()
	fields.Set("color", engine.NewTypeLiteral(engine.TString))
	fields.Set("count", engine.NewTypeLiteral(engine.TString))
	recType := engine.RecordTypeInfo{Fields: fields}

	row1 := engine.NewOrderedMap()
	row1.Set("color", engine.NewString("red"))
	row1.Set("count", engine.NewString("5"))
	row2 := engine.NewOrderedMap()
	row2.Set("color", engine.NewString("blue"))
	row2.Set("count", engine.NewString("3"))

	td := engine.TableData{
		Record: recType,
		Rows:   []engine.Value{engine.NewMap(row1), engine.NewMap(row2)},
		SQLite: false, // not backed by SQLite
	}
	tableVal := engine.Value{VType: engine.TList, Data: td}

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

	rows := result[0].AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	r0 := rows[0].AsMap()
	assertField(t, r0, "color", "red")
	if r0.Len() != 1 {
		t.Errorf("expected 1 column, got %d", r0.Len())
	}
}

// --- star word ---

func TestStarWord(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select star from people`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	r0 := rows[0].AsMap()
	assertField(t, r0, "name", "Alice")
	assertField(t, r0, "age", "30")
	assertField(t, r0, "city", "London")
}

// --- currying (partial application via def) ---

func TestCurriedFrom(t *testing.T) {
	// def from01 from people end; select * from01
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	steps := []string{
		`set people ("file/people.csv" read)`,
		`def from01 from people end`,
		`select * from01`,
	}

	var result []engine.Value
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

	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	r0 := rows[0].AsMap()
	assertField(t, r0, "name", "Alice")
}

func TestCurriedSelect(t *testing.T) {
	// def select01 select star end; select01 from people
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	steps := []string{
		`set people ("file/people.csv" read)`,
		`def select01 select star end`,
		`select01 from people`,
	}

	var result []engine.Value
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

	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

func TestCurriedBoth(t *testing.T) {
	// def select01 select star end; def from01 from people end; select01 from01
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	steps := []string{
		`set people ("file/people.csv" read)`,
		`def select01 select star end`,
		`def from01 from people end`,
		`select01 from01`,
	}

	var result []engine.Value
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

	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	r0 := rows[0].AsMap()
	assertField(t, r0, "name", "Alice")
}

func TestCurriedSelectCols(t *testing.T) {
	// def sel_name select [name] end; sel_name from people
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	steps := []string{
		`set people ("file/people.csv" read)`,
		`def sel_name select [name] end`,
		`sel_name from people`,
	}

	var result []engine.Value
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

	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	r0 := rows[0].AsMap()
	assertField(t, r0, "name", "Alice")
	if r0.Len() != 1 {
		t.Errorf("expected 1 column, got %d", r0.Len())
	}
}

// --- where ---

func TestWhereBasic(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people where [name eq "Alice"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, rows[0].AsMap(), "name", "Alice")
	assertField(t, rows[0].AsMap(), "age", "30")
}

func TestWhereNumericComparison(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people where [age gt "25"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (age 30 and 35), got %d", len(rows))
	}
}

func TestWhereLt(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people where [age lt "30"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (age 25), got %d", len(rows))
	}
	assertField(t, rows[0].AsMap(), "name", "Bob")
}

func TestWhereAnd(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people where [age gte "30" and city eq "London"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, rows[0].AsMap(), "name", "Alice")
}

func TestWhereOr(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people where [city eq "London" or city eq "Tokyo"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestWhereWithColumns(t *testing.T) {
	// Use parens so where filters before select projects columns.
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select [name] (from people where [city eq "Paris"])`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, rows[0].AsMap(), "name", "Bob")
	if rows[0].AsMap().Len() != 1 {
		t.Errorf("expected 1 column, got %d", rows[0].AsMap().Len())
	}
}

func TestWhereNoMatch(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people where [name eq "Nobody"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

func TestWhereLike(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people where [name like "A%"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, rows[0].AsMap(), "name", "Alice")
}

func TestWhereNeq(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people where [name neq "Alice"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	assertField(t, rows[0].AsMap(), "name", "Bob")
	assertField(t, rows[1].AsMap(), "name", "Charlie")
}

// --- order ---

func TestOrderByColumn(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people order [name]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Alphabetical: Alice, Bob, Charlie
	assertField(t, rows[0].AsMap(), "name", "Alice")
	assertField(t, rows[1].AsMap(), "name", "Bob")
	assertField(t, rows[2].AsMap(), "name", "Charlie")
}

func TestOrderByDesc(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people order [name desc]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Reverse alphabetical: Charlie, Bob, Alice
	assertField(t, rows[0].AsMap(), "name", "Charlie")
	assertField(t, rows[1].AsMap(), "name", "Bob")
	assertField(t, rows[2].AsMap(), "name", "Alice")
}

func TestOrderByAtom(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people order name`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	assertField(t, rows[0].AsMap(), "name", "Alice")
	assertField(t, rows[2].AsMap(), "name", "Charlie")
}

func TestOrderBySyntax(t *testing.T) {
	// "order by name" should work the same as "order name"
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people order by name`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	assertField(t, rows[0].AsMap(), "name", "Alice")
	assertField(t, rows[2].AsMap(), "name", "Charlie")
}

func TestOrderByListSyntax(t *testing.T) {
	// "order by [name desc]" should work
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people order by [name desc]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	assertField(t, rows[0].AsMap(), "name", "Charlie")
	assertField(t, rows[2].AsMap(), "name", "Alice")
}

// --- limit ---

func TestLimit(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people limit 2`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestLimitOne(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people limit 1`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

func TestLimitZero(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people limit 0`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

// --- chaining where + order + limit ---

func TestWhereOrderLimit(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people where [age gte "25"] order [name] limit 2`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// age >= 25 gives all 3, ordered by name: Alice, Bob, Charlie, limited to 2
	assertField(t, rows[0].AsMap(), "name", "Alice")
	assertField(t, rows[1].AsMap(), "name", "Bob")
}

func TestWhereAndOrder(t *testing.T) {
	// Use parens so where and order are applied before select projects columns.
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select [name] (from people where [age gte "30"] order [name desc])`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// age >= 30: Alice(30), Charlie(35), ordered desc: Charlie, Alice
	assertField(t, rows[0].AsMap(), "name", "Charlie")
	assertField(t, rows[1].AsMap(), "name", "Alice")
}

func TestOrderAndLimit(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people order [age] limit 1`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, rows[0].AsMap(), "name", "Bob") // youngest
}

// --- non-SQLite table ---

func TestWhereOnInternalTable(t *testing.T) {
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	fields := engine.NewOrderedMap()
	fields.Set("color", engine.NewTypeLiteral(engine.TString))
	fields.Set("count", engine.NewTypeLiteral(engine.TString))
	recType := engine.RecordTypeInfo{Fields: fields}

	row1 := engine.NewOrderedMap()
	row1.Set("color", engine.NewString("red"))
	row1.Set("count", engine.NewString("5"))
	row2 := engine.NewOrderedMap()
	row2.Set("color", engine.NewString("blue"))
	row2.Set("count", engine.NewString("3"))

	td := engine.TableData{
		Record: recType,
		Rows:   []engine.Value{engine.NewMap(row1), engine.NewMap(row2)},
		SQLite: false,
	}
	reg.Store["colors"] = engine.Value{VType: engine.TList, Data: td}

	queryVals, err := parser.Parse(`select * from colors where [color eq "red"]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, rows[0].AsMap(), "color", "red")
}

// --- SQLite flag on loaded table ---

// --- offset ---

func TestOffset(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people order [name] limit 2 offset 1`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// Ordered by name: Alice, Bob, Charlie; offset 1 skips Alice
	assertField(t, rows[0].AsMap(), "name", "Bob")
	assertField(t, rows[1].AsMap(), "name", "Charlie")
}

func TestLimitOffset(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people order [name] limit 1 offset 2`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	// Ordered: Alice(0), Bob(1), Charlie(2); offset 2, limit 1 → Charlie
	assertField(t, rows[0].AsMap(), "name", "Charlie")
}

// --- distinct ---

func TestDistinct(t *testing.T) {
	// Create a table with duplicate city values.
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select [city] from people`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows without distinct, got %d", len(rows))
	}

	// With distinct — all cities happen to be unique, so same count.
	result, err = runQuery(t,
		`set people ("file/people.csv" read)`,
		`select [city] (from people distinct)`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows = result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows with distinct (all unique), got %d", len(rows))
	}
}

func TestDistinctDuplicates(t *testing.T) {
	// Build a table with duplicate values.
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	fields := engine.NewOrderedMap()
	fields.Set("color", engine.NewTypeLiteral(engine.TString))
	recType := engine.RecordTypeInfo{Fields: fields}

	mkRow := func(color string) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("color", engine.NewString(color))
		return engine.NewMap(om)
	}
	td := engine.TableData{
		Record: recType,
		Rows:   []engine.Value{mkRow("red"), mkRow("blue"), mkRow("red"), mkRow("blue"), mkRow("red")},
		SQLite: false,
	}
	reg.Store["colors"] = engine.Value{VType: engine.TList, Data: td}

	queryVals, err := parser.Parse(`select [color] (from colors distinct)`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 distinct colors, got %d", len(rows))
	}
}

// --- nulls first / nulls last ---

func TestOrderNullsFirst(t *testing.T) {
	// Build a table with some NULL values.
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	fields := engine.NewOrderedMap()
	fields.Set("name", engine.NewTypeLiteral(engine.TString))
	fields.Set("score", engine.NewTypeLiteral(engine.TString))
	recType := engine.RecordTypeInfo{Fields: fields}

	mkRow := func(name, score string) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("name", engine.NewString(name))
		if score != "" {
			om.Set("score", engine.NewString(score))
		} else {
			om.Set("score", engine.NewString(""))
		}
		return engine.NewMap(om)
	}
	td := engine.TableData{
		Record: recType,
		Rows:   []engine.Value{mkRow("Alice", "90"), mkRow("Bob", ""), mkRow("Charlie", "80")},
		SQLite: false,
	}
	reg.Store["scores"] = engine.Value{VType: engine.TList, Data: td}

	// Order by score with nulls first — empty strings sort first.
	queryVals, err := parser.Parse(`select * from scores order [score asc nulls first]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Empty string sorts first with NULLS FIRST.
	assertField(t, rows[0].AsMap(), "name", "Bob")
}

// --- order by position ---

func TestOrderByPosition(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people order [1]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Column 1 is "name" — alphabetical: Alice, Bob, Charlie
	assertField(t, rows[0].AsMap(), "name", "Alice")
	assertField(t, rows[2].AsMap(), "name", "Charlie")
}

func TestOrderByPositionDesc(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people order [1 desc]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	assertField(t, rows[0].AsMap(), "name", "Charlie")
	assertField(t, rows[2].AsMap(), "name", "Alice")
}

// --- is null / is not null ---

func TestWhereIsNull(t *testing.T) {
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	fields := engine.NewOrderedMap()
	fields.Set("name", engine.NewTypeLiteral(engine.TString))
	fields.Set("email", engine.NewTypeLiteral(engine.TString))
	recType := engine.RecordTypeInfo{Fields: fields}

	mkRow := func(name, email string) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("name", engine.NewString(name))
		om.Set("email", engine.NewString(email))
		return engine.NewMap(om)
	}
	td := engine.TableData{
		Record: recType,
		Rows:   []engine.Value{mkRow("Alice", "alice@test.com"), mkRow("Bob", ""), mkRow("Charlie", "charlie@test.com")},
		SQLite: false,
	}
	reg.Store["users"] = engine.Value{VType: engine.TList, Data: td}

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

	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (all non-null TEXT), got %d", len(rows))
	}
}

// --- between ---

func TestWhereBetween(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people where [age between "25" "30"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (age 25 and 30), got %d", len(rows))
	}
}

func TestWhereNotBetween(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people where [age not between "25" "30"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (age 35), got %d", len(rows))
	}
	assertField(t, rows[0].AsMap(), "name", "Charlie")
}

func TestWhereBetweenAndOther(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people where [age between "25" "35" and city eq "London"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, rows[0].AsMap(), "name", "Alice")
}

// --- glob ---

func TestWhereGlob(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people where [name glob "A*"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertField(t, rows[0].AsMap(), "name", "Alice")
}

func TestWhereGlobCaseSensitive(t *testing.T) {
	// GLOB is case-sensitive unlike LIKE.
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people where [name glob "a*"]`,
	)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows (GLOB is case-sensitive), got %d", len(rows))
	}
}

// --- typed columns ---

func TestTypedIntegerColumn(t *testing.T) {
	// Create a table where "age" is TInteger, not TString.
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	fields := engine.NewOrderedMap()
	fields.Set("name", engine.NewTypeLiteral(engine.TString))
	fields.Set("age", engine.NewTypeLiteral(engine.TInteger))
	recType := engine.RecordTypeInfo{Fields: fields}

	mkRow := func(name string, age int64) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("name", engine.NewString(name))
		om.Set("age", engine.NewInteger(age))
		return engine.NewMap(om)
	}
	td := engine.TableData{
		Record: recType,
		Rows: []engine.Value{
			mkRow("Alice", 30),
			mkRow("Bob", 25),
			mkRow("Charlie", 35),
		},
		SQLite: false,
	}
	reg.Store["people"] = engine.Value{VType: engine.TList, Data: td}

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

	rows := result[0].AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (age 30 and 35), got %d", len(rows))
	}

	// Results should come back as integers, not strings.
	ageVal, ok := rows[0].AsMap().Get("age")
	if !ok {
		t.Fatal("expected age field")
	}
	if !ageVal.VType.Matches(engine.TInteger) {
		t.Errorf("expected age to be integer type, got %s", ageVal.VType)
	}
	if ageVal.AsInteger() != 30 {
		t.Errorf("expected age 30, got %d", ageVal.AsInteger())
	}

	// Ordered by age: 30, 35.
	age2, _ := rows[1].AsMap().Get("age")
	if age2.AsInteger() != 35 {
		t.Errorf("expected second row age 35, got %d", age2.AsInteger())
	}
}

func TestTypedBooleanColumn(t *testing.T) {
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	fields := engine.NewOrderedMap()
	fields.Set("name", engine.NewTypeLiteral(engine.TString))
	fields.Set("active", engine.NewTypeLiteral(engine.TBoolean))
	recType := engine.RecordTypeInfo{Fields: fields}

	mkRow := func(name string, active bool) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("name", engine.NewString(name))
		om.Set("active", engine.NewBoolean(active))
		return engine.NewMap(om)
	}
	td := engine.TableData{
		Record: recType,
		Rows: []engine.Value{
			mkRow("Alice", true),
			mkRow("Bob", false),
			mkRow("Charlie", true),
		},
		SQLite: false,
	}
	reg.Store["users"] = engine.Value{VType: engine.TList, Data: td}

	queryVals, err := parser.Parse(`select * from users where [active eq 1]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 active users, got %d", len(rows))
	}

	// Results should come back as booleans.
	activeVal, ok := rows[0].AsMap().Get("active")
	if !ok {
		t.Fatal("expected active field")
	}
	if !activeVal.VType.Matches(engine.TBoolean) {
		t.Errorf("expected active to be boolean type, got %s", activeVal.VType)
	}
	if !activeVal.AsBoolean() {
		t.Error("expected active to be true")
	}
}

func TestTypedIntegerOrdering(t *testing.T) {
	// This test verifies that INTEGER columns sort numerically, not lexically.
	// With TEXT: "9" > "25" > "100" (wrong). With INTEGER: 9 < 25 < 100 (correct).
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	fields := engine.NewOrderedMap()
	fields.Set("val", engine.NewTypeLiteral(engine.TInteger))
	recType := engine.RecordTypeInfo{Fields: fields}

	mkRow := func(val int64) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("val", engine.NewInteger(val))
		return engine.NewMap(om)
	}
	td := engine.TableData{
		Record: recType,
		Rows:   []engine.Value{mkRow(100), mkRow(9), mkRow(25)},
		SQLite: false,
	}
	reg.Store["nums"] = engine.Value{VType: engine.TList, Data: td}

	queryVals, err := parser.Parse(`select * from nums order [val]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Should be: 9, 25, 100 (numeric order, not "100", "25", "9").
	v0, _ := rows[0].AsMap().Get("val")
	v1, _ := rows[1].AsMap().Get("val")
	v2, _ := rows[2].AsMap().Get("val")
	if v0.AsInteger() != 9 || v1.AsInteger() != 25 || v2.AsInteger() != 100 {
		t.Errorf("expected [9, 25, 100], got [%d, %d, %d]", v0.AsInteger(), v1.AsInteger(), v2.AsInteger())
	}
}

// --- IN / NOT IN ---

func TestWhereIn(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people where [city in ["London" "Tokyo"]]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestWhereNotIn(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people where [city not in ["London" "Tokyo"]]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r0 := rows[0].AsMap()
	name, _ := r0.Get("name")
	if name.AsString() != "Bob" {
		t.Errorf("expected Bob, got %s", name.AsString())
	}
}

// --- GROUP BY / HAVING ---

func TestGroupByWithCount(t *testing.T) {
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	fields := engine.NewOrderedMap()
	fields.Set("dept", engine.NewTypeLiteral(engine.TString))
	fields.Set("name", engine.NewTypeLiteral(engine.TString))
	recType := engine.RecordTypeInfo{Fields: fields}

	mkRow := func(dept, name string) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("dept", engine.NewString(dept))
		om.Set("name", engine.NewString(name))
		return engine.NewMap(om)
	}
	td := engine.TableData{
		Record: recType,
		Rows: []engine.Value{
			mkRow("eng", "Alice"),
			mkRow("eng", "Bob"),
			mkRow("sales", "Charlie"),
			mkRow("eng", "Dave"),
			mkRow("sales", "Eve"),
		},
	}
	reg.Store["staff"] = engine.Value{VType: engine.TList, Data: td}

	queryVals, err := parser.Parse(`select [dept [count name cnt]] from staff groupby [dept] order [dept]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(rows))
	}

	r0 := rows[0].AsMap()
	dept0, _ := r0.Get("dept")
	cnt0, _ := r0.Get("cnt")
	if dept0.AsString() != "eng" {
		t.Errorf("expected dept eng, got %s", dept0.AsString())
	}
	if cnt0.AsInteger() != 3 {
		t.Errorf("expected count 3, got %d", cnt0.AsInteger())
	}

	r1 := rows[1].AsMap()
	cnt1, _ := r1.Get("cnt")
	if cnt1.AsInteger() != 2 {
		t.Errorf("expected count 2, got %d", cnt1.AsInteger())
	}
}

func TestHaving(t *testing.T) {
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	fields := engine.NewOrderedMap()
	fields.Set("dept", engine.NewTypeLiteral(engine.TString))
	fields.Set("name", engine.NewTypeLiteral(engine.TString))
	recType := engine.RecordTypeInfo{Fields: fields}

	mkRow := func(dept, name string) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("dept", engine.NewString(dept))
		om.Set("name", engine.NewString(name))
		return engine.NewMap(om)
	}
	td := engine.TableData{
		Record: recType,
		Rows: []engine.Value{
			mkRow("eng", "Alice"),
			mkRow("eng", "Bob"),
			mkRow("sales", "Charlie"),
			mkRow("eng", "Dave"),
		},
	}
	reg.Store["staff"] = engine.Value{VType: engine.TList, Data: td}

	// Only groups with count > 1
	queryVals, err := parser.Parse(`select [dept [count name cnt]] from staff groupby [dept] having [cnt gt 1]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 group (eng has 3), got %d", len(rows))
	}
	r0 := rows[0].AsMap()
	dept, _ := r0.Get("dept")
	if dept.AsString() != "eng" {
		t.Errorf("expected eng, got %s", dept.AsString())
	}
}

// --- Table aliases ---

func TestFromAlias(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select * from people as p`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

// --- JOINs ---

func TestInnerJoin(t *testing.T) {
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	// orders table
	oFields := engine.NewOrderedMap()
	oFields.Set("order_id", engine.NewTypeLiteral(engine.TString))
	oFields.Set("product", engine.NewTypeLiteral(engine.TString))
	oFields.Set("qty", engine.NewTypeLiteral(engine.TString))
	oRec := engine.RecordTypeInfo{Fields: oFields}
	mkOrder := func(id, product, qty string) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("order_id", engine.NewString(id))
		om.Set("product", engine.NewString(product))
		om.Set("qty", engine.NewString(qty))
		return engine.NewMap(om)
	}
	oTD := engine.TableData{
		Record: oRec,
		Rows: []engine.Value{
			mkOrder("1", "widget", "10"),
			mkOrder("2", "gadget", "5"),
			mkOrder("3", "widget", "3"),
		},
	}
	reg.Store["orders"] = engine.Value{VType: engine.TList, Data: oTD}

	// products table
	pFields := engine.NewOrderedMap()
	pFields.Set("product", engine.NewTypeLiteral(engine.TString))
	pFields.Set("price", engine.NewTypeLiteral(engine.TString))
	pRec := engine.RecordTypeInfo{Fields: pFields}
	mkProduct := func(name, price string) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("product", engine.NewString(name))
		om.Set("price", engine.NewString(price))
		return engine.NewMap(om)
	}
	pTD := engine.TableData{
		Record: pRec,
		Rows: []engine.Value{
			mkProduct("widget", "9.99"),
			mkProduct("gadget", "19.99"),
		},
	}
	reg.Store["products"] = engine.Value{VType: engine.TList, Data: pTD}

	queryVals, err := parser.Parse(`select * from orders join products using [product] order [order_id]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 joined rows, got %d", len(rows))
	}

	// First row should have both order and product fields.
	r0 := rows[0].AsMap()
	oid, _ := r0.Get("order_id")
	price, _ := r0.Get("price")
	if oid.AsString() != "1" {
		t.Errorf("expected order_id 1, got %s", oid.AsString())
	}
	if price.AsString() != "9.99" {
		t.Errorf("expected price 9.99, got %s", price.AsString())
	}
}

func TestLeftJoin(t *testing.T) {
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	// people
	pFields := engine.NewOrderedMap()
	pFields.Set("name", engine.NewTypeLiteral(engine.TString))
	pFields.Set("dept_id", engine.NewTypeLiteral(engine.TString))
	pRec := engine.RecordTypeInfo{Fields: pFields}
	mkPerson := func(name, deptID string) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("name", engine.NewString(name))
		om.Set("dept_id", engine.NewString(deptID))
		return engine.NewMap(om)
	}
	pTD := engine.TableData{
		Record: pRec,
		Rows: []engine.Value{
			mkPerson("Alice", "1"),
			mkPerson("Bob", "2"),
			mkPerson("Charlie", "99"), // no matching dept
		},
	}
	reg.Store["people"] = engine.Value{VType: engine.TList, Data: pTD}

	// depts
	dFields := engine.NewOrderedMap()
	dFields.Set("dept_id", engine.NewTypeLiteral(engine.TString))
	dFields.Set("dept_name", engine.NewTypeLiteral(engine.TString))
	dRec := engine.RecordTypeInfo{Fields: dFields}
	mkDept := func(id, name string) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("dept_id", engine.NewString(id))
		om.Set("dept_name", engine.NewString(name))
		return engine.NewMap(om)
	}
	dTD := engine.TableData{
		Record: dRec,
		Rows: []engine.Value{
			mkDept("1", "Engineering"),
			mkDept("2", "Sales"),
		},
	}
	reg.Store["depts"] = engine.Value{VType: engine.TList, Data: dTD}

	queryVals, err := parser.Parse(`select * from people leftjoin depts using [dept_id] order [name]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (left join preserves all left rows), got %d", len(rows))
	}

	// Charlie should have NULL dept_name.
	r2 := rows[2].AsMap()
	name2, _ := r2.Get("name")
	if name2.AsString() != "Charlie" {
		t.Errorf("expected Charlie, got %s", name2.AsString())
	}
}

// --- Set operations ---

func TestUnion(t *testing.T) {
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	mkTable := func(names ...string) engine.TableData {
		fields := engine.NewOrderedMap()
		fields.Set("name", engine.NewTypeLiteral(engine.TString))
		recType := engine.RecordTypeInfo{Fields: fields}
		rows := make([]engine.Value, len(names))
		for i, n := range names {
			om := engine.NewOrderedMap()
			om.Set("name", engine.NewString(n))
			rows[i] = engine.NewMap(om)
		}
		return engine.TableData{Record: recType, Rows: rows}
	}

	reg.Store["t1"] = engine.Value{VType: engine.TList, Data: mkTable("Alice", "Bob")}
	reg.Store["t2"] = engine.Value{VType: engine.TList, Data: mkTable("Bob", "Charlie")}

	// UNION removes duplicates.
	queryVals, err := parser.Parse(`select * (from t1 union from t2) order [name]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 unique rows, got %d", len(rows))
	}
}

func TestUnionAll(t *testing.T) {
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	mkTable := func(names ...string) engine.TableData {
		fields := engine.NewOrderedMap()
		fields.Set("name", engine.NewTypeLiteral(engine.TString))
		recType := engine.RecordTypeInfo{Fields: fields}
		rows := make([]engine.Value, len(names))
		for i, n := range names {
			om := engine.NewOrderedMap()
			om.Set("name", engine.NewString(n))
			rows[i] = engine.NewMap(om)
		}
		return engine.TableData{Record: recType, Rows: rows}
	}

	reg.Store["t1"] = engine.Value{VType: engine.TList, Data: mkTable("Alice", "Bob")}
	reg.Store["t2"] = engine.Value{VType: engine.TList, Data: mkTable("Bob", "Charlie")}

	// UNION ALL keeps duplicates.
	queryVals, err := parser.Parse(`select * (from t1 unionall from t2) order [name]`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows (with duplicate Bob), got %d", len(rows))
	}
}

func TestIntersect(t *testing.T) {
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	mkTable := func(names ...string) engine.TableData {
		fields := engine.NewOrderedMap()
		fields.Set("name", engine.NewTypeLiteral(engine.TString))
		recType := engine.RecordTypeInfo{Fields: fields}
		rows := make([]engine.Value, len(names))
		for i, n := range names {
			om := engine.NewOrderedMap()
			om.Set("name", engine.NewString(n))
			rows[i] = engine.NewMap(om)
		}
		return engine.TableData{Record: recType, Rows: rows}
	}

	reg.Store["t1"] = engine.Value{VType: engine.TList, Data: mkTable("Alice", "Bob")}
	reg.Store["t2"] = engine.Value{VType: engine.TList, Data: mkTable("Bob", "Charlie")}

	queryVals, err := parser.Parse(`select * (from t1 intersect from t2)`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (Bob), got %d", len(rows))
	}
	name, _ := rows[0].AsMap().Get("name")
	if name.AsString() != "Bob" {
		t.Errorf("expected Bob, got %s", name.AsString())
	}
}

func TestExcept(t *testing.T) {
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	mkTable := func(names ...string) engine.TableData {
		fields := engine.NewOrderedMap()
		fields.Set("name", engine.NewTypeLiteral(engine.TString))
		recType := engine.RecordTypeInfo{Fields: fields}
		rows := make([]engine.Value, len(names))
		for i, n := range names {
			om := engine.NewOrderedMap()
			om.Set("name", engine.NewString(n))
			rows[i] = engine.NewMap(om)
		}
		return engine.TableData{Record: recType, Rows: rows}
	}

	reg.Store["t1"] = engine.Value{VType: engine.TList, Data: mkTable("Alice", "Bob")}
	reg.Store["t2"] = engine.Value{VType: engine.TList, Data: mkTable("Bob", "Charlie")}

	queryVals, err := parser.Parse(`select * (from t1 except from t2)`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (Alice), got %d", len(rows))
	}
	name, _ := rows[0].AsMap().Get("name")
	if name.AsString() != "Alice" {
		t.Errorf("expected Alice, got %s", name.AsString())
	}
}

// --- CAST ---

func TestCast(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select [[cast age integer age_int]] from people order [age_int]`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// CAST should produce integer ordering: 25, 30, 35 (not "25", "30", "35").
	v0, _ := rows[0].AsMap().Get("age_int")
	v1, _ := rows[1].AsMap().Get("age_int")
	v2, _ := rows[2].AsMap().Get("age_int")
	if v0.AsInteger() != 25 || v1.AsInteger() != 30 || v2.AsInteger() != 35 {
		t.Errorf("expected [25, 30, 35], got [%d, %d, %d]", v0.AsInteger(), v1.AsInteger(), v2.AsInteger())
	}
}

// --- Aggregate words standalone ---

func TestCountStar(t *testing.T) {
	result, err := runQuery(t,
		`set people ("file/people.csv" read)`,
		`select [[count * total]] from people`,
	)
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	cnt, _ := rows[0].AsMap().Get("total")
	if cnt.AsInteger() != 3 {
		t.Errorf("expected count 3, got %d", cnt.AsInteger())
	}
}

func TestSumAggregate(t *testing.T) {
	reg := engine.DefaultRegistry()
	eng := engine.New(reg)

	fields := engine.NewOrderedMap()
	fields.Set("val", engine.NewTypeLiteral(engine.TInteger))
	recType := engine.RecordTypeInfo{Fields: fields}

	mkRow := func(v int64) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("val", engine.NewInteger(v))
		return engine.NewMap(om)
	}
	td := engine.TableData{
		Record: recType,
		Rows:   []engine.Value{mkRow(10), mkRow(20), mkRow(30)},
	}
	reg.Store["nums"] = engine.Value{VType: engine.TList, Data: td}

	queryVals, err := parser.Parse(`select [[sum val total]] from nums`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := eng.Run(queryVals)
	if err != nil {
		t.Fatal(err)
	}

	rows := result[0].AsList()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	total, _ := rows[0].AsMap().Get("total")
	if total.AsInteger() != 60 {
		t.Errorf("expected sum 60, got %d", total.AsInteger())
	}
}

// --- SQLite flag on loaded table ---

func TestFileLoadSetsSQLiteFlag(t *testing.T) {
	result, err := runWithOSFiles(t, `"file/people.csv" read`)
	if err != nil {
		t.Fatal(err)
	}

	v := result[0]
	td, ok := v.Data.(engine.TableData)
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
