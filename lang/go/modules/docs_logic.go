package modules

func init() {
	registerDocs("aql:logic-util", map[string]string{
		"iff":     "Biconditional: true when both operands are equal.",
		"implies": "Logical implication of two booleans.",
		"nand":    "Logical NAND: not (a and b).",
		"nor":     "Logical NOR: not (a or b).",
		"xnor":    "Logical XNOR (iff alias): true when operands are equal.",
	})
}
