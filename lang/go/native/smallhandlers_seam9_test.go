package native

import (
	"errors"
	"strings"
	"testing"
)

// Seam-9 coverage (W9_nativeC) for the small struct-util / xml / sort
// handler error arms. Handlers are driven directly with crafted args
// (design/TEST-SEAMS.10.md): wrong-type inputs and the structConvert seam.

// --- native_xml.go: wrong-type rejection arms ---

func TestW9XmlHandlersWrongType(t *testing.T) {
	r := seam5Reg(t)

	// elemHandler: non-Xml element rejected (63.9,65.3).
	if _, err := elemHandler([]Value{NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "xml-elem") {
		t.Fatalf("elemHandler: expected xml-elem error, got %v", err)
	}
	// textHandler: non-Xml element rejected (76.26,78.3).
	if _, err := textHandler([]Value{NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "xml-text") {
		t.Fatalf("textHandler: expected xml-text error, got %v", err)
	}
	// xmlAttrHandler: non-Xml element (args[1]) rejected (108.9,110.3).
	if _, err := xmlAttrHandler([]Value{NewString("k"), NewInteger(1)}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "xml-attr") {
		t.Fatalf("xmlAttrHandler: expected xml-attr error, got %v", err)
	}
}

// --- getpath.go / merge.go: structConvert error arms ---

func TestW9GetpathMergeConvertError(t *testing.T) {
	r := seam5Reg(t)

	om := NewOrderedMap()
	om.Set("a", NewInteger(1))
	data := NewMap(om)

	// Positive: getpath reads a nested value.
	if _, err := getpathHandler([]Value{NewString("a"), data}, nil, nil, r); err != nil {
		t.Fatalf("getpathHandler positive: %v", err)
	}

	saved := structConvert
	t.Cleanup(func() { structConvert = saved })
	structConvert = func(any) (Value, error) { return Value{}, errors.New("boom-convert") }

	// getpath.go:23.16,25.3 — convert-back failure wrapped with "getpath:".
	if _, err := getpathHandler([]Value{NewString("a"), data}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "getpath:") {
		t.Fatalf("getpathHandler convert error: got %v", err)
	}

	// merge.go:21.16,23.3 — convert-back failure wrapped with "merge:".
	if _, err := mergeHandler([]Value{data, data}, nil, nil, r); err == nil ||
		!strings.Contains(err.Error(), "merge:") {
		t.Fatalf("mergeHandler convert error: got %v", err)
	}
}
