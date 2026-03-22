package codegen

import (
	"fmt"

	"golang.org/x/tools/go/packages"
)

const thxPkgPath = "github.com/go-thx/thx"

func LoadPackages(dir, pattern string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedImports |
			packages.NeedDeps,
		Dir: dir,
	}

	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}

	var errs []error
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			errs = append(errs, e)
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("package errors: %v", errs)
	}

	var filtered []*packages.Package
	for _, pkg := range pkgs {
		if importsThx(pkg) {
			filtered = append(filtered, pkg)
		}
	}

	return filtered, nil
}

func importsThx(pkg *packages.Package) bool {
	for path := range pkg.Imports {
		if path == thxPkgPath || path == thxPkgPath+"/auth" {
			return true
		}
	}
	return false
}
