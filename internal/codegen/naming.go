package codegen

import (
	"regexp"
	"strings"
	"unicode"
)

var pathParamRe = regexp.MustCompile(`\{(\w+)\}`)

func extractPathParams(path string) []PathParam {
	matches := pathParamRe.FindAllStringSubmatch(path, -1)
	params := make([]PathParam, 0, len(matches))
	for _, m := range matches {
		params = append(params, PathParam{Name: m[1]})
	}
	return params
}

func groupNameFromPath(prefix string) string {
	prefix = strings.TrimPrefix(prefix, "/")
	prefix = strings.TrimSuffix(prefix, "/")

	parts := strings.Split(prefix, "/")
	var result string
	for _, p := range parts {
		if p == "" || strings.HasPrefix(p, "{") {
			continue
		}
		result += titleCase(p)
	}
	if result == "" {
		return "Root"
	}
	return result
}

func routeMethodName(method, path string) string {
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")

	path = pathParamRe.ReplaceAllString(path, "")
	path = strings.TrimSuffix(path, "/")

	segments := strings.Split(path, "/")
	var namePart string
	for _, s := range segments {
		if s == "" {
			continue
		}
		namePart += titleCase(s)
	}

	if namePart == "" {
		namePart = "Index"
	}

	switch strings.ToUpper(method) {
	case "GET":
		return "Get" + namePart
	case "POST":
		return "Post" + namePart
	case "PUT":
		return "Put" + namePart
	case "PATCH":
		return "Patch" + namePart
	case "DELETE":
		return "Delete" + namePart
	default:
		return namePart
	}
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
