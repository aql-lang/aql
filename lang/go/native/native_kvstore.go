package native

import (
	"fmt"

	eng "github.com/aql-lang/aql/eng/go"
)

// TKVStore is Ideal/KVStore — the carrier type for an open key-value
// store handle. FixedID 5007 (5000 Module, 5001 ModuleExport, 5002
// KeyVal, 5003 MiniLangCompiled, 5004 Patrun, 5005 DBConn, 5006
// RemoteTable).
var TKVStore = registerKVStoreType()

func registerKVStoreType() *eng.Type {
	t, err := eng.Builtin.RegisterExternalBuiltin("Ideal/KVStore", 5007, nil)
	if err != nil {
		recordTypeInitErr(fmt.Errorf("native_kvstore: register Ideal/KVStore: %w", err))
	}
	return t
}

// KVStore is the payload behind a KVStore value: an open backend plus
// display metadata. The backend rides as a pointer in an
// ExtensionPayload, so every copy shares one underlying store (and one
// Close). Any credentials in a connection string are kept only as the
// redacted Label.
type KVStore struct {
	Backend KeyValBackend
	Label   string // redacted identity for display
	Kind    string // backend kind: memory / file / redis / mongo / s3
	closed  bool
}

// NewKVStore wraps an open backend as a KVStore value.
func NewKVStore(s *KVStore) Value {
	return eng.NewExtension(TKVStore, s)
}

// AsKVStore extracts the *KVStore payload from a value, if present.
func AsKVStore(v Value) (*KVStore, bool) {
	ep, ok := v.Data.(eng.ExtensionPayload)
	if !ok {
		return nil, false
	}
	s, ok := ep.Body.(*KVStore)
	return s, ok
}

// Close closes the underlying backend once; further calls are no-ops.
func (s *KVStore) Close() error {
	if s == nil || s.closed || s.Backend == nil {
		return nil
	}
	s.closed = true
	return s.Backend.Close()
}

// OpenKVStore opens a backend of the given kind and wraps it as a
// *KVStore with a redacted label.
func OpenKVStore(kind, dsn string) (*KVStore, error) {
	backend, err := OpenKVBackend(kind, dsn)
	if err != nil {
		return nil, err
	}
	return &KVStore{Backend: backend, Label: redactKVLabel(kind, dsn), Kind: backend.Kind()}, nil
}

// redactKVLabel produces a display-safe label for a store. An empty DSN
// (memory) yields just the kind; URL DSNs are password-stripped.
func redactKVLabel(kind, dsn string) string {
	if dsn == "" {
		return kind
	}
	return kind + ":" + redactConnString(dsn)
}
