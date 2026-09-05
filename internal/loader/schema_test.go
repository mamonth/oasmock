package loader

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Loading single schema from file
Given a file path to an OpenAPI or AsyncAPI YAML
When loadSingleSchema is called
Then it returns the parsed spec with the correct kind or error for missing/invalid files

Related spec scenarios: RS.MSC.1, RS.MSC.3, RS.AAL.1
*/
func TestLoadSingleSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        string
		wantErr     bool
		wantKind    Kind
		errContains string
	}{
		{
			name:     "valid OpenAPI YAML",
			path:     "../../test/_shared/resources/test.yaml",
			wantErr:  false,
			wantKind: KindOpenAPI,
		},
		{
			name:     "control API OpenAPI YAML",
			path:     "../../api/openapi.yaml",
			wantErr:  false,
			wantKind: KindOpenAPI,
		},
		{
			name:     "control API AsyncAPI YAML",
			path:     "../../api/asyncapi.yaml",
			wantErr:  false,
			wantKind: KindAsyncAPI,
		},
		{
			name:     "valid AsyncAPI 3.0.0 YAML",
			path:     "../../test/_shared/resources/asyncapi-30.yaml",
			wantErr:  false,
			wantKind: KindAsyncAPI,
		},
		{
			name:     "valid AsyncAPI 3.1.0 YAML",
			path:     "../../test/_shared/resources/asyncapi-31.yaml",
			wantErr:  false,
			wantKind: KindAsyncAPI,
		},
		{
			name:        "non-existent file",
			path:        "non-existent.yaml",
			wantErr:     true,
			errContains: "cannot read file",
		},
		{
			name:        "invalid OpenAPI content",
			path:        "../../test/_shared/resources/test-invalid.yaml",
			wantErr:     true,
			errContains: "not a valid OpenAPI or AsyncAPI schema",
		},
		{
			name:        "non-spec file",
			path:        "../../test/_shared/resources/not-a-spec.yaml",
			wantErr:     true,
			errContains: "neither an 'openapi' nor an 'asyncapi' root key",
		},
		{
			name:        "unsupported asyncapi version",
			path:        "../../test/_shared/resources/asyncapi-26.yaml",
			wantErr:     true,
			errContains: "unsupported AsyncAPI version",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			info, err := loadSingleSchema(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantKind, info.Kind)
			if tt.wantKind == KindOpenAPI {
				assert.NotNil(t, info.Spec)
			} else {
				assert.NotNil(t, info.Async)
			}
		})
	}
}

/*
Scenario: Detecting the spec kind from raw bytes
Given raw YAML/JSON bytes with an openapi or asyncapi root key
When detectKind is called
Then it returns the matching kind, and an error for files with neither key

Related spec scenarios: RS.AAL.1, RS.AAL.2, RS.AAL.4
*/
func TestDetectKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    string
		want    Kind
		wantErr bool
	}{
		{name: "openapi yaml", data: "openapi: 3.0.0\ninfo:\n  title: x\n  version: 1.0.0\n", want: KindOpenAPI},
		{name: "asyncapi yaml", data: "asyncapi: 3.0.0\ninfo:\n  title: x\n  version: 1.0.0\n", want: KindAsyncAPI},
		{name: "openapi json", data: `{"openapi":"3.0.0","info":{"title":"x","version":"1.0.0"}}`, want: KindOpenAPI},
		{name: "asyncapi json", data: `{"asyncapi":"3.0.0","info":{"title":"x","version":"1.0.0"}}`, want: KindAsyncAPI},
		{name: "neither key", data: "foo: bar\n", wantErr: true},
		{name: "empty", data: "", wantErr: true},
		{name: "garbage", data: "{{{", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			kind, err := detectKind([]byte(tt.data))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, kind)
		})
	}
}

/*
Scenario: Loading multiple OpenAPI schemas with optional prefixes
Given a list of source file paths and corresponding prefixes
When LoadSchemas is called
Then it returns schema infos with correct prefixes, handling mismatched lengths and errors

Related spec scenarios: RS.MSC.2
*/
func TestLoadSchemas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		sources     []string
		prefixes    []string
		wantErr     bool
		errContains string
	}{
		{
			name:     "single schema no prefix",
			sources:  []string{"../../test/_shared/resources/test.yaml"},
			prefixes: []string{""},
			wantErr:  false,
		},
		{
			name:     "single schema with prefix",
			sources:  []string{"../../test/_shared/resources/test.yaml"},
			prefixes: []string{"/api"},
			wantErr:  false,
		},
		{
			name:     "multiple schemas with prefixes",
			sources:  []string{"../../test/_shared/resources/test.yaml", "../../test/_shared/resources/test-params.yaml"},
			prefixes: []string{"/v1", "/v2"},
			wantErr:  false,
		},
		{
			name:     "mismatched sources and prefixes",
			sources:  []string{"../../test/_shared/resources/test.yaml"},
			prefixes: []string{"/api", "/extra"},
			wantErr:  true,
		},
		{
			name:     "empty prefixes treated as empty strings",
			sources:  []string{"../../test/_shared/resources/test.yaml", "../../test/_shared/resources/test-params.yaml"},
			prefixes: []string{},
			wantErr:  false,
		},
		{
			name:     "invalid schema path",
			sources:  []string{"non-existent.yaml"},
			prefixes: []string{""},
			wantErr:  true,
		},
		{
			name:     "mixing openapi and asyncapi sources",
			sources:  []string{"../../test/_shared/resources/test.yaml", "../../test/_shared/resources/asyncapi-30.yaml"},
			prefixes: []string{"/v1", "/v2"},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			infos, err := LoadSchemas(tt.sources, tt.prefixes)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			require.Len(t, infos, len(tt.sources))
			for i, info := range infos {
				if info.Kind == KindAsyncAPI {
					assert.NotNil(t, info.Async, "info[%d].Async is nil", i)
					continue
				}
				assert.NotNil(t, info.Spec, "info[%d].Spec is nil", i)
				expectedPrefix := ""
				if i < len(tt.prefixes) {
					expectedPrefix = tt.prefixes[i]
				}
				assert.Equal(t, expectedPrefix, info.Prefix, "info[%d].Prefix mismatch", i)
			}
		})
	}
}
