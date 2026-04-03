//go:build query
// +build query

package engine

import (
	"strings"
	"testing"
)

// ========================
// Helper: create tables for query coverage tests
// ========================

// makeProductsTable creates a "products" table with id, name, price, category.
func makeProductsTable(r *Registry) {
	fields := NewOrderedMap()
	fields.Set("id", NewTypeLiteral(TInteger))
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("price", NewTypeLiteral(TNumber))
	fields.Set("category", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	mkRow := func(id int64, name string, price int64, category string) Value {
		om := NewOrderedMap()
		om.Set("id", NewInteger(id))
		om.Set("name", NewString(name))
		om.Set("price", NewInteger(price))
		om.Set("category", NewString(category))
		return NewMap(om)
	}

	td := TableData{
		Record: rec,
		Rows: []Value{
			mkRow(1, "Widget", 10, "hardware"),
			mkRow(2, "Gadget", 25, "hardware"),
			mkRow(3, "Gizmo", 15, "electronics"),
			mkRow(4, "Doohickey", 30, "electronics"),
			mkRow(5, "Thingamajig", 5, "misc"),
		},
	}
	r.ContextSet("products", Value{VType: TList, Data: td})
}

// makeOrdersTable creates an "orders" table with order_id, product_id, qty.
func makeOrdersTable(r *Registry) {
	fields := NewOrderedMap()
	fields.Set("order_id", NewTypeLiteral(TInteger))
	fields.Set("product_id", NewTypeLiteral(TInteger))
	fields.Set("qty", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}

	mkRow := func(orderID, productID, qty int64) Value {
		om := NewOrderedMap()
		om.Set("order_id", NewInteger(orderID))
		om.Set("product_id", NewInteger(productID))
		om.Set("qty", NewInteger(qty))
		return NewMap(om)
	}

	td := TableData{
		Record: rec,
		Rows: []Value{
			mkRow(100, 1, 3),
			mkRow(101, 2, 1),
			mkRow(102, 3, 5),
			mkRow(103, 1, 2),
		},
	}
	r.ContextSet("orders", Value{VType: TList, Data: td})
}

// makeCitiesTable creates a "cities" table with city, country.
func makeCitiesTable(r *Registry) {
	fields := NewOrderedMap()
	fields.Set("city", NewTypeLiteral(TString))
	fields.Set("country", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	mkRow := func(city, country string) Value {
		om := NewOrderedMap()
		om.Set("city", NewString(city))
		om.Set("country", NewString(country))
		return NewMap(om)
	}

	td := TableData{
		Record: rec,
		Rows: []Value{
			mkRow("Paris", "France"),
			mkRow("London", "UK"),
			mkRow("Berlin", "Germany"),
		},
	}
	r.ContextSet("cities", Value{VType: TList, Data: td})
}

// makeSalesTable creates a "sales" table with region, product, amount.
func makeSalesTable(r *Registry) {
	fields := NewOrderedMap()
	fields.Set("region", NewTypeLiteral(TString))
	fields.Set("product", NewTypeLiteral(TString))
	fields.Set("amount", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: fields}

	mkRow := func(region, product string, amount int64) Value {
		om := NewOrderedMap()
		om.Set("region", NewString(region))
		om.Set("product", NewString(product))
		om.Set("amount", NewInteger(amount))
		return NewMap(om)
	}

	td := TableData{
		Record: rec,
		Rows: []Value{
			mkRow("east", "A", 100),
			mkRow("east", "B", 200),
			mkRow("west", "A", 150),
			mkRow("west", "B", 50),
			mkRow("east", "A", 300),
		},
	}
	r.ContextSet("sales", Value{VType: TList, Data: td})
}

// extractTD extracts TableData from a result Value (either TableData or QueryBuilder).
func extractTD(t *testing.T, v Value) TableData {
	t.Helper()
	if td, ok := v.Data.(TableData); ok {
		return td
	}
	if qb, ok := v.Data.(QueryBuilder); ok {
		td, err := qb.Materialize()
		if err != nil {
			t.Fatalf("materialize error: %v", err)
		}
		return td
	}
	t.Fatalf("expected TableData or QueryBuilder, got %T", v.Data)
	return TableData{}
}

// ========================
// SELECT with column specs
// ========================

func TestQueryCovSelectColumnRename(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// select [[name product_name] [price cost]] from products
	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{
			NewList([]Value{NewAtom("name"), NewAtom("product_name")}),
			NewList([]Value{NewAtom("price"), NewAtom("cost")}),
		}),
		NewWord("from"), NewWord("products"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	td := extractTD(t, result[0])
	if len(td.Rows) != 5 {
		t.Errorf("expected 5 rows, got %d", len(td.Rows))
	}
	// Check that renamed columns exist
	cols := td.Record.Fields.Keys()
	found := false
	for _, c := range cols {
		if c == "product_name" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected column 'product_name' in result, got %v", cols)
	}
}

func TestQueryCovSelectCastNoAlias(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// select [[cast price real]] from products — cast without explicit alias
	castSpec := NewList([]Value{NewAtom("cast"), NewAtom("price"), NewAtom("real")})
	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{castSpec}),
		NewWord("from"), NewWord("products"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	td := extractTD(t, result[0])
	if len(td.Rows) != 5 {
		t.Errorf("expected 5 rows, got %d", len(td.Rows))
	}
}

func TestQueryCovSelectSumAggregate(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// select [[sum price total_price]] from products
	sumSpec := NewList([]Value{NewAtom("sum"), NewAtom("price"), NewAtom("total_price")})
	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{sumSpec}),
		NewWord("from"), NewWord("products"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	td := extractTD(t, result[0])
	if len(td.Rows) != 1 {
		t.Errorf("expected 1 row for aggregate, got %d", len(td.Rows))
	}
	// total should be 10+25+15+30+5 = 85
	row := td.Rows[0].AsMap()
	val, ok := row.Get("total_price")
	if !ok {
		t.Fatal("missing total_price column")
	}
	_as0, _ := val.AsInteger()
	if _as0 != 85 {
		t.Errorf("expected sum=85, got %v", val)
	}
}

func TestQueryCovSelectAvgAggregate(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// select [[avg price]] from products — aggregate without explicit alias
	avgSpec := NewList([]Value{NewAtom("avg"), NewAtom("price")})
	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{avgSpec}),
		NewWord("from"), NewWord("products"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	td := extractTD(t, result[0])
	if len(td.Rows) != 1 {
		t.Errorf("expected 1 row for aggregate, got %d", len(td.Rows))
	}
}

func TestQueryCovSelectMinMax(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// select [[min price cheapest] [max price most_expensive]] from products
	minSpec := NewList([]Value{NewAtom("min"), NewAtom("price"), NewAtom("cheapest")})
	maxSpec := NewList([]Value{NewAtom("max"), NewAtom("price"), NewAtom("most_expensive")})
	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{minSpec, maxSpec}),
		NewWord("from"), NewWord("products"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	td := extractTD(t, result[0])
	row := td.Rows[0].AsMap()
	minVal, _ := row.Get("cheapest")
	maxVal, _ := row.Get("most_expensive")
	_as1, _ := minVal.AsInteger()
	if _as1 != 5 {
		t.Errorf("expected min=5, got %v", minVal)
	}
	_as2, _ := maxVal.AsInteger()
	if _as2 != 30 {
		t.Errorf("expected max=30, got %v", maxVal)
	}
}

func TestQueryCovSelectColumnWithStringNames(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// select ["name" "price"] from products — columns as strings
	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{NewString("name"), NewString("price")}),
		NewWord("from"), NewWord("products"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	td := extractTD(t, result[0])
	if len(td.Rows) != 5 {
		t.Errorf("expected 5 rows, got %d", len(td.Rows))
	}
}

// ========================
// WHERE clauses with various operators
// ========================

func TestQueryCovWhereEq(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// from products where [category eq "hardware"] select star
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("category"), NewAtom("eq"), NewString("hardware"),
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	td := extractTD(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 hardware rows, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereNeq(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("category"), NewAtom("neq"), NewString("misc"),
		}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	td := extractTD(t, result[0])
	if len(td.Rows) != 4 {
		t.Errorf("expected 4 non-misc rows, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereLtGt(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// price < 15
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("price"), NewAtom("lt"), NewInteger(15),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows with price<15, got %d", len(td.Rows))
	}

	// price > 20
	result = runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("price"), NewAtom("gt"), NewInteger(20),
		}),
	})
	td = extractTD(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows with price>20, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereLike(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// name like "G%"
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("name"), NewAtom("like"), NewString("G%"),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows matching G%%, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereAndOr(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// (category eq "hardware" and price gt 15) or category eq "misc"
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewList([]Value{
				NewAtom("category"), NewAtom("eq"), NewString("hardware"),
				NewAtom("and"),
				NewAtom("price"), NewAtom("gt"), NewInteger(15),
			}),
			NewAtom("or"),
			NewAtom("category"), NewAtom("eq"), NewString("misc"),
		}),
	})
	td := extractTD(t, result[0])
	// hardware with price>15: Gadget(25), plus misc: Thingamajig(5) = 2
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereIn(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// id in [1 3 5]
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("id"), NewAtom("in"),
			NewList([]Value{NewInteger(1), NewInteger(3), NewInteger(5)}),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 3 {
		t.Errorf("expected 3 rows for IN [1,3,5], got %d", len(td.Rows))
	}
}

func TestQueryCovWhereNotIn(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// id not in [1 2]
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("id"), NewAtom("not"), NewAtom("in"),
			NewList([]Value{NewInteger(1), NewInteger(2)}),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 3 {
		t.Errorf("expected 3 rows for NOT IN [1,2], got %d", len(td.Rows))
	}
}

func TestQueryCovWhereNotBetween(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// price not between 10 25
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("price"), NewAtom("not"), NewAtom("between"), NewInteger(10), NewInteger(25),
		}),
	})
	td := extractTD(t, result[0])
	// prices outside 10-25: 30 and 5 = 2 rows
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows for NOT BETWEEN 10 25, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereIsNull(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Create a table with a nullable column
	fields := NewOrderedMap()
	fields.Set("name", NewTypeLiteral(TString))
	fields.Set("note", NewTypeLiteral(TString))
	rec := RecordTypeInfo{Fields: fields}

	row1 := NewOrderedMap()
	row1.Set("name", NewString("a"))
	row1.Set("note", NewString("has note"))
	row2 := NewOrderedMap()
	row2.Set("name", NewString("b"))
	row2.Set("note", Value{VType: TNone})

	td := TableData{
		Record: rec,
		Rows:   []Value{NewMap(row1), NewMap(row2)},
	}
	r.ContextSet("items", Value{VType: TList, Data: td})

	// where [note is null]
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("items"),
		NewWord("where"), NewList([]Value{
			NewAtom("note"), NewAtom("is"), NewAtom("null"),
		}),
	})
	td2 := extractTD(t, result[0])
	if len(td2.Rows) != 1 {
		t.Errorf("expected 1 row with null note, got %d", len(td2.Rows))
	}
}

func TestQueryCovWhereNotGroup(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// not [category eq "misc"]
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("not"),
			NewList([]Value{NewAtom("category"), NewAtom("eq"), NewString("misc")}),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 4 {
		t.Errorf("expected 4 rows for NOT [misc], got %d", len(td.Rows))
	}
}

func TestQueryCovWhereNotSingle(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// not category eq "misc"
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("not"), NewAtom("category"), NewAtom("eq"), NewString("misc"),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 4 {
		t.Errorf("expected 4 rows for NOT category=misc, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereNotGroupAndMore(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// not [category eq "misc"] and price gt 10
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("not"),
			NewList([]Value{NewAtom("category"), NewAtom("eq"), NewString("misc")}),
			NewAtom("and"),
			NewAtom("price"), NewAtom("gt"), NewInteger(10),
		}),
	})
	td := extractTD(t, result[0])
	// Not misc AND price > 10: Widget(10 not >10), Gadget(25), Gizmo(15), Doohickey(30) => 3
	if len(td.Rows) != 3 {
		t.Errorf("expected 3 rows, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereNotSingleAndMore(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// not category eq "misc" and price gt 10
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("not"), NewAtom("category"), NewAtom("eq"), NewString("misc"),
			NewAtom("and"),
			NewAtom("price"), NewAtom("gt"), NewInteger(10),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 3 {
		t.Errorf("expected 3 rows, got %d", len(td.Rows))
	}
}

// ========================
// JOIN operations
// ========================

func TestQueryCovJoinUsing(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Create two tables with a shared "pid" column for USING join
	f1 := NewOrderedMap()
	f1.Set("pid", NewTypeLiteral(TInteger))
	f1.Set("oname", NewTypeLiteral(TString))
	rec1 := RecordTypeInfo{Fields: f1}

	f2 := NewOrderedMap()
	f2.Set("pid", NewTypeLiteral(TInteger))
	f2.Set("pname", NewTypeLiteral(TString))
	rec2 := RecordTypeInfo{Fields: f2}

	mk1 := func(pid int64, oname string) Value {
		om := NewOrderedMap()
		om.Set("pid", NewInteger(pid))
		om.Set("oname", NewString(oname))
		return NewMap(om)
	}
	mk2 := func(pid int64, pname string) Value {
		om := NewOrderedMap()
		om.Set("pid", NewInteger(pid))
		om.Set("pname", NewString(pname))
		return NewMap(om)
	}

	td1 := TableData{Record: rec1, Rows: []Value{mk1(1, "order1"), mk1(2, "order2"), mk1(1, "order3")}}
	td2 := TableData{Record: rec2, Rows: []Value{mk2(1, "Widget"), mk2(2, "Gadget"), mk2(3, "Gizmo")}}
	r.ContextSet("jorders", Value{VType: TList, Data: td1})
	r.ContextSet("jproducts", Value{VType: TList, Data: td2})

	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("jorders"),
		NewWord("join"), NewWord("jproducts"),
		NewWord("using"), NewList([]Value{NewAtom("pid")}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	td := extractTD(t, result[0])
	if len(td.Rows) != 3 {
		t.Errorf("expected 3 joined rows, got %d", len(td.Rows))
	}
}

func TestQueryCovLeftJoin(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Create tables with a shared column for USING
	f1 := NewOrderedMap()
	f1.Set("pid", NewTypeLiteral(TInteger))
	f1.Set("pname", NewTypeLiteral(TString))
	rec1 := RecordTypeInfo{Fields: f1}

	f2 := NewOrderedMap()
	f2.Set("pid", NewTypeLiteral(TInteger))
	f2.Set("qty", NewTypeLiteral(TInteger))
	rec2 := RecordTypeInfo{Fields: f2}

	mk1 := func(pid int64, pname string) Value {
		om := NewOrderedMap()
		om.Set("pid", NewInteger(pid))
		om.Set("pname", NewString(pname))
		return NewMap(om)
	}
	mk2 := func(pid, qty int64) Value {
		om := NewOrderedMap()
		om.Set("pid", NewInteger(pid))
		om.Set("qty", NewInteger(qty))
		return NewMap(om)
	}

	td1 := TableData{Record: rec1, Rows: []Value{mk1(1, "A"), mk1(2, "B"), mk1(3, "C")}}
	td2 := TableData{Record: rec2, Rows: []Value{mk2(1, 10), mk2(1, 20)}}
	r.ContextSet("ljProducts", Value{VType: TList, Data: td1})
	r.ContextSet("ljOrders", Value{VType: TList, Data: td2})

	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("ljProducts"),
		NewWord("leftjoin"), NewWord("ljOrders"),
		NewWord("using"), NewList([]Value{NewAtom("pid")}),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	td := extractTD(t, result[0])
	// Product 1 matches 2 orders, products 2 and 3 have no orders (NULL) => 4 rows
	if len(td.Rows) != 4 {
		t.Errorf("expected 4 rows from left join, got %d", len(td.Rows))
	}
}

func TestQueryCovCrossJoin(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Small tables for cross join
	f1 := NewOrderedMap()
	f1.Set("x", NewTypeLiteral(TInteger))
	r1 := NewOrderedMap()
	r1.Set("x", NewInteger(1))
	r2 := NewOrderedMap()
	r2.Set("x", NewInteger(2))
	td1 := TableData{Record: RecordTypeInfo{Fields: f1}, Rows: []Value{NewMap(r1), NewMap(r2)}}
	r.ContextSet("tblA", Value{VType: TList, Data: td1})

	f2 := NewOrderedMap()
	f2.Set("y", NewTypeLiteral(TString))
	ra := NewOrderedMap()
	ra.Set("y", NewString("a"))
	rb := NewOrderedMap()
	rb.Set("y", NewString("b"))
	td2 := TableData{Record: RecordTypeInfo{Fields: f2}, Rows: []Value{NewMap(ra), NewMap(rb)}}
	r.ContextSet("tblB", Value{VType: TList, Data: td2})

	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("tblA"),
		NewWord("crossjoin"), NewWord("tblB"),
	})
	td := extractTD(t, result[0])
	// 2 x 2 = 4 rows
	if len(td.Rows) != 4 {
		t.Errorf("expected 4 cross join rows, got %d", len(td.Rows))
	}
}

// ========================
// SET operations
// ========================

func TestQueryCovUnion(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Two simple single-column tables
	f := NewOrderedMap()
	f.Set("val", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: f}

	mkRow := func(v int64) Value {
		om := NewOrderedMap()
		om.Set("val", NewInteger(v))
		return NewMap(om)
	}

	td1 := TableData{Record: rec, Rows: []Value{mkRow(1), mkRow(2), mkRow(3)}}
	td2 := TableData{Record: rec, Rows: []Value{mkRow(2), mkRow(3), mkRow(4)}}
	r.ContextSet("setA", Value{VType: TList, Data: td1})
	r.ContextSet("setB", Value{VType: TList, Data: td2})

	// from setA union from setB select star — UNION removes duplicates
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("setA"),
		NewWord("union"),
		NewWord("from"), NewWord("setB"),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 4 {
		t.Errorf("expected 4 rows from UNION (1,2,3,4), got %d", len(td.Rows))
	}
}

func TestQueryCovUnionAll(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	f := NewOrderedMap()
	f.Set("val", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: f}
	mkRow := func(v int64) Value {
		om := NewOrderedMap()
		om.Set("val", NewInteger(v))
		return NewMap(om)
	}

	td1 := TableData{Record: rec, Rows: []Value{mkRow(1), mkRow(2)}}
	td2 := TableData{Record: rec, Rows: []Value{mkRow(2), mkRow(3)}}
	r.ContextSet("uaA", Value{VType: TList, Data: td1})
	r.ContextSet("uaB", Value{VType: TList, Data: td2})

	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("uaA"),
		NewWord("unionall"),
		NewWord("from"), NewWord("uaB"),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 4 {
		t.Errorf("expected 4 rows from UNION ALL, got %d", len(td.Rows))
	}
}

func TestQueryCovIntersect(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	f := NewOrderedMap()
	f.Set("val", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: f}
	mkRow := func(v int64) Value {
		om := NewOrderedMap()
		om.Set("val", NewInteger(v))
		return NewMap(om)
	}

	td1 := TableData{Record: rec, Rows: []Value{mkRow(1), mkRow(2), mkRow(3)}}
	td2 := TableData{Record: rec, Rows: []Value{mkRow(2), mkRow(3), mkRow(4)}}
	r.ContextSet("isA", Value{VType: TList, Data: td1})
	r.ContextSet("isB", Value{VType: TList, Data: td2})

	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("isA"),
		NewWord("intersect"),
		NewWord("from"), NewWord("isB"),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows from INTERSECT (2,3), got %d", len(td.Rows))
	}
}

func TestQueryCovExcept(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	f := NewOrderedMap()
	f.Set("val", NewTypeLiteral(TInteger))
	rec := RecordTypeInfo{Fields: f}
	mkRow := func(v int64) Value {
		om := NewOrderedMap()
		om.Set("val", NewInteger(v))
		return NewMap(om)
	}

	td1 := TableData{Record: rec, Rows: []Value{mkRow(1), mkRow(2), mkRow(3)}}
	td2 := TableData{Record: rec, Rows: []Value{mkRow(2), mkRow(3), mkRow(4)}}
	r.ContextSet("exA", Value{VType: TList, Data: td1})
	r.ContextSet("exB", Value{VType: TList, Data: td2})

	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("exA"),
		NewWord("except"),
		NewWord("from"), NewWord("exB"),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 1 {
		t.Errorf("expected 1 row from EXCEPT (just 1), got %d", len(td.Rows))
	}
}

// ========================
// ORDER BY, GROUP BY with aggregates
// ========================

func TestQueryCovOrderByDesc(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("order"), NewList([]Value{NewAtom("price"), NewAtom("desc")}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(td.Rows))
	}
	// First row should be the most expensive (30)
	first := td.Rows[0].AsMap()
	price, _ := first.Get("price")
	_as3, _ := price.AsNumber()
	if _as3 != 30 {
		t.Errorf("expected first price=30 (desc), got %v", price)
	}
}

func TestQueryCovOrderByMultiple(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// order by [category asc price desc]
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("order"), NewList([]Value{
			NewAtom("category"), NewAtom("asc"),
			NewAtom("price"), NewAtom("desc"),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 5 {
		t.Errorf("expected 5 rows, got %d", len(td.Rows))
	}
}

func TestQueryCovGroupByWithSum(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeSalesTable(r)

	// select [region [sum amount total]] from sales group by [region]
	sumSpec := NewList([]Value{NewAtom("sum"), NewAtom("amount"), NewAtom("total")})
	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{NewAtom("region"), sumSpec}),
		NewWord("from"), NewWord("sales"),
		NewWord("group"), NewWord("by"), NewList([]Value{NewAtom("region")}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 groups (east, west), got %d", len(td.Rows))
	}
}

func TestQueryCovGroupByAtom(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeSalesTable(r)

	// select [region [count * cnt]] from sales group region
	countSpec := NewList([]Value{NewAtom("count"), NewAtom("*"), NewAtom("cnt")})
	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{NewAtom("region"), countSpec}),
		NewWord("from"), NewWord("sales"),
		NewWord("group"), NewWord("region"),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 groups, got %d", len(td.Rows))
	}
}

func TestQueryCovGroupByMultiple(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeSalesTable(r)

	// group by [region product]
	countSpec := NewList([]Value{NewAtom("count"), NewAtom("*"), NewAtom("cnt")})
	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{NewAtom("region"), NewAtom("product"), countSpec}),
		NewWord("from"), NewWord("sales"),
		NewWord("group"), NewWord("by"), NewList([]Value{NewAtom("region"), NewAtom("product")}),
	})
	td := extractTD(t, result[0])
	// east/A: 2, east/B: 1, west/A: 1, west/B: 1 => 4 groups
	if len(td.Rows) != 4 {
		t.Errorf("expected 4 groups, got %d", len(td.Rows))
	}
}

func TestQueryCovHaving(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeSalesTable(r)

	// select [region [sum amount total]] from sales group by [region] having [total gt 300]
	sumSpec := NewList([]Value{NewAtom("sum"), NewAtom("amount"), NewAtom("total")})
	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{NewAtom("region"), sumSpec}),
		NewWord("from"), NewWord("sales"),
		NewWord("group"), NewWord("by"), NewList([]Value{NewAtom("region")}),
		NewWord("having"), NewList([]Value{
			NewAtom("total"), NewAtom("gt"), NewInteger(300),
		}),
	})
	td := extractTD(t, result[0])
	// east: 100+200+300=600 > 300, west: 150+50=200 <= 300
	if len(td.Rows) != 1 {
		t.Errorf("expected 1 group after HAVING, got %d", len(td.Rows))
	}
}

// ========================
// Subqueries in WHERE (IN subquery)
// ========================

func TestQueryCovWhereInSubquery(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)
	makeOrdersTable(r)

	// from products where [id in (select [product_id] from orders)] select star
	// The "(" and ")" are words used as parentheses for subquery evaluation
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("id"), NewAtom("in"),
			NewWord("("),
			NewWord("select"), NewList([]Value{NewAtom("product_id")}),
			NewWord("from"), NewWord("orders"),
			NewWord(")"),
		}),
	})
	td := extractTD(t, result[0])
	// Orders reference products 1, 2, 3 => 3 distinct products
	if len(td.Rows) != 3 {
		t.Errorf("expected 3 rows from IN subquery, got %d", len(td.Rows))
	}
}

// ========================
// Subqueries in SELECT (scalar subquery)
// ========================

func TestQueryCovSelectScalarSubquery(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// select [name [(select [[max price]] from products) max_price]] from products limit 1
	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{
			NewAtom("name"),
			NewList([]Value{
				NewWord("("),
				NewWord("select"), NewList([]Value{
					NewList([]Value{NewAtom("max"), NewAtom("price")}),
				}),
				NewWord("from"), NewWord("products"),
				NewWord(")"),
				NewAtom("max_price"),
			}),
		}),
		NewWord("from"), NewWord("products"),
		NewWord("limit"), NewInteger(1),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	td := extractTD(t, result[0])
	if len(td.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(td.Rows))
	}
}

// ========================
// DISTINCT
// ========================

func TestQueryCovDistinctValues(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	result := runAQL(t, r, []Value{
		NewWord("select"), NewList([]Value{NewAtom("category")}),
		NewWord("from"), NewWord("products"),
		NewWord("distinct"),
	})
	td := extractTD(t, result[0])
	// 3 distinct categories: hardware, electronics, misc
	if len(td.Rows) != 3 {
		t.Errorf("expected 3 distinct categories, got %d", len(td.Rows))
	}
}

// ========================
// toQueryBuilder edge case — non-table argument
// ========================

func TestQueryCovFromUnknownTable(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	err = runAQLError(t, r, []Value{
		NewWord("from"), NewWord("nonexistent_table"),
	})
	if err == nil {
		t.Error("expected error for unknown table")
	}
}

func TestQueryCovFromNonTable(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	// Store a non-table value
	r.ContextSet("notatable", NewInteger(42))

	err = runAQLError(t, r, []Value{
		NewWord("from"), NewWord("notatable"),
	})
	if err == nil {
		t.Error("expected error for non-table value")
	}
}

// ========================
// clone coverage — ensure clone copies slices
// ========================

func TestQueryCovCloneWithJoinsAndSetOps(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Create tables with shared "pid" column for USING join
	f1 := NewOrderedMap()
	f1.Set("pid", NewTypeLiteral(TInteger))
	f1.Set("price", NewTypeLiteral(TInteger))
	rec1 := RecordTypeInfo{Fields: f1}

	f2 := NewOrderedMap()
	f2.Set("pid", NewTypeLiteral(TInteger))
	f2.Set("qty", NewTypeLiteral(TInteger))
	rec2 := RecordTypeInfo{Fields: f2}

	mk1 := func(pid, price int64) Value {
		om := NewOrderedMap()
		om.Set("pid", NewInteger(pid))
		om.Set("price", NewInteger(price))
		return NewMap(om)
	}
	mk2 := func(pid, qty int64) Value {
		om := NewOrderedMap()
		om.Set("pid", NewInteger(pid))
		om.Set("qty", NewInteger(qty))
		return NewMap(om)
	}

	td1 := TableData{Record: rec1, Rows: []Value{mk1(1, 10), mk1(2, 20)}}
	td2 := TableData{Record: rec2, Rows: []Value{mk2(1, 5), mk2(2, 3)}}
	r.ContextSet("cloneP", Value{VType: TList, Data: td1})
	r.ContextSet("cloneO", Value{VType: TList, Data: td2})

	// Build a query with join AND then clone it via the where word
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("cloneP"),
		NewWord("join"), NewWord("cloneO"),
		NewWord("using"), NewList([]Value{NewAtom("pid")}),
		NewWord("where"), NewList([]Value{
			NewAtom("price"), NewAtom("gt"), NewInteger(5),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) < 1 {
		t.Errorf("expected at least 1 row, got %d", len(td.Rows))
	}
}

// ========================
// buildInList edge cases
// ========================

func TestQueryCovBuildInListSingleValue(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// where [id in 1] — single value, not a list
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("id"), NewAtom("in"), NewInteger(1),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 1 {
		t.Errorf("expected 1 row for IN single value, got %d", len(td.Rows))
	}
}

// ========================
// resolveScalarValue with non-table value
// ========================

func TestQueryCovWhereWithBooleanValue(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// where [id eq 1] — boolean comparison should work
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("id"), NewAtom("eq"), NewInteger(1),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(td.Rows))
	}
}

// ========================
// mergedSchema coverage via join
// ========================

func TestQueryCovMergedSchemaOnJoin(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	// Two tables with different columns, shared "pid"
	f1 := NewOrderedMap()
	f1.Set("pid", NewTypeLiteral(TInteger))
	f1.Set("oname", NewTypeLiteral(TString))
	rec1 := RecordTypeInfo{Fields: f1}

	f2 := NewOrderedMap()
	f2.Set("pid", NewTypeLiteral(TInteger))
	f2.Set("pname", NewTypeLiteral(TString))
	rec2 := RecordTypeInfo{Fields: f2}

	mk1 := func(pid int64, oname string) Value {
		om := NewOrderedMap()
		om.Set("pid", NewInteger(pid))
		om.Set("oname", NewString(oname))
		return NewMap(om)
	}
	mk2 := func(pid int64, pname string) Value {
		om := NewOrderedMap()
		om.Set("pid", NewInteger(pid))
		om.Set("pname", NewString(pname))
		return NewMap(om)
	}

	td1 := TableData{Record: rec1, Rows: []Value{mk1(1, "o1")}}
	td2 := TableData{Record: rec2, Rows: []Value{mk2(1, "p1")}}
	r.ContextSet("msOrders", Value{VType: TList, Data: td1})
	r.ContextSet("msProducts", Value{VType: TList, Data: td2})

	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("msOrders"),
		NewWord("join"), NewWord("msProducts"),
		NewWord("using"), NewList([]Value{NewAtom("pid")}),
	})
	td := extractTD(t, result[0])
	cols := td.Record.Fields.Keys()
	if len(cols) < 3 {
		t.Errorf("expected merged schema with at least 3 columns, got %d: %v", len(cols), cols)
	}
}

// ========================
// parseColumnSpec error paths
// ========================

func TestQueryCovSelectBadColumnSpecType(t *testing.T) {
	// Build a column list with an unsupported type (e.g., boolean)
	colList := NewList([]Value{NewBoolean(true)})
	_, err := parseColumnSpec(colList)
	if err == nil {
		t.Error("expected error for boolean in column spec")
	}
	if !strings.Contains(err.Error(), "unsupported column spec type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestQueryCovSelectSubListTooShort(t *testing.T) {
	// Column spec sub-list with only 1 element
	colList := NewList([]Value{NewList([]Value{NewAtom("x")})})
	_, err := parseColumnSpec(colList)
	if err == nil {
		t.Error("expected error for sub-list with 1 element")
	}
}

func TestQueryCovSelectBadAliasLength(t *testing.T) {
	// Column spec pair with 3 elements (not aggregate/cast) -> error
	colList := NewList([]Value{
		NewList([]Value{NewAtom("x"), NewAtom("y"), NewAtom("z")}),
	})
	_, err := parseColumnSpec(colList)
	if err == nil {
		t.Error("expected error for column alias with 3 elements")
	}
}

// ========================
// buildWhereClause error paths
// ========================

func TestQueryCovWhereUnknownOperator(t *testing.T) {
	cond := NewList([]Value{NewAtom("x"), NewAtom("badop"), NewInteger(1)})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for unknown operator")
	}
}

func TestQueryCovWhereIncompleteAfterColumn(t *testing.T) {
	cond := NewList([]Value{NewAtom("x")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for incomplete condition")
	}
}

func TestQueryCovWhereBetweenIncomplete(t *testing.T) {
	cond := NewList([]Value{NewAtom("x"), NewAtom("between"), NewInteger(1)})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for incomplete between")
	}
}

func TestQueryCovWhereInEmpty(t *testing.T) {
	cond := NewList([]Value{NewAtom("x"), NewAtom("in")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for in without list")
	}
}

func TestQueryCovWhereIsNotBadToken(t *testing.T) {
	cond := NewList([]Value{NewAtom("x"), NewAtom("is"), NewAtom("bad")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for 'is bad'")
	}
}

func TestQueryCovWhereIsNotNullMissing(t *testing.T) {
	cond := NewList([]Value{NewAtom("x"), NewAtom("is"), NewAtom("not"), NewAtom("bad")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for 'is not bad' (expected null)")
	}
}

func TestQueryCovWhereNotBadFollower(t *testing.T) {
	cond := NewList([]Value{NewAtom("x"), NewAtom("not"), NewAtom("badop")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for 'not badop'")
	}
}

func TestQueryCovWhereNotInNoList(t *testing.T) {
	cond := NewList([]Value{NewAtom("x"), NewAtom("not"), NewAtom("in")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for 'not in' without list")
	}
}

func TestQueryCovWhereNotBetweenIncomplete(t *testing.T) {
	cond := NewList([]Value{NewAtom("x"), NewAtom("not"), NewAtom("between"), NewInteger(1)})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for incomplete 'not between'")
	}
}

func TestQueryCovWhereEmpty(t *testing.T) {
	cond := NewList([]Value{})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if clause != "1=1" {
		t.Errorf("empty where should be '1=1', got %q", clause)
	}
}

func TestQueryCovWhereIncompleteAfterOp(t *testing.T) {
	cond := NewList([]Value{NewAtom("x"), NewAtom("eq")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for incomplete condition after operator")
	}
}

func TestQueryCovWhereNotIncomplete(t *testing.T) {
	cond := NewList([]Value{NewAtom("not")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for standalone not")
	}
}

func TestQueryCovWhereIsIncomplete(t *testing.T) {
	cond := NewList([]Value{NewAtom("x"), NewAtom("is")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for incomplete 'is'")
	}
}

func TestQueryCovWhereIsNotIncomplete(t *testing.T) {
	cond := NewList([]Value{NewAtom("x"), NewAtom("is"), NewAtom("not")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for 'is not' without null")
	}
}

// ========================
// buildInList edge case: empty list
// ========================

func TestQueryCovBuildInListEmpty(t *testing.T) {
	emptyList := NewList([]Value{})
	_, err := buildInList(emptyList)
	if err == nil {
		t.Error("expected error for empty IN list")
	}
}

// ========================
// Cast spec error paths
// ========================

func TestQueryCovCastTooFewElements(t *testing.T) {
	// [cast col] — only 2 elements, need at least 3
	_, err := parseCastSpec([]Value{NewAtom("cast"), NewAtom("col")})
	if err == nil {
		t.Error("expected error for cast with too few elements")
	}
}

func TestQueryCovCastTooManyElements(t *testing.T) {
	// [cast col type alias extra] — 5 elements, max is 4
	_, err := parseCastSpec([]Value{NewAtom("cast"), NewAtom("col"), NewAtom("int"), NewAtom("a"), NewAtom("b")})
	if err == nil {
		t.Error("expected error for cast with too many elements")
	}
}

// ========================
// Aggregate spec error paths
// ========================

func TestQueryCovAggregateNoArgs(t *testing.T) {
	_, err := parseAggregateSpec("count", nil)
	if err == nil {
		t.Error("expected error for aggregate with no args")
	}
}

func TestQueryCovAggregateTooManyArgs(t *testing.T) {
	_, err := parseAggregateSpec("count", []Value{NewAtom("a"), NewAtom("b"), NewAtom("c")})
	if err == nil {
		t.Error("expected error for aggregate with 3 args")
	}
}

// ========================
// valueToSQL coverage
// ========================

func TestQueryCovValueToSQLTypes(t *testing.T) {
	// Test each supported type
	s, err := valueToSQL(NewString("hello"))
	if err != nil || s != "'hello'" {
		t.Errorf("string: %v %v", s, err)
	}

	s, err = valueToSQL(NewInteger(42))
	if err != nil || s != "42" {
		t.Errorf("integer: %v %v", s, err)
	}

	s, err = valueToSQL(NewBoolean(true))
	if err != nil || s != "'true'" {
		t.Errorf("boolean true: %v %v", s, err)
	}

	s, err = valueToSQL(NewBoolean(false))
	if err != nil || s != "'false'" {
		t.Errorf("boolean false: %v %v", s, err)
	}

	s, err = valueToSQL(NewAtom("test"))
	if err != nil || s != "'test'" {
		t.Errorf("atom: %v %v", s, err)
	}

	s, err = valueToSQL(Value{VType: TNone})
	if err != nil || s != "NULL" {
		t.Errorf("none: %v %v", s, err)
	}

	// Unsupported type
	_, err = valueToSQL(NewList([]Value{NewInteger(1)}))
	if err == nil {
		t.Error("expected error for unsupported type in valueToSQL")
	}
}

func TestQueryCovValueToSQLEscaping(t *testing.T) {
	s, err := valueToSQL(NewString("it's"))
	if err != nil {
		t.Fatal(err)
	}
	if s != "'it''s'" {
		t.Errorf("expected escaped single quote, got %q", s)
	}
}

// ========================
// Order by atom handler
// ========================

func TestQueryCovOrderByAtom(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// from products order price select star — single atom order
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("order"), NewWord("price"),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 5 {
		t.Errorf("expected 5 rows, got %d", len(td.Rows))
	}
}

// ========================
// buildSQL coverage — verify GROUP BY, HAVING, OFFSET in SQL
// ========================

func TestQueryCovBuildSQLGroupByHaving(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	td := TableData{
		Record: RecordTypeInfo{Fields: NewOrderedMap()},
	}
	qb := NewQueryBuilder(r, td)
	qb.GroupBy = `"category"`
	qb.Having = `COUNT(*) > 1`
	qb.Distinct = true
	qb.Offset = 0

	sql := qb.buildSQL("test", "*")
	if !strings.Contains(sql, "GROUP BY") {
		t.Errorf("expected GROUP BY in SQL, got %q", sql)
	}
	if !strings.Contains(sql, "HAVING") {
		t.Errorf("expected HAVING in SQL, got %q", sql)
	}
	if !strings.Contains(sql, "DISTINCT") {
		t.Errorf("expected DISTINCT in SQL, got %q", sql)
	}
	if !strings.Contains(sql, "OFFSET") {
		t.Errorf("expected OFFSET in SQL, got %q", sql)
	}
}

// ========================
// Collate coverage in WHERE (binary, rtrim)
// ========================

func TestQueryCovWhereCollateBinary(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("name"), NewAtom("eq"), NewString("Widget"),
			NewAtom("collate"), NewAtom("binary"),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 1 {
		t.Errorf("expected 1 row with collate binary, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereCollateRtrim(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("name"), NewAtom("eq"), NewString("Widget"),
			NewAtom("collate"), NewAtom("rtrim"),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 1 {
		t.Errorf("expected 1 row with collate rtrim, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereCollateBadType(t *testing.T) {
	cond := NewList([]Value{
		NewAtom("x"), NewAtom("eq"), NewString("y"),
		NewAtom("collate"), NewAtom("badcollation"),
	})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for bad collation type")
	}
}

func TestQueryCovWhereCollateMissingType(t *testing.T) {
	cond := NewList([]Value{
		NewAtom("x"), NewAtom("eq"), NewString("y"),
		NewAtom("collate"),
	})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for collate without type")
	}
}

// ========================
// aqlTypenameToSQLType edge cases
// ========================

func TestQueryCovAqlTypenameToSQLType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"integer", "INTEGER"},
		{"int", "INTEGER"},
		{"real", "REAL"},
		{"float", "REAL"},
		{"number", "REAL"},
		{"decimal", "REAL"},
		{"text", "TEXT"},
		{"string", "TEXT"},
		{"boolean", "INTEGER"},
		{"bool", "INTEGER"},
		{"CUSTOM", "CUSTOM"},
	}
	for _, tt := range tests {
		got := aqlTypenameToSQLType(tt.input)
		if got != tt.expected {
			t.Errorf("aqlTypenameToSQLType(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// ========================
// on/using error without preceding join
// ========================

func TestQueryCovOnWithoutJoin(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	err = runAQLError(t, r, []Value{
		NewWord("from"), NewWord("products"),
		NewWord("on"), NewList([]Value{NewAtom("x"), NewAtom("eq"), NewAtom("y")}),
	})
	if err == nil {
		t.Error("expected error for on without preceding join")
	}
}

func TestQueryCovUsingWithoutJoin(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	err = runAQLError(t, r, []Value{
		NewWord("from"), NewWord("products"),
		NewWord("using"), NewList([]Value{NewAtom("id")}),
	})
	if err == nil {
		t.Error("expected error for using without preceding join")
	}
}

// ========================
// Word values in column spec
// ========================

func TestQueryCovSelectColumnWithWordValues(t *testing.T) {
	// parseColumnSpec should handle word values as column names
	colList := NewList([]Value{NewWord("mycolumn")})
	specs, err := parseColumnSpec(colList)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0].Name != "mycolumn" {
		t.Errorf("expected column name 'mycolumn', got %+v", specs)
	}
}

// ========================
// resolveWhereSubExprs with nested lists
// ========================

func TestQueryCovResolveWhereNestedList(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// where [[price gt 10] and [price lt 30]] — nested condition groups
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewList([]Value{NewAtom("price"), NewAtom("gt"), NewInteger(10)}),
			NewAtom("and"),
			NewList([]Value{NewAtom("price"), NewAtom("lt"), NewInteger(30)}),
		}),
	})
	td := extractTD(t, result[0])
	// prices > 10 and < 30: 25, 15 = 2 rows
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(td.Rows))
	}
}

// ========================
// ensureSource — SQLite table already in store
// ========================

func TestQueryCovEnsureSourceAlreadySQLite(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// First select materializes, the result has SQLite=true
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
	})
	td := extractTD(t, result[0])

	// Store the result back and query it again (already SQLite)
	r.ContextSet("products_sq", Value{VType: TList, Data: td})

	result = runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products_sq"),
	})
	td2 := extractTD(t, result[0])
	if len(td2.Rows) != 5 {
		t.Errorf("expected 5 rows from already-SQLite table, got %d", len(td2.Rows))
	}
}

// ========================
// GTE, LTE operators in WHERE
// ========================

func TestQueryCovWhereGteLte(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// price >= 15 and price <= 25
	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("price"), NewAtom("gte"), NewInteger(15),
			NewAtom("and"),
			NewAtom("price"), NewAtom("lte"), NewInteger(25),
		}),
	})
	td := extractTD(t, result[0])
	// prices 15, 25 = 2 rows
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows for gte/lte, got %d", len(td.Rows))
	}
}

// ========================
// Between in WHERE
// ========================

func TestQueryCovWhereBetween(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	result := runAQL(t, r, []Value{
		NewWord("select"), NewWord("star"),
		NewWord("from"), NewWord("products"),
		NewWord("where"), NewList([]Value{
			NewAtom("price"), NewAtom("between"), NewInteger(10), NewInteger(20),
		}),
	})
	td := extractTD(t, result[0])
	// 10, 15 = 2 rows
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows for between 10 20, got %d", len(td.Rows))
	}
}

// ========================
// buildSQL with alias
// ========================

func TestQueryCovBuildSQLWithAlias(t *testing.T) {
	r, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}

	td := TableData{Record: RecordTypeInfo{Fields: NewOrderedMap()}}
	qb := NewQueryBuilder(r, td)
	qb.Alias = "t"
	qb.Where = `"x" > 1`

	sql := qb.buildSQL("mytable", "*")
	if !strings.Contains(sql, "AS") {
		t.Errorf("expected AS in SQL for alias, got %q", sql)
	}
}
