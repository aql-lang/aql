package vault

import "sort"

// publishRecipe presents a single vault secret in the exact environment
// form a package publisher reads its credential from, so
// `aql vault exec --for=<name> <alias> -- <publish cmd>` is a one-liner
// and the token never touches disk or argv. All first-cut recipes are
// pure environment injection (no temp files); file-only publishers
// (docker, maven) are intentionally not covered yet.
type publishRecipe struct {
	name string
	// defaultRegistry is the host used by registry-scoped recipes (npm)
	// when --registry is not given; "" for recipes that aren't
	// registry-host-scoped.
	defaultRegistry string
	// env builds the environment overrides that expose secret to the
	// publisher. registry is the resolved registry host (recipe default
	// or the --registry override).
	env func(secret, registry string) map[string]string
}

const npmDefaultRegistry = "registry.npmjs.org"

var publishRecipes = map[string]publishRecipe{
	"npm": {
		name:            "npm",
		defaultRegistry: npmDefaultRegistry,
		// npm treats any npm_config_<key> env var as a config setting, so
		// the per-registry auth token can be supplied entirely from the
		// environment — no ~/.npmrc needed. For a non-default registry
		// (e.g. GitHub Packages) also point npm's default registry at it.
		env: func(secret, registry string) map[string]string {
			m := map[string]string{"npm_config_//" + registry + "/:_authToken": secret}
			if registry != npmDefaultRegistry {
				m["npm_config_registry"] = "https://" + registry + "/"
			}
			return m
		},
	},
	"cargo": {name: "cargo", env: func(s, _ string) map[string]string {
		return map[string]string{"CARGO_REGISTRY_TOKEN": s}
	}},
	"gem": {name: "gem", env: func(s, _ string) map[string]string {
		return map[string]string{"GEM_HOST_API_KEY": s}
	}},
	"pypi": {name: "pypi", env: func(s, _ string) map[string]string {
		// twine/PyPI: an API token authenticates as the literal user
		// "__token__" with the token as the password.
		return map[string]string{"TWINE_USERNAME": "__token__", "TWINE_PASSWORD": s}
	}},
	"uv": {name: "uv", env: func(s, _ string) map[string]string {
		return map[string]string{"UV_PUBLISH_TOKEN": s}
	}},
}

// recipeAliases maps alternate spellings to canonical recipe names.
var recipeAliases = map[string]string{"twine": "pypi"}

// lookupPublishRecipe resolves a recipe by name or alias.
func lookupPublishRecipe(name string) (publishRecipe, bool) {
	if canon, ok := recipeAliases[name]; ok {
		name = canon
	}
	r, ok := publishRecipes[name]
	return r, ok
}

// publishRecipeNames returns the canonical recipe names in stable order
// for usage and error text.
func publishRecipeNames() []string {
	out := make([]string, 0, len(publishRecipes))
	for n := range publishRecipes {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
