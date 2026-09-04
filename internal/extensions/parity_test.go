package extensions

import (
	"encoding/json"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Identical selection metadata across OpenAPI and AsyncAPI examples
Given equivalent OpenAPI and AsyncAPI examples carrying the same x-mock-* set
When the wrapper-based extractors run on both
Then selection decisions (match/skip/once/state/headers) are identical

Related spec scenarios: RS.ATM.6, RS.ATM.7, RS.ATM.9, RS.ATM.10, RS.ATM.11, RS.ATM.14
*/
func TestExampleValue_OpenAPIAsyncAPIParity(t *testing.T) {
	t.Parallel()

	ext := map[string]any{
		"x-mock-match": map[string]any{
			"{$message.payload.id}": map[string]any{"type": "integer"},
		},
		"x-mock-once":      true,
		"x-mock-set-state": map[string]any{"counter": 1},
		"x-mock-headers":   map[string]any{"X-Trace": "abc"},
	}

	asyncValue := NewExampleValue(map[string]any{"id": 1}, nil, ext)
	openapiValue := OpenAPIExampleValue(&openapi3.Example{
		Value:      map[string]any{"id": 1},
		Extensions: ext,
	})

	for _, tc := range []struct {
		name string
		got  func(ExampleValue) any
	}{
		{name: "match", got: func(v ExampleValue) any {
			m, ok := ValueMatch(v)
			return []any{m, ok}
		}},
		{name: "skip", got: func(v ExampleValue) any { return ValueSkip(v) }},
		{name: "once", got: func(v ExampleValue) any { return ValueOnce(v) }},
		{name: "set-state", got: func(v ExampleValue) any {
			m, ok := ValueSetState(v)
			return []any{m, ok}
		}},
		{name: "headers", got: func(v ExampleValue) any {
			m, ok := ValueHeaders(v)
			return []any{m, ok}
		}},
	} {
		a := tc.got(asyncValue)
		b := tc.got(openapiValue)
		require.NotNil(t, a)
		require.NotNil(t, b)
		aj := parityJSON(t, a)
		bj := parityJSON(t, b)
		assert.Equal(t, aj, bj, "parity mismatch for %s", tc.name)
	}
}

// parityJSON renders a value as deterministic JSON for comparison.
func parityJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return string(data)
}
