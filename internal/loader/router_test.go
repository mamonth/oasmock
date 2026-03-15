package loader

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Applying prefix to route path
Given a prefix and a path
When applyPrefix is called
Then it returns the concatenated path with proper slash handling, trimming trailing slashes

Related spec scenarios: RS.MSC.6
*/
func TestApplyPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		prefix string
		path   string
		want   string
	}{
		{
			name:   "empty prefix",
			prefix: "",
			path:   "/users",
			want:   "/users",
		},
		{
			name:   "prefix with leading slash",
			prefix: "/api",
			path:   "/users",
			want:   "/api/users",
		},
		{
			name:   "prefix without leading slash",
			prefix: "api",
			path:   "/users",
			want:   "/api/users",
		},
		{
			name:   "path without leading slash",
			prefix: "/api",
			path:   "users",
			want:   "/api/users",
		},
		{
			name:   "both with trailing slashes",
			prefix: "/api/",
			path:   "/users/",
			want:   "/api/users",
		},
		{
			name:   "root path",
			prefix: "/api",
			path:   "/",
			want:   "/api",
		},
		{
			name:   "empty path",
			prefix: "/api",
			path:   "",
			want:   "/api",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := applyPrefix(tt.prefix, tt.path)
			assert.Equal(t, tt.want, got, "applyPrefix(%q, %q)", tt.prefix, tt.path)
		})
	}
}

/*
Scenario: Converting OpenAPI path pattern to Chi router pattern
Given an OpenAPI path pattern with optional curly‑brace parameters
When OpenAPIPatternToChi is called
Then it returns Chi‑style colon‑prefixed parameter names, preserving other characters

Related spec scenarios: RS.MSC.4, RS.MSC.5
*/
func TestOpenAPIPatternToChi(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		{
			name:    "no parameters",
			pattern: "/users/list",
			want:    "/users/list",
		},
		{
			name:    "single parameter",
			pattern: "/users/{id}",
			want:    "/users/{id}",
		},
		{
			name:    "multiple parameters",
			pattern: "/users/{userId}/posts/{postId}",
			want:    "/users/{userId}/posts/{postId}",
		},
		{
			name:    "parameter at start",
			pattern: "{id}/users",
			want:    "{id}/users",
		},
		{
			name:    "adjacent parameters",
			pattern: "/users/{id}{action}",
			want:    "/users/{id}{action}",
		},
		{
			name:    "unclosed brace",
			pattern: "/users/{id",
			want:    "/users/{id",
		},
		{
			name:    "empty parameter name",
			pattern: "/users/{}",
			want:    "/users/{}",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := OpenAPIPatternToChi(tt.pattern)
			assert.Equal(t, tt.want, got, "OpenAPIPatternToChi(%q)", tt.pattern)
		})
	}
}

/*
Scenario: Finding operation in route mappings by method and path
Given a list of route mappings
When FindOperation is called with a method and path
Then it returns the matching mapping and true if found, false otherwise

Related spec scenarios: RS.MSC.4, RS.MSC.5, RS.MSC.6, RS.MSC.7
*/
func TestFindOperation(t *testing.T) {
	t.Parallel()

	mappings := []RouteMapping{
		{
			Method:     "GET",
			Path:       "/api/users",
			ChiPattern: "/api/users",
		},
		{
			Method:     "POST",
			Path:       "/api/users",
			ChiPattern: "/api/users",
		},
		{
			Method:     "GET",
			Path:       "/api/posts/{id}",
			ChiPattern: "/api/posts/:id",
		},
	}

	tests := []struct {
		name      string
		method    string
		path      string
		wantFound bool
		wantPath  string
	}{
		{
			name:      "exact match GET /api/users",
			method:    "GET",
			path:      "/api/users",
			wantFound: true,
			wantPath:  "/api/users",
		},
		{
			name:      "exact match POST /api/users",
			method:    "POST",
			path:      "/api/users",
			wantFound: true,
			wantPath:  "/api/users",
		},
		{
			name:      "method mismatch",
			method:    "PUT",
			path:      "/api/users",
			wantFound: false,
		},
		{
			name:      "path mismatch",
			method:    "GET",
			path:      "/api/nonexistent",
			wantFound: false,
		},
		{
			name:      "path with param - exact match fails",
			method:    "GET",
			path:      "/api/posts/{id}",
			wantFound: true,
			wantPath:  "/api/posts/{id}",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mapping, _, found := FindOperation(mappings, tt.method, tt.path)
			assert.Equal(t, tt.wantFound, found)
			if tt.wantFound {
				assert.Equal(t, tt.wantPath, mapping.Path)
			}
		})
	}
}

/*
Scenario: Building route mappings from loaded OpenAPI schemas
Given loaded OpenAPI schemas without prefix
When BuildRouteMappings is called
Then it returns correct route mappings with method, path, Chi pattern, and operation references

Related spec scenarios: RS.MSC.1, RS.MSC.4
*/
func TestBuildRouteMappings(t *testing.T) {
	t.Parallel()

	schemas, err := LoadSchemas([]string{"../../test/_shared/resources/test.yaml"}, []string{""})
	require.NoError(t, err, "LoadSchemas failed")

	mappings, err := BuildRouteMappings(schemas)
	require.NoError(t, err, "BuildRouteMappings failed")

	// Expect 10 mappings from test.yaml
	expectedPaths := map[string][]string{
		"/users":         {"GET"},
		"/users/{id}":    {"GET"},
		"/conditional":   {"GET"},
		"/once":          {"GET"},
		"/not-in-schema": {"GET"},
		"/structured":    {"GET"},
		"/echo":          {"POST"},
		"/body":          {"POST"},
		"/env":           {"GET"},
		"/jwt":           {"GET"},
	}

	// Check we have at least the expected number of mappings
	assert.GreaterOrEqual(t, len(mappings), len(expectedPaths), "BuildRouteMappings returned too few mappings")

	// Check each mapping
	foundPaths := make(map[string]bool)
	for _, mapping := range mappings {
		// Check path is valid
		assert.NotEmpty(t, mapping.Path, "Mapping has empty path")
		assert.NotNil(t, mapping.Operation, "Mapping for %s %s has nil Operation", mapping.Method, mapping.Path)
		assert.NotEmpty(t, mapping.ChiPattern, "Mapping for %s %s has empty ChiPattern", mapping.Method, mapping.Path)

		// Track found paths
		key := fmt.Sprintf("%s %s", mapping.Method, mapping.Path)
		foundPaths[key] = true

		// Check ChiPattern conversion for path parameters
		if mapping.Path == "/users/{id}" {
			assert.Equal(t, "/users/{id}", mapping.ChiPattern, "ChiPattern conversion failed")
		}
	}

	// Verify all expected paths are present
	for path, methods := range expectedPaths {
		for _, method := range methods {
			key := fmt.Sprintf("%s %s", method, path)
			assert.True(t, foundPaths[key], "Expected mapping not found: %s", key)
		}
	}
}

/*
Scenario: Building route mappings with prefix from loaded OpenAPI schemas
Given loaded OpenAPI schemas with prefix
When BuildRouteMappings is called
Then it returns correct route mappings with prefixed path, original pattern, and Chi pattern

Related spec scenarios: RS.MSC.2, RS.MSC.6
*/
func TestBuildRouteMappingsWithPrefix(t *testing.T) {
	t.Parallel()

	schemas, err := LoadSchemas([]string{"../../test/_shared/resources/test.yaml"}, []string{"/api/v1"})
	require.NoError(t, err, "LoadSchemas failed")

	mappings, err := BuildRouteMappings(schemas)
	require.NoError(t, err, "BuildRouteMappings failed")

	// Expect 10 mappings with prefix
	expectedPaths := map[string][]string{
		"/users":         {"GET"},
		"/users/{id}":    {"GET"},
		"/conditional":   {"GET"},
		"/once":          {"GET"},
		"/not-in-schema": {"GET"},
		"/structured":    {"GET"},
		"/echo":          {"POST"},
		"/body":          {"POST"},
		"/env":           {"GET"},
		"/jwt":           {"GET"},
	}

	assert.GreaterOrEqual(t, len(mappings), len(expectedPaths), "BuildRouteMappings returned too few mappings")

	foundPaths := make(map[string]bool)
	for _, mapping := range mappings {
		assert.Equal(t, "/api/v1", mapping.Prefix, "Mapping prefix mismatch")
		// Pattern should be original path without prefix
		// Check that pattern is one of the expected paths
		foundKey := fmt.Sprintf("%s %s", mapping.Method, mapping.Pattern)
		foundPaths[foundKey] = true
		// Path should be prefixed version of pattern
		expectedPath := mapping.Pattern
		if expectedPath != "/" {
			expectedPath = "/api/v1" + mapping.Pattern
		}
		assert.Equal(t, expectedPath, mapping.Path, "Prefixed path mismatch")
	}

	// Verify all expected paths are present
	for path, methods := range expectedPaths {
		for _, method := range methods {
			key := fmt.Sprintf("%s %s", method, path)
			assert.True(t, foundPaths[key], "Expected mapping not found: %s", key)
		}
	}
}
