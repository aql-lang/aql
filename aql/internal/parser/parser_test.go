package parser

import (
	"testing"

	"github.com/metsitaba/voxgig-exp/aql/internal/lexer"
)

func TestParseProgram(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantStatements int
		wantErrors     int
	}{
		{
			name:           "empty input",
			input:          "",
			wantStatements: 0,
			wantErrors:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.New(tt.input)
			p := New(l)
			program := p.ParseProgram()

			if len(p.Errors()) != tt.wantErrors {
				t.Errorf("got %d errors, want %d: %v", len(p.Errors()), tt.wantErrors, p.Errors())
			}
			if len(program.Statements) != tt.wantStatements {
				t.Errorf("got %d statements, want %d", len(program.Statements), tt.wantStatements)
			}
		})
	}
}
