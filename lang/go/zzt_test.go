package lang

import (
	"fmt"
	"os"
	"testing"
)

func TestZZT(t *testing.T) {
	os.Setenv("ZZTRACE", "1")
	defer os.Unsetenv("ZZTRACE")
	src := `def g fn [[c:Function][List][[1 2] each [(1 2 c)]]] g (([a:Integer b:Integer]=>[a add b])/r)`
	a, _ := New()
	got, compiled, err := a.RunCompiled(src)
	fmt.Printf("compiled=%v got=%v err=%v\n", compiled, got, err)
}
