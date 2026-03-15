package loader

import (
	"net/http"
	"slices"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// RouteMapping holds a mapping from HTTP method and path pattern to an OpenAPI operation.
type RouteMapping struct {
	Method     string
	Path       string // The full path pattern with prefix (e.g., "/v1/users/{id}")
	Pattern    string // The path pattern without prefix (e.g., "/users/{id}")
	Prefix     string // The prefix for this route (e.g., "/v1")
	ChiPattern string // Path converted to Chi pattern (e.g., "/v1/users/:id")
	Operation  *openapi3.Operation
	Parameters openapi3.Parameters
	Responses  *openapi3.Responses
}

// BuildRouteMappings creates route mappings from loaded schemas.
// For each schema, each path in the schema is combined with the schema's prefix
// to produce the full path pattern used for routing.
func BuildRouteMappings(infos []SchemaInfo) ([]RouteMapping, error) {
	var mappings []RouteMapping

	for _, info := range infos {
		spec := info.Spec
		prefix := strings.TrimSuffix(info.Prefix, "/")

		// Collect and sort paths for deterministic iteration
		pathMap := spec.Paths.Map()
		paths := make([]string, 0, len(pathMap))
		for path := range pathMap {
			paths = append(paths, path)
		}
		slices.Sort(paths)
		for _, path := range paths {
			pathItem := pathMap[path]
			if pathItem == nil {
				continue
			}

			// Apply prefix to the path
			fullPath := applyPrefix(prefix, path)

			// Create mappings for each HTTP method defined in the path item
			mappings = append(mappings, createMappingsForPath(path, fullPath, prefix, pathItem)...)
		}
	}

	return mappings, nil
}

func applyPrefix(prefix, path string) string {
	if prefix == "" {
		return path
	}
	// Ensure prefix starts with "/" and path doesn't have double slash
	prefix = "/" + strings.Trim(prefix, "/")
	path = "/" + strings.Trim(path, "/")
	if path == "/" {
		return prefix
	}
	return prefix + path
}

func createMappingsForPath(originalPath, fullPath, prefix string, pathItem *openapi3.PathItem) []RouteMapping {
	var mappings []RouteMapping

	// Helper to add mapping if operation exists
	addMapping := func(method string, op *openapi3.Operation) {
		if op != nil {
			mappings = append(mappings, RouteMapping{
				Method:     method,
				Path:       fullPath,
				Pattern:    originalPath,
				Prefix:     prefix,
				ChiPattern: OpenAPIPatternToChi(fullPath),
				Operation:  op,
				Parameters: pathItem.Parameters,
				Responses:  op.Responses,
			})
		}
	}

	addMapping(http.MethodGet, pathItem.Get)
	addMapping(http.MethodPost, pathItem.Post)
	addMapping(http.MethodPut, pathItem.Put)
	addMapping(http.MethodPatch, pathItem.Patch)
	addMapping(http.MethodDelete, pathItem.Delete)
	addMapping(http.MethodHead, pathItem.Head)
	addMapping(http.MethodOptions, pathItem.Options)
	addMapping(http.MethodTrace, pathItem.Trace) // Note: OpenAPI 3.0 doesn't officially support TRACE

	return mappings
}

// OpenAPIPatternToChi converts an OpenAPI path pattern (with {param}) to a Chi pattern.
// Chi supports both {param} and :param syntax. We keep the OpenAPI braces.
func OpenAPIPatternToChi(pattern string) string {
	return pattern
}

// FindOperation finds the operation that matches the given method and path.
// It returns the route mapping and extracted path parameters.
func FindOperation(mappings []RouteMapping, method, path string) (*RouteMapping, map[string]string, bool) {
	for _, mapping := range mappings {
		if mapping.Method != method {
			continue
		}

		// Simple exact match for now; later we need to handle path parameters
		// For MVP, we'll do exact match on path
		if mapping.Path == path {
			return &mapping, nil, true
		}
	}
	return nil, nil, false
}
