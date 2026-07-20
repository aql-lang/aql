package eng

import "testing"

// W9 module_export_growth.go coverage: the ledger register/note/poison guard
// arms and moduleExportAbsenceStable's operand-shape declines. All pure.

func TestW9ModuleExportGrowthGuards(t *testing.T) {
	r := newTestRegistry(t)
	fields := NewOrderedMap()

	// Note / Poison before any table exists → t==nil arms.
	NoteModuleExportAdd(r, fields, "k")
	PoisonModuleExportGrowth(r, fields)

	// Register guards: nil fields, and a nil registry (table creation fails).
	RegisterModuleExportGrowth(r, nil)
	RegisterModuleExportGrowth(nil, fields)

	// Register a DIFFERENT map so the table now exists, then Note/Poison an
	// UNregistered map → ledger==nil arms.
	other := NewOrderedMap()
	RegisterModuleExportGrowth(r, other)
	NoteModuleExportAdd(r, fields, "k")
	PoisonModuleExportGrowth(r, fields)

	// The unrelated (registered) map's absence is stable for an unnoted key.
	if !moduleExportAbsenceStable(r, absenceArgsW9(other, "missing")) {
		t.Error("an unpoisoned, unnoted key should fold to a stable absence")
	}
	// After noting the key, the fold declines.
	NoteModuleExportAdd(r, other, "seen")
	if moduleExportAbsenceStable(r, absenceArgsW9(other, "seen")) {
		t.Error("a noted key must decline the absence fold")
	}

	// Poisoning the registered ledger declines EVERY absence fold.
	PoisonModuleExportGrowth(r, other)
	if moduleExportAbsenceStable(r, absenceArgsW9(other, "unnoted")) {
		t.Error("a poisoned ledger must decline every absence fold")
	}
}

// exportCarrierW9 is a test ExportFieldsCarrier body for wrapping in an
// ExtensionPayload behind a synthetic Ideal/ModuleExport value.
type exportCarrierW9 struct{ m *OrderedMap }

func (e exportCarrierW9) ExportFields() *OrderedMap { return e.m }

// absenceArgsW9 builds the [moduleExportValue, keyAtom] operand pair.
func absenceArgsW9(fields *OrderedMap, key string) []Value {
	mt := &Type{tmeta: &typeMeta{Name: "Ideal/ModuleExport"}}
	mod := Value{Parent: mt, Data: ExtensionPayload{Body: exportCarrierW9{m: fields}}}
	return []Value{mod, NewAtom(key)}
}

func TestW9ModuleExportAbsenceStableDeclines(t *testing.T) {
	r := newTestRegistry(t)

	// A module-family value whose payload is NOT an ExportFieldsCarrier → decline.
	mt := &Type{tmeta: &typeMeta{Name: "Ideal/Module"}}
	opaque := Value{Parent: mt, Data: ExtensionPayload{Body: 42}}
	if moduleExportAbsenceStable(r, []Value{opaque, NewAtom("k")}) {
		t.Error("an opaque module descriptor must decline")
	}

	// Two key candidates → not the 2-operand get shape.
	if moduleExportAbsenceStable(r, []Value{NewAtom("a"), NewAtom("b")}) {
		t.Error("two key operands must decline")
	}

	// A String key with no receiver → fields nil → decline (exercises the
	// AsConcreteString branch and the fields==nil guard).
	if moduleExportAbsenceStable(r, []Value{NewString("k")}) {
		t.Error("a lone string key must decline")
	}

	// An operand that is neither module-family nor a concrete atom/string.
	if moduleExportAbsenceStable(r, []Value{NewInteger(1)}) {
		t.Error("a non-key non-module operand must decline")
	}
}
