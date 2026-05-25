// config.go loads a jsonic-formatted service list for `aql serve -c
// <file>`. The file's top level is a list of service specs; each
// spec is a map with required key "name" (the service name) and
// optional "flags" (an argv-style list forwarded to the service
// factory).
//
// Example:
//
//	[
//	  { name: registry, flags: [-r, ./mods, -p, "8080"] }
//	  { name: lsp,      flags: [-p, "9000"] }
//	]
//
// The same stdio-conflict and duplicate-name rules apply as for the
// inline `+` form, so a config can't sneak in two stdio services.

package serve

import (
	"fmt"
	"os"

	jsonic "github.com/jsonicjs/jsonic/go"
)

// loadConfig parses path and returns the segment list that the
// supervisor would otherwise have parsed from the inline `+` form.
func loadConfig(path string) ([][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	j := jsonic.Make()
	parsed, err := j.Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("config: invalid jsonic: %w", err)
	}
	list, ok := parsed.([]any)
	if !ok {
		return nil, fmt.Errorf("config: top-level must be a list of service specs")
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("config: service list is empty")
	}
	out := make([][]string, 0, len(list))
	for i, entry := range list {
		seg, err := segmentFromSpec(entry)
		if err != nil {
			return nil, fmt.Errorf("config: entry %d: %w", i, err)
		}
		out = append(out, seg)
	}
	return out, nil
}

// segmentFromSpec converts one parsed spec map into [name, flag,
// flag, ...] form for the factory.
func segmentFromSpec(entry any) ([]string, error) {
	m, ok := entry.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected a map, got %T", entry)
	}
	name, _ := m["name"].(string)
	if name == "" {
		return nil, fmt.Errorf("missing \"name\"")
	}
	seg := []string{name}
	rawFlags, ok := m["flags"]
	if !ok {
		return seg, nil
	}
	flags, ok := rawFlags.([]any)
	if !ok {
		return nil, fmt.Errorf("\"flags\" must be a list")
	}
	for j, f := range flags {
		s, ok := f.(string)
		if !ok {
			return nil, fmt.Errorf("flag %d is not a string (got %T)", j, f)
		}
		seg = append(seg, s)
	}
	return seg, nil
}
