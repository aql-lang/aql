package native

import (
	"fmt"
	"strings"

	"github.com/metsitaba/voxgig-exp/aql/internal/engine"

	udk "voxgiguniversalsdk"
)

// getSDK extracts the spec and entity name from an API map ({kind:"api", spec:..., entity:...}),
// looks up or creates the SDK instance, and returns the SDK and entity name.
func getSDK(apiMap *engine.OrderedMap, opName string, r *engine.Registry) (*udk.UniversalSDK, string, error) {
	specVal, _ := apiMap.Get("spec")

	spec := specVal.AsString()

	var entityName string
	if entityVal, ok := apiMap.Get("entity"); ok {
		entityName = entityVal.AsString()
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
func extractQuery(apiMap *engine.OrderedMap) map[string]any {
	if queryVal, ok := apiMap.Get("query"); ok && queryVal.VType.Matches(engine.TMap) {
		return valueToMap(queryVal)
	}
	return nil
}

// extractData extracts an optional data map from the API options map.
func extractData(apiMap *engine.OrderedMap) map[string]any {
	if dataVal, ok := apiMap.Get("data"); ok && dataVal.VType.Matches(engine.TMap) {
		return valueToMap(dataVal)
	}
	return nil
}

// mergeAPIOptions merges the keys of an options map into the base API map.
// For query operations (list, load, remove), options keys go into query.
// For data operations (create, update), options keys go into data.
// Returns a new map; the original is not modified.
func mergeAPIOptions(base *engine.OrderedMap, opts *engine.OrderedMap, field string) *engine.OrderedMap {
	merged := engine.NewOrderedMap()
	for _, k := range base.Keys() {
		v, _ := base.Get(k)
		merged.Set(k, v)
	}

	// Get existing field map or create a new one.
	existing := engine.NewOrderedMap()
	if v, ok := merged.Get(field); ok && v.VType.Matches(engine.TMap) {
		src := v.AsMap()
		for _, k := range src.Keys() {
			val, _ := src.Get(k)
			existing.Set(k, val)
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
