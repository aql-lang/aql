package policy

import _ "embed"

//go:embed profiles/full.jsonic
var builtinFull []byte

//go:embed profiles/trusted.jsonic
var builtinTrusted []byte

//go:embed profiles/sandbox.jsonic
var builtinSandbox []byte

//go:embed profiles/compute.jsonic
var builtinCompute []byte

//go:embed profiles/read-only.jsonic
var builtinReadOnly []byte

//go:embed profiles/client.jsonic
var builtinClient []byte

// builtinProfiles maps short profile names to their embedded jsonic
// source. Keep alphabetised within capability tiers for readability.
var builtinProfiles = map[string][]byte{
	"full":      builtinFull,
	"trusted":   builtinTrusted,
	"sandbox":   builtinSandbox,
	"compute":   builtinCompute,
	"read-only": builtinReadOnly,
	"client":    builtinClient,
}

// BuiltinNames returns the list of built-in profile names. Sorted
// for stable CLI output.
func BuiltinNames() []string {
	out := make([]string, 0, len(builtinProfiles))
	for name := range builtinProfiles {
		out = append(out, name)
	}
	// Sort by tier of trust (full > trusted > client > read-only >
	// sandbox > compute) so list output reads from most-permissive
	// to least.
	order := map[string]int{
		"full":      0,
		"trusted":   1,
		"client":    2,
		"read-only": 3,
		"sandbox":   4,
		"compute":   5,
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && order[out[j-1]] > order[out[j]]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// builtinProfileData returns the embedded jsonic source for a named
// built-in profile. ok is false if name is not a built-in.
func builtinProfileData(name string) (data []byte, ok bool) {
	data, ok = builtinProfiles[name]
	return data, ok
}
