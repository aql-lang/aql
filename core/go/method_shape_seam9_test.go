package core

import "testing"

func TestW9NoteMethodShapeInactive(t *testing.T) {
	r := newTestRegistry(t)
	// The check is inactive on a fresh registry, so the annotation is a no-op.
	r.Check.NoteMethodShape(NewCarrier(TAny), NewFunction(FnDefInfo{Name: "m", Registry: r}))
	if r.Check.MethodShapes != nil {
		t.Error("an inactive check must not record a method shape")
	}
}
