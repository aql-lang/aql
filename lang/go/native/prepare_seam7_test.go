package native

import (
	"errors"
	"strings"
	"testing"

	udk "github.com/voxgig/udk/go"
)

// Seam-7 coverage for prepare.go (design/TEST-SEAMS.10.md).

type fakePrepSDK struct {
	res map[string]any
	err error
}

func (s *fakePrepSDK) Entity(string, map[string]any) udk.UniversalEntity { return nil }
func (s *fakePrepSDK) Prepare(map[string]any) (map[string]any, error)    { return s.res, s.err }
func (s *fakePrepSDK) Direct(map[string]any) (map[string]any, error)     { return nil, nil }

func TestSeam7PrepareAPIHandler(t *testing.T) {
	r := seam5Reg(t)
	sdk := &fakePrepSDK{}
	r.SDKCache["fakespec"] = sdk

	// getSDK failure: missing spec.
	if _, err := prepareAPIHandler([]Value{noSpecMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "spec") {
		t.Fatalf("expected missing-spec error, got %v", err)
	}

	// SDK Prepare failure.
	sdk.err = errors.New("prep boom")
	if _, err := prepareAPIHandler([]Value{apiMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "prep boom") {
		t.Fatalf("expected prepare error, got %v", err)
	}

	// Result-conversion failure: an unconvertible value in the fetchdef.
	sdk.err = nil
	sdk.res = map[string]any{"bad": struct{}{}}
	if _, err := prepareAPIHandler([]Value{apiMap6()}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "converting result") {
		t.Fatalf("expected conversion error, got %v", err)
	}

	// Positive: a convertible fetchdef round-trips.
	sdk.res = map[string]any{"path": "/x", "method": "GET"}
	out, err := prepareAPIHandler([]Value{apiMap6()}, nil, nil, r)
	if err != nil || len(out) != 1 {
		t.Fatalf("prepareAPIHandler positive: %v / %v", out, err)
	}
}
