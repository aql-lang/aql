package modules

func init() {
	registerDocs("boru:logic-util", map[string]string{
		"iff":     "Biconditional: true when both operands are equal.",
		"implies": "Logical implication of two booleans.",
		"nand":    "Logical NAND: not (a and b).",
		"nor":     "Logical NOR: not (a or b).",
		"xnor":    "Logical XNOR (iff alias): true when operands are equal.",
	})

	// The connective words read in the BORU swap convention (args[1] op
	// args[0]), which is the one thing about them a reader can get wrong
	// and a signature of [Boolean Boolean] cannot show.
	registerExamples("boru:logic-util", map[string][]string{
		"nand": {`LogicUtil.nand true true                         ;# false — not (a and b)`},
		"nor":  {`LogicUtil.nor false false                        ;# true — not (a or b)`},
		"xnor": {`LogicUtil.xnor true false                        ;# false — true iff both agree`},
		"iff":  {`LogicUtil.iff true true                          ;# true — same as xnor`},
		"implies": {
			`LogicUtil.implies true false                     ;# true`,
			`LogicUtil.implies false true                     ;# false`,
			`;# Reads RIGHT to left, like every binary BORU word: "implies a b"`,
			`;# is b ⇒ a. The swap form "b LogicUtil.implies a" reads forwards.`,
		},
	})
}
