//go:build kvintegration && !js

package native

import (
	"os"
	"testing"
)

// Live backend integration tests. They are excluded from the normal
// build and run only with:
//
//	go test -tags kvintegration ./lang/go/native/ \
//	    -run KVIntegration
//
// Each backend is exercised against a real server addressed by an
// environment variable; a backend with no DSN set is skipped. Intended
// for CI jobs that stand up Redis / MongoDB / MinIO (e.g. via
// docker-compose or testcontainers).
//
//	AQL_KV_REDIS_DSN  redis://host:6379/0
//	AQL_KV_MONGO_DSN  mongodb://host:27017/aqltest
//	AQL_KV_S3_DSN     s3://key:secret@host:9000/aql-test?ssl=false

// runBackendBattery exercises the full KeyValBackend contract.
func runBackendBattery(t *testing.T, s KeyValBackend) {
	t.Helper()
	defer s.Close()

	m := NewOrderedMap()
	m.Set("name", NewString("ann"))
	if err := s.Set("kvit:user:1", NewMap(m)); err != nil {
		t.Fatalf("set map: %v", err)
	}
	if err := s.Set("kvit:user:2", NewInteger(7)); err != nil {
		t.Fatalf("set int: %v", err)
	}
	t.Cleanup(func() { _, _ = s.Delete("kvit:user:1"); _, _ = s.Delete("kvit:user:2") })

	got, ok, err := s.Get("kvit:user:1")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	gm, merr := AsMap(got)
	if merr != nil {
		t.Fatalf("expected a map, got %v", got.Parent)
	}
	if v, _ := gm.Get("name"); ValToString(v) != "ann" {
		t.Errorf("name: got %q want ann", ValToString(v))
	}

	if _, ok, _ := s.Get("kvit:absent"); ok {
		t.Error("absent key should not be present")
	}

	keys, err := s.Keys("kvit:user:")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "kvit:user:1" || keys[1] != "kvit:user:2" {
		t.Errorf("keys: got %v want [kvit:user:1 kvit:user:2]", keys)
	}

	if h, _ := s.Has("kvit:user:2"); !h {
		t.Error("has user:2 should be true")
	}
	if existed, _ := s.Delete("kvit:user:2"); !existed {
		t.Error("delete should report user:2 existed")
	}
	if h, _ := s.Has("kvit:user:2"); h {
		t.Error("user:2 should be gone after delete")
	}
}

func TestKVIntegrationRedis(t *testing.T) {
	dsn := os.Getenv("AQL_KV_REDIS_DSN")
	if dsn == "" {
		t.Skip("set AQL_KV_REDIS_DSN to run the Redis integration test")
	}
	s, err := NewRedisKVStore(dsn)
	if err != nil {
		t.Fatalf("open redis: %v", err)
	}
	runBackendBattery(t, s)
}

func TestKVIntegrationMongo(t *testing.T) {
	dsn := os.Getenv("AQL_KV_MONGO_DSN")
	if dsn == "" {
		t.Skip("set AQL_KV_MONGO_DSN to run the MongoDB integration test")
	}
	s, err := NewMongoKVStore(dsn)
	if err != nil {
		t.Fatalf("open mongo: %v", err)
	}
	runBackendBattery(t, s)
}

func TestKVIntegrationS3(t *testing.T) {
	dsn := os.Getenv("AQL_KV_S3_DSN")
	if dsn == "" {
		t.Skip("set AQL_KV_S3_DSN to run the S3 integration test")
	}
	s, err := NewS3KVStore(dsn)
	if err != nil {
		t.Fatalf("open s3: %v", err)
	}
	runBackendBattery(t, s)
}
