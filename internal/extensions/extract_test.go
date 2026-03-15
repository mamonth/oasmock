package extensions

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
)

/*
Scenario: Extracting parameter‑match extension from OpenAPI example
Given an OpenAPI example with x‑mock‑params‑match or deprecated x‑mock‑match extension
When ExtractParamsMatch is called
Then it returns appropriate match map and ok flag, handling nil, empty, and malformed extensions

Related spec scenarios: RS.EXT.1, RS.EXT.2
*/
func TestExtractParamsMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		example   *openapi3.Example
		wantMatch bool
		wantOk    bool
	}{
		{
			name:      "nil example",
			example:   nil,
			wantMatch: false,
			wantOk:    false,
		},
		{
			name:      "nil extensions",
			example:   &openapi3.Example{Extensions: nil},
			wantMatch: false,
			wantOk:    false,
		},
		{
			name: "empty extensions",
			example: &openapi3.Example{
				Extensions: map[string]any{},
			},
			wantMatch: false,
			wantOk:    false,
		},
		{
			name: "x-mock-params-match present",
			example: &openapi3.Example{
				Extensions: map[string]any{
					"x-mock-params-match": map[string]any{
						"{$request.query.id}": 42,
					},
				},
			},
			wantMatch: true,
			wantOk:    true,
		},
		{
			name: "x-mock-match present (deprecated)",
			example: &openapi3.Example{
				Extensions: map[string]any{
					"x-mock-match": map[string]any{
						"{$request.query.id}": 42,
					},
				},
			},
			wantMatch: true,
			wantOk:    true,
		},
		{
			name: "both present - uses x-mock-match with warning",
			example: &openapi3.Example{
				Extensions: map[string]any{
					"x-mock-match": map[string]any{
						"{$request.query.id}": 42,
					},
					"x-mock-params-match": map[string]any{
						"{$request.query.id}": 99,
					},
				},
			},
			wantMatch: true,
			wantOk:    true,
		},
		{
			name: "non-map value",
			example: &openapi3.Example{
				Extensions: map[string]any{
					"x-mock-params-match": "not a map",
				},
			},
			wantMatch: false,
			wantOk:    false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ExtractParamsMatch(tt.example)
			assert.Equal(t, tt.wantOk, ok)
			if ok && tt.wantMatch {
				assert.NotNil(t, got)
			}
		})
	}
}

/*
Scenario: Extracting skip extension from OpenAPI example
Given an OpenAPI example with x‑mock‑skip extension
When ExtractSkip is called
Then it returns true if extension is boolean true, false otherwise, handling nil, empty, and malformed extensions

Related spec scenarios: RS.EXT.3
*/
func TestExtractSkip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		example *openapi3.Example
		want    bool
	}{
		{
			name:    "nil example",
			example: nil,
			want:    false,
		},
		{
			name:    "nil extensions",
			example: &openapi3.Example{Extensions: nil},
			want:    false,
		},
		{
			name: "skip true",
			example: &openapi3.Example{
				Extensions: map[string]any{
					"x-mock-skip": true,
				},
			},
			want: true,
		},
		{
			name: "skip false",
			example: &openapi3.Example{
				Extensions: map[string]any{
					"x-mock-skip": false,
				},
			},
			want: false,
		},
		{
			name: "non-bool value",
			example: &openapi3.Example{
				Extensions: map[string]any{
					"x-mock-skip": "true",
				},
			},
			want: false,
		},
		{
			name: "other extensions present",
			example: &openapi3.Example{
				Extensions: map[string]any{
					"x-mock-once": true,
					"x-mock-skip": true,
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractSkip(tt.example)
			assert.Equal(t, tt.want, got)
		})
	}
}

/*
Scenario: Extracting once extension from OpenAPI example
Given an OpenAPI example with x‑mock‑once extension
When ExtractOnce is called
Then it returns true if extension is boolean true, false otherwise, handling nil, empty, and malformed extensions

Related spec scenarios: RS.EXT.8
*/
func TestExtractOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		example *openapi3.Example
		want    bool
	}{
		{
			name:    "nil example",
			example: nil,
			want:    false,
		},
		{
			name:    "nil extensions",
			example: &openapi3.Example{Extensions: nil},
			want:    false,
		},
		{
			name: "once true",
			example: &openapi3.Example{
				Extensions: map[string]any{
					"x-mock-once": true,
				},
			},
			want: true,
		},
		{
			name: "once false",
			example: &openapi3.Example{
				Extensions: map[string]any{
					"x-mock-once": false,
				},
			},
			want: false,
		},
		{
			name: "non-bool value",
			example: &openapi3.Example{
				Extensions: map[string]any{
					"x-mock-once": "true",
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractOnce(tt.example)
			assert.Equal(t, tt.want, got)
		})
	}
}

/*
Scenario: Extracting set‑state extension from OpenAPI example
Given an OpenAPI example with x‑mock‑set‑state extension
When ExtractSetState is called
Then it returns appropriate state map and ok flag, handling nil, empty, and malformed extensions

Related spec scenarios: RS.EXT.4, RS.EXT.5, RS.EXT.6
*/
func TestExtractSetState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		example *openapi3.Example
		wantMap bool
		wantOk  bool
	}{
		{
			name:    "nil example",
			example: nil,
			wantMap: false,
			wantOk:  false,
		},
		{
			name:    "nil extensions",
			example: &openapi3.Example{Extensions: nil},
			wantMap: false,
			wantOk:  false,
		},
		{
			name: "set-state present",
			example: &openapi3.Example{
				Extensions: map[string]any{
					"x-mock-set-state": map[string]any{
						"counter": 42,
					},
				},
			},
			wantMap: true,
			wantOk:  true,
		},
		{
			name: "non-map value",
			example: &openapi3.Example{
				Extensions: map[string]any{
					"x-mock-set-state": "not a map",
				},
			},
			wantMap: false,
			wantOk:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ExtractSetState(tt.example)
			assert.Equal(t, tt.wantOk, ok)
			if ok && tt.wantMap {
				assert.NotNil(t, got)
				assert.Equal(t, 42, got["counter"])
			}
		})
	}
}

/*
Scenario: Extracting headers extension from OpenAPI example
Given an OpenAPI example with x‑mock‑headers extension
When ExtractHeaders is called
Then it returns appropriate headers map and ok flag, handling nil, empty, and malformed extensions

Related spec scenarios: RS.EXT.7
*/
func TestExtractHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		example *openapi3.Example
		wantMap bool
		wantOk  bool
	}{
		{
			name:    "nil example",
			example: nil,
			wantMap: false,
			wantOk:  false,
		},
		{
			name:    "nil extensions",
			example: &openapi3.Example{Extensions: nil},
			wantMap: false,
			wantOk:  false,
		},
		{
			name: "headers present",
			example: &openapi3.Example{
				Extensions: map[string]any{
					"x-mock-headers": map[string]any{
						"X-Custom": "value",
					},
				},
			},
			wantMap: true,
			wantOk:  true,
		},
		{
			name: "non-map value",
			example: &openapi3.Example{
				Extensions: map[string]any{
					"x-mock-headers": []string{"not a map"},
				},
			},
			wantMap: false,
			wantOk:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ExtractHeaders(tt.example)
			assert.Equal(t, tt.wantOk, ok)
			if ok && tt.wantMap {
				assert.NotNil(t, got)
			}
		})
	}
}
