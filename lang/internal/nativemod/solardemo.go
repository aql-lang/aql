package nativemod

import (
	"github.com/metsitaba/voxgig-exp/lang/engine"
)

// BuildSolarDemoModule creates the "aql:solardemo" native module.
// It provides planet and moon data as lists of maps for querying.
func BuildSolarDemoModule(parent *engine.Registry) (engine.ModuleDesc, error) {
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

	planetList := make([]engine.Value, len(planets))
	for i, p := range planets {
		m := engine.NewOrderedMap()
		m.Set("id", engine.NewString(p.id))
		m.Set("name", engine.NewString(p.name))
		m.Set("kind", engine.NewString(p.kind))
		m.Set("diameter", engine.NewInteger(p.diameter))
		m.Set("order", engine.NewInteger(p.order))
		planetList[i] = engine.NewMap(m)
	}

	moonList := make([]engine.Value, len(moons))
	for i, mn := range moons {
		m := engine.NewOrderedMap()
		m.Set("id", engine.NewString(mn.id))
		m.Set("name", engine.NewString(mn.name))
		m.Set("planet_id", engine.NewString(mn.planetID))
		m.Set("diameter", engine.NewInteger(mn.diameter))
		moonList[i] = engine.NewMap(m)
	}

	exports := engine.NewOrderedMap()
	exports.Set("planets", engine.NewList(planetList))
	exports.Set("moons", engine.NewList(moonList))

	modID := parent.NextModuleID()
	desc := engine.ModuleDesc{
		ID:      modID,
		Exports: map[string]*engine.OrderedMap{"solardemo": exports},
	}
	return desc, nil
}
