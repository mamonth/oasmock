package loader

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Loading single OpenAPI schema from file
Given a file path to an OpenAPI YAML
When loadSingleSchema is called
Then it returns parsed spec or error for missing/invalid files

Related spec scenarios: RS.MSC.1, RS.MSC.3
*/
func TestLoadSingleSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        string
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid OpenAPI YAML",
			path:        "../../test/_shared/resources/test.yaml",
			wantErr:     false,
			errContains: "",
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
			errContains: "invalid OpenAPI schema",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			spec, err := loadSingleSchema(tt.path)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, spec)
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
