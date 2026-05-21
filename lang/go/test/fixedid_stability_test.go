package test

import (
	"testing"

	"github.com/aql-lang/aql/eng/go"
	// Side-effect imports: trigger the init() / var initialisers that
	// register externally-owned types into eng.Builtin so the snapshot
	// below sees the full set.
	_ "github.com/aql-lang/aql/lang/go/modules"
	_ "github.com/aql-lang/aql/lang/go/native"
)

// TestFixedIDStability is the regression gate for type-identity
// FixedIDs. FixedIDs are baked into serialised Value IDs (see
// eng/go/typetable.go::formatFixedID), so changing an existing
// type's FixedID can break round-tripping for any caller that
// persists or transports a Value ID.
//
// Every known *Type with a non-zero FixedID — both kernel
// builtins (declared in eng/go/typetable.go::builtinDecls) and
// externally-registered types (Fetch in lang/go/native, the
// Tensor/Matrix/Vector kinds in lang/go/modules, Timeout/Interval
// and the Time family in lang/go/native) — is checked against the
// snapshot below. A
// failure here means EITHER a stable ID has drifted OR a new
// type was added without registering it in the snapshot.
//
// To add a new entry: register it via RegisterExternalBuiltin
// with a FixedID from the documented per-module allocation
// range, then add the path → FixedID mapping below.
func TestFixedIDStability(t *testing.T) {
	expected := map[string]int{
		// --- Kernel builtins (eng/go/typetable.go::builtinDecls) ---
		"Any":                         1,
		"None":                        2,
		"Scalar":                      3,
		"Scalar/String":               4,
		"Scalar/String/ProperString":  5,
		"Scalar/String/EmptyString":   6,
		"Scalar/Number":               7,
		"Scalar/Number/Integer":       8,
		"Scalar/Number/Decimal":       9,
		"Scalar/Boolean":              10,
		"Node":                        11,
		"Node/List":                   12,
		"Node/List/Args":              13,
		"Node/Map":                    14,
		"Object/Table":                15,
		"Object/Record":               16,
		"Word":                        17,
		"Scalar/Atom":                 18,
		"Type/Function":               19,
		"Word/__IN":                   20,
		"Word/__FW":                   21,
		"Word/__OP":                   22,
		"Word/__FN":                   23,
		"Type/FunctionSignature":      24,
		"Word/__RC":                   25,
		"Type/Disjunct":               26,
		"Word/__MK":                   27,
		"Word/__MV":                   28,
		"Word/__MD":                   29,
		"Object":                      30,
		"Node/Map/Inspect":            31,
		"Object/Fetch":                3000,
		"Object/Fetch/Request":        3001,
		"Object/Fetch/Response":       3002,
		"Object/Resource":             36,
		"Object/Resource/Entity":      37,
		"Node/Map/Options":            38,
		"Type":                        39,
		"Type/ScalarType":             40,
		"Type/NodeType":               41,
		"Object/Store":                42,
		"Object/Store/System":         43,
		"Object/Array":                44,
		"Object/Error":                45,
		"Type/ObjectType":             46,
		"Scalar/Path":                 47,
		"Scalar/Number/Tensor":        2001, // matrix module range (2000-2999)
		"Scalar/Number/Tensor/Matrix": 2000, // historical Matrix FixedID, kept
		"Scalar/Number/Tensor/Vector": 2002,
		"Word/__IS":                   51,
		"Type/Disjunct/Enum":          62,
		"Word/__PE":                   63,
		"Word/__IN/__DC":              64,
		"Type/Dependent":              65,
		"Type/Dependent/DepInteger":   66,
		"Type/Dependent/DepDecimal":   67,
		"Type/Dependent/DepNumber":    68,
		"Type/Dependent/DepString":    69,
		"Type/Dependent/DepBoolean":   70,
		"Type/Dependent/DepAtom":      71,
		"Word/__CP":                   72,
		"Word/__ED":                   73,
		"Never":                       61,
		// --- Externally-registered types (Step 8 migration) ---
		"Scalar/Time":                      1000, // time family — lang/go/engine/native_temporal.go
		"Scalar/Time/Date":                 1001,
		"Scalar/Time/DateTime":             1002,
		"Scalar/Time/Instant":              1003,
		"Scalar/Time/TimeOfDay":            1004,
		"Scalar/Time/Duration":             1005,
		"Scalar/Time/Duration/CalDuration": 1006,
		"Scalar/Time/Duration/ClkDuration": 1007,
		"Scalar/Time/Timezone":             1008,
		"Object/Timeout":                   4000, // timer types — lang/go/engine/native_misc.go
		"Object/Interval":                  4001,
	}

	for path, want := range expected {
		def := eng.Builtin.Lookup(path)
		if def == nil {
			t.Errorf("Builtin.Lookup(%q) returned nil — type not registered", path)
			continue
		}
		if def.FixedID != want {
			t.Errorf("FixedID drift: %s has FixedID=%d, snapshot says %d", path, def.FixedID, want)
		}
	}
}
