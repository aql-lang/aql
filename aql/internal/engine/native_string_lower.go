package engine

import "strings"

func RegisterLower(r *Registry) {
	registerUnaryStringWord(r, "lower", strings.ToLower)
}
