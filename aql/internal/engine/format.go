package engine

import (
	"fmt"
	"strings"

	jsonic "github.com/jsonicjs/jsonic/go"
)

// Format handles encoding and decoding file content for a specific format.
type Format interface {
	Decode(content string) ([]Value, error)
	Encode(v Value) (string, error)
}

// TextFormat handles plain text (no parsing).
type TextFormat struct{}

func (f *TextFormat) Decode(content string) ([]Value, error) {
	return []Value{NewString(content)}, nil
}

func (f *TextFormat) Encode(v Value) (string, error) {
	if v.VType.Matches(TString) {
		return v.AsString(), nil
	}
	return v.String(), nil
}

// JSONFormat handles strict JSON via jsonic.Parse (standard JSON is valid jsonic).
type JSONFormat struct{}

func (f *JSONFormat) Decode(content string) ([]Value, error) {
	result, err := jsonic.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	v, err := jsonicToValue(result)
	if err != nil {
		return nil, err
	}
	return []Value{v}, nil
}

func (f *JSONFormat) Encode(v Value) (string, error) {
	return valueToJsonic(v), nil
}

// JsonicFormat handles relaxed JSON (unquoted keys, trailing commas, etc.).
type JsonicFormat struct{}

func (f *JsonicFormat) Decode(content string) ([]Value, error) {
	j := jsonic.Make()
	result, err := j.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("invalid jsonic: %w", err)
	}
	if result == nil {
		return []Value{NewTypeLiteral(TNone)}, nil
	}
	v, err := jsonicToValue(result)
	if err != nil {
		return nil, err
	}
	return []Value{v}, nil
}

func (f *JsonicFormat) Encode(v Value) (string, error) {
	return valueToJsonic(v), nil
}

// LinesFormat splits/joins on newlines, producing/consuming a list of strings.
type LinesFormat struct{}

func (f *LinesFormat) Decode(content string) ([]Value, error) {
	lines := strings.Split(content, "\n")
	elems := make([]Value, len(lines))
	for i, line := range lines {
		elems[i] = NewString(line)
	}
	return []Value{NewList(elems)}, nil
}

func (f *LinesFormat) Encode(v Value) (string, error) {
	if v.VType.Equal(TList) {
		if elems, ok := v.Data.([]Value); ok {
			parts := make([]string, len(elems))
			for i, e := range elems {
				if e.VType.Matches(TString) {
					parts[i] = e.AsString()
				} else {
					parts[i] = e.String()
				}
			}
			return strings.Join(parts, "\n"), nil
		}
	}
	return v.String(), nil
}

// DefaultFormats returns the built-in format registry.
func DefaultFormats() map[string]Format {
	return map[string]Format{
		"text":   &TextFormat{},
		"json":   &JSONFormat{},
		"jsonic": &JsonicFormat{},
		"lines":  &LinesFormat{},
	}
}
