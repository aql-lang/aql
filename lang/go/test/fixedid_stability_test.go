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
		"Any":                        1,
		"None":                       2,
		"Scalar":                     3,
		"Scalar/String":              4,
		"Scalar/String/ProperString": 5,
		"Scalar/String/EmptyString":  6,
		"Scalar/Number":              7,
		"Scalar/Number/Integer":      8,
		"Scalar/Number/Float":        9,
		"Scalar/Number/BigInteger":   100,
		"Scalar/Number/BigDecimal":   101,
		"Scalar/Boolean":             10,
		"Node":                       11,
		"Node/List":                  12,
		"Node/List/Args":             13,
		"Node/List/FlexList":         79,
		"Node/Map":                   14,
		"Node/Map/FlexMap":           78,
		"Node/Xml":                   108,
		"Node/Xml/FlexXml":           110,
		"Ideal/Table":                15,
		"Ideal/Reach":                29,
		"Ideal/Record":               16,
		"Word":                       17,
		"Scalar/Atom":                18,
		"Type/Function":              19,
		"Word/__IN":                  20,
		"Word/__FW":                  21,
		"Word/__OP":                  22,
		"Word/__FN":                  23,
		"Type/FunctionSignature":     24,
		"Word/__RC":                  25,
		"Type/Disjunct":              26,
		"Word/__MK":                  27,
		"Word/__MV":                  28,
		"Ideal":                      48,
		// FixedID 30 retired with Ideal/Object (the bare open container);
		// class instances live under Ideal/Class. Not recycled.
		"Node/Map/Inspect":      31,
		"Ideal/Fetch":           3000,
		"Ideal/Fetch/Request":   3001,
		"Ideal/Fetch/Response":  3002,
		"Ideal/Resource":        36,
		"Ideal/Resource/Entity": 37,
		"Ideal/Options":         38,
		"Type":                  39,
		// FixedIDs 40, 41, 46 retired with Type/ScalarType,
		// Type/NodeType, Type/IdealType. Sig "type literal here" is
		// now expressed via Signature.TypeArgs[i]=true.
		"Ideal/Store":        42,
		"Ideal/Store/System": 43,
		// FixedID 44 retired with Ideal/Array (removed); not recycled.
		"Ideal/Error": 45,
		"Scalar/Path": 47,
		// Tensor family (former FixedIDs 2000-2002) moved to
		// aql:matrix-util as per-import module mints with no FixedID —
		// see MintTensorTypes and design/OPEN-WORDS.0.md.
		"Ideal/Module":           5000,
		"Ideal/ModuleExport":     5001,
		"Node/Map/KeyVal":        5002, // map-iteration entry — lang/go/native/native_keyval.go
		"Ideal/MiniLangCompiled": 5003, // compiled-minilang carrier — lang/go/modules/minilang.go
		"Ideal/Patrun":           5004, // pattern-dispatch table — lang/go/native/native_patrun.go
		"Ideal/ParseGrammar":     5005, // custom-parser builder — lang/go/modules/parse.go
		"Ideal/Model":            5006, // voxgig/model handle — lang/go/modules/model.go
		"Word/__IS":              51,
		"Word/__XI":              109, // interpolated XML literal skeleton
		"Type/Disjunct/Enum":     62,
		"Word/__PE":              63,
		"Word/__IN/__DC":         64,
		// FixedIDs 65-71 retired with the Type/Dependent subtree;
		// DepScalar values now carry their base scalar type directly
		// (typeof (Integer gte 0) → Integer) with the constraint in
		// the DepScalarInfo payload. See depscalar.go.
		"Word/__CP":      72,
		"Word/__ED":      73,
		"Never":          61,
		"Absent":         74,
		"Word/__SP":      75,
		"Word/__DM":      76,
		"Type/Negation":  77,
		"Ideal/Class":    102,
		"Ideal/Surface":  103,
		"Type/Self":      104,
		"Type/TypeParam": 105,
		"Type/GenSpec":   106,
		"Type/GenParam":  107,
		// --- Externally-registered types (Step 8 migration) ---
		// Time family — lang/go/native/native_temporal.go. Only the
		// family root and the three instant-bearing leaves are global;
		// TimeOfDay / Duration / CalendarDuration / ClockDuration / Timezone
		// (former FixedIDs 1004-1008) moved to aql:time-util as
		// per-import module mints with no FixedID — see
		// MintTemporalModuleTypes and design/OPEN-WORDS.0.md.
		"Scalar/Time":          1000,
		"Scalar/Time/Date":     1001,
		"Scalar/Time/DateTime": 1002,
		"Scalar/Time/Instant":  1003,
		"Ideal/Timeout":        4000, // timer types — lang/go/engine/native_misc.go
		"Ideal/Interval":       4001,
		"Scalar/Bytes":         1009, // byte-string leaf — lang/go/native/native_bytes.go
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
