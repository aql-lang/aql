package native

import "testing"

// These golden tests pin the SQLite SQL the query builder emits. They
// are the regression net for the Dialect extraction: the strings here
// must stay byte-identical to the pre-Dialect query path, and they
// document the exact output every other dialect is measured against.

// sqliteQB returns a fresh SQLite-dialect builder with no LIMIT/OFFSET.
func sqliteQB() QueryBuilder {
	return QueryBuilder{Dialect: SQLiteDialect{}, Limit: -1, Offset: -1}
}

func TestSQLiteDialectPrimitives(t *testing.T) {
	d := SQLiteDialect{}
	if got := d.QuoteIdent("age"); got != "`age`" {
		t.Errorf("QuoteIdent: got %q want %q", got, "`age`")
	}
	if got := d.QuoteIdent("a`b"); got != "`a``b`" {
		t.Errorf("QuoteIdent escape: got %q want %q", got, "`a``b`")
	}
	if got := d.Placeholder(3); got != "?" {
		t.Errorf("Placeholder: got %q want %q", got, "?")
	}
	if got := d.BooleanLiteral(true); got != "'true'" {
		t.Errorf("BooleanLiteral(true): got %q", got)
	}
	if got := d.BooleanLiteral(false); got != "'false'" {
		t.Errorf("BooleanLiteral(false): got %q", got)
	}
}

func TestSQLiteLimitOffsetGolden(t *testing.T) {
	d := SQLiteDialect{}
	cases := []struct {
		limit, offset int
		want          string
	}{
		{-1, -1, ""},
		{10, -1, " LIMIT 10"},
		{-1, 5, " LIMIT -1 OFFSET 5"}, // SQLite bare-offset quirk
		{10, 5, " LIMIT 10 OFFSET 5"},
		{0, -1, " LIMIT 0"},
	}
	for _, c := range cases {
		if got := d.LimitOffset(c.limit, c.offset, false); got != c.want {
			t.Errorf("LimitOffset(%d,%d): got %q want %q", c.limit, c.offset, got, c.want)
		}
	}
}

func TestSQLiteBuildSQLGolden(t *testing.T) {
	cases := []struct {
		name  string
		setup func(qb *QueryBuilder)
		want  string
	}{
		{
			name:  "select star",
			setup: func(qb *QueryBuilder) {},
			want:  "SELECT * FROM `people`",
		},
		{
			name:  "distinct",
			setup: func(qb *QueryBuilder) { qb.Distinct = true },
			want:  "SELECT DISTINCT * FROM `people`",
		},
		{
			name: "where + order + limit + offset",
			setup: func(qb *QueryBuilder) {
				qb.Where = "`age` > 18"
				qb.OrderBy = "`age` DESC"
				qb.Limit = 10
				qb.Offset = 5
			},
			want: "SELECT * FROM `people` WHERE `age` > 18 ORDER BY `age` DESC LIMIT 10 OFFSET 5",
		},
		{
			name:  "group + having",
			setup: func(qb *QueryBuilder) { qb.GroupBy = "`city`"; qb.Having = "COUNT(*) > 1" },
			want:  "SELECT * FROM `people` GROUP BY `city` HAVING COUNT(*) > 1",
		},
		{
			name: "left join with on",
			setup: func(qb *QueryBuilder) {
				qb.Joins = []JoinClause{{Type: "LEFT JOIN", Table: "orders", On: "`people`.`id` = `orders`.`pid`"}}
			},
			want: "SELECT * FROM `people` LEFT JOIN `orders` ON `people`.`id` = `orders`.`pid`",
		},
		{
			name:  "bare offset uses LIMIT -1",
			setup: func(qb *QueryBuilder) { qb.Offset = 7 },
			want:  "SELECT * FROM `people` LIMIT -1 OFFSET 7",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			qb := sqliteQB()
			c.setup(&qb)
			if got := qb.buildSQL("people", "*"); got != c.want {
				t.Errorf("buildSQL:\n got %q\nwant %q", got, c.want)
			}
		})
	}
}

func TestSQLiteWhereClauseGolden(t *testing.T) {
	d := SQLiteDialect{}
	cases := []struct {
		name string
		cond Value
		want string
	}{
		{"comparison", NewList([]Value{NewWord("age"), NewWord("gt"), NewInteger(18)}), "`age` > 18"},
		{"string eq", NewList([]Value{NewWord("name"), NewWord("eq"), NewString("ann")}), "`name` = 'ann'"},
		{"like", NewList([]Value{NewWord("name"), NewWord("like"), NewString("a%")}), "`name` LIKE 'a%'"},
		{"glob", NewList([]Value{NewWord("name"), NewWord("glob"), NewString("a*")}), "`name` GLOB 'a*'"},
		{"regexp", NewList([]Value{NewWord("name"), NewWord("regexp"), NewString("^a")}), "`name` REGEXP '^a'"},
		{"is null", NewList([]Value{NewWord("mid"), NewWord("is"), NewWord("null")}), "`mid` IS NULL"},
		{"between", NewList([]Value{NewWord("age"), NewWord("between"), NewInteger(1), NewInteger(9)}), "`age` BETWEEN 1 AND 9"},
		{"collate", NewList([]Value{NewWord("name"), NewWord("eq"), NewString("x"), NewWord("collate"), NewWord("nocase")}), "`name` = 'x' COLLATE NOCASE"},
		{"empty", NewList([]Value{}), "1=1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildWhereClause(d, c.cond)
			if err != nil {
				t.Fatalf("buildWhereClause: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestSQLiteOrderClauseGolden(t *testing.T) {
	d := SQLiteDialect{}
	cases := []struct {
		name string
		cols Value
		want string
	}{
		{"plain", NewList([]Value{NewWord("age")}), "`age`"},
		{"desc", NewList([]Value{NewWord("age"), NewWord("desc")}), "`age` DESC"},
		{"nulls last", NewList([]Value{NewWord("age"), NewWord("asc"), NewWord("nulls"), NewWord("last")}), "`age` ASC NULLS LAST"},
		{"positional", NewList([]Value{NewInteger(1), NewInteger(2), NewWord("desc")}), "1, 2 DESC"},
		{"collate", NewList([]Value{NewWord("name"), NewWord("collate"), NewWord("nocase")}), "`name` COLLATE NOCASE"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildOrderClause(d, c.cols)
			if err != nil {
				t.Fatalf("buildOrderClause: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

// noFeatureDialect is a SQLite dialect with all optional capabilities
// disabled — used to prove the builder REJECTS features a target cannot
// express, rather than emitting SQL the backend would choke on.
type noFeatureDialect struct{ SQLiteDialect }

func (noFeatureDialect) Name() string      { return "nofeature" }
func (noFeatureDialect) Caps() DialectCaps { return DialectCaps{Collations: map[string]string{}} }

func TestDialectRejectsUnsupportedFeatures(t *testing.T) {
	d := noFeatureDialect{}

	// GLOB is rejected when the dialect lacks it.
	if _, err := buildWhereClause(d, NewList([]Value{NewWord("n"), NewWord("glob"), NewString("a*")})); err == nil {
		t.Error("expected error for glob on dialect without Glob cap")
	}
	// REGEXP is rejected when the dialect lacks it.
	if _, err := buildWhereClause(d, NewList([]Value{NewWord("n"), NewWord("regexp"), NewString("^a")})); err == nil {
		t.Error("expected error for regexp on dialect without Regexp cap")
	}
	// COLLATE in WHERE is rejected when the collation is unknown.
	if _, err := buildWhereClause(d, NewList([]Value{NewWord("n"), NewWord("eq"), NewString("x"), NewWord("collate"), NewWord("nocase")})); err == nil {
		t.Error("expected error for collate on dialect without collations")
	}
	// COLLATE in ORDER BY is rejected too.
	if _, err := buildOrderClause(d, NewList([]Value{NewWord("n"), NewWord("collate"), NewWord("nocase")})); err == nil {
		t.Error("expected error for order collate on dialect without collations")
	}

	// Sanity: a plain comparison still works on the bare dialect.
	if got, err := buildWhereClause(d, NewList([]Value{NewWord("age"), NewWord("gt"), NewInteger(1)})); err != nil || got != "`age` > 1" {
		t.Errorf("plain comparison: got %q err %v", got, err)
	}
}
