//go:build !js

package native

import "testing"

func TestCanonicalKVKind(t *testing.T) {
	cases := map[string]struct {
		want string
		ok   bool
	}{
		"memory":  {"memory", true},
		"mem":     {"memory", true},
		"":        {"memory", true},
		"file":    {"file", true},
		"bbolt":   {"file", true},
		"redis":   {"redis", true},
		"valkey":  {"redis", true},
		"mongo":   {"mongo", true},
		"mongodb": {"mongo", true},
		"s3":      {"s3", true},
		"minio":   {"s3", true},
		"oracle":  {"", false},
	}
	for kind, exp := range cases {
		got, ok := CanonicalKVKind(kind)
		if ok != exp.ok || got != exp.want {
			t.Errorf("CanonicalKVKind(%q): got (%q,%v) want (%q,%v)", kind, got, ok, exp.want, exp.ok)
		}
	}
}

func TestParseMongoDSN(t *testing.T) {
	db, coll, err := parseMongoDSN("mongodb://host:27017/mydb")
	if err != nil || db != "mydb" || coll != "kv" {
		t.Errorf("default collection: got (%q,%q,%v)", db, coll, err)
	}
	db, coll, err = parseMongoDSN("mongodb://host/mydb?collection=sessions")
	if err != nil || db != "mydb" || coll != "sessions" {
		t.Errorf("explicit collection: got (%q,%q,%v)", db, coll, err)
	}
	// Missing database path is an error.
	if _, _, err := parseMongoDSN("mongodb://host:27017"); err == nil {
		t.Error("expected error for mongo URL without a database path")
	}
}

func TestParseS3DSN(t *testing.T) {
	cfg, err := parseS3DSN("s3://ak:sk@minio.local:9000/mybucket?region=us-west-2&ssl=false")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.endpoint != "minio.local:9000" || cfg.bucket != "mybucket" ||
		cfg.region != "us-west-2" || cfg.secure || cfg.accessKey != "ak" || cfg.secretKey != "sk" {
		t.Errorf("parsed config mismatch: %+v", cfg)
	}
	// Defaults: TLS on, region us-east-1.
	cfg, err = parseS3DSN("s3://endpoint/bucket")
	if err != nil || !cfg.secure || cfg.region != "us-east-1" {
		t.Errorf("defaults: got %+v err %v", cfg, err)
	}
	// Missing bucket / endpoint error.
	if _, err := parseS3DSN("s3://endpoint"); err == nil {
		t.Error("expected error for s3 URL without a bucket")
	}
	if _, err := parseS3DSN("s3:///bucket"); err == nil {
		t.Error("expected error for s3 URL without an endpoint")
	}
}

func TestMongoS3ConstructionErrors(t *testing.T) {
	// These fail at DSN parsing, before any network I/O, so they stay
	// hermetic. (Live round-trips are in the kvintegration build.)
	if _, err := NewMongoKVStore("mongodb://host"); err == nil {
		t.Error("expected error: mongo URL without database")
	}
	if _, err := NewS3KVStore("s3://endpoint"); err == nil {
		t.Error("expected error: s3 URL without bucket")
	}
}
