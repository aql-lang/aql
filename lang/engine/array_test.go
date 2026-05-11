package engine_test

import (
	"github.com/aql-lang/aql/lang/engine"
	"github.com/aql-lang/aql/lang/native"
	"testing"
)

// --- iota ---

func TestIota(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{engine.NewWord("iota"), engine.NewInteger(5)})
	list := result[0].AsList()
	if list.Len() != 5 {
		t.Fatalf("iota 5: length = %d, want 5", list.Len())
	}
	for i := 0; i < 5; i++ {
		_as0, _ := list.Get(i).AsInteger()
		if _as0 != int64(i) {
			_as1, _ := list.Get(i).AsInteger()
			t.Errorf("iota 5[%d] = %d, want %d", i, _as1, i)
		}
	}
}

func TestIotaZero(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{engine.NewWord("iota"), engine.NewInteger(0)})
	list := result[0].AsList()
	if list.Len() != 0 {
		t.Errorf("iota 0: length = %d, want 0", list.Len())
	}
}

// --- shape ---

func TestShapeFlat(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3)}),
		engine.NewWord("shape"),
	})
	list := result[0].AsList()
	_as2, _ := list.Get(0).AsInteger()
	if list.Len() != 1 || _as2 != 3 {
		t.Errorf("shape [1,2,3] = %v, want [3]", result[0])
	}
}

func TestShapeNested(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	input := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2)}),
		engine.NewList([]engine.Value{engine.NewInteger(3), engine.NewInteger(4)}),
		engine.NewList([]engine.Value{engine.NewInteger(5), engine.NewInteger(6)}),
	})
	result := runAQL(t, r, []engine.Value{input, engine.NewWord("shape")})
	list := result[0].AsList()
	_as4, _ := list.Get(0).AsInteger()
	_as3, _ := list.Get(1).AsInteger()
	if list.Len() != 2 || _as4 != 3 || _as3 != 2 {
		t.Errorf("shape [[1,2],[3,4],[5,6]] = %v, want [3,2]", result[0])
	}
}

// --- rank ---

func TestRank(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2)}),
		engine.NewWord("rank"),
	})
	_as5, _ := result[0].AsInteger()
	if _as5 != 1 {
		_as6, _ := result[0].AsInteger()
		t.Errorf("rank [1,2] = %d, want 1", _as6)
	}

	input := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2)}),
		engine.NewList([]engine.Value{engine.NewInteger(3), engine.NewInteger(4)}),
	})
	result = runAQL(t, r, []engine.Value{input, engine.NewWord("rank")})
	_as7, _ := result[0].AsInteger()
	if _as7 != 2 {
		_as8, _ := result[0].AsInteger()
		t.Errorf("rank [[1,2],[3,4]] = %d, want 2", _as8)
	}
}

// --- length ---

func TestLength(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(10), engine.NewInteger(20), engine.NewInteger(30)}),
		engine.NewWord("length"),
	})
	_as9, _ := result[0].AsInteger()
	if _as9 != 3 {
		_as10, _ := result[0].AsInteger()
		t.Errorf("length = %d, want 3", _as10)
	}
}

// --- reshape ---

func TestReshape(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("reshape"),
		engine.NewList([]engine.Value{engine.NewInteger(2), engine.NewInteger(3)}),
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3), engine.NewInteger(4), engine.NewInteger(5), engine.NewInteger(6)}),
	})
	outer := result[0].AsList()
	if outer.Len() != 2 {
		t.Fatalf("reshape rows = %d, want 2", outer.Len())
	}
	row0 := outer.Get(0).AsList()
	_as12, _ := row0.Get(0).AsInteger()
	_as11, _ := row0.Get(2).AsInteger()
	if row0.Len() != 3 || _as12 != 1 || _as11 != 3 {
		t.Errorf("reshape row 0 = %v, want [1,2,3]", outer.Get(0))
	}
}

// --- arr-flatten ---

func TestArrFlatten(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	input := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2)}),
		engine.NewList([]engine.Value{engine.NewInteger(3), engine.NewInteger(4)}),
	})
	result := runAQL(t, r, []engine.Value{input, engine.NewWord("arr-flatten")})
	list := result[0].AsList()
	if list.Len() != 4 {
		t.Fatalf("arr-flatten length = %d, want 4", list.Len())
	}
	for i := 0; i < 4; i++ {
		_as13, _ := list.Get(i).AsInteger()
		if _as13 != int64(i+1) {
			_as14, _ := list.Get(i).AsInteger()
			t.Errorf("arr-flatten[%d] = %d, want %d", i, _as14, i+1)
		}
	}
}

// --- arr-transpose ---

func TestArrTranspose(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	input := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3)}),
		engine.NewList([]engine.Value{engine.NewInteger(4), engine.NewInteger(5), engine.NewInteger(6)}),
	})
	result := runAQL(t, r, []engine.Value{input, engine.NewWord("arr-transpose")})
	outer := result[0].AsList()
	if outer.Len() != 3 {
		t.Fatalf("transpose rows = %d, want 3", outer.Len())
	}
	// First column: [1,4]
	col0 := outer.Get(0).AsList()
	_as16, _ := col0.Get(0).AsInteger()
	_as15, _ := col0.Get(1).AsInteger()
	if _as16 != 1 || _as15 != 4 {
		t.Errorf("transpose col 0 = %v, want [1,4]", outer.Get(0))
	}
}

// --- reverse ---

func TestReverse(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3)}),
		engine.NewWord("reverse"),
	})
	list := result[0].AsList()
	_as19, _ := list.Get(0).AsInteger()
	_as18, _ := list.Get(1).AsInteger()
	_as17, _ := list.Get(2).AsInteger()
	if _as19 != 3 || _as18 != 2 || _as17 != 1 {
		t.Errorf("reverse [1,2,3] = %v, want [3,2,1]", result[0])
	}
}

// --- take ---

func TestTake(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("take"), engine.NewInteger(2),
		engine.NewList([]engine.Value{engine.NewInteger(10), engine.NewInteger(20), engine.NewInteger(30), engine.NewInteger(40)}),
	})
	list := result[0].AsList()
	_as21, _ := list.Get(0).AsInteger()
	_as20, _ := list.Get(1).AsInteger()
	if list.Len() != 2 || _as21 != 10 || _as20 != 20 {
		t.Errorf("take 2 = %v, want [10,20]", result[0])
	}
}

func TestTakeNegative(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("take"), engine.NewInteger(-2),
		engine.NewList([]engine.Value{engine.NewInteger(10), engine.NewInteger(20), engine.NewInteger(30), engine.NewInteger(40)}),
	})
	list := result[0].AsList()
	_as23, _ := list.Get(0).AsInteger()
	_as22, _ := list.Get(1).AsInteger()
	if list.Len() != 2 || _as23 != 30 || _as22 != 40 {
		t.Errorf("take -2 = %v, want [30,40]", result[0])
	}
}

// --- shed ---

func TestShed(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("shed"), engine.NewInteger(1),
		engine.NewList([]engine.Value{engine.NewInteger(10), engine.NewInteger(20), engine.NewInteger(30), engine.NewInteger(40)}),
	})
	list := result[0].AsList()
	_as24, _ := list.Get(0).AsInteger()
	if list.Len() != 3 || _as24 != 20 {
		t.Errorf("shed 1 = %v, want [20,30,40]", result[0])
	}
}

// --- where ---

func TestWhere(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewList([]engine.Value{engine.NewBoolean(true), engine.NewBoolean(false), engine.NewBoolean(true), engine.NewBoolean(false), engine.NewBoolean(true)}),
		engine.NewWord("where"),
	})
	list := result[0].AsList()
	_as27, _ := list.Get(0).AsInteger()
	_as26, _ := list.Get(1).AsInteger()
	_as25, _ := list.Get(2).AsInteger()
	if list.Len() != 3 || _as27 != 0 || _as26 != 2 || _as25 != 4 {
		t.Errorf("where = %v, want [0,2,4]", result[0])
	}
}

// --- unique ---

func TestUnique(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(3), engine.NewInteger(1), engine.NewInteger(4), engine.NewInteger(1), engine.NewInteger(5)}),
		engine.NewWord("unique"),
	})
	list := result[0].AsList()
	if list.Len() != 4 {
		t.Fatalf("unique length = %d, want 4", list.Len())
	}
	expected := []int64{3, 1, 4, 5}
	for i, want := range expected {
		_as28, _ := list.Get(i).AsInteger()
		if _as28 != want {
			_as29, _ := list.Get(i).AsInteger()
			t.Errorf("unique[%d] = %d, want %d", i, _as29, want)
		}
	}
}

// --- grade ---

func TestGrade(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(30), engine.NewInteger(10), engine.NewInteger(40), engine.NewInteger(20)}),
		engine.NewWord("grade"),
	})
	list := result[0].AsList()
	// Sorted order: 10(1), 20(3), 30(0), 40(2)
	expected := []int64{1, 3, 0, 2}
	for i, want := range expected {
		_as30, _ := list.Get(i).AsInteger()
		if _as30 != want {
			_as31, _ := list.Get(i).AsInteger()
			t.Errorf("grade[%d] = %d, want %d", i, _as31, want)
		}
	}
}

// --- at ---

func TestAt(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("at"),
		engine.NewList([]engine.Value{engine.NewInteger(2), engine.NewInteger(0), engine.NewInteger(1)}),
		engine.NewList([]engine.Value{engine.NewString("a"), engine.NewString("b"), engine.NewString("c")}),
	})
	list := result[0].AsList()
	_as34, _ := list.Get(0).AsString()
	_as33, _ := list.Get(1).AsString()
	_as32, _ := list.Get(2).AsString()
	if _as34 != "c" || _as33 != "a" || _as32 != "b" {
		t.Errorf("at [2,0,1] = %v, want [c,a,b]", result[0])
	}
}

// --- sortby ---

func TestSortby(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("sortby"),
		engine.NewList([]engine.Value{engine.NewInteger(3), engine.NewInteger(1), engine.NewInteger(2)}),
		engine.NewList([]engine.Value{engine.NewString("c"), engine.NewString("a"), engine.NewString("b")}),
	})
	list := result[0].AsList()
	_as37, _ := list.Get(0).AsString()
	_as36, _ := list.Get(1).AsString()
	_as35, _ := list.Get(2).AsString()
	if _as37 != "a" || _as36 != "b" || _as35 != "c" {
		t.Errorf("sortby = %v, want [a,b,c]", result[0])
	}
}

// --- member ---

func TestMember(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("member"),
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3)}),
		engine.NewList([]engine.Value{engine.NewInteger(2), engine.NewInteger(4), engine.NewInteger(6)}),
	})
	list := result[0].AsList()
	_as38, _ := list.Get(1).AsBoolean()
	if !_as38 {
		t.Error("member: 2 should be in [2,4,6]")
	}
	_as39, _ := list.Get(0).AsBoolean()
	if _as39 {
		t.Error("member: 1 should NOT be in [2,4,6]")
	}
}

// --- window ---

func TestWindow(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("window"), engine.NewInteger(2),
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3), engine.NewInteger(4)}),
	})
	list := result[0].AsList()
	if list.Len() != 3 {
		t.Fatalf("window 2: length = %d, want 3", list.Len())
	}
	w0 := list.Get(0).AsList()
	_as41, _ := w0.Get(0).AsInteger()
	_as40, _ := w0.Get(1).AsInteger()
	if _as41 != 1 || _as40 != 2 {
		t.Errorf("window[0] = %v, want [1,2]", list.Get(0))
	}
}

// --- pairs ---

func TestPairs(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3), engine.NewInteger(4)}),
		engine.NewWord("pairs"),
	})
	list := result[0].AsList()
	if list.Len() != 3 {
		t.Fatalf("pairs: length = %d, want 3", list.Len())
	}
}

// --- replicate ---

func TestReplicate(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("replicate"),
		engine.NewList([]engine.Value{engine.NewInteger(2), engine.NewInteger(0), engine.NewInteger(3)}),
		engine.NewList([]engine.Value{engine.NewInteger(10), engine.NewInteger(20), engine.NewInteger(30)}),
	})
	list := result[0].AsList()
	// [10,10,30,30,30]
	if list.Len() != 5 {
		t.Fatalf("replicate length = %d, want 5", list.Len())
	}
	expected := []int64{10, 10, 30, 30, 30}
	for i, want := range expected {
		_as42, _ := list.Get(i).AsInteger()
		if _as42 != want {
			_as43, _ := list.Get(i).AsInteger()
			t.Errorf("replicate[%d] = %d, want %d", i, _as43, want)
		}
	}
}

// --- group ---

func TestGroupTwoArgs(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("group"),
		engine.NewList([]engine.Value{engine.NewAtom("a"), engine.NewAtom("b"), engine.NewAtom("a")}),
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3)}),
	})
	m := result[0].AsMap()
	aVal, ok := m.Get("a")
	if !ok {
		t.Fatal("group: key 'a' not found")
	}
	aList := aVal.AsList()
	_as45, _ := aList.Get(0).AsInteger()
	_as44, _ := aList.Get(1).AsInteger()
	if aList.Len() != 2 || _as45 != 1 || _as44 != 3 {
		t.Errorf("group a = %v, want [1,3]", aVal)
	}
}

// --- each (higher-order) ---

func TestEach(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("each"),
		engine.NewList([]engine.Value{engine.NewWord("mul"), engine.NewInteger(2)}),
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3)}),
	})
	list := result[0].AsList()
	expected := []int64{2, 4, 6}
	for i, want := range expected {
		_as46, _ := list.Get(i).AsInteger()
		if _as46 != want {
			_as47, _ := list.Get(i).AsInteger()
			t.Errorf("each[%d] = %d, want %d", i, _as47, want)
		}
	}
}

// --- fold ---

func TestFoldSum(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("fold"),
		engine.NewList([]engine.Value{engine.NewWord("add")}),
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3), engine.NewInteger(4)}),
	})
	_as48, _ := result[0].AsInteger()
	if _as48 != 10 {
		t.Errorf("fold [add] = %v, want 10", result[0])
	}
}

func TestFoldProduct(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("fold"),
		engine.NewList([]engine.Value{engine.NewWord("mul")}),
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3), engine.NewInteger(4)}),
	})
	_as49, _ := result[0].AsInteger()
	if _as49 != 24 {
		t.Errorf("fold [mul] = %v, want 24", result[0])
	}
}

// --- scan ---

func TestScan(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("scan"),
		engine.NewList([]engine.Value{engine.NewWord("add")}),
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3), engine.NewInteger(4)}),
	})
	list := result[0].AsList()
	expected := []int64{1, 3, 6, 10}
	for i, want := range expected {
		_as50, _ := list.Get(i).AsInteger()
		if _as50 != want {
			_as51, _ := list.Get(i).AsInteger()
			t.Errorf("scan[%d] = %d, want %d", i, _as51, want)
		}
	}
}

// --- outer ---

func TestOuterMul(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("outer"),
		engine.NewList([]engine.Value{engine.NewWord("mul")}),
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3)}),
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2), engine.NewInteger(3), engine.NewInteger(4)}),
	})
	// [[1,2,3,4],[2,4,6,8],[3,6,9,12]]
	outer := result[0].AsList()
	if outer.Len() != 3 {
		t.Fatalf("outer rows = %d, want 3", outer.Len())
	}
	row0 := outer.Get(0).AsList()
	_as52, _ := row0.Get(3).AsInteger()
	if _as52 != 4 {
		_as53, _ := row0.Get(3).AsInteger()
		t.Errorf("outer[0][3] = %d, want 4", _as53)
	}
	row2 := outer.Get(2).AsList()
	_as54, _ := row2.Get(2).AsInteger()
	if _as54 != 9 {
		_as55, _ := row2.Get(2).AsInteger()
		t.Errorf("outer[2][2] = %d, want 9", _as55)
	}
}

// --- inner (matrix multiply) ---

func TestInnerMatMul(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	left := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(1), engine.NewInteger(2)}),
		engine.NewList([]engine.Value{engine.NewInteger(3), engine.NewInteger(4)}),
	})
	right := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewInteger(5), engine.NewInteger(6)}),
		engine.NewList([]engine.Value{engine.NewInteger(7), engine.NewInteger(8)}),
	})
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("inner"),
		engine.NewList([]engine.Value{engine.NewWord("mul")}),
		engine.NewList([]engine.Value{engine.NewWord("add")}),
		left, right,
	})
	// [[1*5+2*7, 1*6+2*8], [3*5+4*7, 3*6+4*8]] = [[19,22],[43,50]]
	outer := result[0].AsList()
	r0 := outer.Get(0).AsList()
	r1 := outer.Get(1).AsList()
	_as57, _ := r0.Get(0).AsInteger()
	_as56, _ := r0.Get(1).AsInteger()
	if _as57 != 19 || _as56 != 22 {
		_as59, _ := r0.Get(0).AsInteger()
		_as58, _ := r0.Get(1).AsInteger()
		t.Errorf("inner row 0 = [%d,%d], want [19,22]", _as59, _as58)
	}
	_as61, _ := r1.Get(0).AsInteger()
	_as60, _ := r1.Get(1).AsInteger()
	if _as61 != 43 || _as60 != 50 {
		_as63, _ := r1.Get(0).AsInteger()
		_as62, _ := r1.Get(1).AsInteger()
		t.Errorf("inner row 1 = [%d,%d], want [43,50]", _as63, _as62)
	}
}

// --- composition: fold [add] each [dup mul] iota 5 => sum of squares 0+1+4+9+16=30 ---

func TestCompositionSumOfSquares(t *testing.T) {
	r, _ := engine.DefaultRegistry(native.Register)
	// (each [dup mul] (iota 5)) produces [0,1,4,9,16]
	// fold [add] over that produces 30
	result := runAQL(t, r, []engine.Value{
		engine.NewWord("fold"),
		engine.NewList([]engine.Value{engine.NewWord("add")}),
		engine.NewWord("("),
		engine.NewWord("each"),
		engine.NewList([]engine.Value{engine.NewWord("dup"), engine.NewWord("mul")}),
		engine.NewWord("("), engine.NewWord("iota"), engine.NewInteger(5), engine.NewWord(")"),
		engine.NewWord(")"),
	})
	_as64, _ := result[0].AsInteger()
	if _as64 != 30 {
		t.Errorf("sum of squares = %v, want 30", result[0])
	}
}
