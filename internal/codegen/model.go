package codegen

// RouteTree is the top-level structure holding all extracted route information.
type RouteTree struct {
	Routes []RouteEntry
	Groups []RouteGroup
	Assets []AssetGroup
}

// AssetGroup represents a set of static assets served from one directory.
type AssetGroup struct {
	Name    string // derived from the FS variable name (e.g. "publicFS" → "PublicFS")
	Prefix  string
	Dir     string // absolute path to the directory on disk
	Entries []AssetEntry
}

// RouteGroup represents a group of routes under a common URL prefix.
type RouteGroup struct {
	Name   string
	Prefix string
	Routes []RouteEntry
	Groups []RouteGroup
}

// RouteEntry represents a single route with its HTTP method and path.
type RouteEntry struct {
	Name       string
	Method     string
	Path       string
	PathParams []PathParam
	QueryType  *StructInfo
}

// PathParam represents a named path parameter extracted from a route pattern.
type PathParam struct {
	Name string
}

// StructInfo holds metadata about a query parameter struct's fields.
type StructInfo struct {
	Fields []StructField
}

// StructField represents a single field in a query parameter struct.
type StructField struct {
	Name      string
	Type      string
	SchemaTag string
}
