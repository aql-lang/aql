package native

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// makeTable creates a table Value with the given column names and rows.
// Each row is a map with the column names as keys.
func makeTable(columns []string, rows [][]engine.Value) engine.Value {
	fields := engine.NewOrderedMap()
	for _, col := range columns {
		fields.Set(col, engine.NewTypeLiteral(engine.TAny))
	}
	rec := engine.RecordTypeInfo{Fields: fields}

	var rowValues []engine.Value
	for _, row := range rows {
		m := engine.NewOrderedMap()
		for i, col := range columns {
			m.Set(col, row[i])
		}
		rowValues = append(rowValues, engine.NewMap(m))
	}

	return engine.Value{VType: engine.TList, Data: engine.TableData{
		Record: rec,
		Rows:   rowValues,
	}}
}

func TestListAllHandler(t *testing.T) {
	table := makeTable(
		[]string{"name", "age"},
		[][]engine.Value{
			{engine.NewString("alice"), engine.NewInteger(30)},
			{engine.NewString("bob"), engine.NewInteger(25)},
		},
	)

	result, err := listAllHandler([]engine.Value{table}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	list := result[0].AsList().Slice()
	if len(list) != 2 {
		t.Errorf("expected 2 rows, got %d", len(list))
	}
}

func TestListFilterHandler(t *testing.T) {
	table := makeTable(
		[]string{"name", "age", "city"},
		[][]engine.Value{
			{engine.NewString("alice"), engine.NewInteger(30), engine.NewString("london")},
			{engine.NewString("bob"), engine.NewInteger(25), engine.NewString("paris")},
			{engine.NewString("carol"), engine.NewInteger(30), engine.NewString("london")},
		},
	)

	// Filter by city == "london"
	filter := engine.NewOrderedMap()
	filter.Set("city", engine.NewString("london"))

	result, err := listFilterHandler(
		[]engine.Value{engine.NewMap(filter), table},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	list := result[0].AsList().Slice()
	if len(list) != 2 {
		t.Errorf("expected 2 matching rows, got %d", len(list))
	}

	// Check the names
	for _, row := range list {
		m := row.AsMap()
		nameVal, _ := m.Get("name")
		name, _ := nameVal.AsString()
		if name != "alice" && name != "carol" {
			t.Errorf("unexpected name: %s", name)
		}
	}
}

func TestListFilterMultipleKeys(t *testing.T) {
	table := makeTable(
		[]string{"name", "age", "city"},
		[][]engine.Value{
			{engine.NewString("alice"), engine.NewInteger(30), engine.NewString("london")},
			{engine.NewString("bob"), engine.NewInteger(25), engine.NewString("paris")},
			{engine.NewString("carol"), engine.NewInteger(30), engine.NewString("paris")},
		},
	)

	// Filter by age == 30 AND city == "london"
	filter := engine.NewOrderedMap()
	filter.Set("age", engine.NewInteger(30))
	filter.Set("city", engine.NewString("london"))

	result, err := listFilterHandler(
		[]engine.Value{engine.NewMap(filter), table},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 1 {
		t.Errorf("expected 1 matching row, got %d", len(list))
	}
	if len(list) > 0 {
		m := list[0].AsMap()
		nameVal, _ := m.Get("name")
		ns, _ := nameVal.AsString()
		if ns != "alice" {
			t.Errorf("expected alice, got %s", ns)
		}
	}
}

func TestListFilterNoMatch(t *testing.T) {
	table := makeTable(
		[]string{"name", "age"},
		[][]engine.Value{
			{engine.NewString("alice"), engine.NewInteger(30)},
		},
	)

	filter := engine.NewOrderedMap()
	filter.Set("name", engine.NewString("nobody"))

	result, err := listFilterHandler(
		[]engine.Value{engine.NewMap(filter), table},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 matching rows, got %d", len(list))
	}
}

func TestListFilterMissingField(t *testing.T) {
	table := makeTable(
		[]string{"name", "age"},
		[][]engine.Value{
			{engine.NewString("alice"), engine.NewInteger(30)},
		},
	)

	// Filter on field that doesn't exist in records
	filter := engine.NewOrderedMap()
	filter.Set("city", engine.NewString("london"))

	result, err := listFilterHandler(
		[]engine.Value{engine.NewMap(filter), table},
		nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 matching rows, got %d", len(list))
	}
}

func TestListAllEmptyTable(t *testing.T) {
	table := makeTable([]string{"name"}, nil)

	result, err := listAllHandler([]engine.Value{table}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].AsList().Slice()
	if len(list) != 0 {
		t.Errorf("expected 0 rows, got %d", len(list))
	}
}

func TestValuesEqual(t *testing.T) {
	tests := []struct {
		a, b engine.Value
		want bool
	}{
		{engine.NewInteger(1), engine.NewInteger(1), true},
		{engine.NewInteger(1), engine.NewInteger(2), false},
		{engine.NewString("a"), engine.NewString("a"), true},
		{engine.NewString("a"), engine.NewString("b"), false},
		{engine.NewBoolean(true), engine.NewBoolean(true), true},
		{engine.NewBoolean(true), engine.NewBoolean(false), false},
		{engine.NewAtom("x"), engine.NewAtom("x"), true},
		{engine.NewAtom("x"), engine.NewString("x"), true},
		{engine.NewString("x"), engine.NewAtom("x"), true},
	}
	for i, tt := range tests {
		got := valuesEqual(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("test %d: valuesEqual(%s, %s) = %v, want %v",
				i, tt.a, tt.b, got, tt.want)
		}
	}
}
