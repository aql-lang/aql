package aqleng

// Format encodes and decodes file content for a named representation
// (text, json, csv, …). The engine itself never instantiates a Format —
// the host package registers concrete implementations into
// Registry.Formats and the read/write words look them up by name.
type Format interface {
	Decode(content string) ([]Value, error)
	Encode(v Value) (string, error)
}
