package vault

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Provider describes how to inject a credential into outbound
// requests for a given third-party API. Built-in presets cover the
// common patterns; operator-defined presets (`vault provider add`)
// persist in Store.CustomProviders with this same shape; a "generic"
// preset is used when an alias has no provider tag or a provider name
// neither the binary nor the store recognizes. The JSON tags are the
// store's on-disk form for custom presets (schema v7).
type Provider struct {
	Name string `json:"name"`
	// BaseURL is prepended to the incoming request path when the
	// proxy forwards the request. Must not include a trailing slash.
	BaseURL string `json:"base_url"`
	// AuthStyle selects how the secret is attached to the outbound
	// request. Recognized values:
	//   "bearer"        — Authorization: Bearer <secret>
	//   "x-api-key"     — x-api-key: <secret>
	//   "header:<name>" — <name>: <secret>
	//   "query:<name>"  — appended as ?<name>=<secret>
	//   "none"          — request is forwarded unmodified (for testing)
	AuthStyle string `json:"auth_style"`
}

// providers is the registry of built-in presets. The "generic"
// entry is the fallback when an alias has no provider tag.
var providers = map[string]Provider{
	"openai":    {Name: "openai", BaseURL: "https://api.openai.com", AuthStyle: "bearer"},
	"anthropic": {Name: "anthropic", BaseURL: "https://api.anthropic.com", AuthStyle: "x-api-key"},
	"github":    {Name: "github", BaseURL: "https://api.github.com", AuthStyle: "bearer"},
	"generic":   {Name: "generic", BaseURL: "", AuthStyle: "bearer"},
}

// LookupProvider returns the named built-in preset, or the generic
// preset when name is empty or unknown. Store-defined presets are not
// consulted — callers holding a store use LookupProviderIn.
func LookupProvider(name string) Provider {
	if p, ok := providers[name]; ok {
		return p
	}
	return providers["generic"]
}

// LookupProviderIn resolves name against the built-in presets first,
// then s's operator-defined presets, then the generic fallback.
// Built-ins winning means a store entry can never shadow — and so
// silently redirect — a compiled-in provider, even if one is smuggled
// into the file (`provider add` refuses built-in names at mint time; this
// ordering is the defence in depth behind that refusal). A nil store
// degrades to the built-in lookup.
func LookupProviderIn(s *Store, name string) Provider {
	if _, ok := providers[name]; ok || s == nil {
		return LookupProvider(name)
	}
	if p, _ := s.FindCustomProvider(name); p != nil {
		return *p
	}
	return providers["generic"]
}

// builtinProvider reports whether name is a compiled-in preset
// (including "generic"), i.e. a name `provider add`/`provider rm` must
// refuse to touch.
func builtinProvider(name string) bool {
	_, ok := providers[name]
	return ok
}

// ListProviders returns the built-in presets in stable name order.
func ListProviders() []Provider {
	names := make([]string, 0, len(providers))
	for n := range providers {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Provider, 0, len(names))
	for _, n := range names {
		out = append(out, providers[n])
	}
	return out
}

// ListProvidersIn returns the built-in presets plus s's operator-defined
// ones, in stable name order. A custom entry that (illegitimately)
// collides with a built-in name is dropped, mirroring LookupProviderIn's
// built-ins-win rule so the listing never advertises a preset the broker
// would not use. A nil store lists just the built-ins.
func ListProvidersIn(s *Store) []Provider {
	out := ListProviders()
	if s == nil {
		return out
	}
	for _, p := range s.CustomProviders {
		if !builtinProvider(p.Name) {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// validateAuthStyle checks that style is one of the injection forms
// InjectAuth understands, so `provider add` fails at mint time instead
// of every request failing at injection time. An empty style is allowed
// (InjectAuth treats it as "bearer").
func validateAuthStyle(style string) error {
	switch {
	case style == "" || style == "bearer" || style == "x-api-key" || style == "none":
		return nil
	case strings.HasPrefix(style, "header:"):
		if strings.TrimPrefix(style, "header:") == "" {
			return fmt.Errorf("auth style %q needs a header name (e.g. header:X-Api-Key)", style)
		}
		return nil
	case strings.HasPrefix(style, "query:"):
		if strings.TrimPrefix(style, "query:") == "" {
			return fmt.Errorf("auth style %q needs a parameter name (e.g. query:api_key)", style)
		}
		return nil
	}
	return fmt.Errorf("unknown auth style %q (want bearer, x-api-key, header:<name>, query:<name>, or none)", style)
}

// validateProviderBaseURL checks and canonicalizes a `provider add`
// base URL: it must parse, use the http or https scheme, and carry a
// host; a query or fragment is refused (the broker appends the caller's
// path and query, so either would be silently clobbered or spliced); a
// trailing slash is trimmed to honour Provider.BaseURL's no-trailing-
// slash contract. Returns the canonical form.
func validateProviderBaseURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid base URL %q: %v", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("base URL %q must use http:// or https://", raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("base URL %q has no host", raw)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("base URL %q must not carry a query or fragment (the broker appends the request's own)", raw)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}

// InjectAuth attaches secret to req per the provider's AuthStyle.
// Existing Authorization or provider-specific headers are
// overwritten so the upstream API never sees the capability token.
func (p Provider) InjectAuth(req *http.Request, secret string) error {
	style := p.AuthStyle
	switch {
	case style == "" || style == "bearer":
		req.Header.Set("Authorization", "Bearer "+secret)
	case style == "x-api-key":
		req.Header.Set("x-api-key", secret)
	case strings.HasPrefix(style, "header:"):
		name := strings.TrimPrefix(style, "header:")
		if name == "" {
			return fmt.Errorf("provider %q: header: auth style requires a header name", p.Name)
		}
		req.Header.Set(name, secret)
	case strings.HasPrefix(style, "query:"):
		name := strings.TrimPrefix(style, "query:")
		if name == "" {
			return fmt.Errorf("provider %q: query: auth style requires a parameter name", p.Name)
		}
		q := req.URL.Query()
		q.Set(name, secret)
		req.URL.RawQuery = q.Encode()
	case style == "none":
		// Intentionally no-op; useful for test fixtures.
	default:
		return fmt.Errorf("provider %q: unknown AuthStyle %q", p.Name, style)
	}
	return nil
}
