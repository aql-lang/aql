package engine

import "strings"

func RegisterUpper(r *Registry) {
	registerUnaryStringWord(r, "upper", strings.ToUpper)
}
