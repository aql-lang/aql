package evaluator

import (
	"github.com/metsitaba/voxgig-exp/aql/internal/ast"
	"github.com/metsitaba/voxgig-exp/aql/internal/object"
)

// NULL is a shared Null singleton.
var NULL = &object.Null{}

// Eval evaluates an AST node and returns the resulting object.
// Stub: returns NULL for all input.
func Eval(node ast.Node) object.Object {
	switch node := node.(type) {
	case *ast.Program:
		return evalProgram(node)
	}
	return NULL
}

func evalProgram(program *ast.Program) object.Object {
	var result object.Object
	result = NULL
	for _, stmt := range program.Statements {
		result = Eval(stmt)
	}
	return result
}
