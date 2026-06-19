package native

import "testing"

// Golden SQL-generation tests for the external dialects. No database is
// involved — these pin the SQL string each dialect emits and prove the
// builder rejects features a target cannot express. They are the bulk
// of the multi-RDBMS correctness guarantee and run in plain `make test`.

func TestDialectPrimitives(t *testing.T) {
	cases := []struct {
		d                       Dialect
		name, quoted, ph, boolT string
	}{
		{PostgresDialect{}, "postgres", `"a"`, "$2", "TRUE"},
		{MySQLDialect{}, "mysql", "`a`", "?", "TRUE"},
		{MSSQLDialect{}, "sqlserver", "[a]", "@p2", "1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.d.Name() != c.name {
				t.Errorf("Name: got %q want %q", c.d.Name(), c.name)
			}
			if got := c.d.QuoteIdent("a"); got != c.quoted {
				t.Errorf("QuoteIdent: got %q want %q", got, c.quoted)
			}
			if got := c.d.Placeholder(2); got != c.ph {
				t.Errorf("Placeholder: got %q want %q", got, c.ph)
			}
			if got := c.d.BooleanLiteral(true); got != c.boolT {
				t.Errorf("BooleanLiteral: got %q want %q", got, c.boolT)
			}
		})
	}
}

func TestDialectIdentEscaping(t *testing.T) {
	if got := (PostgresDialect{}).QuoteIdent(`a"b`); got != `"a""b"` {
		t.Errorf("postgres escape: got %q", got)
	}
	if got := (MSSQLDialect{}).QuoteIdent("a]b"); got != "[a]]b]" {
		t.Errorf("mssql escape: got %q", got)
	}
}

func TestLimitOffsetDivergence(t *testing.T) {
	cases := []struct {
		name          string
		d             Dialect
		limit, offset int
		hasOrderBy    bool
		want          string
	}{
		// Postgres: independent clauses, bare OFFSET allowed.
		{"pg both", PostgresDialect{}, 10, 5, false, " LIMIT 10 OFFSET 5"},
		{"pg offset only", PostgresDialect{}, -1, 5, false, " OFFSET 5"},
		{"pg limit only", PostgresDialect{}, 10, -1, false, " LIMIT 10"},
		// MySQL: OFFSET needs a LIMIT; offset-only uses the max-limit idiom.
		{"my both", MySQLDialect{}, 10, 5, false, " LIMIT 10 OFFSET 5"},
		{"my offset only", MySQLDialect{}, -1, 5, false, " LIMIT 18446744073709551615 OFFSET 5"},
		{"my limit only", MySQLDialect{}, 10, -1, false, " LIMIT 10"},
		// SQL Server: OFFSET..FETCH, OFFSET mandatory, ORDER BY required.
		{"mssql both w/order", MSSQLDialect{}, 10, 5, true, " OFFSET 5 ROWS FETCH NEXT 10 ROWS ONLY"},
		{"mssql limit only no order", MSSQLDialect{}, 10, -1, false, " ORDER BY (SELECT NULL) OFFSET 0 ROWS FETCH NEXT 10 ROWS ONLY"},
		{"mssql offset only w/order", MSSQLDialect{}, -1, 5, true, " OFFSET 5 ROWS"},
		{"mssql none", MSSQLDialect{}, -1, -1, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.d.LimitOffset(c.limit, c.offset, c.hasOrderBy); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestExternalBuildSQLGolden(t *testing.T) {
	// Same logical query, three dialects — demonstrates divergence in
	// quoting and paging through the shared builder.
	mk := func(d Dialect) QueryBuilder {
		qb := QueryBuilder{Dialect: d, Limit: 10, Offset: 5}
		qb.Where = ""
		return qb
	}
	cases := []struct {
		name string
		d    Dialect
		want string
	}{
		{"postgres", PostgresDialect{}, "SELECT * FROM \"people\" LIMIT 10 OFFSET 5"},
		{"mysql", MySQLDialect{}, "SELECT * FROM `people` LIMIT 10 OFFSET 5"},
		{"mssql", MSSQLDialect{}, "SELECT * FROM [people] ORDER BY (SELECT NULL) OFFSET 5 ROWS FETCH NEXT 10 ROWS ONLY"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			qb := mk(c.d)
			if got := qb.buildSQL("people", "*"); got != c.want {
				t.Errorf("buildSQL:\n got %q\nwant %q", got, c.want)
			}
		})
	}
}

func TestExternalWhereRegexp(t *testing.T) {
	cond := NewList([]Value{NewWord("name"), NewWord("regexp"), NewString("^a")})

	// Postgres renders regexp as `~`.
	if got, err := buildWhereClause(PostgresDialect{}, cond); err != nil || got != `"name" ~ '^a'` {
		t.Errorf("postgres regexp: got %q err %v", got, err)
	}
	// MySQL uses the REGEXP keyword.
	if got, err := buildWhereClause(MySQLDialect{}, cond); err != nil || got != "`name` REGEXP '^a'" {
		t.Errorf("mysql regexp: got %q err %v", got, err)
	}
	// SQL Server has no regexp — must error, not emit bad SQL.
	if _, err := buildWhereClause(MSSQLDialect{}, cond); err == nil {
		t.Error("mssql regexp: expected error")
	}
}

func TestExternalRejectsGlob(t *testing.T) {
	cond := NewList([]Value{NewWord("name"), NewWord("glob"), NewString("a*")})
	for _, d := range []Dialect{PostgresDialect{}, MySQLDialect{}, MSSQLDialect{}} {
		if _, err := buildWhereClause(d, cond); err == nil {
			t.Errorf("%s: expected glob to be rejected", d.Name())
		}
	}
}

func TestExternalRejectsCollate(t *testing.T) {
	// COLLATE nocase/binary/rtrim are SQLite-only; external dialects
	// must reject them rather than emit a non-existent collation.
	where := NewList([]Value{NewWord("n"), NewWord("eq"), NewString("x"), NewWord("collate"), NewWord("nocase")})
	order := NewList([]Value{NewWord("n"), NewWord("collate"), NewWord("nocase")})
	for _, d := range []Dialect{PostgresDialect{}, MySQLDialect{}, MSSQLDialect{}} {
		if _, err := buildWhereClause(d, where); err == nil {
			t.Errorf("%s: expected where collate to be rejected", d.Name())
		}
		if _, err := buildOrderClause(d, order); err == nil {
			t.Errorf("%s: expected order collate to be rejected", d.Name())
		}
	}
}

func TestExternalCastGolden(t *testing.T) {
	castSpec := NewList([]Value{NewWord("cast"), NewWord("age"), NewWord("integer")})
	cases := []struct {
		d   Dialect
		raw string
	}{
		{PostgresDialect{}, `CAST("age" AS BIGINT)`},
		{MySQLDialect{}, "CAST(`age` AS SIGNED)"},
		{MSSQLDialect{}, "CAST([age] AS BIGINT)"},
	}
	for _, c := range cases {
		t.Run(c.d.Name(), func(t *testing.T) {
			specs, err := parseColumnSpec(c.d, NewList([]Value{castSpec}))
			if err != nil {
				t.Fatalf("parseColumnSpec: %v", err)
			}
			if len(specs) != 1 || specs[0].Raw != c.raw {
				t.Errorf("cast Raw: got %q want %q", specs[0].Raw, c.raw)
			}
			if specs[0].ResultType != TInteger {
				t.Errorf("cast ResultType: got %v want Integer", specs[0].ResultType)
			}
		})
	}
}

func TestExternalColumnTypeToAQLType(t *testing.T) {
	cases := []struct {
		d       Dialect
		sqlType string
		want    *Type
	}{
		{PostgresDialect{}, "int8", TInteger},
		{PostgresDialect{}, "double precision", TFloat},
		{PostgresDialect{}, "boolean", TBoolean},
		{PostgresDialect{}, "varchar(255)", TString},
		{MySQLDialect{}, "BIGINT", TInteger},
		{MySQLDialect{}, "DECIMAL(10,2)", TFloat},
		{MySQLDialect{}, "TEXT", TString},
		{MSSQLDialect{}, "bigint", TInteger},
		{MSSQLDialect{}, "bit", TBoolean},
		{MSSQLDialect{}, "nvarchar(max)", TString},
		// Unknown driver type never panics — falls back to String.
		{PostgresDialect{}, "jsonb", TString},
		{MSSQLDialect{}, "geography", TString},
	}
	for _, c := range cases {
		if got := c.d.ColumnTypeToAQLType(c.sqlType); got != c.want {
			t.Errorf("%s ColumnTypeToAQLType(%q): got %v want %v", c.d.Name(), c.sqlType, got, c.want)
		}
	}
}
