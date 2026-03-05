package aql

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/fileops"
	"github.com/metsitaba/voxgig-exp/aql/internal/parser"
)

// FileOps is the interface for file system operations used by read/write words.
type FileOps = fileops.FileOps

// Format handles encoding and decoding file content for a specific format.
type Format = engine.Format

// NewMemFileOps creates an in-memory file system for testing.
func NewMemFileOps() *fileops.MemFileOps {
	return fileops.NewMem()
}

// AQL is an independent AQL execution instance.
// Each instance has its own state (set/get storage is isolated).
// Create multiple instances with New() for independent execution contexts.
type AQL struct {
	registry *engine.Registry
}

// New creates a new AQL instance with built-in functions.
func New() *AQL {
	return &AQL{registry: engine.DefaultRegistry()}
}

// SetFileOps replaces the file operations implementation used by read/write.
func (a *AQL) SetFileOps(ops FileOps) {
	a.registry.SetFileOps(ops)
}

// RegisterFormat adds or replaces a format in the format registry.
// Formats are used by read/write words via the {fmt:"name"} option.
func (a *AQL) RegisterFormat(name string, f Format) {
	a.registry.Formats[name] = f
}

// Run parses and executes an AQL source string.
// The source may span multiple lines; newlines and tabs are treated as
// whitespace (equivalent to spaces).
//
// Returns the result stack as Go values:
//   - int64 for integers
//   - string for strings
//
// State from set/get persists across multiple Run calls on the same instance.
func (a *AQL) Run(src string) ([]any, error) {
	values, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}

	eng := engine.New(a.registry)
	result, err := eng.Run(values)
	if err != nil {
		return nil, err
	}

	out := make([]any, len(result))
	for i, v := range result {
		switch {
		case v.VType.Matches(engine.TInteger):
			out[i] = v.AsInteger()
		case v.VType.Matches(engine.TString):
			out[i] = v.AsString()
		default:
			out[i] = v.String()
		}
	}
	return out, nil
}
