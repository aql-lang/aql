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
// ordering is the defence in depth behind that refusal). A store is only
// consulted for a name that could have been legitimately minted
// (validCustomProviderName) — critically, this makes the empty name (the
// tag every provider-untagged alias carries) resolve to the URL-less
// generic preset and be refused, rather than being captured by a
// smuggled {"name":""} entry that would then broker every untagged
// alias to an attacker host. A nil store degrades to the built-in
// lookup.
func LookupProviderIn(s *Store, name string) Provider {
	if s == nil || !validCustomProviderName(name) {
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

// validCustomProviderName reports whether name is one a custom preset
// may legitimately carry: a single alias segment (so never "", never a
// namespace-colon name) that does not collide with a built-in. Both the
// resolver and the listing gate on it, so a store entry with any other
// name (only reachable by tampering — the CLI enforces the same rule at
// mint time) is inert rather than load-bearing.
func validCustomProviderName(name string) bool {
	return validAliasSegment(name) && !builtinProvider(name)
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
		if validCustomProviderName(p.Name) {
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
		name := strings.TrimPrefix(style, "header:")
		if name == "" {
			return fmt.Errorf("auth style %q needs a header name (e.g. header:X-Api-Key)", style)
		}
		// Validate the header field name now so a preset can never mint
		// with a name net/http rejects at write time (which would fail
		// every brokered request as an opaque 502, the very "fail one
		// request at a time" mode mint-time validation exists to prevent).
		if !validHeaderFieldName(name) {
			return fmt.Errorf("auth style %q: %q is not a valid HTTP header name (letters, digits, and !#$%%&'*+-.^_`|~)", style, name)
		}
		return nil
	case strings.HasPrefix(style, "query:"):
		if strings.TrimPrefix(style, "query:") == "" {
			return fmt.Errorf("auth style %q needs a parameter name (e.g. query:api_key)", style)
		}
		// A query parameter name needs no charset guard: InjectAuth writes
		// it through url.Values.Encode, which percent-escapes anything odd.
		return nil
	}
	return fmt.Errorf("unknown auth style %q (want bearer, x-api-key, header:<name>, query:<name>, or none)", style)
}

// validHeaderFieldName reports whether name is a non-empty RFC 7230
// token (the character set net/http requires of a header field name);
// it is the mint-time guard behind the `header:<name>` auth style.
func validHeaderFieldName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("!#$%&'*+-.^_`|~", r):
		default:
			return false
		}
	}
	return true
}

// validateProviderBaseURL checks and canonicalizes a `provider add`
// base URL: it must parse, use the http or https scheme, and carry a
// host; embedded userinfo (user:pass@host) is refused — it would be a
// second, unsealed credential printed by `provider add`/`providers`,
// stored in cleartext, shown to the model in the MCP tool description,
// and (for non-bearer styles) injected upstream as Basic auth by
// net/http; a query or fragment is refused, including a bare trailing
// `?` (ForceQuery), since the broker appends the caller's own path and
// query and either would be clobbered or misroute the request; a
// trailing slash is trimmed — from both Path and RawPath so a
// percent-escape like %2F is preserved rather than silently decoded to a
// path separator — to honour Provider.BaseURL's no-trailing-slash
// contract. Returns the canonical form.
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
	if u.User != nil {
		return "", fmt.Errorf("base URL must not embed credentials (user:pass@host); attach the secret via the alias, not the provider URL")
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", fmt.Errorf("base URL %q must not carry a query or fragment (the broker appends the request's own)", raw)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = strings.TrimRight(u.RawPath, "/")
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
