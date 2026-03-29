package codegen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"

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

	// Don't fail on type-check errors — generated code may reference
	// symbols that don't exist yet (e.g. routes.Assets on first run).
	// The AST and partial type info are still usable for extraction.

	// Start with packages that import thx (these contain routes).
	// Also collect their direct imports so that references to
	// non-thx packages (e.g. asset packages) can be resolved.
	all := make(map[string]*packages.Package)
	for _, pkg := range pkgs {
		if !importsThx(pkg) {
			continue
		}
		all[pkg.PkgPath] = pkg
		for _, imp := range pkg.Imports {
			if _, ok := all[imp.PkgPath]; !ok {
				all[imp.PkgPath] = imp
			}
		}
	}

	result := make([]*packages.Package, 0, len(all))
	for _, p := range all {
		result = append(result, p)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].PkgPath < result[j].PkgPath
	})

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
