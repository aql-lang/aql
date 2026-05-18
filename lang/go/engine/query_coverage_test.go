//go:build query
// +build query

package engine_test

import (
	"github.com/aql-lang/aql/lang/go/engine"
	"github.com/aql-lang/aql/lang/go/native"
	"strings"
	"testing"
)

// ========================
// Helper: create tables for query coverage tests
// ========================

// makeProductsTable creates a "products" table with id, name, price, category.
func makeProductsTable(r *engine.Registry) {
	fields := engine.NewOrderedMap()
	fields.Set("id", engine.NewTypeLiteral(engine.TInteger))
	fields.Set("name", engine.NewTypeLiteral(engine.TString))
	fields.Set("price", engine.NewTypeLiteral(engine.TNumber))
	fields.Set("category", engine.NewTypeLiteral(engine.TString))
	rec := engine.RecordTypeInfo{Fields: fields}

	mkRow := func(id int64, name string, price int64, category string) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("id", engine.NewInteger(id))
		om.Set("name", engine.NewString(name))
		om.Set("price", engine.NewInteger(price))
		om.Set("category", engine.NewString(category))
		return engine.NewMap(om)
	}

	td := engine.TableData{
		Record: rec,
		Rows: []engine.Value{
			mkRow(1, "Widget", 10, "hardware"),
			mkRow(2, "Gadget", 25, "hardware"),
			mkRow(3, "Gizmo", 15, "electronics"),
			mkRow(4, "Doohickey", 30, "electronics"),
			mkRow(5, "Thingamajig", 5, "misc"),
		},
	}
	r.ContextSet("products", engine.Value{VType: engine.TList, Data: td})
}

// makeOrdersTable creates an "orders" table with order_id, product_id, qty.
func makeOrdersTable(r *engine.Registry) {
	fields := engine.NewOrderedMap()
	fields.Set("order_id", engine.NewTypeLiteral(engine.TInteger))
	fields.Set("product_id", engine.NewTypeLiteral(engine.TInteger))
	fields.Set("qty", engine.NewTypeLiteral(engine.TInteger))
	rec := engine.RecordTypeInfo{Fields: fields}

	mkRow := func(orderID, productID, qty int64) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("order_id", engine.NewInteger(orderID))
		om.Set("product_id", engine.NewInteger(productID))
		om.Set("qty", engine.NewInteger(qty))
		return engine.NewMap(om)
	}

	td := engine.TableData{
		Record: rec,
		Rows: []engine.Value{
			mkRow(100, 1, 3),
			mkRow(101, 2, 1),
			mkRow(102, 3, 5),
			mkRow(103, 1, 2),
		},
	}
	r.ContextSet("orders", engine.Value{VType: engine.TList, Data: td})
}

// makeCitiesTable creates a "cities" table with city, country.
func makeCitiesTable(r *engine.Registry) {
	fields := engine.NewOrderedMap()
	fields.Set("city", engine.NewTypeLiteral(engine.TString))
	fields.Set("country", engine.NewTypeLiteral(engine.TString))
	rec := engine.RecordTypeInfo{Fields: fields}

	mkRow := func(city, country string) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("city", engine.NewString(city))
		om.Set("country", engine.NewString(country))
		return engine.NewMap(om)
	}

	td := engine.TableData{
		Record: rec,
		Rows: []engine.Value{
			mkRow("Paris", "France"),
			mkRow("London", "UK"),
			mkRow("Berlin", "Germany"),
		},
	}
	r.ContextSet("cities", engine.Value{VType: engine.TList, Data: td})
}

// makeSalesTable creates a "sales" table with region, product, amount.
func makeSalesTable(r *engine.Registry) {
	fields := engine.NewOrderedMap()
	fields.Set("region", engine.NewTypeLiteral(engine.TString))
	fields.Set("product", engine.NewTypeLiteral(engine.TString))
	fields.Set("amount", engine.NewTypeLiteral(engine.TInteger))
	rec := engine.RecordTypeInfo{Fields: fields}

	mkRow := func(region, product string, amount int64) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("region", engine.NewString(region))
		om.Set("product", engine.NewString(product))
		om.Set("amount", engine.NewInteger(amount))
		return engine.NewMap(om)
	}

	td := engine.TableData{
		Record: rec,
		Rows: []engine.Value{
			mkRow("east", "A", 100),
			mkRow("east", "B", 200),
			mkRow("west", "A", 150),
			mkRow("west", "B", 50),
			mkRow("east", "A", 300),
		},
	}
	r.ContextSet("sales", engine.Value{VType: engine.TList, Data: td})
}

// extractTD extracts TableData from a result Value (either TableData or QueryBuilder).
func extractTD(t *testing.T, v engine.Value) engine.TableData {
	t.Helper()
	if td, ok := v.Data.(engine.TableData); ok {
		return td
	}
	if qb, ok := v.Data.(engine.QueryBuilder); ok {
		td, err := qb.Materialize()
		if err != nil {
			t.Fatalf("materialize error: %v", err)
		}
		return td
	}
	t.Fatalf("expected TableData or QueryBuilder, got %T", v.Data)
	return engine.TableData{}
}

// ========================
// SELECT with column specs
// ========================

func TestQueryCovSelectColumnRename(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// select [[name product_name] [price cost]] from products
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewList([]engine.Value{
			engine.NewList([]engine.Value{engine.NewAtom("name"), engine.NewAtom("product_name")}),
			engine.NewList([]engine.Value{engine.NewAtom("price"), engine.NewAtom("cost")}),
		}),
		engine.NewWord("from"), engine.NewWord("products"),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// select [[cast price real]] from products — cast without explicit alias
	castSpec := engine.NewList([]engine.Value{engine.NewAtom("cast"), engine.NewAtom("price"), engine.NewAtom("real")})
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewList([]engine.Value{castSpec}),
		engine.NewWord("from"), engine.NewWord("products"),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// select [[sum price total_price]] from products
	sumSpec := engine.NewList([]engine.Value{engine.NewAtom("sum"), engine.NewAtom("price"), engine.NewAtom("total_price")})
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewList([]engine.Value{sumSpec}),
		engine.NewWord("from"), engine.NewWord("products"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	td := extractTD(t, result[0])
	if len(td.Rows) != 1 {
		t.Errorf("expected 1 row for aggregate, got %d", len(td.Rows))
	}
	// total should be 10+25+15+30+5 = 85
	row, _ := engine.AsMap(td.Rows[0])
	val, ok := row.Get("total_price")
	if !ok {
		t.Fatal("missing total_price column")
	}
	_as0, _ := engine.AsInteger(val)
	if _as0 != 85 {
		t.Errorf("expected sum=85, got %v", val)
	}
}

func TestQueryCovSelectAvgAggregate(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// select [[avg price]] from products — aggregate without explicit alias
	avgSpec := engine.NewList([]engine.Value{engine.NewAtom("avg"), engine.NewAtom("price")})
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewList([]engine.Value{avgSpec}),
		engine.NewWord("from"), engine.NewWord("products"),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// select [[min price cheapest] [max price most_expensive]] from products
	minSpec := engine.NewList([]engine.Value{engine.NewAtom("min"), engine.NewAtom("price"), engine.NewAtom("cheapest")})
	maxSpec := engine.NewList([]engine.Value{engine.NewAtom("max"), engine.NewAtom("price"), engine.NewAtom("most_expensive")})
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewList([]engine.Value{minSpec, maxSpec}),
		engine.NewWord("from"), engine.NewWord("products"),
	})
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	td := extractTD(t, result[0])
	row, _ := engine.AsMap(td.Rows[0])
	minVal, _ := row.Get("cheapest")
	maxVal, _ := row.Get("most_expensive")
	_as1, _ := engine.AsInteger(minVal)
	if _as1 != 5 {
		t.Errorf("expected min=5, got %v", minVal)
	}
	_as2, _ := engine.AsInteger(maxVal)
	if _as2 != 30 {
		t.Errorf("expected max=30, got %v", maxVal)
	}
}

func TestQueryCovSelectColumnWithStringNames(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// select ["name" "price"] from products — columns as strings
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewList([]engine.Value{engine.NewString("name"), engine.NewString("price")}),
		engine.NewWord("from"), engine.NewWord("products"),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// from products where [category eq "hardware"] select star
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("category"), engine.NewAtom("eq"), engine.NewString("hardware"),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("category"), engine.NewAtom("neq"), engine.NewString("misc"),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// price < 15
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("price"), engine.NewAtom("lt"), engine.NewInteger(15),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows with price<15, got %d", len(td.Rows))
	}

	// price > 20
	result = runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("price"), engine.NewAtom("gt"), engine.NewInteger(20),
		}),
	})
	td = extractTD(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows with price>20, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereLike(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// name like "G%"
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("name"), engine.NewAtom("like"), engine.NewString("G%"),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows matching G%%, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereAndOr(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// (category eq "hardware" and price gt 15) or category eq "misc"
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewList([]engine.Value{
				engine.NewAtom("category"), engine.NewAtom("eq"), engine.NewString("hardware"),
				engine.NewAtom("and"),
				engine.NewAtom("price"), engine.NewAtom("gt"), engine.NewInteger(15),
			}),
			engine.NewAtom("or"),
			engine.NewAtom("category"), engine.NewAtom("eq"), engine.NewString("misc"),
		}),
	})
	td := extractTD(t, result[0])
	// hardware with price>15: Gadget(25), plus misc: Thingamajig(5) = 2
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereIn(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// id in [1 3 5]
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("id"), engine.NewAtom("in"),
			engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(3), engine.NewInteger(5)}),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 3 {
		t.Errorf("expected 3 rows for IN [1,3,5], got %d", len(td.Rows))
	}
}

func TestQueryCovWhereNotIn(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// id not in [1 2]
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("id"), engine.NewAtom("not"), engine.NewAtom("in"),
			engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2)}),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 3 {
		t.Errorf("expected 3 rows for NOT IN [1,2], got %d", len(td.Rows))
	}
}

func TestQueryCovWhereNotBetween(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// price not between 10 25
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("price"), engine.NewAtom("not"), engine.NewAtom("between"), engine.NewInteger(10), engine.NewInteger(25),
		}),
	})
	td := extractTD(t, result[0])
	// prices outside 10-25: 30 and 5 = 2 rows
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows for NOT BETWEEN 10 25, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereIsNull(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// Create a table with a nullable column
	fields := engine.NewOrderedMap()
	fields.Set("name", engine.NewTypeLiteral(engine.TString))
	fields.Set("note", engine.NewTypeLiteral(engine.TString))
	rec := engine.RecordTypeInfo{Fields: fields}

	row1 := engine.NewOrderedMap()
	row1.Set("name", engine.NewString("a"))
	row1.Set("note", engine.NewString("has note"))
	row2 := engine.NewOrderedMap()
	row2.Set("name", engine.NewString("b"))
	row2.Set("note", engine.Value{VType: engine.TNone})

	td := engine.TableData{
		Record: rec,
		Rows:   []engine.Value{engine.NewMap(row1), engine.NewMap(row2)},
	}
	r.ContextSet("items", engine.Value{VType: engine.TList, Data: td})

	// where [note is null]
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("items"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("note"), engine.NewAtom("is"), engine.NewAtom("null"),
		}),
	})
	td2 := extractTD(t, result[0])
	if len(td2.Rows) != 1 {
		t.Errorf("expected 1 row with null note, got %d", len(td2.Rows))
	}
}

func TestQueryCovWhereNotGroup(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// not [category eq "misc"]
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("not"),
			engine.NewList([]engine.Value{engine.NewAtom("category"), engine.NewAtom("eq"), engine.NewString("misc")}),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 4 {
		t.Errorf("expected 4 rows for NOT [misc], got %d", len(td.Rows))
	}
}

func TestQueryCovWhereNotSingle(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// not category eq "misc"
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("not"), engine.NewAtom("category"), engine.NewAtom("eq"), engine.NewString("misc"),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 4 {
		t.Errorf("expected 4 rows for NOT category=misc, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereNotGroupAndMore(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// not [category eq "misc"] and price gt 10
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("not"),
			engine.NewList([]engine.Value{engine.NewAtom("category"), engine.NewAtom("eq"), engine.NewString("misc")}),
			engine.NewAtom("and"),
			engine.NewAtom("price"), engine.NewAtom("gt"), engine.NewInteger(10),
		}),
	})
	td := extractTD(t, result[0])
	// Not misc AND price > 10: Widget(10 not >10), Gadget(25), Gizmo(15), Doohickey(30) => 3
	if len(td.Rows) != 3 {
		t.Errorf("expected 3 rows, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereNotSingleAndMore(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// not category eq "misc" and price gt 10
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("not"), engine.NewAtom("category"), engine.NewAtom("eq"), engine.NewString("misc"),
			engine.NewAtom("and"),
			engine.NewAtom("price"), engine.NewAtom("gt"), engine.NewInteger(10),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// Create two tables with a shared "pid" column for USING join
	f1 := engine.NewOrderedMap()
	f1.Set("pid", engine.NewTypeLiteral(engine.TInteger))
	f1.Set("oname", engine.NewTypeLiteral(engine.TString))
	rec1 := engine.RecordTypeInfo{Fields: f1}

	f2 := engine.NewOrderedMap()
	f2.Set("pid", engine.NewTypeLiteral(engine.TInteger))
	f2.Set("pname", engine.NewTypeLiteral(engine.TString))
	rec2 := engine.RecordTypeInfo{Fields: f2}

	mk1 := func(pid int64, oname string) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("pid", engine.NewInteger(pid))
		om.Set("oname", engine.NewString(oname))
		return engine.NewMap(om)
	}
	mk2 := func(pid int64, pname string) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("pid", engine.NewInteger(pid))
		om.Set("pname", engine.NewString(pname))
		return engine.NewMap(om)
	}

	td1 := engine.TableData{Record: rec1, Rows: []engine.Value{mk1(1, "order1"), mk1(2, "order2"), mk1(1, "order3")}}
	td2 := engine.TableData{Record: rec2, Rows: []engine.Value{mk2(1, "Widget"), mk2(2, "Gadget"), mk2(3, "Gizmo")}}
	r.ContextSet("jorders", engine.Value{VType: engine.TList, Data: td1})
	r.ContextSet("jproducts", engine.Value{VType: engine.TList, Data: td2})

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("jorders"),
		engine.NewWord("join"), engine.NewWord("jproducts"),
		engine.NewWord("using"), engine.NewList([]engine.Value{engine.NewAtom("pid")}),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// Create tables with a shared column for USING
	f1 := engine.NewOrderedMap()
	f1.Set("pid", engine.NewTypeLiteral(engine.TInteger))
	f1.Set("pname", engine.NewTypeLiteral(engine.TString))
	rec1 := engine.RecordTypeInfo{Fields: f1}

	f2 := engine.NewOrderedMap()
	f2.Set("pid", engine.NewTypeLiteral(engine.TInteger))
	f2.Set("qty", engine.NewTypeLiteral(engine.TInteger))
	rec2 := engine.RecordTypeInfo{Fields: f2}

	mk1 := func(pid int64, pname string) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("pid", engine.NewInteger(pid))
		om.Set("pname", engine.NewString(pname))
		return engine.NewMap(om)
	}
	mk2 := func(pid, qty int64) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("pid", engine.NewInteger(pid))
		om.Set("qty", engine.NewInteger(qty))
		return engine.NewMap(om)
	}

	td1 := engine.TableData{Record: rec1, Rows: []engine.Value{mk1(1, "A"), mk1(2, "B"), mk1(3, "C")}}
	td2 := engine.TableData{Record: rec2, Rows: []engine.Value{mk2(1, 10), mk2(1, 20)}}
	r.ContextSet("ljProducts", engine.Value{VType: engine.TList, Data: td1})
	r.ContextSet("ljOrders", engine.Value{VType: engine.TList, Data: td2})

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("ljProducts"),
		engine.NewWord("leftjoin"), engine.NewWord("ljOrders"),
		engine.NewWord("using"), engine.NewList([]engine.Value{engine.NewAtom("pid")}),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// Small tables for cross join
	f1 := engine.NewOrderedMap()
	f1.Set("x", engine.NewTypeLiteral(engine.TInteger))
	r1 := engine.NewOrderedMap()
	r1.Set("x", engine.NewInteger(1))
	r2 := engine.NewOrderedMap()
	r2.Set("x", engine.NewInteger(2))
	td1 := engine.TableData{Record: engine.RecordTypeInfo{Fields: f1}, Rows: []engine.Value{engine.NewMap(r1), engine.NewMap(r2)}}
	r.ContextSet("tblA", engine.Value{VType: engine.TList, Data: td1})

	f2 := engine.NewOrderedMap()
	f2.Set("y", engine.NewTypeLiteral(engine.TString))
	ra := engine.NewOrderedMap()
	ra.Set("y", engine.NewString("a"))
	rb := engine.NewOrderedMap()
	rb.Set("y", engine.NewString("b"))
	td2 := engine.TableData{Record: engine.RecordTypeInfo{Fields: f2}, Rows: []engine.Value{engine.NewMap(ra), engine.NewMap(rb)}}
	r.ContextSet("tblB", engine.Value{VType: engine.TList, Data: td2})

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("tblA"),
		engine.NewWord("crossjoin"), engine.NewWord("tblB"),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// Two simple single-column tables
	f := engine.NewOrderedMap()
	f.Set("val", engine.NewTypeLiteral(engine.TInteger))
	rec := engine.RecordTypeInfo{Fields: f}

	mkRow := func(v int64) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("val", engine.NewInteger(v))
		return engine.NewMap(om)
	}

	td1 := engine.TableData{Record: rec, Rows: []engine.Value{mkRow(1), mkRow(2), mkRow(3)}}
	td2 := engine.TableData{Record: rec, Rows: []engine.Value{mkRow(2), mkRow(3), mkRow(4)}}
	r.ContextSet("setA", engine.Value{VType: engine.TList, Data: td1})
	r.ContextSet("setB", engine.Value{VType: engine.TList, Data: td2})

	// from setA union from setB select star — UNION removes duplicates
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("setA"),
		engine.NewWord("union"),
		engine.NewWord("from"), engine.NewWord("setB"),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 4 {
		t.Errorf("expected 4 rows from UNION (1,2,3,4), got %d", len(td.Rows))
	}
}

func TestQueryCovUnionAll(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	f := engine.NewOrderedMap()
	f.Set("val", engine.NewTypeLiteral(engine.TInteger))
	rec := engine.RecordTypeInfo{Fields: f}
	mkRow := func(v int64) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("val", engine.NewInteger(v))
		return engine.NewMap(om)
	}

	td1 := engine.TableData{Record: rec, Rows: []engine.Value{mkRow(1), mkRow(2)}}
	td2 := engine.TableData{Record: rec, Rows: []engine.Value{mkRow(2), mkRow(3)}}
	r.ContextSet("uaA", engine.Value{VType: engine.TList, Data: td1})
	r.ContextSet("uaB", engine.Value{VType: engine.TList, Data: td2})

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("uaA"),
		engine.NewWord("unionall"),
		engine.NewWord("from"), engine.NewWord("uaB"),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 4 {
		t.Errorf("expected 4 rows from UNION ALL, got %d", len(td.Rows))
	}
}

func TestQueryCovIntersect(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	f := engine.NewOrderedMap()
	f.Set("val", engine.NewTypeLiteral(engine.TInteger))
	rec := engine.RecordTypeInfo{Fields: f}
	mkRow := func(v int64) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("val", engine.NewInteger(v))
		return engine.NewMap(om)
	}

	td1 := engine.TableData{Record: rec, Rows: []engine.Value{mkRow(1), mkRow(2), mkRow(3)}}
	td2 := engine.TableData{Record: rec, Rows: []engine.Value{mkRow(2), mkRow(3), mkRow(4)}}
	r.ContextSet("isA", engine.Value{VType: engine.TList, Data: td1})
	r.ContextSet("isB", engine.Value{VType: engine.TList, Data: td2})

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("isA"),
		engine.NewWord("intersect"),
		engine.NewWord("from"), engine.NewWord("isB"),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 rows from INTERSECT (2,3), got %d", len(td.Rows))
	}
}

func TestQueryCovExcept(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	f := engine.NewOrderedMap()
	f.Set("val", engine.NewTypeLiteral(engine.TInteger))
	rec := engine.RecordTypeInfo{Fields: f}
	mkRow := func(v int64) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("val", engine.NewInteger(v))
		return engine.NewMap(om)
	}

	td1 := engine.TableData{Record: rec, Rows: []engine.Value{mkRow(1), mkRow(2), mkRow(3)}}
	td2 := engine.TableData{Record: rec, Rows: []engine.Value{mkRow(2), mkRow(3), mkRow(4)}}
	r.ContextSet("exA", engine.Value{VType: engine.TList, Data: td1})
	r.ContextSet("exB", engine.Value{VType: engine.TList, Data: td2})

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("exA"),
		engine.NewWord("except"),
		engine.NewWord("from"), engine.NewWord("exB"),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("order"), engine.NewList([]engine.Value{engine.NewAtom("price"), engine.NewAtom("desc")}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(td.Rows))
	}
	// First row should be the most expensive (30)
	first, _ := engine.AsMap(td.Rows[0])
	price, _ := first.Get("price")
	_as3, _ := engine.AsNumber(price)
	if _as3 != 30 {
		t.Errorf("expected first price=30 (desc), got %v", price)
	}
}

func TestQueryCovOrderByMultiple(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// order by [category asc price desc]
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("order"), engine.NewList([]engine.Value{
			engine.NewAtom("category"), engine.NewAtom("asc"),
			engine.NewAtom("price"), engine.NewAtom("desc"),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 5 {
		t.Errorf("expected 5 rows, got %d", len(td.Rows))
	}
}

func TestQueryCovGroupByWithSum(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeSalesTable(r)

	// select [region [sum amount total]] from sales group by [region]
	sumSpec := engine.NewList([]engine.Value{engine.NewAtom("sum"), engine.NewAtom("amount"), engine.NewAtom("total")})
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewList([]engine.Value{engine.NewAtom("region"), sumSpec}),
		engine.NewWord("from"), engine.NewWord("sales"),
		engine.NewWord("group"), engine.NewWord("by"), engine.NewList([]engine.Value{engine.NewAtom("region")}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 groups (east, west), got %d", len(td.Rows))
	}
}

func TestQueryCovGroupByAtom(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeSalesTable(r)

	// select [region [count * cnt]] from sales group region
	countSpec := engine.NewList([]engine.Value{engine.NewAtom("count"), engine.NewAtom("*"), engine.NewAtom("cnt")})
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewList([]engine.Value{engine.NewAtom("region"), countSpec}),
		engine.NewWord("from"), engine.NewWord("sales"),
		engine.NewWord("group"), engine.NewWord("region"),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 2 {
		t.Errorf("expected 2 groups, got %d", len(td.Rows))
	}
}

func TestQueryCovGroupByMultiple(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeSalesTable(r)

	// group by [region product]
	countSpec := engine.NewList([]engine.Value{engine.NewAtom("count"), engine.NewAtom("*"), engine.NewAtom("cnt")})
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewList([]engine.Value{engine.NewAtom("region"), engine.NewAtom("product"), countSpec}),
		engine.NewWord("from"), engine.NewWord("sales"),
		engine.NewWord("group"), engine.NewWord("by"), engine.NewList([]engine.Value{engine.NewAtom("region"), engine.NewAtom("product")}),
	})
	td := extractTD(t, result[0])
	// east/A: 2, east/B: 1, west/A: 1, west/B: 1 => 4 groups
	if len(td.Rows) != 4 {
		t.Errorf("expected 4 groups, got %d", len(td.Rows))
	}
}

func TestQueryCovHaving(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeSalesTable(r)

	// select [region [sum amount total]] from sales group by [region] having [total gt 300]
	sumSpec := engine.NewList([]engine.Value{engine.NewAtom("sum"), engine.NewAtom("amount"), engine.NewAtom("total")})
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewList([]engine.Value{engine.NewAtom("region"), sumSpec}),
		engine.NewWord("from"), engine.NewWord("sales"),
		engine.NewWord("group"), engine.NewWord("by"), engine.NewList([]engine.Value{engine.NewAtom("region")}),
		engine.NewWord("having"), engine.NewList([]engine.Value{
			engine.NewAtom("total"), engine.NewAtom("gt"), engine.NewInteger(300),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)
	makeOrdersTable(r)

	// from products where [id in (select [product_id] from orders)] select star
	// The "(" and ")" are words used as parentheses for subquery evaluation
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("id"), engine.NewAtom("in"),
			engine.NewOpenParen(),
			engine.NewWord("select"), engine.NewList([]engine.Value{engine.NewAtom("product_id")}),
			engine.NewWord("from"), engine.NewWord("orders"),
			engine.NewCloseParen(),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// select [name [(select [[max price]] from products) max_price]] from products limit 1
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewList([]engine.Value{
			engine.NewAtom("name"),
			engine.NewList([]engine.Value{
				engine.NewOpenParen(),
				engine.NewWord("select"), engine.NewList([]engine.Value{
					engine.NewList([]engine.Value{engine.NewAtom("max"), engine.NewAtom("price")}),
				}),
				engine.NewWord("from"), engine.NewWord("products"),
				engine.NewCloseParen(),
				engine.NewAtom("max_price"),
			}),
		}),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("limit"), engine.NewInteger(1),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewList([]engine.Value{engine.NewAtom("category")}),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("distinct"),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	err = runAQLError(t, r, []engine.Value{
		engine.NewWord("from"), engine.NewWord("nonexistent_table"),
	})
	if err == nil {
		t.Error("expected error for unknown table")
	}
}

func TestQueryCovFromNonTable(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	// Store a non-table value
	r.ContextSet("notatable", engine.NewInteger(42))

	err = runAQLError(t, r, []engine.Value{
		engine.NewWord("from"), engine.NewWord("notatable"),
	})
	if err == nil {
		t.Error("expected error for non-table value")
	}
}

// ========================
// clone coverage — ensure clone copies slices
// ========================

func TestQueryCovCloneWithJoinsAndSetOps(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// Create tables with shared "pid" column for USING join
	f1 := engine.NewOrderedMap()
	f1.Set("pid", engine.NewTypeLiteral(engine.TInteger))
	f1.Set("price", engine.NewTypeLiteral(engine.TInteger))
	rec1 := engine.RecordTypeInfo{Fields: f1}

	f2 := engine.NewOrderedMap()
	f2.Set("pid", engine.NewTypeLiteral(engine.TInteger))
	f2.Set("qty", engine.NewTypeLiteral(engine.TInteger))
	rec2 := engine.RecordTypeInfo{Fields: f2}

	mk1 := func(pid, price int64) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("pid", engine.NewInteger(pid))
		om.Set("price", engine.NewInteger(price))
		return engine.NewMap(om)
	}
	mk2 := func(pid, qty int64) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("pid", engine.NewInteger(pid))
		om.Set("qty", engine.NewInteger(qty))
		return engine.NewMap(om)
	}

	td1 := engine.TableData{Record: rec1, Rows: []engine.Value{mk1(1, 10), mk1(2, 20)}}
	td2 := engine.TableData{Record: rec2, Rows: []engine.Value{mk2(1, 5), mk2(2, 3)}}
	r.ContextSet("cloneP", engine.Value{VType: engine.TList, Data: td1})
	r.ContextSet("cloneO", engine.Value{VType: engine.TList, Data: td2})

	// Build a query with join AND then clone it via the where word
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("cloneP"),
		engine.NewWord("join"), engine.NewWord("cloneO"),
		engine.NewWord("using"), engine.NewList([]engine.Value{engine.NewAtom("pid")}),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("price"), engine.NewAtom("gt"), engine.NewInteger(5),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// where [id in 1] — single value, not a list
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("id"), engine.NewAtom("in"), engine.NewInteger(1),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// where [id eq 1] — boolean comparison should work
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("id"), engine.NewAtom("eq"), engine.NewInteger(1),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	// Two tables with different columns, shared "pid"
	f1 := engine.NewOrderedMap()
	f1.Set("pid", engine.NewTypeLiteral(engine.TInteger))
	f1.Set("oname", engine.NewTypeLiteral(engine.TString))
	rec1 := engine.RecordTypeInfo{Fields: f1}

	f2 := engine.NewOrderedMap()
	f2.Set("pid", engine.NewTypeLiteral(engine.TInteger))
	f2.Set("pname", engine.NewTypeLiteral(engine.TString))
	rec2 := engine.RecordTypeInfo{Fields: f2}

	mk1 := func(pid int64, oname string) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("pid", engine.NewInteger(pid))
		om.Set("oname", engine.NewString(oname))
		return engine.NewMap(om)
	}
	mk2 := func(pid int64, pname string) engine.Value {
		om := engine.NewOrderedMap()
		om.Set("pid", engine.NewInteger(pid))
		om.Set("pname", engine.NewString(pname))
		return engine.NewMap(om)
	}

	td1 := engine.TableData{Record: rec1, Rows: []engine.Value{mk1(1, "o1")}}
	td2 := engine.TableData{Record: rec2, Rows: []engine.Value{mk2(1, "p1")}}
	r.ContextSet("msOrders", engine.Value{VType: engine.TList, Data: td1})
	r.ContextSet("msProducts", engine.Value{VType: engine.TList, Data: td2})

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("msOrders"),
		engine.NewWord("join"), engine.NewWord("msProducts"),
		engine.NewWord("using"), engine.NewList([]engine.Value{engine.NewAtom("pid")}),
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
	colList := engine.NewList([]engine.Value{engine.NewBoolean(true)})
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
	colList := engine.NewList([]engine.Value{engine.NewList([]engine.Value{engine.NewAtom("x")})})
	_, err := parseColumnSpec(colList)
	if err == nil {
		t.Error("expected error for sub-list with 1 element")
	}
}

func TestQueryCovSelectBadAliasLength(t *testing.T) {
	// Column spec pair with 3 elements (not aggregate/cast) -> error
	colList := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewAtom("x"), engine.NewAtom("y"), engine.NewAtom("z")}),
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
	cond := engine.NewList([]engine.Value{engine.NewAtom("x"), engine.NewAtom("badop"), engine.NewInteger(1)})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for unknown operator")
	}
}

func TestQueryCovWhereIncompleteAfterColumn(t *testing.T) {
	cond := engine.NewList([]engine.Value{engine.NewAtom("x")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for incomplete condition")
	}
}

func TestQueryCovWhereBetweenIncomplete(t *testing.T) {
	cond := engine.NewList([]engine.Value{engine.NewAtom("x"), engine.NewAtom("between"), engine.NewInteger(1)})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for incomplete between")
	}
}

func TestQueryCovWhereInEmpty(t *testing.T) {
	cond := engine.NewList([]engine.Value{engine.NewAtom("x"), engine.NewAtom("in")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for in without list")
	}
}

func TestQueryCovWhereIsNotBadToken(t *testing.T) {
	cond := engine.NewList([]engine.Value{engine.NewAtom("x"), engine.NewAtom("is"), engine.NewAtom("bad")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for 'is bad'")
	}
}

func TestQueryCovWhereIsNotNullMissing(t *testing.T) {
	cond := engine.NewList([]engine.Value{engine.NewAtom("x"), engine.NewAtom("is"), engine.NewAtom("not"), engine.NewAtom("bad")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for 'is not bad' (expected null)")
	}
}

func TestQueryCovWhereNotBadFollower(t *testing.T) {
	cond := engine.NewList([]engine.Value{engine.NewAtom("x"), engine.NewAtom("not"), engine.NewAtom("badop")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for 'not badop'")
	}
}

func TestQueryCovWhereNotInNoList(t *testing.T) {
	cond := engine.NewList([]engine.Value{engine.NewAtom("x"), engine.NewAtom("not"), engine.NewAtom("in")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for 'not in' without list")
	}
}

func TestQueryCovWhereNotBetweenIncomplete(t *testing.T) {
	cond := engine.NewList([]engine.Value{engine.NewAtom("x"), engine.NewAtom("not"), engine.NewAtom("between"), engine.NewInteger(1)})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for incomplete 'not between'")
	}
}

func TestQueryCovWhereEmpty(t *testing.T) {
	cond := engine.NewList([]engine.Value{})
	clause, err := buildWhereClause(cond)
	if err != nil {
		t.Fatal(err)
	}
	if clause != "1=1" {
		t.Errorf("empty where should be '1=1', got %q", clause)
	}
}

func TestQueryCovWhereIncompleteAfterOp(t *testing.T) {
	cond := engine.NewList([]engine.Value{engine.NewAtom("x"), engine.NewAtom("eq")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for incomplete condition after operator")
	}
}

func TestQueryCovWhereNotIncomplete(t *testing.T) {
	cond := engine.NewList([]engine.Value{engine.NewAtom("not")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for standalone not")
	}
}

func TestQueryCovWhereIsIncomplete(t *testing.T) {
	cond := engine.NewList([]engine.Value{engine.NewAtom("x"), engine.NewAtom("is")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for incomplete 'is'")
	}
}

func TestQueryCovWhereIsNotIncomplete(t *testing.T) {
	cond := engine.NewList([]engine.Value{engine.NewAtom("x"), engine.NewAtom("is"), engine.NewAtom("not")})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for 'is not' without null")
	}
}

// ========================
// buildInList edge case: empty list
// ========================

func TestQueryCovBuildInListEmpty(t *testing.T) {
	emptyList := engine.NewList([]engine.Value{})
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
	_, err := parseCastSpec([]engine.Value{engine.NewAtom("cast"), engine.NewAtom("col")})
	if err == nil {
		t.Error("expected error for cast with too few elements")
	}
}

func TestQueryCovCastTooManyElements(t *testing.T) {
	// [cast col type alias extra] — 5 elements, max is 4
	_, err := parseCastSpec([]engine.Value{engine.NewAtom("cast"), engine.NewAtom("col"), engine.NewAtom("int"), engine.NewAtom("a"), engine.NewAtom("b")})
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
	_, err := parseAggregateSpec("count", []engine.Value{engine.NewAtom("a"), engine.NewAtom("b"), engine.NewAtom("c")})
	if err == nil {
		t.Error("expected error for aggregate with 3 args")
	}
}

// ========================
// valueToSQL coverage
// ========================

func TestQueryCovValueToSQLTypes(t *testing.T) {
	// Test each supported type
	s, err := valueToSQL(engine.NewString("hello"))
	if err != nil || s != "'hello'" {
		t.Errorf("string: %v %v", s, err)
	}

	s, err = valueToSQL(engine.NewInteger(42))
	if err != nil || s != "42" {
		t.Errorf("integer: %v %v", s, err)
	}

	s, err = valueToSQL(engine.NewBoolean(true))
	if err != nil || s != "'true'" {
		t.Errorf("boolean true: %v %v", s, err)
	}

	s, err = valueToSQL(engine.NewBoolean(false))
	if err != nil || s != "'false'" {
		t.Errorf("boolean false: %v %v", s, err)
	}

	s, err = valueToSQL(engine.NewAtom("test"))
	if err != nil || s != "'test'" {
		t.Errorf("atom: %v %v", s, err)
	}

	s, err = valueToSQL(engine.Value{VType: engine.TNone})
	if err != nil || s != "NULL" {
		t.Errorf("none: %v %v", s, err)
	}

	// Unsupported type
	_, err = valueToSQL(engine.NewList([]engine.Value{engine.NewInteger(1)}))
	if err == nil {
		t.Error("expected error for unsupported type in valueToSQL")
	}
}

func TestQueryCovValueToSQLEscaping(t *testing.T) {
	s, err := valueToSQL(engine.NewString("it's"))
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// from products order price select star — single atom order
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("order"), engine.NewWord("price"),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	td := engine.TableData{
		Record: engine.RecordTypeInfo{Fields: engine.NewOrderedMap()},
	}
	qb := engine.NewQueryBuilder(r, td)
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("name"), engine.NewAtom("eq"), engine.NewString("Widget"),
			engine.NewAtom("collate"), engine.NewAtom("binary"),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 1 {
		t.Errorf("expected 1 row with collate binary, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereCollateRtrim(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("name"), engine.NewAtom("eq"), engine.NewString("Widget"),
			engine.NewAtom("collate"), engine.NewAtom("rtrim"),
		}),
	})
	td := extractTD(t, result[0])
	if len(td.Rows) != 1 {
		t.Errorf("expected 1 row with collate rtrim, got %d", len(td.Rows))
	}
}

func TestQueryCovWhereCollateBadType(t *testing.T) {
	cond := engine.NewList([]engine.Value{
		engine.NewAtom("x"), engine.NewAtom("eq"), engine.NewString("y"),
		engine.NewAtom("collate"), engine.NewAtom("badcollation"),
	})
	_, err := buildWhereClause(cond)
	if err == nil {
		t.Error("expected error for bad collation type")
	}
}

func TestQueryCovWhereCollateMissingType(t *testing.T) {
	cond := engine.NewList([]engine.Value{
		engine.NewAtom("x"), engine.NewAtom("eq"), engine.NewString("y"),
		engine.NewAtom("collate"),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	err = runAQLError(t, r, []engine.Value{
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("on"), engine.NewList([]engine.Value{engine.NewAtom("x"), engine.NewAtom("eq"), engine.NewAtom("y")}),
	})
	if err == nil {
		t.Error("expected error for on without preceding join")
	}
}

func TestQueryCovUsingWithoutJoin(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	err = runAQLError(t, r, []engine.Value{
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("using"), engine.NewList([]engine.Value{engine.NewAtom("id")}),
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
	colList := engine.NewList([]engine.Value{engine.NewWord("mycolumn")})
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// where [[price gt 10] and [price lt 30]] — nested condition groups
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewList([]engine.Value{engine.NewAtom("price"), engine.NewAtom("gt"), engine.NewInteger(10)}),
			engine.NewAtom("and"),
			engine.NewList([]engine.Value{engine.NewAtom("price"), engine.NewAtom("lt"), engine.NewInteger(30)}),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// First select materializes, the result has SQLite=true
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
	})
	td := extractTD(t, result[0])

	// Store the result back and query it again (already SQLite)
	r.ContextSet("products_sq", engine.Value{VType: engine.TList, Data: td})

	result = runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products_sq"),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	// price >= 15 and price <= 25
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("price"), engine.NewAtom("gte"), engine.NewInteger(15),
			engine.NewAtom("and"),
			engine.NewAtom("price"), engine.NewAtom("lte"), engine.NewInteger(25),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	makeProductsTable(r)

	result := runAQL(t, r, []engine.Value{
		engine.NewWord("select"), engine.NewWord("star"),
		engine.NewWord("from"), engine.NewWord("products"),
		engine.NewWord("where"), engine.NewList([]engine.Value{
			engine.NewAtom("price"), engine.NewAtom("between"), engine.NewInteger(10), engine.NewInteger(20),
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
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}

	td := engine.TableData{Record: engine.RecordTypeInfo{Fields: engine.NewOrderedMap()}}
	qb := engine.NewQueryBuilder(r, td)
	qb.Alias = "t"
	qb.Where = `"x" > 1`

	sql := qb.buildSQL("mytable", "*")
	if !strings.Contains(sql, "AS") {
		t.Errorf("expected AS in SQL for alias, got %q", sql)
	}
}
