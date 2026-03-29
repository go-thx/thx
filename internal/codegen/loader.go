package codegen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"

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
		Dir:       dir,
		ParseFile: func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
			return parser.ParseFile(fset, filename, src, parser.ParseComments)
		},
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

	// Collect all transitively imported packages so that
	// references to non-thx packages (e.g. asset packages) can be resolved.
	all := make(map[string]*packages.Package)
	var collect func(p *packages.Package)
	collect = func(p *packages.Package) {
		if _, ok := all[p.PkgPath]; ok {
			return
		}
		all[p.PkgPath] = p
		for _, imp := range p.Imports {
			collect(imp)
		}
	}
	for _, p := range pkgs {
		collect(p)
	}

	result := make([]*packages.Package, 0, len(all))
	for _, p := range all {
		result = append(result, p)
	}

	return result, nil
}

func importsThx(pkg *packages.Package) bool {
	for path := range pkg.Imports {
		if path == thxPkgPath || path == thxPkgPath+"/auth" {
			return true
		}
	}
	return false
}
