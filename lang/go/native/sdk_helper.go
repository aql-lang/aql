package native

import (
	"fmt"
	"strings"

	"github.com/aql-lang/aql/lang/go/engine"

	udk "voxgiguniversalsdk"
)

// getSDK extracts the spec and entity name from an API map ({kind:"api", spec:..., entity:...}),
// looks up or creates the SDK instance, and returns the SDK and entity name.
func getSDK(apiMap engine.ReadMap, opName string, r *engine.Registry) (*udk.UniversalSDK, string, error) {
	specVal, ok := apiMap.Get("spec")
	if !ok {
		return nil, "", fmt.Errorf("%s: missing required \"spec\" field", opName)
	}

	spec, err := engine.AsString(specVal)
	if err != nil {
		return nil, "", fmt.Errorf("%s: spec: %w", opName, err)
	}

	var entityName string
	if entityVal, ok := apiMap.Get("entity"); ok {
		entityName, err = engine.AsString(entityVal)
		if err != nil {
			return nil, "", fmt.Errorf("%s: entity: %w", opName, err)
		}
	}

	// Strip .json extension if present.
	spec = strings.TrimSuffix(spec, ".json")

	// Get or create SDK.
	var sdkInst *udk.UniversalSDK
	if cached, ok := r.SDKCache[spec]; ok {
		sdkInst, _ = cached.(*udk.UniversalSDK)
	}
	if sdkInst == nil {
		mgr, ok := r.Manager.(*udk.UniversalManager)
		if !ok || mgr == nil {
			return nil, "", fmt.Errorf("%s: no manager configured", opName)
		}
		sdkInst = mgr.Make(spec)
		r.SDKCache[spec] = sdkInst
	}

	return sdkInst, entityName, nil
}

// entityToAPIMap converts an Object/Resource/Entity instance into the
// OrderedMap that getSDK expects ({kind:..., spec:..., entity:...}).
func entityToAPIMap(v engine.Value) *engine.OrderedMap {
	m := engine.NewOrderedMap()
	if v.Data == nil {
		return m
	}
	inst, _ := engine.AsObjectInstance(v)
	if kind, ok := inst.GetField("kind"); ok {
		m.Set("kind", kind)
	}
	if spec, ok := inst.GetField("spec"); ok {
		m.Set("spec", spec)
	}
	if entity, ok := inst.GetField("entity"); ok {
		m.Set("entity", entity)
	}
	return m
}

// entityToAPIMapWithOpts converts an Entity instance into an API map and
// merges an options map into the given field (query or data).
func entityToAPIMapWithOpts(v engine.Value, opts engine.ReadMap, field string) *engine.OrderedMap {
	m := entityToAPIMap(v)
	return mergeAPIOptions(m, opts, field)
}

// convertResultList converts a []any result from the SDK into an AQL list of maps.
func convertResultList(items []any, opName string) ([]engine.Value, error) {
	rows := make([]engine.Value, 0, len(items))
	for _, item := range items {
		if ent, ok := item.(udk.Entity); ok {
			item = ent.Data()
		}
		v, err := anyToValue(item)
		if err != nil {
			return nil, fmt.Errorf("%s: converting result: %w", opName, err)
		}
		rows = append(rows, v)
	}
	return rows, nil
}

// convertResultItem converts a single any result from the SDK into an AQL value.
func convertResultItem(item any, opName string) (engine.Value, error) {
	if ent, ok := item.(udk.Entity); ok {
		item = ent.Data()
	}
	v, err := anyToValue(item)
	if err != nil {
		return engine.Value{}, fmt.Errorf("%s: converting result: %w", opName, err)
	}
	return v, nil
}

// extractQuery extracts an optional query map from the API options map.
func extractQuery(apiMap engine.ReadMap) map[string]any {
	if queryVal, ok := apiMap.Get("query"); ok && queryVal.VType.Matches(engine.TMap) {
		return valueToMap(queryVal)
	}
	return nil
}

// extractData extracts an optional data map from the API options map.
func extractData(apiMap engine.ReadMap) map[string]any {
	if dataVal, ok := apiMap.Get("data"); ok && dataVal.VType.Matches(engine.TMap) {
		return valueToMap(dataVal)
	}
	return nil
}

// mergeAPIOptions merges the keys of an options map into the base API map.
// For query operations (list, load, remove), options keys go into query.
// For data operations (create, update), options keys go into data.
// Returns a new map; the original is not modified.
func mergeAPIOptions(base engine.ReadMap, opts engine.ReadMap, field string) *engine.OrderedMap {
	merged := engine.NewOrderedMap()
	for _, k := range base.Keys() {
		v, _ := base.Get(k)
		merged.Set(k, v)
	}

	// Get existing field map or create a new one.
	existing := engine.NewOrderedMap()
	if v, ok := merged.Get(field); ok && v.VType.Matches(engine.TMap) {
		if src, _ := engine.AsMap(v); src != nil {
			for _, k := range src.Keys() {
				val, _ := src.Get(k)
				existing.Set(k, val)
			}
		}
	}

	// Merge opts into the field map.
	for _, k := range opts.Keys() {
		v, _ := opts.Get(k)
		existing.Set(k, v)
	}

	merged.Set(field, engine.NewMap(existing))
	return merged
}
