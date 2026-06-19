//go:build !js

package native

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"  // MySQL / MariaDB driver ("mysql")
	_ "github.com/jackc/pgx/v5/stdlib"  // PostgreSQL driver ("pgx")
	_ "github.com/microsoft/go-mssqldb" // SQL Server driver ("sqlserver")
)

// SQLDatabase is an external SQL backend implemented over database/sql.
// It is a read-mostly source: SELECT and raw DML/DDL execute against
// the remote server, but table materialization (StoreTable) is the
// embedded engine's job, so those methods are rejected here.
type SQLDatabase struct {
	db      *sql.DB
	dialect Dialect
	label   string
}

var _ SQLBackend = (*SQLDatabase)(nil)

// resolveDriver maps a user-facing backend kind to a database/sql
// driver name and its matching Dialect.
func resolveDriver(kind string) (driver string, d Dialect, err error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "postgres", "postgresql", "pg", "pgx":
		return "pgx", PostgresDialect{}, nil
	case "mysql", "mariadb":
		return "mysql", MySQLDialect{}, nil
	case "sqlserver", "mssql", "azuresql":
		return "sqlserver", MSSQLDialect{}, nil
	case "sqlite", "sqlite3":
		// The embedded engine, opened as a database/sql connection
		// (e.g. a file path or ":memory:"). Distinct from the ambient
		// HostSQLite compute store.
		return "sqlite", SQLiteDialect{}, nil
	default:
		return "", nil, fmt.Errorf("unknown database kind %q (want postgres, mysql, sqlserver, or sqlite)", kind)
	}
}

// CanonicalDBKind normalizes a user-facing backend kind to its
// canonical name, or returns false if the kind is unknown. Exposed so
// the aql:db module can validate kinds without opening a connection.
func CanonicalDBKind(kind string) (string, bool) {
	_, d, err := resolveDriver(kind)
	if err != nil {
		return "", false
	}
	return d.Name(), true
}

// OpenSQLDatabase opens and verifies a connection to an external SQL
// database of the given kind using the supplied DSN. It pings the
// server so a bad DSN or unreachable host fails fast, with credentials
// redacted from any error.
func OpenSQLDatabase(kind, dsn string) (*SQLDatabase, error) {
	driver, dialect, err := resolveDriver(kind)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("%s: open: %w", dialect.Name(), redactError(err, dsn))
	}
	if driver == "sqlite" {
		// A SQLite ":memory:" database is per-connection, so the pool
		// must hold exactly one connection or successive statements
		// would hit different (empty) databases. File-backed SQLite is
		// also single-writer, so one connection is the safe default.
		db.SetMaxOpenConns(1)
	} else {
		db.SetMaxOpenConns(8)
		db.SetMaxIdleConns(4)
		db.SetConnMaxLifetime(30 * time.Minute)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%s: connect: %w", dialect.Name(), redactError(err, dsn))
	}
	return &SQLDatabase{db: db, dialect: dialect, label: redactDSN(dsn)}, nil
}

// OpenDBConn opens an external connection and wraps it as a *DBConn —
// the build-agnostic entry point the aql:db module calls (the wasm stub
// returns a not-supported error). The raw DSN is dropped; only the
// redacted label is retained.
func OpenDBConn(kind, dsn string) (*DBConn, error) {
	sdb, err := OpenSQLDatabase(kind, dsn)
	if err != nil {
		return nil, err
	}
	return &DBConn{Backend: sdb, Label: sdb.label, Kind: sdb.dialect.Name()}, nil
}

func (s *SQLDatabase) Dialect() Dialect { return s.dialect }

// Label is the redacted connection identity for display.
func (s *SQLDatabase) Label() string { return s.label }

func (s *SQLDatabase) Close() error { return s.db.Close() }

func (s *SQLDatabase) Query(q string, schema *RecordTypeInfo) (TableData, error) {
	return s.QueryParams(q, nil, schema)
}

func (s *SQLDatabase) QueryParams(q string, params []any, schema *RecordTypeInfo) (TableData, error) {
	rows, err := s.db.Query(q, params...)
	if err != nil {
		return TableData{}, fmt.Errorf("%s query: %w", s.dialect.Name(), err)
	}
	defer rows.Close()
	td, err := scanSQLRows(rows, schema, s.dialect)
	if err != nil {
		return TableData{}, fmt.Errorf("%s: %w", s.dialect.Name(), err)
	}
	return td, nil
}

func (s *SQLDatabase) Exec(q string, params []any) (int64, error) {
	res, err := s.db.Exec(q, params...)
	if err != nil {
		return 0, fmt.Errorf("%s exec: %w", s.dialect.Name(), err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil // driver doesn't report it; not an error
	}
	return n, nil
}

func (s *SQLDatabase) ListTables() ([]string, error) {
	td, err := s.Query(s.dialect.IntrospectTablesSQL(), nil)
	if err != nil {
		return nil, err
	}
	return firstColumnStrings(td), nil
}

func (s *SQLDatabase) DescribeTable(name string) (RecordTypeInfo, error) {
	td, err := s.Query(s.dialect.IntrospectColumnsSQL(name), nil)
	if err != nil {
		return RecordTypeInfo{}, err
	}
	fields := NewOrderedMap()
	for _, row := range td.Rows {
		m, err := AsMap(row)
		if err != nil {
			continue
		}
		nv, _ := m.Get("name")
		tv, _ := m.Get("type")
		cn := ValToString(nv)
		if cn == "" {
			continue
		}
		fields.Set(cn, NewTypeLiteral(s.dialect.ColumnTypeToAQLType(ValToString(tv))))
	}
	if fields.Len() == 0 {
		return RecordTypeInfo{}, fmt.Errorf("%s: no such table %q", s.dialect.Name(), name)
	}
	return RecordTypeInfo{Fields: fields}, nil
}

func (s *SQLDatabase) HasTable(name string) bool {
	_, err := s.DescribeTable(name)
	return err == nil
}

// StoreTable / StoreTempTable: external connections are read-only
// sources; the embedded engine materializes data.
func (s *SQLDatabase) StoreTable(string, TableData) error {
	return fmt.Errorf("%s: external connection is read-only (cannot create tables)", s.dialect.Name())
}

func (s *SQLDatabase) StoreTempTable(TableData) (string, error) {
	return "", fmt.Errorf("%s: external connection is read-only (cannot create temp tables)", s.dialect.Name())
}

// DropTable is a no-op on read-only external backends.
func (s *SQLDatabase) DropTable(string) {}

// kvSecretRE matches password/pwd tokens in a key-value DSN.
var kvSecretRE = regexp.MustCompile(`(?i)\b(password|pwd)=([^;\s]*)`)

// redactDSN returns a display-safe form of a DSN with any password
// removed. URL-form DSNs are redacted via url.Redacted (password →
// "xxxxx"); key-value DSNs have password/pwd tokens masked.
func redactDSN(dsn string) string {
	if strings.Contains(dsn, "://") {
		if u, err := url.Parse(dsn); err == nil && u.Scheme != "" {
			return u.Redacted()
		}
	}
	return kvSecretRE.ReplaceAllString(dsn, "$1=***")
}

// redactError strips a DSN (and any embedded password) from a driver
// error so credentials never surface in messages shown to the user.
func redactError(err error, dsn string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if dsn != "" {
		msg = strings.ReplaceAll(msg, dsn, "<dsn>")
	}
	msg = kvSecretRE.ReplaceAllString(msg, "$1=***")
	return errors.New(msg)
}
