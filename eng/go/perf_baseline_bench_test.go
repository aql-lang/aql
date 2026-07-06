package eng

import (
	"strconv"
	"testing"
)

// Kernel performance baseline (perf-baseline suite). One benchmark per
// kernel primitive that dominates interpreter hot paths: value
// comparison, unification, canonical rendering, and the OrderedMap the
// Map payload is built on. Run with:
//
//	go test -bench 'BenchmarkKernel' -benchmem -run '^$' .
//
// or `make bench` from the repo root. These pin the CURRENT cost of
// each primitive so an algorithmic regression (a new allocation in
// CompareValues, a quadratic walk in canon) shows up as a step change
// against the committed baseline rather than anecdote.

func benchScalarPairs() [][2]Value {
	return [][2]Value{
		{NewInteger(7), NewInteger(9)},
		{NewFloat(1.5), NewInteger(2)},
		{NewString("alpha"), NewString("beta")},
		{NewBoolean(true), NewBoolean(false)},
		{NewAtom("x"), NewAtom("y")},
	}
}

func benchList(n int) Value {
	elems := make([]Value, n)
	for i := range elems {
		elems[i] = NewInteger(int64(i))
	}
	return NewList(elems)
}

func benchMap(n int) Value {
	om := NewOrderedMap()
	for i := 0; i < n; i++ {
		om.Set("k"+strconv.Itoa(i), NewInteger(int64(i)))
	}
	return NewMap(om)
}

func BenchmarkKernelCompareScalars(b *testing.B) {
	pairs := benchScalarPairs()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, p := range pairs {
			if _, err := CompareValues(p[0], p[1]); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkKernelCompareList64(b *testing.B) {
	l1, l2 := benchList(64), benchList(64)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := CompareValues(l1, l2); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkKernelCompareMap64(b *testing.B) {
	m1, m2 := benchMap(64), benchMap(64)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := CompareValues(m1, m2); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkKernelUnifyScalarType(b *testing.B) {
	v := NewInteger(42)
	t := NewTypeLiteral(TInteger)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := Unify(v, t); !ok {
			b.Fatal("unify failed")
		}
	}
}

func BenchmarkKernelUnifyList32(b *testing.B) {
	l := benchList(32)
	t := NewTypedList(NewTypeLiteral(TInteger))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := Unify(l, t); !ok {
			b.Fatal("unify failed")
		}
	}
}

func BenchmarkKernelCanonScalar(b *testing.B) {
	vals := []Value{NewInteger(42), NewFloat(2.5), NewString("hello"), NewBoolean(true)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, v := range vals {
			_ = CanonValue(v)
		}
	}
}

func BenchmarkKernelCanonNested(b *testing.B) {
	om := NewOrderedMap()
	om.Set("xs", benchList(16))
	om.Set("m", benchMap(16))
	v := NewMap(om)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CanonValue(v)
	}
}

func BenchmarkKernelOrderedMapSetGet(b *testing.B) {
	keys := make([]string, 64)
	for i := range keys {
		keys[i] = "key" + strconv.Itoa(i)
	}
	one := NewInteger(1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		om := NewOrderedMap()
		for _, k := range keys {
			om.Set(k, one)
		}
		for _, k := range keys {
			if _, ok := om.Get(k); !ok {
				b.Fatal("missing key")
			}
		}
	}
}
