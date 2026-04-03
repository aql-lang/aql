package native

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
)

// makeEntityTable creates a table with id, name, city columns for entity tests.
func makeEntityTable(rows [][]engine.Value) engine.Value {
	return makeTable(
		[]string{"id", "name", "city"},
		rows,
	)
}

func TestCreateHandler(t *testing.T) {
	table := makeEntityTable([][]engine.Value{
		{engine.NewString("1"), engine.NewString("Alice"), engine.NewString("London")},
	})

	rec := engine.NewOrderedMap()
	rec.Set("id", engine.NewString("2"))
	rec.Set("name", engine.NewString("Bob"))
	rec.Set("city", engine.NewString("Paris"))

	result, err := createHandler([]engine.Value{table, engine.NewMap(rec)}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList().Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	m := rows[1].AsMap()
	v, _ := m.Get("name")
	vs, _ := v.AsString()
	if vs != "Bob" {
		t.Errorf("expected Bob, got %s", vs)
	}
}

func TestCreateHandlerDuplicateId(t *testing.T) {
	table := makeEntityTable([][]engine.Value{
		{engine.NewString("1"), engine.NewString("Alice"), engine.NewString("London")},
	})

	rec := engine.NewOrderedMap()
	rec.Set("id", engine.NewString("1"))
	rec.Set("name", engine.NewString("Bob"))

	_, err := createHandler([]engine.Value{table, engine.NewMap(rec)}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for duplicate id")
	}
}

func TestCreateHandlerNoId(t *testing.T) {
	table := makeEntityTable(nil)

	rec := engine.NewOrderedMap()
	rec.Set("name", engine.NewString("Bob"))

	_, err := createHandler([]engine.Value{table, engine.NewMap(rec)}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestLoadHandler(t *testing.T) {
	table := makeEntityTable([][]engine.Value{
		{engine.NewString("1"), engine.NewString("Alice"), engine.NewString("London")},
		{engine.NewString("2"), engine.NewString("Bob"), engine.NewString("Paris")},
	})

	filter := engine.NewOrderedMap()
	filter.Set("id", engine.NewString("2"))

	result, err := loadHandler([]engine.Value{table, engine.NewMap(filter)}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m := result[0].AsMap()
	v, _ := m.Get("name")
	vs, _ := v.AsString()
	if vs != "Bob" {
		t.Errorf("expected Bob, got %s", vs)
	}
}

func TestLoadHandlerNotFound(t *testing.T) {
	table := makeEntityTable([][]engine.Value{
		{engine.NewString("1"), engine.NewString("Alice"), engine.NewString("London")},
	})

	filter := engine.NewOrderedMap()
	filter.Set("id", engine.NewString("99"))

	_, err := loadHandler([]engine.Value{table, engine.NewMap(filter)}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestUpdateHandler(t *testing.T) {
	table := makeEntityTable([][]engine.Value{
		{engine.NewString("1"), engine.NewString("Alice"), engine.NewString("London")},
		{engine.NewString("2"), engine.NewString("Bob"), engine.NewString("Paris")},
	})

	patch := engine.NewOrderedMap()
	patch.Set("id", engine.NewString("1"))
	patch.Set("city", engine.NewString("Berlin"))

	result, err := updateHandler([]engine.Value{table, engine.NewMap(patch)}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList().Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// Check first row was updated.
	m := rows[0].AsMap()
	city, _ := m.Get("city")
	cs, _ := city.AsString()
	if cs != "Berlin" {
		t.Errorf("expected Berlin, got %s", cs)
	}
	// Name should be preserved.
	name, _ := m.Get("name")
	ns, _ := name.AsString()
	if ns != "Alice" {
		t.Errorf("expected Alice, got %s", ns)
	}
	// Second row should be unchanged.
	m2 := rows[1].AsMap()
	city2, _ := m2.Get("city")
	cs2, _ := city2.AsString()
	if cs2 != "Paris" {
		t.Errorf("expected Paris, got %s", cs2)
	}
}

func TestUpdateHandlerNotFound(t *testing.T) {
	table := makeEntityTable([][]engine.Value{
		{engine.NewString("1"), engine.NewString("Alice"), engine.NewString("London")},
	})

	patch := engine.NewOrderedMap()
	patch.Set("id", engine.NewString("99"))
	patch.Set("city", engine.NewString("Berlin"))

	_, err := updateHandler([]engine.Value{table, engine.NewMap(patch)}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestUpdateHandlerNoId(t *testing.T) {
	table := makeEntityTable(nil)

	patch := engine.NewOrderedMap()
	patch.Set("city", engine.NewString("Berlin"))

	_, err := updateHandler([]engine.Value{table, engine.NewMap(patch)}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestRemoveHandler(t *testing.T) {
	table := makeEntityTable([][]engine.Value{
		{engine.NewString("1"), engine.NewString("Alice"), engine.NewString("London")},
		{engine.NewString("2"), engine.NewString("Bob"), engine.NewString("Paris")},
		{engine.NewString("3"), engine.NewString("Carol"), engine.NewString("Berlin")},
	})

	filter := engine.NewOrderedMap()
	filter.Set("id", engine.NewString("2"))

	result, err := removeHandler([]engine.Value{table, engine.NewMap(filter)}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows := result[0].AsList().Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// Verify Bob is gone.
	for _, row := range rows {
		m := row.AsMap()
		v, _ := m.Get("name")
		vs, _ := v.AsString()
		if vs == "Bob" {
			t.Error("Bob should have been removed")
		}
	}
}

func TestRemoveHandlerNotFound(t *testing.T) {
	table := makeEntityTable([][]engine.Value{
		{engine.NewString("1"), engine.NewString("Alice"), engine.NewString("London")},
	})

	filter := engine.NewOrderedMap()
	filter.Set("id", engine.NewString("99"))

	_, err := removeHandler([]engine.Value{table, engine.NewMap(filter)}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestRemoveHandlerNoId(t *testing.T) {
	table := makeEntityTable(nil)

	filter := engine.NewOrderedMap()
	filter.Set("name", engine.NewString("Alice"))

	_, err := removeHandler([]engine.Value{table, engine.NewMap(filter)}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}
