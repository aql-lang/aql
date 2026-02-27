package object

import "testing"

func TestObjectInspect(t *testing.T) {
	tests := []struct {
		name     string
		obj      Object
		expected string
	}{
		{"integer", &Integer{Value: 42}, "42"},
		{"boolean true", &Boolean{Value: true}, "true"},
		{"boolean false", &Boolean{Value: false}, "false"},
		{"null", &Null{}, "null"},
		{"error", &Error{Message: "oops"}, "ERROR: oops"},
		{"string", &String{Value: "hello"}, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.obj.Inspect()
			if got != tt.expected {
				t.Errorf("Inspect() = %q, want %q", got, tt.expected)
			}
		})
	}
}
