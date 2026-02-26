package object

import "fmt"

// ObjectType identifies the kind of object.
type ObjectType string

const (
	INTEGER_OBJ ObjectType = "INTEGER"
	BOOLEAN_OBJ ObjectType = "BOOLEAN"
	NULL_OBJ    ObjectType = "NULL"
	ERROR_OBJ   ObjectType = "ERROR"
	STRING_OBJ  ObjectType = "STRING"
)

// Object is the interface all hay values implement.
type Object interface {
	Type() ObjectType
	Inspect() string
}

// Integer wraps a Go int64.
type Integer struct {
	Value int64
}

func (i *Integer) Type() ObjectType { return INTEGER_OBJ }
func (i *Integer) Inspect() string  { return fmt.Sprintf("%d", i.Value) }

// Boolean wraps a Go bool.
type Boolean struct {
	Value bool
}

func (b *Boolean) Type() ObjectType { return BOOLEAN_OBJ }
func (b *Boolean) Inspect() string  { return fmt.Sprintf("%t", b.Value) }

// Null represents the absence of a value.
type Null struct{}

func (n *Null) Type() ObjectType { return NULL_OBJ }
func (n *Null) Inspect() string  { return "null" }

// Error represents a runtime error.
type Error struct {
	Message string
}

func (e *Error) Type() ObjectType { return ERROR_OBJ }
func (e *Error) Inspect() string  { return "ERROR: " + e.Message }

// String wraps a Go string.
type String struct {
	Value string
}

func (s *String) Type() ObjectType { return STRING_OBJ }
func (s *String) Inspect() string  { return s.Value }
