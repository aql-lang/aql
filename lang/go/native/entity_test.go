package native

import (
	"testing"
)

// makeEntityTable creates a table with id, name, city columns for entity tests.
func makeEntityTable(rows [][]Value) Value {
	return makeTable(
		[]string{"id", "name", "city"},
		rows,
	)
}

func TestCreateHandler(t *testing.T) {
	table := makeEntityTable([][]Value{
		{NewString("1"), NewString("Alice"), NewString("London")},
	})

	rec := NewOrderedMap()
	rec.Set("id", NewString("2"))
	rec.Set("name", NewString("Bob"))
	rec.Set("city", NewString("Paris"))

	result, err := createHandler([]Value{NewMap(rec), table}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	m, _ := AsMap(rows[1])
	v, _ := m.Get("name")
	vs, _ := AsString(v)
	if vs != "Bob" {
		t.Errorf("expected Bob, got %s", vs)
	}
}

func TestCreateHandlerDuplicateId(t *testing.T) {
	table := makeEntityTable([][]Value{
		{NewString("1"), NewString("Alice"), NewString("London")},
	})

	rec := NewOrderedMap()
	rec.Set("id", NewString("1"))
	rec.Set("name", NewString("Bob"))

	_, err := createHandler([]Value{NewMap(rec), table}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for duplicate id")
	}
}

func TestCreateHandlerNoId(t *testing.T) {
	table := makeEntityTable(nil)

	rec := NewOrderedMap()
	rec.Set("name", NewString("Bob"))

	_, err := createHandler([]Value{NewMap(rec), table}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestLoadHandler(t *testing.T) {
	table := makeEntityTable([][]Value{
		{NewString("1"), NewString("Alice"), NewString("London")},
		{NewString("2"), NewString("Bob"), NewString("Paris")},
	})

	filter := NewOrderedMap()
	filter.Set("id", NewString("2"))

	result, err := loadHandler([]Value{NewMap(filter), table}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	m, _ := AsMap(result[0])
	v, _ := m.Get("name")
	vs, _ := AsString(v)
	if vs != "Bob" {
		t.Errorf("expected Bob, got %s", vs)
	}
}

func TestLoadHandlerNotFound(t *testing.T) {
	table := makeEntityTable([][]Value{
		{NewString("1"), NewString("Alice"), NewString("London")},
	})

	filter := NewOrderedMap()
	filter.Set("id", NewString("99"))

	_, err := loadHandler([]Value{NewMap(filter), table}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestUpdateHandler(t *testing.T) {
	table := makeEntityTable([][]Value{
		{NewString("1"), NewString("Alice"), NewString("London")},
		{NewString("2"), NewString("Bob"), NewString("Paris")},
	})

	patch := NewOrderedMap()
	patch.Set("id", NewString("1"))
	patch.Set("city", NewString("Berlin"))

	result, err := updateHandler([]Value{NewMap(patch), table}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// Check first row was updated.
	m, _ := AsMap(rows[0])
	city, _ := m.Get("city")
	cs, _ := AsString(city)
	if cs != "Berlin" {
		t.Errorf("expected Berlin, got %s", cs)
	}
	// Name should be preserved.
	name, _ := m.Get("name")
	ns, _ := AsString(name)
	if ns != "Alice" {
		t.Errorf("expected Alice, got %s", ns)
	}
	// Second row should be unchanged.
	m2, _ := AsMap(rows[1])
	city2, _ := m2.Get("city")
	cs2, _ := AsString(city2)
	if cs2 != "Paris" {
		t.Errorf("expected Paris, got %s", cs2)
	}
}

func TestUpdateHandlerNotFound(t *testing.T) {
	table := makeEntityTable([][]Value{
		{NewString("1"), NewString("Alice"), NewString("London")},
	})

	patch := NewOrderedMap()
	patch.Set("id", NewString("99"))
	patch.Set("city", NewString("Berlin"))

	_, err := updateHandler([]Value{NewMap(patch), table}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestUpdateHandlerNoId(t *testing.T) {
	table := makeEntityTable(nil)

	patch := NewOrderedMap()
	patch.Set("city", NewString("Berlin"))

	_, err := updateHandler([]Value{NewMap(patch), table}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestRemoveHandler(t *testing.T) {
	table := makeEntityTable([][]Value{
		{NewString("1"), NewString("Alice"), NewString("London")},
		{NewString("2"), NewString("Bob"), NewString("Paris")},
		{NewString("3"), NewString("Carol"), NewString("Berlin")},
	})

	filter := NewOrderedMap()
	filter.Set("id", NewString("2"))

	result, err := removeHandler([]Value{NewMap(filter), table}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_lst, _ := AsList(result[0])
	rows := _lst.Slice()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// Verify Bob is gone.
	for _, row := range rows {
		m, _ := AsMap(row)
		v, _ := m.Get("name")
		vs, _ := AsString(v)
		if vs == "Bob" {
			t.Error("Bob should have been removed")
		}
	}
}

func TestRemoveHandlerNotFound(t *testing.T) {
	table := makeEntityTable([][]Value{
		{NewString("1"), NewString("Alice"), NewString("London")},
	})

	filter := NewOrderedMap()
	filter.Set("id", NewString("99"))

	_, err := removeHandler([]Value{NewMap(filter), table}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestRemoveHandlerNoId(t *testing.T) {
	table := makeEntityTable(nil)

	filter := NewOrderedMap()
	filter.Set("name", NewString("Alice"))

	_, err := removeHandler([]Value{NewMap(filter), table}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}
