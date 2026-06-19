//go:build !js

package native

import "strings"

// openExternalKV opens a non-memory backend on the native build. The
// file backend (bbolt) ships now; redis/mongo/s3 are added in later
// phases and report unknown until then.
func openExternalKV(kind, dsn string) (KeyValBackend, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "file", "bolt", "bbolt":
		return NewFileKVStore(dsn)
	case "redis", "valkey":
		return NewRedisKVStore(dsn)
	case "mongo", "mongodb":
		return NewMongoKVStore(dsn)
	case "s3", "minio":
		return NewS3KVStore(dsn)
	default:
		return nil, kvUnknownKind(kind)
	}
}

// canonicalExternalKVKind canonicalizes a non-memory backend kind.
func canonicalExternalKVKind(kind string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "file", "bolt", "bbolt":
		return "file", true
	case "redis", "valkey":
		return "redis", true
	case "mongo", "mongodb":
		return "mongo", true
	case "s3", "minio":
		return "s3", true
	default:
		return "", false
	}
}
