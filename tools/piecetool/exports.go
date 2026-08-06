package main

// exports mode: print every exported package-scope name of a module.
//
//	piecetool -exports <module dir>

import (
	"fmt"

	"golang.org/x/tools/go/packages"
)

func exportsMode(dir string) error {
	cfg := &packages.Config{Mode: packages.NeedTypes | packages.NeedName, Dir: dir}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return fmt.Errorf("load %s: %w", dir, err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		return fmt.Errorf("load %s: package has type errors", dir)
	}
	for _, n := range pkgs[0].Types.Scope().Names() {
		if o := pkgs[0].Types.Scope().Lookup(n); o.Exported() {
			fmt.Println(n)
		}
	}
	return nil
}
