package codegen

import (
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"golang.org/x/tools/go/packages"
)

const authPkgPath = thxPkgPath + "/auth"

// extractor traverses loaded Go packages to find and extract route definitions.
type extractor struct {
	pkgs    []*packages.Package
	methods map[string]*methodInfo // "pkg.Type.Method" -> info
	errors  []error
}

type methodInfo struct {
	pkg  *packages.Package
	decl *ast.FuncDecl
}

// Extract analyzes the given packages and returns a RouteTree containing
// all discovered routes, groups, and static asset definitions.
func Extract(pkgs []*packages.Package) (*RouteTree, error) {
	e := &extractor{
		pkgs:    pkgs,
		methods: make(map[string]*methodInfo),
	}

	e.indexMethods()

	roots := e.findRoots()
	if len(roots) == 0 {
		return &RouteTree{}, nil
	}

	tree := &RouteTree{}
	for _, root := range roots {
		e.extractFromFunc(root.pkg, root.decl, tree)
	}

	if len(e.errors) > 0 {
		return nil, errors.Join(e.errors...)
	}

	// Scan asset directories and populate entries.
	for i := range tree.Assets {
		entries, err := ScanAssets(tree.Assets[i].Dir, tree.Assets[i].Prefix)
		if err != nil {
			return nil, err
		}
		tree.Assets[i].Entries = entries
	}

	return tree, nil
}

// indexMethods builds a lookup table of all methods across all packages.
func (e *extractor) indexMethods() {
	for _, pkg := range e.pkgs {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Recv == nil {
					continue
				}

				recvType := resolveRecvType(fd.Recv)
				if recvType == "" {
					continue
				}

				key := pkg.PkgPath + "." + recvType + "." + fd.Name.Name
				e.methods[key] = &methodInfo{pkg: pkg, decl: fd}
			}
		}
	}
}

// findRoots finds top-level Routes() methods that are not called by other Routes() methods.
func (e *extractor) findRoots() []*methodInfo {
	calledBy := make(map[string]bool)

	for _, mi := range e.methods {
		if mi.decl.Name.Name != "Routes" {
			continue
		}
		e.findRoutesCalls(mi.pkg, mi.decl, calledBy)
	}

	var roots []*methodInfo
	for key, mi := range e.methods {
		if mi.decl.Name.Name == "Routes" && e.returnsThxRoutes(mi.pkg, mi.decl) && !calledBy[key] {
			roots = append(roots, mi)
		}
	}

	return roots
}

// findRoutesCalls marks all Routes() methods called within a function body.
func (e *extractor) findRoutesCalls(pkg *packages.Package, decl *ast.FuncDecl, called map[string]bool) {
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Routes" {
			return true
		}

		obj := pkg.TypesInfo.ObjectOf(sel.Sel)
		if obj == nil {
			return true
		}

		fn, ok := obj.(*types.Func)
		if !ok {
			return true
		}

		recv := fn.Type().(*types.Signature).Recv()
		if recv == nil {
			return true
		}

		recvType := stripPointer(recv.Type())
		named, ok := recvType.(*types.Named)
		if !ok {
			return true
		}

		key := named.Obj().Pkg().Path() + "." + named.Obj().Name() + ".Routes"
		called[key] = true

		return true
	})
}

// returnsThxRoutes checks if a function declaration returns thx.Routes.
func (e *extractor) returnsThxRoutes(pkg *packages.Package, decl *ast.FuncDecl) bool {
	obj := pkg.TypesInfo.Defs[decl.Name]
	if obj == nil {
		return false
	}

	fn, ok := obj.(*types.Func)
	if !ok {
		return false
	}

	sig := fn.Type().(*types.Signature)
	results := sig.Results()
	if results.Len() != 1 {
		return false
	}

	resType := results.At(0).Type()
	named, ok := resType.(*types.Named)
	if !ok {
		return false
	}

	return named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == thxPkgPath && named.Obj().Name() == "Routes"
}

// extractFromFunc extracts route definitions from the return statements of a function.
func (e *extractor) extractFromFunc(pkg *packages.Package, decl *ast.FuncDecl, tree *RouteTree) {
	if decl.Body == nil {
		return
	}

	for _, stmt := range decl.Body.List {
		ret, ok := stmt.(*ast.ReturnStmt)
		if !ok {
			continue
		}
		for _, expr := range ret.Results {
			e.extractExpr(pkg, expr, tree)
		}
	}
}

// extractExpr dispatches expression extraction by type (call or composite literal).
func (e *extractor) extractExpr(pkg *packages.Package, expr ast.Expr, tree *RouteTree) {
	switch x := expr.(type) {
	case *ast.CallExpr:
		e.extractCall(pkg, x, tree)
	case *ast.CompositeLit:
		for _, elt := range x.Elts {
			e.extractExpr(pkg, elt, tree)
		}
	}
}

// extractCall inspects a function call and extracts route info based on the called function.
func (e *extractor) extractCall(pkg *packages.Package, call *ast.CallExpr, tree *RouteTree) {
	fnName, fnPkg := e.resolveFuncName(pkg, call.Fun)

	switch {
	case fnPkg == thxPkgPath && (fnName == "Get" || fnName == "Post" || fnName == "Put" || fnName == "Patch" || fnName == "Delete"):
		entry := e.extractRoute(pkg, call, strings.ToUpper(fnName))
		if entry != nil {
			tree.Routes = append(tree.Routes, *entry)
		}

	case fnPkg == thxPkgPath && fnName == "SSE":
		entry := e.extractRoute(pkg, call, "SSE")
		if entry != nil {
			tree.Routes = append(tree.Routes, *entry)
		}

	case fnPkg == thxPkgPath && fnName == "WS":
		entry := e.extractRoute(pkg, call, "WS")
		if entry != nil {
			tree.Routes = append(tree.Routes, *entry)
		}

	case fnPkg == thxPkgPath && fnName == "WithPath":
		e.extractWithPath(pkg, call, tree)

	case fnPkg == authPkgPath && fnName == "WithGuard":
		e.extractWithGuard(pkg, call, tree)

	case fnPkg == thxPkgPath && (fnName == "WithLayout" || fnName == "WithMiddleware"):
		e.extractTransparent(pkg, call, tree)

	case fnPkg == thxPkgPath && fnName == "Static":
		if err := e.extractStatic(pkg, call, tree); err != nil {
			e.errors = append(e.errors, err)
		}

	case fnPkg == thxPkgPath && (fnName == "HandleNotFound" || fnName == "HandleInternalError"):
		// skip

	default:
		if e.isRoutesCall(pkg, call) {
			e.followRoutesCall(pkg, call, tree)
		}
	}
}

// extractRoute extracts a single route entry from a thx.Get/Post/etc. call.
func (e *extractor) extractRoute(pkg *packages.Package, call *ast.CallExpr, method string) *RouteEntry {
	if len(call.Args) < 2 {
		return nil
	}

	path := stringLitValue(call.Args[0])
	if path == "" {
		return nil
	}

	pathParams := extractPathParams(path)

	handlerExpr := call.Args[1]

	// Unwrap auth.Get(handler) / auth.Route(handler)
	if innerCall, ok := handlerExpr.(*ast.CallExpr); ok {
		fn, fp := e.resolveFuncName(pkg, innerCall.Fun)
		if fp == authPkgPath && (fn == "Get" || fn == "Route") && len(innerCall.Args) > 0 {
			handlerExpr = innerCall.Args[0]
		}
	}

	name := e.resolveHandlerName(pkg, handlerExpr)
	if name == "" {
		name = routeMethodName(method, path)
	} else {
		name = titleCase(name)
	}

	entry := &RouteEntry{
		Name:       name,
		Method:     method,
		Path:       path,
		PathParams: pathParams,
	}

	queryType := e.resolveQueryType(pkg, handlerExpr)
	if queryType != nil && len(queryType.Fields) > 0 {
		entry.QueryType = queryType
	}

	return entry
}

// resolveHandlerName extracts the handler function or method name from the expression.
func (e *extractor) resolveHandlerName(pkg *packages.Package, expr ast.Expr) string {
	switch h := expr.(type) {
	case *ast.SelectorExpr:
		return h.Sel.Name
	case *ast.Ident:
		return h.Name
	}
	return ""
}

// extractWithPath extracts a route group from a thx.WithPath call.
func (e *extractor) extractWithPath(pkg *packages.Package, call *ast.CallExpr, tree *RouteTree) {
	if len(call.Args) < 2 {
		return
	}

	prefix := stringLitValue(call.Args[0])
	if prefix == "" {
		return
	}

	// Derive group name from the Routes() call if present
	name := ""
	for _, arg := range call.Args[1:] {
		name = e.resolveGroupName(pkg, arg)
		if name != "" {
			break
		}
	}
	if name == "" {
		name = groupNameFromPath(prefix)
	}

	group := RouteGroup{
		Name:   name,
		Prefix: prefix,
	}

	subTree := &RouteTree{}
	for _, arg := range call.Args[1:] {
		e.extractExpr(pkg, arg, subTree)
	}

	group.Routes = subTree.Routes
	group.Groups = subTree.Groups
	tree.Groups = append(tree.Groups, group)
}

// extractWithGuard extracts a route group from an auth.WithGuard call.
func (e *extractor) extractWithGuard(pkg *packages.Package, call *ast.CallExpr, tree *RouteTree) {
	if len(call.Args) < 2 {
		return
	}

	prefix := stringLitValue(call.Args[0])
	if prefix == "" {
		return
	}

	// Derive group name from the routes arg (second arg)
	name := e.resolveGroupName(pkg, call.Args[1])
	if name == "" {
		name = groupNameFromPath(prefix)
	}

	group := RouteGroup{
		Name:   name,
		Prefix: prefix,
	}

	subTree := &RouteTree{}
	e.extractExpr(pkg, call.Args[1], subTree)

	group.Routes = subTree.Routes
	group.Groups = subTree.Groups
	tree.Groups = append(tree.Groups, group)
}

// resolveGroupName derives a group name from a Routes() call's receiver type.
func (e *extractor) resolveGroupName(pkg *packages.Package, expr ast.Expr) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Routes" {
		return ""
	}

	// Get the type of the receiver to derive the package name
	obj := pkg.TypesInfo.ObjectOf(sel.Sel)
	if obj == nil {
		return ""
	}

	fn, ok := obj.(*types.Func)
	if !ok {
		return ""
	}

	recv := fn.Type().(*types.Signature).Recv()
	if recv == nil {
		return ""
	}

	recvType := stripPointer(recv.Type())
	named, ok := recvType.(*types.Named)
	if !ok {
		return ""
	}

	return titleCase(named.Obj().Pkg().Name())
}

// extractTransparent extracts routes from wrappers like WithLayout and WithMiddleware
// that don't create a new route group.
func (e *extractor) extractTransparent(pkg *packages.Package, call *ast.CallExpr, tree *RouteTree) {
	for _, arg := range call.Args[1:] {
		e.extractExpr(pkg, arg, tree)
	}
}

// isRoutesCall checks if a call expression is a method call returning thx.Routes.
func (e *extractor) isRoutesCall(pkg *packages.Package, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Routes" {
		return false
	}

	typ := pkg.TypesInfo.TypeOf(call)
	if typ == nil {
		return false
	}

	named, ok := typ.(*types.Named)
	if !ok {
		return false
	}

	return named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == thxPkgPath && named.Obj().Name() == "Routes"
}

// followRoutesCall resolves a Routes() method call and extracts routes from its body.
func (e *extractor) followRoutesCall(pkg *packages.Package, call *ast.CallExpr, tree *RouteTree) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}

	obj := pkg.TypesInfo.ObjectOf(sel.Sel)
	if obj == nil {
		return
	}

	fn, ok := obj.(*types.Func)
	if !ok {
		return
	}

	recv := fn.Type().(*types.Signature).Recv()
	if recv == nil {
		return
	}

	recvType := stripPointer(recv.Type())
	named, ok := recvType.(*types.Named)
	if !ok {
		return
	}

	key := named.Obj().Pkg().Path() + "." + named.Obj().Name() + ".Routes"
	mi, ok := e.methods[key]
	if !ok {
		return
	}

	e.extractFromFunc(mi.pkg, mi.decl, tree)
}

// resolveQueryType extracts the query parameter struct type from a handler's signature.
func (e *extractor) resolveQueryType(pkg *packages.Package, handlerExpr ast.Expr) *StructInfo {
	typ := pkg.TypesInfo.TypeOf(handlerExpr)
	if typ == nil {
		return nil
	}

	sig, ok := typ.Underlying().(*types.Signature)
	if !ok {
		return nil
	}

	// Q is the second parameter (index 1): func(Context, Q) O
	if sig.Params().Len() < 2 {
		return nil
	}

	qType := sig.Params().At(1).Type()
	return extractStructInfo(qType)
}

// extractStructInfo extracts schema-tagged fields from a struct type.
func extractStructInfo(typ types.Type) *StructInfo {
	st, ok := typ.Underlying().(*types.Struct)
	if !ok {
		return nil
	}

	if st.NumFields() == 0 {
		return nil
	}

	info := &StructInfo{}
	for i := range st.NumFields() {
		field := st.Field(i)
		tag := reflect.StructTag(st.Tag(i))
		schemaTag := tag.Get("schema")
		if schemaTag == "" || schemaTag == "-" {
			continue
		}

		if idx := strings.Index(schemaTag, ","); idx >= 0 {
			schemaTag = schemaTag[:idx]
		}

		info.Fields = append(info.Fields, StructField{
			Name:      field.Name(),
			Type:      field.Type().String(),
			SchemaTag: schemaTag,
		})
	}

	if len(info.Fields) == 0 {
		return nil
	}

	return info
}

// extractStatic extracts a static asset group from a thx.Static call,
// resolving the embed.FS variable and its //go:embed directory.
func (e *extractor) extractStatic(pkg *packages.Package, call *ast.CallExpr, tree *RouteTree) error {
	if len(call.Args) < 2 {
		return nil
	}

	prefix := stringLitValue(call.Args[0])
	if prefix == "" {
		return nil
	}

	var name string
	var targetPkg *packages.Package
	var targetObj types.Object // the specific embed.FS variable to match

	switch arg := call.Args[1].(type) {
	case *ast.Ident:
		// thx.Static("/assets", assetsFS)
		name = arg.Name
		obj := pkg.TypesInfo.ObjectOf(arg)
		if obj == nil {
			return nil
		}
		targetObj = obj
		targetPkg = e.findPkg(obj.Pkg().Path())

	case *ast.CallExpr:
		// thx.Static("/assets", assets.Assets())
		fnName, fnPkgPath := e.resolveFuncName(pkg, arg.Fun)
		if fnName == "" {
			return nil
		}
		name = fnName
		targetPkg = e.findPkg(fnPkgPath)

	default:
		return nil
	}

	if targetPkg == nil {
		return nil
	}

	// Find the //go:embed directive on the FS variable declaration.
	for _, file := range targetPkg.Syntax {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}

			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}

				// Match the specific variable if known (ident case),
				// otherwise match any embed.FS in the package (call case).
				matched := false
				for _, n := range vs.Names {
					obj := targetPkg.TypesInfo.ObjectOf(n)
					if obj == nil {
						continue
					}
					if targetObj != nil {
						matched = obj == targetObj
					} else {
						matched = obj.Type().String() == "embed.FS"
					}
					if matched {
						break
					}
				}
				if !matched {
					continue
				}

				embedDir := findEmbedDir(vs.Doc)
				if embedDir == "" {
					embedDir = findEmbedDir(gd.Doc)
				}
				if embedDir == "" {
					continue
				}

				pkgDir := filepath.Dir(targetPkg.Fset.Position(file.Pos()).Filename)
				if pkgDir == "" {
					return fmt.Errorf("thx.Static: could not resolve package directory for %s", targetPkg.PkgPath)
				}

				absDir := filepath.Join(pkgDir, embedDir)
				if _, err := os.Stat(absDir); err != nil {
					return fmt.Errorf("thx.Static: embed directory %q does not exist (from //go:embed in %s)", absDir, targetPkg.PkgPath)
				}

				tree.Assets = append(tree.Assets, AssetGroup{
					Name:   titleCase(name),
					Prefix: prefix,
					Dir:    absDir,
				})
				return nil
			}
		}
	}

	return nil
}

// findPkg looks up a package by its import path.
func (e *extractor) findPkg(pkgPath string) *packages.Package {
	for _, p := range e.pkgs {
		if p.PkgPath == pkgPath {
			return p
		}
	}
	return nil
}

// findEmbedDir extracts the directory from a //go:embed comment group.
func findEmbedDir(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	for _, c := range cg.List {
		if !strings.HasPrefix(c.Text, "//go:embed ") {
			continue
		}
		raw := strings.TrimPrefix(c.Text, "//go:embed ")

		// Parse space-separated patterns and derive the directory.
		// "public/*"    → "public"
		// "assets/*.js" → "assets"
		// "assets"      → "assets" (directory by name)
		// "*"           → "."
		// "all:assets"  → "assets"
		for _, pattern := range strings.Fields(raw) {
			pattern = strings.TrimPrefix(pattern, "all:")

			// No wildcards and no path separator → directory by name.
			if !strings.ContainsAny(pattern, "*?/") {
				return pattern
			}

			dir := filepath.Dir(pattern)
			if dir == "." {
				return "."
			}
			return dir
		}
	}
	return ""
}

// resolveFuncName resolves a function expression to its name and package path.
func (e *extractor) resolveFuncName(pkg *packages.Package, fun ast.Expr) (name, pkgPath string) {
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		ident, ok := f.X.(*ast.Ident)
		if !ok {
			return f.Sel.Name, ""
		}

		obj := pkg.TypesInfo.ObjectOf(ident)
		if obj == nil {
			return f.Sel.Name, ""
		}

		if pkgName, ok := obj.(*types.PkgName); ok {
			return f.Sel.Name, pkgName.Imported().Path()
		}

		return f.Sel.Name, ""

	case *ast.IndexExpr:
		return e.resolveFuncName(pkg, f.X)

	case *ast.IndexListExpr:
		return e.resolveFuncName(pkg, f.X)

	case *ast.Ident:
		return f.Name, pkg.PkgPath
	}

	return "", ""
}

// resolveRecvType returns the receiver type name from a method's field list.
func resolveRecvType(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}

	t := recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}

	if ident, ok := t.(*ast.Ident); ok {
		return ident.Name
	}

	return ""
}

// stripPointer unwraps a pointer type to its element type.
func stripPointer(t types.Type) types.Type {
	if ptr, ok := t.(*types.Pointer); ok {
		return ptr.Elem()
	}
	return t
}

// stringLitValue extracts the unquoted value from a string literal expression.
func stringLitValue(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok {
		return ""
	}
	return strings.Trim(lit.Value, `"`)
}
