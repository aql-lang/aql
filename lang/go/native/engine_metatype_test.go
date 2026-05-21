package native

import (
	"testing"
)

// --- Metatype tests ---

func TestMetatypeFor(t *testing.T) {
	tests := []struct {
		name string
		typ  *Type
		want *Type
	}{
		{"String → ScalarType", TString, TScalarType},
		{"Number → ScalarType", TNumber, TScalarType},
		{"Integer → ScalarType", TInteger, TScalarType},
		{"Decimal → ScalarType", TDecimal, TScalarType},
		{"Boolean → ScalarType", TBoolean, TScalarType},
		{"List → NodeType", TList, TNodeType},
		{"Map → NodeType", TMap, TNodeType},
		{"Scalar → Type", TScalar, TType},
		{"Node → Type", TNode, TType},
		{"Any → Type", TAny, TType},
		{"None → Type", TNone, TType},
		{"Object → IdealType", TObject, TIdealType},
		{"Table → IdealType", TTable, TIdealType},
		{"Record → IdealType", TRecord, TIdealType},
		{"Resource → IdealType", TResource, TIdealType},
		{"Options → IdealType", TOptions, TIdealType},
		{"Atom → ScalarType", TAtom, TScalarType},
		{"Type → Type", TType, TType},
		{"ScalarType → Type", TScalarType, TType},
		{"NodeType → Type", TNodeType, TType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MetatypeFor(tt.typ)
			if !got.Equal(tt.want) {
				t.Errorf("MetatypeFor(%s) = %s, want %s", tt.typ, got, tt.want)
			}
		})
	}
}

func TestIsMetaType(t *testing.T) {
	tests := []struct {
		name string
		typ  *Type
		want bool
	}{
		{"Type", TType, true},
		{"ScalarType", TScalarType, true},
		{"NodeType", TNodeType, true},
		{"String", TString, false},
		{"Integer", TInteger, false},
		{"List", TList, false},
		{"Map", TMap, false},
		{"Any", TAny, false},
		{"Scalar", TScalar, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsMetaType(tt.typ)
			if got != tt.want {
				t.Errorf("IsMetaType(%s) = %v, want %v", tt.typ, got, tt.want)
			}
		})
	}
}
