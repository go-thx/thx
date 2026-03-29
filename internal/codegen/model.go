package codegen

type RouteTree struct {
	Routes []RouteEntry
	Groups []RouteGroup
	Assets []AssetGroup
}

type AssetGroup struct {
	Name    string // derived from the FS variable name (e.g. "publicFS" → "PublicFS")
	Prefix  string
	Dir     string // absolute path to the directory on disk
	Entries []AssetEntry
}

type RouteGroup struct {
	Name   string
	Prefix string
	Routes []RouteEntry
	Groups []RouteGroup
}

type RouteEntry struct {
	Name       string
	Method     string
	Path       string
	PathParams []PathParam
	QueryType  *StructInfo
}

type PathParam struct {
	Name string
}

type StructInfo struct {
	Fields []StructField
}

type StructField struct {
	Name      string
	Type      string
	SchemaTag string
}
