//go:build js

package native

import "errors"

// External key-value backends rely on drivers and filesystem access that
// are unavailable in the wasm build, so only the in-memory store is
// offered there.
func openExternalKV(kind, _ string) (KeyValBackend, error) {
	return nil, errors.New("keyval backend " + kind + " is not supported in the wasm build (only memory)")
}

func canonicalExternalKVKind(string) (string, bool) { return "", false }
