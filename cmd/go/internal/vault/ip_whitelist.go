package vault

import (
	"fmt"
	"net"
	"strings"
)

// IP whitelisting restricts which client source IPs may use a secret
// through the `proxy` credential broker (Alias.IPWhitelist). It is a
// broker-level control — `vault get` / `exec` on the host are unaffected
// — and only bites once the proxy is bound off-loopback.

// parseIPWhitelist parses a comma-separated list of client source IPs or
// CIDR blocks into a normalized, de-duplicated slice. Empty input yields
// (nil, nil), meaning "not supplied / no restriction". Each entry must be
// a valid IP (203.0.113.7, 2001:db8::1) or CIDR (10.0.0.0/8,
// 2001:db8::/32); anything else is an error, so a typo can't silently
// widen access.
func parseIPWhitelist(csv string) ([]string, error) {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return nil, nil
	}
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(csv, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		norm, err := normalizeIPEntry(p)
		if err != nil {
			return nil, err
		}
		if !seen[norm] {
			seen[norm] = true
			out = append(out, norm)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// normalizeIPEntry validates one entry and returns its canonical form (a
// CIDR is masked to its network address; an IP is canonicalized).
func normalizeIPEntry(s string) (string, error) {
	if strings.Contains(s, "/") {
		_, ipnet, err := net.ParseCIDR(s)
		if err != nil {
			return "", fmt.Errorf("invalid CIDR %q in --ip-whitelist (want e.g. 10.0.0.0/8)", s)
		}
		return ipnet.String(), nil
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return "", fmt.Errorf("invalid IP %q in --ip-whitelist (want e.g. 203.0.113.7 or 2001:db8::1)", s)
	}
	return ip.String(), nil
}

// ipAllowed reports whether clientIP (a bare IP string) is permitted by
// whitelist. An empty whitelist allows everything; a non-empty one never
// admits an unparseable client IP.
func ipAllowed(clientIP string, whitelist []string) bool {
	if len(whitelist) == 0 {
		return true
	}
	ip := net.ParseIP(clientIP)
	if ip == nil {
		return false
	}
	for _, entry := range whitelist {
		if strings.Contains(entry, "/") {
			if _, ipnet, err := net.ParseCIDR(entry); err == nil && ipnet.Contains(ip) {
				return true
			}
			continue
		}
		if e := net.ParseIP(entry); e != nil && e.Equal(ip) {
			return true
		}
	}
	return false
}

// clientIP extracts the bare source IP from an `ip:port` RemoteAddr, or
// "" when it can't be parsed (which a non-empty whitelist then denies).
func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return ""
	}
	return host
}
