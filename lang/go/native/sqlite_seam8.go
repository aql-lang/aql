//go:build !js

package native

import "database/sql"

// w8SQLOpen is the seam for opening the SQLite database in NewSQLiteStore.
// It defaults to sql.Open; tests swap it (with restore) to drive the
// otherwise-unreachable open-failure error path. See the seam discipline
// in design/TEST-SEAMS.10.md.
var w8SQLOpen = sql.Open
