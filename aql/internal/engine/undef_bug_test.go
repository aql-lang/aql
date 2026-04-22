package engine_test
import (
	"github.com/metsitaba/voxgig-exp/aql/internal/engine"
	"github.com/metsitaba/voxgig-exp/aql/internal/native"
	"testing"
)
func TestUndefBugNamedStringParams(t *testing.T) {
	r, err := engine.DefaultRegistry(native.Register)
	if err != nil {
		t.Fatal(err)
	}
	// def joiner fn [[a:string b:string c:string] [string] [a b add c add]] end
	// Named string params: body concatenates a+b+c via add.
	// All prefix: "A" "B" "C" joiner → nearest to joiner is "C"→sig[0](a),
	// "B"→sig[1](b), "A"→sig[2](c). Body: a b add c add → "CB" + "A" → "CBA".
	aParam := engine.NewOrderedMap()
	aParam.Set("a", engine.NewWord("String"))
	bParam := engine.NewOrderedMap()
	bParam.Set("b", engine.NewWord("String"))
	cParam := engine.NewOrderedMap()
	cParam.Set("c", engine.NewWord("String"))

	fnBody := engine.NewList([]engine.Value{
		engine.NewList([]engine.Value{engine.NewImplicitMap(aParam), engine.NewImplicitMap(bParam), engine.NewImplicitMap(cParam)}),
		engine.NewList([]engine.Value{engine.NewWord("String")}),
		engine.NewList([]engine.Value{engine.NewWord("a"), engine.NewWord("b"), engine.NewWord("add"), engine.NewWord("c"), engine.NewWord("add")}),
	})
	tokens := []engine.Value{
		engine.NewWord("def"), engine.NewWord("joiner"), engine.NewWord("fn"), fnBody, engine.NewWord("end"),
		engine.NewString("A"), engine.NewString("B"), engine.NewString("C"), engine.NewWord("joiner"),
	}
	result, err := engine.NewTop(r).Run(tokens)
	if err != nil {
		t.Fatalf("bug confirmed: %v", err)
	}
	_as0, _ := result[0].AsString()
	if len(result) != 1 || _as0 != "CBA" {
		t.Errorf("got %v, want [CBA]", result)
	}
}
