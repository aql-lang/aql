//go:build !js

package native

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveDriver(t *testing.T) {
	cases := []struct {
		kind    string
		driver  string
		dialect string
	}{
		{"postgres", "pgx", "postgres"},
		{"postgresql", "pgx", "postgres"},
		{"pg", "pgx", "postgres"},
		{"mysql", "mysql", "mysql"},
		{"mariadb", "mysql", "mysql"},
		{"sqlserver", "sqlserver", "sqlserver"},
		{"mssql", "sqlserver", "sqlserver"},
		{"  MySQL  ", "mysql", "mysql"}, // trimmed + case-insensitive
	}
	for _, c := range cases {
		drv, d, err := resolveDriver(c.kind)
		if err != nil {
			t.Errorf("resolveDriver(%q): %v", c.kind, err)
			continue
		}
		if drv != c.driver || d.Name() != c.dialect {
			t.Errorf("resolveDriver(%q): got (%s,%s) want (%s,%s)", c.kind, drv, d.Name(), c.driver, c.dialect)
		}
	}

	// Negative: unknown kind errors.
	if _, _, err := resolveDriver("oracle"); err == nil {
		t.Error("expected error for unknown kind 'oracle'")
	}
}

func TestCanonicalDBKind(t *testing.T) {
	if got, ok := CanonicalDBKind("pg"); !ok || got != "postgres" {
		t.Errorf("CanonicalDBKind(pg): got %q ok=%v", got, ok)
	}
	if _, ok := CanonicalDBKind("redis"); ok {
		t.Error("CanonicalDBKind(redis): expected not ok")
	}
}

func TestRedactDSN(t *testing.T) {
	cases := []struct {
		dsn         string
		mustHave    string
		mustNotHave string
	}{
		{"postgres://user:secretpw@host:5432/db", "user", "secretpw"},
		{"mysql://root:hunter2@127.0.0.1:3306/app", "root", "hunter2"},
		{"host=localhost user=sa password=Topsecret1 dbname=app", "user=sa", "Topsecret1"},
		{"server=x;user id=sa;password=p@ss;database=y", "user id=sa", "p@ss"},
	}
	for _, c := range cases {
		got := redactDSN(c.dsn)
		if c.mustNotHave != "" && strings.Contains(got, c.mustNotHave) {
			t.Errorf("redactDSN(%q) = %q, leaked %q", c.dsn, got, c.mustNotHave)
		}
		if c.mustHave != "" && !strings.Contains(got, c.mustHave) {
			t.Errorf("redactDSN(%q) = %q, missing %q", c.dsn, got, c.mustHave)
		}
	}
}

func TestRedactError(t *testing.T) {
	dsn := "postgres://user:secretpw@host/db"
	err := redactError(errors.New("dial failed for postgres://user:secretpw@host/db: timeout"), dsn)
	if strings.Contains(err.Error(), "secretpw") {
		t.Errorf("redactError leaked password: %q", err.Error())
	}
	// nil passes through.
	if redactError(nil, dsn) != nil {
		t.Error("redactError(nil) should be nil")
	}
	// key-value password masked even when the dsn substring differs.
	err2 := redactError(errors.New("login failed: password=Topsecret1"), "")
	if strings.Contains(err2.Error(), "Topsecret1") {
		t.Errorf("redactError leaked kv password: %q", err2.Error())
	}
}

func TestOpenSQLDatabaseErrors(t *testing.T) {
	// Unknown kind fails before any connection attempt.
	if _, err := OpenSQLDatabase("oracle", "whatever"); err == nil {
		t.Error("expected error for unknown kind")
	}
	// A malformed/unreachable DSN fails, and the error must not leak
	// the password. Use an unroutable address so this is fast.
	if _, err := OpenDBConn("postgres", "postgres://u:leakme@127.0.0.1:1/db?connect_timeout=1&sslmode=disable"); err != nil {
		if strings.Contains(err.Error(), "leakme") {
			t.Errorf("open error leaked password: %q", err.Error())
		}
	}
}
