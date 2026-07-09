package vault

import (
	"encoding/json"
	"sort"
	"strings"
)

// publishRecipe presents a single vault secret in the exact environment
// form a tool reads its credential from, so
// `aql vault exec --for=<name> <alias> -- <cmd>` is a one-liner and the
// token never touches disk or argv. Every recipe here is pure environment
// injection (no temp files, a single secret). Tools that read their
// credential another way (maven, gradle, conan, docker/helm, nuget's
// --api-key, pub.dev, jsr) are intentionally not covered here — plain
// `vault exec` feeds them via a flag/stdin/config-file; see CLI.md
// "Publishers without a recipe".
type publishRecipe struct {
	name string
	// defaultRegistry is the host used by registry-scoped recipes (npm,
	// pnpm, composer, terraform) when --registry is not given; "" for
	// recipes that aren't registry-host-scoped.
	defaultRegistry string
	// env builds the environment overrides that expose secret to the tool.
	// registry is the resolved registry host (recipe default or --registry).
	env func(secret, registry string) map[string]string
}

const (
	npmDefaultRegistry      = "registry.npmjs.org"
	composerDefaultRegistry = "repo.packagist.com"
	tfcDefaultRegistry      = "app.terraform.io"
)

// npmStyleAuth builds the file-free per-registry auth-token config that
// npm-family tools read from <prefix>_config_* env vars. npm treats any
// <prefix>_config_<key> env var as a config setting, so the per-registry
// token (and, for a non-default registry, the default registry itself) can
// be supplied entirely from the environment with no .npmrc.
func npmStyleAuth(prefix, secret, registry string) map[string]string {
	m := map[string]string{prefix + "_config_//" + registry + "/:_authToken": secret}
	if registry != npmDefaultRegistry {
		m[prefix+"_config_registry"] = "https://" + registry + "/"
	}
	return m
}

var publishRecipes = map[string]publishRecipe{
	// --- Node / JavaScript ---
	"npm": {name: "npm", defaultRegistry: npmDefaultRegistry, env: func(s, reg string) map[string]string {
		return npmStyleAuth("npm", s, reg)
	}},
	"yarn": {name: "yarn", env: func(s, _ string) map[string]string {
		// Yarn Berry reads YARN_NPM_AUTH_TOKEN for all npm registries.
		return map[string]string{"YARN_NPM_AUTH_TOKEN": s}
	}},
	"pnpm": {name: "pnpm", defaultRegistry: npmDefaultRegistry, env: func(s, reg string) map[string]string {
		// pnpm 11.6+ reads pnpm_config_* env vars file-free; also set the
		// npm_config_* form so older pnpm and npm-compatible paths work.
		m := npmStyleAuth("pnpm", s, reg)
		for k, v := range npmStyleAuth("npm", s, reg) {
			m[k] = v
		}
		return m
	}},
	"bun": {name: "bun", env: func(s, _ string) map[string]string {
		return map[string]string{"NPM_CONFIG_TOKEN": s}
	}},

	// --- Python ---
	"pypi": {name: "pypi", env: func(s, _ string) map[string]string {
		// twine/PyPI: an API token authenticates as the literal user
		// "__token__" with the token as the password.
		return map[string]string{"TWINE_USERNAME": "__token__", "TWINE_PASSWORD": s}
	}},
	"uv": {name: "uv", env: func(s, _ string) map[string]string {
		return map[string]string{"UV_PUBLISH_TOKEN": s} // implies username __token__
	}},
	"poetry": {name: "poetry", env: func(s, _ string) map[string]string {
		return map[string]string{"POETRY_PYPI_TOKEN_PYPI": s}
	}},
	"hatch": {name: "hatch", env: func(s, _ string) map[string]string {
		return map[string]string{"HATCH_INDEX_USER": "__token__", "HATCH_INDEX_AUTH": s}
	}},
	"flit": {name: "flit", env: func(s, _ string) map[string]string {
		return map[string]string{"FLIT_USERNAME": "__token__", "FLIT_PASSWORD": s}
	}},

	// --- Rust / Ruby / Elixir ---
	"cargo": {name: "cargo", env: func(s, _ string) map[string]string {
		return map[string]string{"CARGO_REGISTRY_TOKEN": s}
	}},
	"gem": {name: "gem", env: func(s, _ string) map[string]string {
		return map[string]string{"GEM_HOST_API_KEY": s}
	}},
	"hex": {name: "hex", env: func(s, _ string) map[string]string {
		return map[string]string{"HEX_API_KEY": s}
	}},

	// --- Haskell ---
	"hackage": {name: "hackage", env: func(s, _ string) map[string]string {
		// Hackage API token (revocable, replaces the account password),
		// consumed as `cabal upload --token $HACKAGE_KEY` — HACKAGE_KEY is
		// the env var Haskell CI setups conventionally read.
		return map[string]string{"HACKAGE_KEY": s}
	}},

	// --- Swift / iOS ---
	"swift": {name: "swift", env: func(s, _ string) map[string]string {
		return map[string]string{"SWIFTPM_REGISTRY_TOKEN": s}
	}},
	"cocoapods": {name: "cocoapods", env: func(s, _ string) map[string]string {
		return map[string]string{"COCOAPODS_TRUNK_TOKEN": s}
	}},

	// --- PHP ---
	"composer": {name: "composer", defaultRegistry: composerDefaultRegistry, env: func(s, reg string) map[string]string {
		// Composer reads http-basic credentials from the COMPOSER_AUTH JSON
		// env var; a token authenticates as user "token".
		doc := map[string]map[string]map[string]string{
			"http-basic": {reg: {"username": "token", "password": s}},
		}
		b, _ := json.Marshal(doc)
		return map[string]string{"COMPOSER_AUTH": string(b)}
	}},

	// --- Single-token CLI tools (not package publishers) ---
	"github": {name: "github", env: func(s, _ string) map[string]string {
		return map[string]string{"GH_TOKEN": s, "GITHUB_TOKEN": s}
	}},
	"gitlab": {name: "gitlab", env: func(s, _ string) map[string]string {
		return map[string]string{"GITLAB_TOKEN": s, "GLAB_TOKEN": s}
	}},
	"terraform": {name: "terraform", defaultRegistry: tfcDefaultRegistry, env: func(s, reg string) map[string]string {
		// Terraform reads TF_TOKEN_<host>, with "." -> "_" and "-" -> "__".
		host := strings.ReplaceAll(reg, "-", "__")
		host = strings.ReplaceAll(host, ".", "_")
		return map[string]string{"TF_TOKEN_" + host: s}
	}},
}

// recipeAliases maps alternate spellings to canonical recipe names.
var recipeAliases = map[string]string{
	"twine":     "pypi",
	"pod":       "cocoapods",
	"swiftpm":   "swift",
	"packagist": "composer",
	"mix":       "hex",
	"gh":        "github",
	"glab":      "gitlab",
	"tf":        "terraform",
}

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
