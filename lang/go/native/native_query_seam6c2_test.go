package native

import (
	"strings"
	"testing"
)

// Seam-6 coverage for native_query.go handler error arms
// (design/TEST-SEAMS.10.md).

func seam6c2Table() Value {
	fields := NewOrderedMap()
	fields.Set("a", NewTypeLiteral(TInteger))
	row := NewOrderedMap()
	row.Set("a", NewInteger(1))
	return NewValueRaw(TTable, TableData{
		Record: RecordTypeInfo{Fields: fields},
		Rows:   []Value{NewMap(row)},
	})
}

func TestSeam6C2FromNameHandlerArms(t *testing.T) {
	r := seam5Reg(t)

	// Non-atom table name.
	_, err := fromNameHandler([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "from:") {
		t.Fatalf("expected from name-coercion error, got %v", err)
	}

	// Valid table but a non-coercible upstream builder.
	r.ContextSet("tbl", seam6c2Table())
	_, err = fromNameHandler([]Value{NewAtom("tbl"), NewInteger(2)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "not a table or query") {
		t.Fatalf("expected builder coercion error, got %v", err)
	}

	// Positive pair: a select builder upstream succeeds.
	sel := wrapQB(newSelectBuilder(r, nil))
	out, err := fromNameHandler([]Value{NewAtom("tbl"), sel}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("expected wrapped builder, got %v / %v", out, err)
	}
}

func TestSeam6C2FromValueHandlerArms(t *testing.T) {
	r := seam5Reg(t)

	// Source not coercible at all.
	_, err := fromValueHandler([]Value{NewInteger(1), NewInteger(2)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "not a table or query") {
		t.Fatalf("expected source coercion error, got %v", err)
	}

	// A builder with no FROM table is not a usable source.
	empty := wrapQB(QueryBuilder{})
	_, err = fromValueHandler([]Value{empty, NewInteger(2)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "value is not a table") {
		t.Fatalf("expected no-source error, got %v", err)
	}

	// Valid source but a non-coercible upstream builder.
	_, err = fromValueHandler([]Value{seam6c2Table(), NewInteger(2)}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "not a table or query") {
		t.Fatalf("expected upstream coercion error, got %v", err)
	}

	// Positive pair: table source + select builder.
	sel := wrapQB(newSelectBuilder(r, nil))
	out, err := fromValueHandler([]Value{seam6c2Table(), sel}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("expected wrapped builder, got %v / %v", out, err)
	}
	qb, err2 := toQueryBuilder(r, out[0])
	if err2 != nil || !qb.HasSource {
		t.Fatalf("expected builder with source, got %v / %v", qb, err2)
	}
}

func TestSeam6C2QueryClauseSubExprErrors(t *testing.T) {
	r := seam5Reg(t)
	unmatched := NewList([]Value{NewOpenParen()})
	sel := wrapQB(newSelectBuilder(r, nil))

	_, err := queryWhereHandler([]Value{unmatched, sel}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "where: unmatched parenthesis") {
		t.Fatalf("expected where paren error, got %v", err)
	}

	_, err = queryHavingHandler([]Value{unmatched, sel}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "having: unmatched parenthesis") {
		t.Fatalf("expected having paren error, got %v", err)
	}

	_, err = selectColsHandler([]Value{unmatched}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "select: unmatched parenthesis") {
		t.Fatalf("expected select paren error, got %v", err)
	}

	// A malformed column spec (nested list with fewer than 2 elements).
	badCols := NewList([]Value{NewList([]Value{NewAtom("a")})})
	_, err = selectColsHandler([]Value{badCols}, nil, nil, r)
	if err == nil || !strings.Contains(err.Error(), "at least 2 elements") {
		t.Fatalf("expected column spec error, got %v", err)
	}

	// Positive pair: plain columns parse.
	cols := NewList([]Value{NewAtom("a")})
	out, err := selectColsHandler([]Value{cols}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("expected select builder, got %v / %v", out, err)
	}
}
