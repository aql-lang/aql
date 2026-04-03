package test

import (
	"testing"
)

// aliasCase defines a single alias test: an original AQL expression
// and the same expression using a def alias.
type aliasCase struct {
	name      string            // test name
	files     map[string]string // optional virtual files
	defStep   string            // e.g. "def plus [add]"
	origSteps []string          // expression with original name
	aliaSteps []string          // expression with alias name
}

// runAliasTest runs original and aliased expressions and asserts they produce
// identical results (same count and string representation).
func runAliasTest(t *testing.T, tc aliasCase) {
	t.Helper()

	origResult, err := runNativeSteps(t, tc.files, tc.origSteps)
	if err != nil {
		t.Fatalf("%s: original failed: %v", tc.name, err)
	}

	allAlias := append([]string{tc.defStep}, tc.aliaSteps...)
	aliasResult, err := runNativeSteps(t, tc.files, allAlias)
	if err != nil {
		t.Fatalf("%s: alias failed: %v", tc.name, err)
	}

	if len(origResult) != len(aliasResult) {
		t.Fatalf("%s: result count mismatch: orig=%d alias=%d",
			tc.name, len(origResult), len(aliasResult))
	}
	for i := range origResult {
		o := origResult[i].String()
		a := aliasResult[i].String()
		if o != a {
			t.Errorf("%s: result[%d] mismatch: orig=%s alias=%s",
				tc.name, i, o, a)
		}
	}
}

// ==========================================================================
// Builtin: Arithmetic
// ==========================================================================

func TestAliasAdd(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "add",
		defStep:   "def plus [add]",
		origSteps: []string{"3 add 5"},
		aliaSteps: []string{"3 plus 5"},
	})
}

func TestAliasSub(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "sub",
		defStep:   "def minus [sub]",
		origSteps: []string{"10 sub 3"},
		aliaSteps: []string{"10 minus 3"},
	})
}

func TestAliasMul(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "mul",
		defStep:   "def times [mul]",
		origSteps: []string{"4 mul 5"},
		aliaSteps: []string{"4 times 5"},
	})
}

func TestAliasDiv(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "div",
		defStep:   "def divide [div]",
		origSteps: []string{"20 div 4"},
		aliaSteps: []string{"20 divide 4"},
	})
}

func TestAliasMod(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "mod",
		defStep:   "def modulo [mod]",
		origSteps: []string{"10 mod 3"},
		aliaSteps: []string{"10 modulo 3"},
	})
}

func TestAliasAbs(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "math.abs",
		defStep:   "def magnitude fn [[Integer] [Integer] [math.abs]]",
		origSteps: []string{"-5 math.abs"},
		aliaSteps: []string{"-5 magnitude"},
	})
}

func TestAliasNegate(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "math.negate",
		defStep:   "def neg fn [[Integer] [Integer] [math.negate]]",
		origSteps: []string{"5 math.negate"},
		aliaSteps: []string{"5 neg"},
	})
}

func TestAliasMin(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "math.min",
		defStep:   "def smallest fn [[Integer Integer] [Integer] [math.min]]",
		origSteps: []string{"5 3 math.min"},
		aliaSteps: []string{"5 smallest 3"},
	})
}

func TestAliasMax(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "math.max",
		defStep:   "def largest fn [[Integer Integer] [Integer] [math.max]]",
		origSteps: []string{"5 3 math.max"},
		aliaSteps: []string{"5 largest 3"},
	})
}

// ==========================================================================
// Builtin: Stack Manipulation
// ==========================================================================

func TestAliasDup(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "dup",
		defStep:   "def mydup [dup]",
		origSteps: []string{"5 dup"},
		aliaSteps: []string{"5 mydup"},
	})
}

func TestAliasSwap(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "swap",
		defStep:   "def myswap [swap]",
		origSteps: []string{"do [1 2 swap]"},
		aliaSteps: []string{"do [1 2 myswap]"},
	})
}

func TestAliasDrop(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "drop",
		defStep:   "def mydrop [drop]",
		origSteps: []string{"do [1 2 drop]"},
		aliaSteps: []string{"do [1 2 mydrop]"},
	})
}

func TestAliasOver(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "over",
		defStep:   "def myover [over]",
		origSteps: []string{"do [1 2 over]"},
		aliaSteps: []string{"do [1 2 myover]"},
	})
}

func TestAliasRot(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "rot",
		defStep:   "def myrot [rot]",
		origSteps: []string{"do [1 2 3 rot]"},
		aliaSteps: []string{"do [1 2 3 myrot]"},
	})
}

func TestAliasNip(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "nip",
		defStep:   "def mynip [nip]",
		origSteps: []string{"do [1 2 nip]"},
		aliaSteps: []string{"do [1 2 mynip]"},
	})
}

func TestAliasTuck(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "tuck",
		defStep:   "def mytuck [tuck]",
		origSteps: []string{"do [1 2 tuck]"},
		aliaSteps: []string{"do [1 2 mytuck]"},
	})
}

func TestAlias2Dup(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "2dup",
		defStep:   "def my2dup [2dup]",
		origSteps: []string{"do [1 2 2dup]"},
		aliaSteps: []string{"do [1 2 my2dup]"},
	})
}

func TestAlias2Swap(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "2swap",
		defStep:   "def my2swap [2swap]",
		origSteps: []string{"do [1 2 3 4 2swap]"},
		aliaSteps: []string{"do [1 2 3 4 my2swap]"},
	})
}

func TestAlias2Drop(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "2drop",
		defStep:   "def my2drop [2drop]",
		origSteps: []string{"do [1 2 3 2drop]"},
		aliaSteps: []string{"do [1 2 3 my2drop]"},
	})
}

func TestAlias2Over(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "2over",
		defStep:   "def my2over [2over]",
		origSteps: []string{"do [1 2 3 4 2over]"},
		aliaSteps: []string{"do [1 2 3 4 my2over]"},
	})
}

func TestAliasDepth(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "depth",
		defStep:   "def mydepth [depth]",
		origSteps: []string{"do [1 2 3 depth]"},
		aliaSteps: []string{"do [1 2 3 mydepth]"},
	})
}

func TestAliasPick(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "pick",
		defStep:   "def mypick [pick]",
		origSteps: []string{"do [10 20 30 0 pick]"},
		aliaSteps: []string{"do [10 20 30 0 mypick]"},
	})
}

func TestAliasRoll(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "roll",
		defStep:   "def myroll [roll]",
		origSteps: []string{"do [1 2 3 2 roll]"},
		aliaSteps: []string{"do [1 2 3 2 myroll]"},
	})
}

// ==========================================================================
// Builtin: Boolean
// ==========================================================================

func TestAliasAnd(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "and",
		defStep:   "def myand [and]",
		origSteps: []string{"true and false"},
		aliaSteps: []string{"true myand false"},
	})
}

func TestAliasOr(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "or",
		defStep:   "def myor [or]",
		origSteps: []string{"false or true"},
		aliaSteps: []string{"false myor true"},
	})
}

func TestAliasXor(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "xor",
		defStep:   "def myxor [xor]",
		origSteps: []string{"true xor false"},
		aliaSteps: []string{"true myxor false"},
	})
}

func TestAliasNand(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "nand",
		defStep:   "def mynand [nand]",
		origSteps: []string{"true nand true"},
		aliaSteps: []string{"true mynand true"},
	})
}

func TestAliasImplies(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "implies",
		defStep:   "def myimplies [implies]",
		origSteps: []string{"true implies false"},
		aliaSteps: []string{"true myimplies false"},
	})
}

func TestAliasNot(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "not",
		defStep:   "def mynot [not]",
		origSteps: []string{"true not"},
		aliaSteps: []string{"true mynot"},
	})
}

// ==========================================================================
// Builtin: Comparison
// ==========================================================================

func TestAliasEq(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "eq",
		defStep:   "def same [eq]",
		origSteps: []string{"5 eq 5"},
		aliaSteps: []string{"5 same 5"},
	})
}

func TestAliasNeq(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "neq",
		defStep:   "def diff [neq]",
		origSteps: []string{"5 neq 3"},
		aliaSteps: []string{"5 diff 3"},
	})
}

func TestAliasLt(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "lt",
		defStep:   "def less [lt]",
		origSteps: []string{"3 lt 5"},
		aliaSteps: []string{"3 less 5"},
	})
}

func TestAliasGt(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "gt",
		defStep:   "def more [gt]",
		origSteps: []string{"5 gt 3"},
		aliaSteps: []string{"5 more 3"},
	})
}

func TestAliasLte(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "lte",
		defStep:   "def atmost [lte]",
		origSteps: []string{"5 lte 5"},
		aliaSteps: []string{"5 atmost 5"},
	})
}

func TestAliasGte(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "gte",
		defStep:   "def atleast [gte]",
		origSteps: []string{"5 gte 3"},
		aliaSteps: []string{"5 atleast 3"},
	})
}

func TestAliasDeq(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "deq",
		defStep:   "def deepeq [deq]",
		origSteps: []string{"{a:1} deq {a:1}"},
		aliaSteps: []string{"{a:1} deepeq {a:1}"},
	})
}

// ==========================================================================
// Builtin: String
// ==========================================================================

func TestAliasUpper(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "upper",
		defStep:   "def shout [upper]",
		origSteps: []string{`"hello" upper`},
		aliaSteps: []string{`"hello" shout`},
	})
}

func TestAliasLower(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "lower",
		defStep:   "def whisper [lower]",
		origSteps: []string{`"HELLO" lower`},
		aliaSteps: []string{`"HELLO" whisper`},
	})
}

// ==========================================================================
// Builtin: Type
// ==========================================================================

func TestAliasTypeof(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "typeof",
		defStep:   "def mytype [typeof]",
		origSteps: []string{"42 typeof"},
		aliaSteps: []string{"42 mytype"},
	})
}

// ==========================================================================
// Builtin: Control flow
// ==========================================================================

func TestAliasIf(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "if",
		defStep:   "def myif [if]",
		origSteps: []string{"true if 42 0"},
		aliaSteps: []string{"true myif 42 0"},
	})
}

func TestAliasDo(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "do",
		defStep:   "def mydo [do]",
		origSteps: []string{"do [3 add 4]"},
		aliaSteps: []string{"mydo [3 add 4]"},
	})
}

// ==========================================================================
// Builtin: Storage
// ==========================================================================

func TestAliasSetGet(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "set",
		defStep:   "def myset [set]",
		origSteps: []string{`context set "x" 42 end context get "x"`},
		aliaSteps: []string{`context myset "x" 42 end context get "x"`},
	})
}

func TestAliasGet(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "get",
		defStep:   "def myget [get]",
		origSteps: []string{`context set "v" 42 end context get "v"`},
		aliaSteps: []string{`context set "v" 42 end context myget "v"`},
	})
}

// ==========================================================================
// Builtin: Higher-order
// ==========================================================================

func TestAliasCall(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "call",
		defStep:   "def mycall [call]",
		origSteps: []string{"5 [dup mul] call"},
		aliaSteps: []string{"5 [dup mul] mycall"},
	})
}

func TestAliasDblcall(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "dblcall",
		defStep:   "def mydbl [dblcall]",
		origSteps: []string{"[dup mul] 5 dblcall"},
		aliaSteps: []string{"[dup mul] 5 mydbl"},
	})
}

// ==========================================================================
// Native: clone
// ==========================================================================

func TestAliasClone(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "clone",
		defStep:   "def myclone [clone]",
		origSteps: []string{"{a:1 b:2} clone"},
		aliaSteps: []string{"{a:1 b:2} myclone"},
	})
}

// ==========================================================================
// Native: merge
// ==========================================================================

func TestAliasMerge(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "merge",
		defStep:   "def mymerge [merge]",
		origSteps: []string{"merge {a:1} {b:2}"},
		aliaSteps: []string{"mymerge {a:1} {b:2}"},
	})
}

// ==========================================================================
// Native: walk
// ==========================================================================

func TestAliasWalkNoCallback(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "walk (no callback)",
		defStep:   "def mywalk [walk]",
		origSteps: []string{"{a:1 b:2} walk"},
		aliaSteps: []string{"{a:1 b:2} mywalk"},
	})
}

func TestAliasWalkWithBefore(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "walk (before callback)",
		defStep:   "def mywalk [walk]",
		origSteps: []string{"{a:1 b:2} (fn [[m:Map] [Any] [m.value]]) walk"},
		aliaSteps: []string{"{a:1 b:2} (fn [[m:Map] [Any] [m.value]]) mywalk"},
	})
}

// ==========================================================================
// Native: transform
// ==========================================================================

func TestAliasTransform(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "transform",
		defStep:   "def mytransform [transform]",
		origSteps: []string{`{a:1 b:2} transform {a:"a"}`},
		aliaSteps: []string{`{a:1 b:2} mytransform {a:"a"}`},
	})
}

// ==========================================================================
// Native: getpath
// ==========================================================================

func TestAliasGetpath(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "getpath",
		defStep:   "def mygetpath [getpath]",
		origSteps: []string{`getpath {a:{b:42}} "a.b"`},
		aliaSteps: []string{`mygetpath {a:{b:42}} "a.b"`},
	})
}

// ==========================================================================
// Native: setpath
// ==========================================================================

func TestAliasSetpath(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "setpath",
		defStep:   "def mysetpath [setpath]",
		origSteps: []string{`{a:1} setpath "b" 99`},
		aliaSteps: []string{`{a:1} mysetpath "b" 99`},
	})
}

// ==========================================================================
// Native: validate
// ==========================================================================

func TestAliasValidate(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "validate",
		defStep:   "def myvalidate [validate]",
		origSteps: []string{`validate {name:"Alice" age:30} {name:"$STRING" age:"$NUMBER"}`},
		aliaSteps: []string{`myvalidate {name:"Alice" age:30} {name:"$STRING" age:"$NUMBER"}`},
	})
}

// ==========================================================================
// Native: inject
// ==========================================================================

func TestAliasInject(t *testing.T) {
	runAliasTest(t, aliasCase{
		name:      "inject",
		defStep:   "def myinject [inject]",
		origSteps: []string{"{a:`b`} inject {b:42}"},
		aliaSteps: []string{"{a:`b`} myinject {b:42}"},
	})
}

// ==========================================================================
// Native: list
// ==========================================================================

func TestAliasList(t *testing.T) {
	csv := "name,age\nAlice,30\nBob,25\n"
	runAliasTest(t, aliasCase{
		name:      "list",
		files:     map[string]string{"data.csv": csv},
		defStep:   "def mylist [list]",
		origSteps: []string{`def tbl [read "data.csv"]`, `list tbl`},
		aliaSteps: []string{`def tbl [read "data.csv"]`, `mylist tbl`},
	})
}

// ==========================================================================
// Native: create
// ==========================================================================

func TestAliasCreate(t *testing.T) {
	csv := "id,name\n1,Alice\n"
	runAliasTest(t, aliasCase{
		name:      "create",
		files:     map[string]string{"data.csv": csv},
		defStep:   "def mycreate [create]",
		origSteps: []string{`def tbl [read "data.csv"]`, `tbl create {id:"2" name:"Bob"}`},
		aliaSteps: []string{`def tbl [read "data.csv"]`, `tbl mycreate {id:"2" name:"Bob"}`},
	})
}

// ==========================================================================
// Native: load
// ==========================================================================

func TestAliasLoad(t *testing.T) {
	csv := "id,name\n1,Alice\n"
	runAliasTest(t, aliasCase{
		name:      "load",
		files:     map[string]string{"data.csv": csv},
		defStep:   "def myload [load]",
		origSteps: []string{`def tbl [read "data.csv"]`, `tbl load {id:"1"}`},
		aliaSteps: []string{`def tbl [read "data.csv"]`, `tbl myload {id:"1"}`},
	})
}

// ==========================================================================
// Native: update
// ==========================================================================

func TestAliasUpdate(t *testing.T) {
	csv := "id,name\n1,Alice\n"
	runAliasTest(t, aliasCase{
		name:      "update",
		files:     map[string]string{"data.csv": csv},
		defStep:   "def myupdate [update]",
		origSteps: []string{`def tbl [read "data.csv"]`, `tbl update {id:"1" name:"Alicia"}`},
		aliaSteps: []string{`def tbl [read "data.csv"]`, `tbl myupdate {id:"1" name:"Alicia"}`},
	})
}

// ==========================================================================
// Native: remove
// ==========================================================================

func TestAliasRemove(t *testing.T) {
	csv := "id,name\n1,Alice\n2,Bob\n"
	runAliasTest(t, aliasCase{
		name:      "remove",
		files:     map[string]string{"data.csv": csv},
		defStep:   "def myremove [remove]",
		origSteps: []string{`def tbl [read "data.csv"]`, `tbl remove {id:"1"}`},
		aliaSteps: []string{`def tbl [read "data.csv"]`, `tbl myremove {id:"1"}`},
	})
}
