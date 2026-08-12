package eng

import (
	"strconv"
	"testing"

	core "github.com/boru-lang/boru/core/go"
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

func benchScalarPairs() [][2]core.Value {
	return [][2]core.Value{
		{core.NewInteger(7), core.NewInteger(9)},
		{core.NewFloat(1.5), core.NewInteger(2)},
		{core.NewString("alpha"), core.NewString("beta")},
		{core.NewBoolean(true), core.NewBoolean(false)},
		{core.NewAtom("x"), core.NewAtom("y")},
	}
}

func benchList(n int) core.Value {
	elems := make([]core.Value, n)
	for i := range elems {
		elems[i] = core.NewInteger(int64(i))
	}
	return core.NewList(elems)
}

func benchMap(n int) core.Value {
	om := core.NewOrderedMap()
	for i := 0; i < n; i++ {
		om.Set("k"+strconv.Itoa(i), core.NewInteger(int64(i)))
	}
	return core.NewMap(om)
}

func BenchmarkKernelCompareScalars(b *testing.B) {
	pairs := benchScalarPairs()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, p := range pairs {
			if _, err := core.CompareValues(p[0], p[1]); err != nil {
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
		if _, err := core.CompareValues(l1, l2); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkKernelCompareMap64(b *testing.B) {
	m1, m2 := benchMap(64), benchMap(64)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := core.CompareValues(m1, m2); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkKernelUnifyScalarType(b *testing.B) {
	v := core.NewInteger(42)
	t := core.NewTypeLiteral(core.TInteger)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := core.Unify(v, t); !ok {
			b.Fatal("unify failed")
		}
	}
}

func BenchmarkKernelUnifyList32(b *testing.B) {
	l := benchList(32)
	t := core.NewTypedList(core.NewTypeLiteral(core.TInteger))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := core.Unify(l, t); !ok {
			b.Fatal("unify failed")
		}
	}
}

func BenchmarkKernelCanonScalar(b *testing.B) {
	vals := []core.Value{core.NewInteger(42), core.NewFloat(2.5), core.NewString("hello"), core.NewBoolean(true)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, v := range vals {
			_ = core.CanonValue(v)
		}
	}
}

func BenchmarkKernelCanonNested(b *testing.B) {
	om := core.NewOrderedMap()
	om.Set("xs", benchList(16))
	om.Set("m", benchMap(16))
	v := core.NewMap(om)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = core.CanonValue(v)
	}
}

func BenchmarkKernelOrderedMapSetGet(b *testing.B) {
	keys := make([]string, 64)
	for i := range keys {
		keys[i] = "key" + strconv.Itoa(i)
	}
	one := core.NewInteger(1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		om := core.NewOrderedMap()
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
