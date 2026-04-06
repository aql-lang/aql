package engine

import "strings"

func registerLower(r *Registry) {
	registerUnaryStringWord(r, "lower", strings.ToLower)
}
