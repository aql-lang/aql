package vault

import (
	"strings"
	"testing"
)

func TestPublishRecipeEnv(t *testing.T) {
	const secret = "sek"
	cases := []struct {
		recipe string
		want   map[string]string // value "S" stands for the secret
	}{
		{"npm", map[string]string{"npm_config_//registry.npmjs.org/:_authToken": "S"}},
		{"yarn", map[string]string{"YARN_NPM_AUTH_TOKEN": "S"}},
		{"pnpm", map[string]string{
			"pnpm_config_//registry.npmjs.org/:_authToken": "S",
			"npm_config_//registry.npmjs.org/:_authToken":  "S",
		}},
		{"bun", map[string]string{"NPM_CONFIG_TOKEN": "S"}},
		{"pypi", map[string]string{"TWINE_USERNAME": "__token__", "TWINE_PASSWORD": "S"}},
		{"uv", map[string]string{"UV_PUBLISH_TOKEN": "S"}},
		{"poetry", map[string]string{"POETRY_PYPI_TOKEN_PYPI": "S"}},
		{"hatch", map[string]string{"HATCH_INDEX_USER": "__token__", "HATCH_INDEX_AUTH": "S"}},
		{"flit", map[string]string{"FLIT_USERNAME": "__token__", "FLIT_PASSWORD": "S"}},
		{"cargo", map[string]string{"CARGO_REGISTRY_TOKEN": "S"}},
		{"gem", map[string]string{"GEM_HOST_API_KEY": "S"}},
		{"hex", map[string]string{"HEX_API_KEY": "S"}},
		{"hackage", map[string]string{"HACKAGE_KEY": "S"}},
		{"swift", map[string]string{"SWIFTPM_REGISTRY_TOKEN": "S"}},
		{"cocoapods", map[string]string{"COCOAPODS_TRUNK_TOKEN": "S"}},
		{"github", map[string]string{"GH_TOKEN": "S", "GITHUB_TOKEN": "S"}},
		{"gitlab", map[string]string{"GITLAB_TOKEN": "S"}},
		{"terraform", map[string]string{"TF_TOKEN_app_terraform_io": "S"}},
	}
	for _, c := range cases {
		t.Run(c.recipe, func(t *testing.T) {
			rec, ok := lookupPublishRecipe(c.recipe)
			if !ok {
				t.Fatalf("recipe %q not found", c.recipe)
			}
			env := rec.env(secret, rec.defaultRegistry)
			for k, v := range c.want {
				want := v
				if v == "S" {
					want = secret
				}
				if env[k] != want {
					t.Errorf("%s: env[%q]=%q, want %q", c.recipe, k, env[k], want)
				}
			}
		})
	}
}

func TestComposerAuthJSON(t *testing.T) {
	rec, _ := lookupPublishRecipe("composer")
	v := rec.env("tok", rec.defaultRegistry)["COMPOSER_AUTH"]
	for _, want := range []string{"http-basic", composerDefaultRegistry, `"password":"tok"`, `"username":"token"`} {
		if !strings.Contains(v, want) {
			t.Errorf("COMPOSER_AUTH %q missing %q", v, want)
		}
	}
}

func TestRecipeAliases(t *testing.T) {
	for alias, canon := range map[string]string{
		"twine": "pypi", "pod": "cocoapods", "swiftpm": "swift", "packagist": "composer",
		"mix": "hex", "gh": "github", "glab": "gitlab", "tf": "terraform",
	} {
		r, ok := lookupPublishRecipe(alias)
		if !ok || r.name != canon {
			t.Errorf("alias %q -> (%q, ok=%v), want %q", alias, r.name, ok, canon)
		}
	}
}

func TestTerraformHostEncoding(t *testing.T) {
	rec, _ := lookupPublishRecipe("terraform")
	// dots -> "_", dashes -> "__"
	if _, ok := rec.env("tok", "my-host.example.com")["TF_TOKEN_my__host_example_com"]; !ok {
		t.Errorf("terraform host encoding wrong: %v", rec.env("tok", "my-host.example.com"))
	}
}

func TestPnpmNonDefaultRegistry(t *testing.T) {
	rec, _ := lookupPublishRecipe("pnpm")
	env := rec.env("tok", "npm.pkg.github.com")
	if env["pnpm_config_//npm.pkg.github.com/:_authToken"] != "tok" {
		t.Errorf("pnpm non-default auth token wrong: %v", env)
	}
	if env["pnpm_config_registry"] != "https://npm.pkg.github.com/" {
		t.Errorf("pnpm non-default registry wrong: %v", env)
	}
}
