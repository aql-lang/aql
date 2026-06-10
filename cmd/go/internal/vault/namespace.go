package vault

import (
	"fmt"
	"os"
	"strings"
)

// Namespaces partition the alias space so two projects can each have
// an `openai_key`. The namespace is part of the alias identity: a
// stored name is either `name` (the root namespace) or `ns:name`
// (exactly one level). `:` was previously rejected by validAlias, so
// no legacy alias can be confused with a qualified one.
//
// CLI references add one piece of sugar on top of stored names:
//
//	ns:name  — explicit, always means itself
//	:name    — explicit root (the leading colon is stripped, never stored)
//	name     — resolves into the default namespace when one is active,
//	           else the root namespace
//
// The default namespace comes from AQL_VAULT_NAMESPACE, then the
// `namespace.default` config key (`vault config --set
// namespace.default=proj`). The value `:` explicitly means root —
// useful for one-shot overrides of a configured default. There is no
// silent fallback between namespaces: a bare ref that resolves to a
// missing qualified name errors (with a hint when the root name
// exists) rather than quietly using a different namespace's secret.
const (
	// EnvNamespace overrides the configured default namespace.
	EnvNamespace = "AQL_VAULT_NAMESPACE"
	// configNamespaceKey is the vault-config key holding the default.
	configNamespaceKey = "namespace.default"
	// rootNamespaceRef is how the root namespace is spelled wherever a
	// namespace is named: name position (`:key`), filter values
	// (`--namespace=:`), and the env var / config value.
	rootNamespaceRef = ":"
)

// splitAlias splits a stored alias name into (namespace, base). A name
// with no colon is in the root namespace: ("", name).
func splitAlias(name string) (ns, base string) {
	if i := strings.IndexByte(name, ':'); i >= 0 {
		return name[:i], name[i+1:]
	}
	return "", name
}

// aliasBase returns the name without its namespace qualifier.
func aliasBase(name string) string {
	_, base := splitAlias(name)
	return base
}

// aliasNamespace returns the single namespace an alias belongs to:
// the name-derived namespace when the name is qualified, else the
// legacy metadata tag (pre-namespacing `--namespace` was tag-only),
// else "" for root.
func aliasNamespace(a Alias) string {
	if ns, _ := splitAlias(a.Name); ns != "" {
		return ns
	}
	return a.Namespace
}

// validNamespaceName accepts a single alias segment as a namespace
// name (so a namespace is itself colon-free).
func validNamespaceName(s string) bool { return validAliasSegment(s) }

// defaultNamespace returns the active default namespace ("" = root):
// AQL_VAULT_NAMESPACE wins over the namespace.default config key, and
// the value ":" explicitly selects root (overriding a configured
// default). An invalid value is an error rather than a silent root
// fallback — a typo must not change which secret a bare name means.
func defaultNamespace(s *Store) (string, error) {
	if v := os.Getenv(EnvNamespace); v != "" {
		if v == rootNamespaceRef {
			return "", nil
		}
		if !validNamespaceName(v) {
			return "", fmt.Errorf("invalid namespace %q in %s (allowed: letters, digits, dot, dash, underscore, or %q for root)", v, EnvNamespace, rootNamespaceRef)
		}
		return v, nil
	}
	if s != nil && s.Config != nil {
		if v, ok := s.Config[configNamespaceKey].(string); ok && v != "" {
			if v == rootNamespaceRef {
				return "", nil
			}
			if !validNamespaceName(v) {
				return "", fmt.Errorf("invalid namespace %q in config %s (allowed: letters, digits, dot, dash, underscore, or %q for root)", v, configNamespaceKey, rootNamespaceRef)
			}
			return v, nil
		}
	}
	return "", nil
}

// resolveAliasRef maps a CLI alias reference to the stored alias name
// it denotes, applying the resolution rule above. The returned name
// always satisfies validAlias.
func resolveAliasRef(s *Store, ref string) (string, error) {
	if name, ok := strings.CutPrefix(ref, rootNamespaceRef); ok {
		if !validAlias(name) || strings.Contains(name, ":") {
			return "", fmt.Errorf("invalid alias reference %q (after the root-namespace %q prefix, expected a plain name)", ref, rootNamespaceRef)
		}
		return name, nil
	}
	if !validAlias(ref) {
		return "", fmt.Errorf("invalid alias %q (allowed: letters, digits, dot, dash, underscore, with one optional ns: qualifier)", ref)
	}
	if strings.Contains(ref, ":") {
		return ref, nil // already qualified
	}
	ns, err := defaultNamespace(s)
	if err != nil {
		return "", err
	}
	if ns == "" {
		return ref, nil
	}
	return ns + ":" + ref, nil
}

// findAliasRef resolves ref and looks the alias up. On a miss caused
// by default-namespace qualification it points at the root-level
// alias of the same base name when one exists, so the user learns the
// `:name` escape instead of wondering where their key went.
func findAliasRef(s *Store, ref string) (*Alias, string, error) {
	name, err := resolveAliasRef(s, ref)
	if err != nil {
		return nil, "", err
	}
	if a, _ := s.FindAlias(name); a != nil {
		return a, name, nil
	}
	if name != ref && !strings.Contains(ref, ":") {
		if root, _ := s.FindAlias(ref); root != nil {
			return nil, name, fmt.Errorf("no alias named %q; a root-level alias %q exists — address it as %q", name, ref, rootNamespaceRef+ref)
		}
	}
	return nil, name, fmt.Errorf("no alias named %q", name)
}

// validAliasRef accepts a CLI alias reference: a stored-form name
// (`name` or `ns:name`), or `:name` forcing the root namespace.
func validAliasRef(ref string) bool {
	if name, ok := strings.CutPrefix(ref, rootNamespaceRef); ok {
		return validAlias(name) && !strings.Contains(name, ":")
	}
	return validAlias(ref)
}

// requalify replaces name's namespace: ns "" yields the bare base
// name (root), otherwise ns:base. Used by bundle import's
// --namespace remap.
func requalify(name, ns string) string {
	base := aliasBase(name)
	if ns == "" {
		return base
	}
	return ns + ":" + base
}

// importNamespace resolves the effective namespace for a batch of
// bare names (.env import): the --namespace flag value when given
// (":" = explicit root), else the active default namespace.
func importNamespace(s *Store, flagVal string) (string, error) {
	switch flagVal {
	case "":
		return defaultNamespace(s)
	case rootNamespaceRef:
		return "", nil
	}
	if !validNamespaceName(flagVal) {
		return "", fmt.Errorf("invalid namespace %q", flagVal)
	}
	return flagVal, nil
}

// normalizeNSFilter canonicalizes a --namespace filter value: ":" (or
// "") selects root and is returned as "", a namespace name is
// returned as itself, anything else is an error. The boolean reports
// whether a filter was given at all.
func normalizeNSFilter(v string) (string, bool, error) {
	switch v {
	case "":
		return "", false, nil
	case rootNamespaceRef:
		return "", true, nil
	}
	if !validNamespaceName(v) {
		return "", false, fmt.Errorf("invalid namespace filter %q (a namespace name, or %q for root)", v, rootNamespaceRef)
	}
	return v, true, nil
}
