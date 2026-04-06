package engine

import "strings"

func registerUpper(r *Registry) {
	registerUnaryStringWord(r, "upper", strings.ToUpper)
}
