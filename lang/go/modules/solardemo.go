package modules

import (
	"github.com/aql-lang/aql/lang/go/native"
)

// BuildSolarDemoModule creates the "aql:solardemo" native module.
// It provides planet and moon data as lists of maps for querying.
func BuildSolarDemoModule(parent *native.Registry) (native.ModuleDesc, error) {
	planets := []struct {
		id, name, kind string
		diameter       int64
		order          int64
	}{
		{"planet01", "Mercury", "terrestrial", 4879, 1},
		{"planet02", "Venus", "terrestrial", 12104, 2},
		{"planet03", "Earth", "terrestrial", 12756, 3},
		{"planet04", "Mars", "terrestrial", 6792, 4},
		{"planet05", "Jupiter", "gas giant", 142984, 5},
		{"planet06", "Saturn", "gas giant", 120536, 6},
		{"planet07", "Uranus", "ice giant", 51118, 7},
		{"planet08", "Neptune", "ice giant", 49528, 8},
	}

	moons := []struct {
		id, name, planetID string
		diameter           int64
	}{
		{"moon01", "Luna", "planet03", 3474},
		{"moon02", "Phobos", "planet04", 22},
		{"moon03", "Deimos", "planet04", 12},
		{"moon04", "Io", "planet05", 3643},
		{"moon05", "Europa", "planet05", 3122},
		{"moon06", "Ganymede", "planet05", 5268},
		{"moon07", "Callisto", "planet05", 4821},
		{"moon08", "Titan", "planet06", 5150},
		{"moon09", "Enceladus", "planet06", 504},
		{"moon10", "Mimas", "planet06", 396},
		{"moon11", "Titania", "planet07", 1578},
		{"moon12", "Oberon", "planet07", 1522},
		{"moon13", "Triton", "planet08", 2707},
	}

	planetList := make([]native.Value, len(planets))
	for i, p := range planets {
		m := native.NewOrderedMap()
		m.Set("id", native.NewString(p.id))
		m.Set("name", native.NewString(p.name))
		m.Set("kind", native.NewString(p.kind))
		m.Set("diameter", native.NewInteger(p.diameter))
		m.Set("order", native.NewInteger(p.order))
		planetList[i] = native.NewMap(m)
	}

	moonList := make([]native.Value, len(moons))
	for i, mn := range moons {
		m := native.NewOrderedMap()
		m.Set("id", native.NewString(mn.id))
		m.Set("name", native.NewString(mn.name))
		m.Set("planet_id", native.NewString(mn.planetID))
		m.Set("diameter", native.NewInteger(mn.diameter))
		moonList[i] = native.NewMap(m)
	}

	exports := native.NewOrderedMap()
	exports.Set("planets", native.NewList(planetList))
	exports.Set("moons", native.NewList(moonList))

	modID := parent.Modules.NextID()
	desc := native.ModuleDesc{
		ID:      modID,
		Exports: map[string]*native.OrderedMap{"solardemo": exports},
	}
	return desc, nil
}
