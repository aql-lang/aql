package vault

import (
	"flag"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Key expiries are an optional, informational reminder attached to an
// alias (Alias.ExpiresAt): when the underlying upstream key is expected
// to expire, so it can be rotated in time. They are never enforced —
// `vault get` still returns an "expired" key — they only drive the
// reminders surfaced by `vault list` and `vault expiry`.

// parseExpiryFlag turns an --expiry flag value into a stored RFC3339
// timestamp. An empty flag yields an empty result, meaning "no expiry"
// on add and "leave the current expiry unchanged" on rotate; any
// non-empty value must parse.
func parseExpiryFlag(s string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", nil
	}
	return parseExpiry(s, time.Now())
}

// parseExpiry interprets an expiry argument as a stored RFC3339 UTC
// timestamp. It accepts, in order: an RFC3339 timestamp, a calendar
// date (YYYY-MM-DD, taken at 00:00:00 UTC), or a relative duration from
// now with day support (90d, 720h, 30d12h). Relative durations must be
// positive; an absolute date in the past is allowed, since recording an
// already-expired key is legitimate.
func parseExpiry(s string, now time.Time) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty expiry")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	if d, err := parseDurationDays(s); err == nil {
		if d <= 0 {
			return "", fmt.Errorf("expiry duration must be positive: %q", s)
		}
		return now.UTC().Add(d).Format(time.RFC3339), nil
	}
	return "", fmt.Errorf("invalid expiry %q (want YYYY-MM-DD, an RFC3339 timestamp, or a duration like 90d, 720h, 30d12h)", s)
}

// parseDurationDays extends time.ParseDuration with an integer "d"
// (days) component, which Go's parser lacks. "90d", "30d12h", and plain
// Go durations such as "720h" or "2h30m" all work; one day is exactly
// 24h.
func parseDurationDays(s string) (time.Duration, error) {
	rest := s
	var days time.Duration
	if i := strings.IndexByte(s, 'd'); i >= 0 {
		n, err := strconv.Atoi(s[:i])
		if err != nil {
			return 0, fmt.Errorf("invalid days in duration %q", s)
		}
		days = time.Duration(n) * 24 * time.Hour
		rest = s[i+1:]
	}
	if rest == "" {
		return days, nil
	}
	d, err := time.ParseDuration(rest)
	if err != nil {
		return 0, err
	}
	return days + d, nil
}

// runExpiry implements `aql vault expiry`. With no subcommand (or with
// list/ls) it reports pending expiries; set and clear manage the expiry
// on an existing alias.
func runExpiry(args []string, homeDir string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "set":
			return runExpirySet(args[1:], homeDir, stdout, stderr)
		case "clear", "unset", "remove":
			return runExpiryClear(args[1:], homeDir, stdout, stderr)
		case "list", "ls":
			return runExpiryList(args[1:], homeDir, stdout, stderr)
		}
	}
	// Default to listing, so `vault expiry` and `vault expiry
	// --namespace=ns` both work without a subcommand.
	return runExpiryList(args, homeDir, stdout, stderr)
}

// runExpiryList prints the aliases that carry an expiry, soonest (and
// most overdue) first, with a human-readable status. --namespace
// restricts the view to one namespace; --within limits it to keys due
// within a window (already-overdue keys always qualify).
func runExpiryList(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault expiry", flag.ContinueOnError)
	fs.SetOutput(stderr)
	namespace := fs.String("namespace", "", "filter by namespace (':' = root only)")
	within := fs.String("within", "", "only keys expiring within this window, or already overdue (e.g. 30d, 48h)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	nsFilter, nsFiltered, err := normalizeNSFilter(*namespace)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	now := time.Now()
	var cutoff time.Time
	haveCutoff := false
	if *within != "" {
		d, err := parseDurationDays(*within)
		if err != nil || d < 0 {
			fmt.Fprintf(stderr, "error: invalid --within %q (want a duration like 30d, 48h)\n", *within)
			return 1
		}
		cutoff = now.Add(d)
		haveCutoff = true
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	type row struct {
		alias   Alias
		expires time.Time
	}
	var rows []row
	for _, a := range s.Aliases {
		if a.ExpiresAt == "" {
			continue
		}
		if nsFiltered && aliasNamespace(a) != nsFilter {
			continue
		}
		t, perr := time.Parse(time.RFC3339, a.ExpiresAt)
		if perr != nil {
			continue // a stored value should always parse; skip if not
		}
		if haveCutoff && t.After(cutoff) {
			continue
		}
		rows = append(rows, row{alias: a, expires: t})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].expires.Equal(rows[j].expires) {
			return rows[i].alias.Name < rows[j].alias.Name
		}
		return rows[i].expires.Before(rows[j].expires)
	})
	if len(rows) == 0 {
		fmt.Fprintln(stdout, "(no pending key expiries)")
		return 0
	}
	fmt.Fprintf(stdout, "%-24s %-16s %-20s %s\n", "ALIAS", "NAMESPACE", "EXPIRES", "STATUS")
	for _, r := range rows {
		fmt.Fprintf(stdout, "%-24s %-16s %-20s %s\n",
			r.alias.Name, dash(aliasNamespace(r.alias)), r.alias.ExpiresAt, expiryStatus(r.expires, now))
	}
	return 0
}

// runExpirySet sets or replaces the expiry on an existing alias without
// touching its secret value.
func runExpirySet(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault expiry set", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 2 {
		fmt.Fprintf(stderr, "error: usage: aql vault expiry set <alias> <when>\n  when: YYYY-MM-DD, an RFC3339 timestamp, or a duration like 90d / 720h\n")
		return 1
	}
	expiresAt, err := parseExpiry(fs.Arg(1), time.Now())
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_, alias, err := findAliasRef(s, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	if err := mutateStore(homeDir, func(s *Store) error {
		a, _ := s.FindAlias(alias)
		if a == nil {
			return fmt.Errorf("no alias named %q", alias)
		}
		a.ExpiresAt = expiresAt
		a.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		return nil
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{Action: "vault.expiry.set", Alias: alias, Outcome: "ok", Reason: "expires=" + expiresAt})
	fmt.Fprintf(stdout, "set expiry for %s to %s\n", alias, expiresAt)
	return 0
}

// runExpiryClear removes the expiry from an alias, leaving its secret
// and other metadata in place.
func runExpiryClear(args []string, homeDir string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vault expiry clear", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(stderr, "error: usage: aql vault expiry clear <alias>\n")
		return 1
	}
	s, err := requireStore(homeDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_, alias, err := findAliasRef(s, fs.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	had := false
	if err := mutateStore(homeDir, func(s *Store) error {
		a, _ := s.FindAlias(alias)
		if a == nil {
			return fmt.Errorf("no alias named %q", alias)
		}
		had = a.ExpiresAt != ""
		a.ExpiresAt = ""
		a.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		return nil
	}); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}
	_ = appendAudit(homeDir, AuditEvent{Action: "vault.expiry.clear", Alias: alias, Outcome: "ok"})
	if had {
		fmt.Fprintf(stdout, "cleared expiry for %s\n", alias)
	} else {
		fmt.Fprintf(stdout, "%s had no expiry set\n", alias)
	}
	return 0
}

// expiryStatus renders the remaining (or overdue) time for an expiry
// relative to now, e.g. "in 90d" or "expired 3d ago".
func expiryStatus(expires, now time.Time) string {
	if !expires.After(now) {
		return "expired " + humanizeDur(now.Sub(expires)) + " ago"
	}
	return "in " + humanizeDur(expires.Sub(now))
}

// humanizeDur renders a duration coarsely: days when at least a day,
// else hours, else minutes (else "<1m").
//
// Each unit is ROUNDED, not truncated, so an expiry set to "now + 90d"
// reads as "90d" the moment it is stored — by render time a few
// milliseconds have passed, leaving 89d23h59m…, which truncation would
// have shown as "89d". The unit thresholds are shifted down by half of
// the next-smaller unit so rounding never produces a boundary value
// like "24h" or "60m".
func humanizeDur(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	switch {
	case d >= 24*time.Hour-30*time.Minute:
		return fmt.Sprintf("%dd", roundDiv(d, 24*time.Hour))
	case d >= time.Hour-30*time.Second:
		return fmt.Sprintf("%dh", roundDiv(d, time.Hour))
	case d >= 30*time.Second:
		return fmt.Sprintf("%dm", roundDiv(d, time.Minute))
	default:
		return "<1m"
	}
}

// roundDiv divides d by unit, rounding to the nearest whole number.
func roundDiv(d, unit time.Duration) int {
	return int((d + unit/2) / unit)
}
