package main

// exports mode: print every exported package-scope name of a module.
//
//	piecetool -exports <module dir>

import (
	"fmt"
	"os"

	"golang.org/x/tools/go/packages"
)

func exportsMode(dir string) {
	cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedName, Dir: dir}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		panic(err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		os.Exit(1)
	}
	for _, n := range pkgs[0].Types.Scope().Names() {
		if o := pkgs[0].Types.Scope().Lookup(n); o.Exported() {
			fmt.Println(n)
		}
	}
}
