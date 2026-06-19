package native

import (
	"fmt"

	eng "github.com/aql-lang/aql/eng/go"
)

// TDBConn is Ideal/DBConn — the carrier type for an open external
// database connection. FixedID 5005, next free in the 5000–9999 host
// range (5000 Module, 5001 ModuleExport, 5002 KeyVal, 5003
// MiniLangCompiled, 5004 Patrun).
var TDBConn = registerDBConnType()

func registerDBConnType() *eng.Type {
	t, err := eng.Builtin.RegisterExternalBuiltin("Ideal/DBConn", 5005, nil)
	if err != nil {
		recordTypeInitErr(fmt.Errorf("native_db_conn: register Ideal/DBConn: %w", err))
	}
	return t
}

// DBConn is the payload behind a DBConn value: an open SQLBackend plus
// display metadata. The connection rides as a pointer in an
// ExtensionPayload, so every copy of the value shares one underlying
// connection (and one Close). The raw DSN is never stored — only the
// redacted Label — so credentials cannot leak through printing or
// error messages.
type DBConn struct {
	Backend SQLBackend
	Label   string // redacted identity, e.g. "postgres://user@host/db"
	Kind    string // backend kind: postgres / mysql / sqlserver / sqlite
	closed  bool
}

// NewDBConn wraps an open connection as a DBConn value.
func NewDBConn(c *DBConn) Value {
	return eng.NewExtension(TDBConn, c)
}

// AsDBConn extracts the *DBConn payload from a value, if present.
func AsDBConn(v Value) (*DBConn, bool) {
	ep, ok := v.Data.(eng.ExtensionPayload)
	if !ok {
		return nil, false
	}
	c, ok := ep.Body.(*DBConn)
	return c, ok
}

// Close closes the underlying backend once; further calls are no-ops.
func (c *DBConn) Close() error {
	if c == nil || c.closed || c.Backend == nil {
		return nil
	}
	c.closed = true
	return c.Backend.Close()
}
