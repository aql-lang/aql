package engine_test
import (
	"github.com/metsitaba/voxgig-exp/lang/internal/engine"
	"strings"
	"testing"
)

func TestValueStringWord(t *testing.T) {
	v := engine.NewWord("test")
	if !strings.Contains(v.String(), "test") {
		t.Errorf("expected word(test), got %s", v.String())
	}
}

func TestValueStringOpenParen(t *testing.T) {
	v := engine.NewWord("(")
	if !strings.Contains(v.String(), "(") {
		t.Errorf("expected word containing (, got %s", v.String())
	}
}

func TestValueStringDecimalCov(t *testing.T) {
	v := engine.NewDecimal(3.14)
	if v.String() != "3.14" {
		t.Errorf("expected 3.14, got %s", v.String())
	}
}

func TestValueStringIntegerCov(t *testing.T) {
	v := engine.NewInteger(42)
	if v.String() != "42" {
		t.Errorf("expected 42, got %s", v.String())
	}
}

func TestValueStringBoolTrue(t *testing.T) {
	v := engine.NewBoolean(true)
	if v.String() != "true" {
		t.Errorf("expected true, got %s", v.String())
	}
}

func TestValueStringBoolFalse(t *testing.T) {
	v := engine.NewBoolean(false)
	if v.String() != "false" {
		t.Errorf("expected false, got %s", v.String())
	}
}

func TestValueStringStringCov(t *testing.T) {
	v := engine.NewString("hello")
	if v.String() != "'hello'" {
		t.Errorf("expected 'hello', got %s", v.String())
	}
}

func TestValueStringAtomCov(t *testing.T) {
	v := engine.NewAtom("myatom")
	if v.String() != "myatom" {
		t.Errorf("expected myatom, got %s", v.String())
	}
}

func TestValueStringTypeLiteralCov(t *testing.T) {
	v := engine.NewTypeLiteral(engine.TNumber)
	s := v.String()
	if s == "" {
		t.Error("expected non-empty type literal string")
	}
}

func TestValueStringTableDataCov(t *testing.T) {
	fields := engine.NewOrderedMap()
	fields.Set("x", engine.NewTypeLiteral(engine.TInteger))
	row := engine.NewOrderedMap()
	row.Set("x", engine.NewInteger(1))
	td := engine.TableData{
		Record: engine.RecordTypeInfo{Fields: fields},
		Rows:   []engine.Value{engine.NewMap(row)},
	}
	v := engine.Value{VType: engine.TList, Data: td}
	s := v.String()
	if !strings.HasPrefix(s, "table{") {
		t.Errorf("expected table{...}, got %s", s)
	}
}

func TestValueAsNumberCov(t *testing.T) {
	v := engine.NewInteger(42)
	_as0, _ := v.AsNumber()
	if _as0 != 42.0 {
		_as1, _ := v.AsNumber()
		t.Errorf("expected 42.0, got %f", _as1)
	}
	v = engine.NewDecimal(3.14)
	_as2, _ := v.AsNumber()
	if _as2 != 3.14 {
		_as3, _ := v.AsNumber()
		t.Errorf("expected 3.14, got %f", _as3)
	}
}

func TestValueAsTableTypeCov(t *testing.T) {
	fields := engine.NewOrderedMap()
	fields.Set("x", engine.NewTypeLiteral(engine.TInteger))
	tti := engine.TableTypeInfo{Record: engine.RecordTypeInfo{Fields: fields}}
	v := engine.Value{VType: engine.TList, Data: tti}
	tt, _ := v.AsTableType()
	if tt.Record.Fields.Len() != 1 {
		t.Errorf("expected 1 field, got %d", tt.Record.Fields.Len())
	}
}

func TestValueAsListCov(t *testing.T) {
	v := engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2)})
	list := v.AsList().Slice()
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}

	fields := engine.NewOrderedMap()
	fields.Set("x", engine.NewTypeLiteral(engine.TInteger))
	row := engine.NewOrderedMap()
	row.Set("x", engine.NewInteger(1))
	td := engine.TableData{
		Record: engine.RecordTypeInfo{Fields: fields},
		Rows:   []engine.Value{engine.NewMap(row)},
	}
	v = engine.Value{VType: engine.TList, Data: td}
	list = v.AsList().Slice()
	if len(list) != 1 {
		t.Fatalf("expected 1 row, got %d", len(list))
	}
}
