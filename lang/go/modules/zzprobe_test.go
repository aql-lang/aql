package modules_test
import (
	"os"; "strings"; "testing"
	"github.com/aql-lang/aql/eng/go/engine"
	"github.com/aql-lang/aql/eng/go/parser"
	"github.com/aql-lang/aql/lang/go/modules"
	"github.com/aql-lang/aql/lang/go/native"
)
func TestZZProbe(t *testing.T) {
	cases := []string{
		`import "aql:math"  max 3 7`,
		`import "aql:math"  max [3 7]`,
		`import "aql:math"  3 max 7`,
		`import "aql:math"  pi`,
		`import "aql:array"  length [10 20 30]`,
		`import "aql:array"  [10 20 30] length`,
		`def m import "aql:math"  m.max 3 7`,
		`import "aql:math" as m  m.max 3 7`,
	}
	var b strings.Builder
	for _, src := range cases {
		r, _ := native.DefaultRegistry(); modules.InstallResolver(r)
		toks, perr := parser.Parse(src)
		if perr != nil { b.WriteString(src+" || PARSE "+perr.Error()+"\n"); continue }
		out, rerr := engine.NewTop(r).Run(toks)
		if rerr != nil { b.WriteString(src+" || RUN "+strings.SplitN(rerr.Error(),"\n",2)[0]+"\n"); continue }
		ps := make([]string, len(out)); for i, v := range out { ps[i]=v.String() }
		b.WriteString(src+" || => "+strings.Join(ps," ")+"\n")
	}
	os.WriteFile("/tmp/wprobe.txt", []byte(b.String()), 0o644)
}
