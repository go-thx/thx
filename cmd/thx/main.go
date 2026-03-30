package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-thx/thx/internal/codegen"
)

// main is the entry point for the thx code generation CLI.
// Usage: thx generate routes [-o <output-dir>] [<package-pattern>]
func main() {
	if len(os.Args) < 3 || os.Args[1] != "generate" || os.Args[2] != "routes" {
		fmt.Fprintln(os.Stderr, "usage: thx generate routes [-o <output-dir>] [<package-pattern>]")
		os.Exit(1)
	}

	fs := flag.NewFlagSet("generate routes", flag.ExitOnError)
	outputDir := fs.String("o", "gen/routes", "output directory for generated routes")
	if err := fs.Parse(os.Args[3:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	pattern := "./..."
	if fs.NArg() > 0 {
		pattern = fs.Arg(0)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to get working directory:", err)
		os.Exit(1)
	}

	pkgs, err := codegen.LoadPackages(cwd, pattern)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load packages:", err)
		os.Exit(1)
	}

	if len(pkgs) == 0 {
		fmt.Fprintln(os.Stderr, "no packages importing thx found")
		os.Exit(1)
	}

	tree, err := codegen.Extract(pkgs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to extract routes:", err)
		os.Exit(1)
	}

	pkgName := filepath.Base(*outputDir)

	absOut := *outputDir
	if !filepath.IsAbs(absOut) {
		absOut = filepath.Join(cwd, absOut)
	}

	if err := os.MkdirAll(absOut, 0755); err != nil {
		fmt.Fprintln(os.Stderr, "failed to create output directory:", err)
		os.Exit(1)
	}

	routeFile, err := codegen.GenerateRouteFile(pkgName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to generate route.go:", err)
		os.Exit(1)
	}

	genFile, err := codegen.Generate(tree, pkgName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to generate gen.go:", err)
		os.Exit(1)
	}

	if err := os.WriteFile(filepath.Join(absOut, "route.go"), routeFile, 0644); err != nil {
		fmt.Fprintln(os.Stderr, "failed to write route.go:", err)
		os.Exit(1)
	}

	if err := os.WriteFile(filepath.Join(absOut, "gen.go"), genFile, 0644); err != nil {
		fmt.Fprintln(os.Stderr, "failed to write gen.go:", err)
		os.Exit(1)
	}

	fmt.Printf("Generated routes in %s\n", *outputDir)
}
