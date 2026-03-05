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
