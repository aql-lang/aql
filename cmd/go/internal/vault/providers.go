package vault

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// Provider describes how to inject a credential into outbound
// requests for a given third-party API. Built-in presets cover the
// common patterns; a "generic" preset is used when an alias has no
// provider tag or a provider name the binary does not recognize.
type Provider struct {
	Name string
	// BaseURL is prepended to the incoming request path when the
	// proxy forwards the request. Must not include a trailing slash.
	BaseURL string
	// AuthStyle selects how the secret is attached to the outbound
	// request. Recognized values:
	//   "bearer"        — Authorization: Bearer <secret>
	//   "x-api-key"     — x-api-key: <secret>
	//   "header:<name>" — <name>: <secret>
	//   "query:<name>"  — appended as ?<name>=<secret>
	//   "none"          — request is forwarded unmodified (for testing)
	AuthStyle string
}

// providers is the registry of built-in presets. The "generic"
// entry is the fallback when an alias has no provider tag.
var providers = map[string]Provider{
	"openai":    {Name: "openai", BaseURL: "https://api.openai.com", AuthStyle: "bearer"},
	"anthropic": {Name: "anthropic", BaseURL: "https://api.anthropic.com", AuthStyle: "x-api-key"},
	"github":    {Name: "github", BaseURL: "https://api.github.com", AuthStyle: "bearer"},
	"generic":   {Name: "generic", BaseURL: "", AuthStyle: "bearer"},
}

// LookupProvider returns the named provider preset, or the generic
// preset when name is empty or unknown.
func LookupProvider(name string) Provider {
	if p, ok := providers[name]; ok {
		return p
	}
	return providers["generic"]
}

// ListProviders returns presets in stable name order.
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
