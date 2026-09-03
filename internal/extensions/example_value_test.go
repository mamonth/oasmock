package extensions

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: OpenAPI example and AsyncAPI message example share a wrapper
Given an OpenAPI example and a map-backed AsyncAPI example with equal extensions
When wrapped as ExampleValue
Then both expose the same extension values, payload and headers

Related spec scenarios: RS.ATM.10, RS.ATM.11
*/
func TestExampleValue_UniformAccess(t *testing.T) {
	t.Parallel()

	oe := &openapi3.Example{
		Value: map[string]any{"id": 1},
		Extensions: map[string]any{
			"x-mock-once":      true,
			"x-mock-set-state": map[string]any{"counter": 1},
		},
	}
	ov := OpenAPIExampleValue(oe)

	once, _ := ov.Get("x-mock-once")
	assert.Equal(t, true, once)
	setState, _ := ov.Get("x-mock-set-state")
	assert.Equal(t, map[string]any{"counter": 1}, setState)
	assert.Equal(t, map[string]any{"id": 1}, ov.Payload())

	// AsyncAPI message example backed by a plain map (as captured by the
	// vendored parser's Example.Extensions).
	ae := NewExampleValue(map[string]any{"id": 2}, nil, map[string]any{
		"x-mock-once":      true,
		"x-mock-set-state": map[string]any{"counter": 2},
	})
	once, _ = ae.Get("x-mock-once")
	assert.Equal(t, true, once)
	setState, _ = ae.Get("x-mock-set-state")
	assert.Equal(t, map[string]any{"counter": 2}, setState)
	assert.Equal(t, map[string]any{"id": 2}, ae.Payload())
}

/*
Scenario: ExampleValue exposes extension extraction uniformly
Given an ExampleValue with x-mock-skip and x-mock-headers
When the generic extractors run
Then skip and headers behave identically to the OpenAPI-specific helpers

Related spec scenarios: RS.ATM.9, RS.ATM.14
*/
func TestExampleValue_ExtractUniform(t *testing.T) {
	t.Parallel()

	ev := NewExampleValue(map[string]any{"x": 1}, nil, map[string]any{
		"x-mock-skip":    true,
		"x-mock-headers": map[string]any{"X-Trace": "abc"},
	})

	assert.True(t, ValueSkip(ev))
	headers, ok := ValueHeaders(ev)
	require.True(t, ok)
	assert.Equal(t, "abc", headers["X-Trace"])
}

/*
Scenario: AsyncAPI message example wrapper resolves from loader spec
Given an AsyncAPI message example spec
When NewExampleValue is called
Then the wrapper exposes extensions and payload

Related spec scenarios: RS.ATM.6, RS.ATM.8
*/
func TestExampleValue_FromAsyncSpec(t *testing.T) {
	t.Parallel()

	ex := map[string]any{
		"x-mock-match": map[string]any{"{$message.payload.id}": map[string]any{"type": "integer"}},
	}
	ev := NewExampleValue(map[string]any{"id": 1}, nil, ex)

	match, ok := ValueMatch(ev)
	require.True(t, ok)
	_, hasExpr := match["{$message.payload.id}"]
	assert.True(t, hasExpr)
}

/*
Scenario: x-mock-match wins over the deprecated x-mock-params-match alias
Given an example value carrying both extensions
When ValueMatch is called
Then the x-mock-match map is returned
And x-mock-params-match alone still resolves

Related spec scenarios: RS.ATM.8
*/
func TestValueMatch_Precedence(t *testing.T) {
	t.Parallel()

	both := NewExampleValue(map[string]any{"id": 1}, nil, map[string]any{
		"x-mock-match":        map[string]any{"{$message.payload.id}": map[string]any{"type": "integer"}},
		"x-mock-params-match": map[string]any{"{$message.payload.id}": map[string]any{"type": "string"}},
	})
	match, ok := ValueMatch(both)
	require.True(t, ok)
	cond, hasCond := match["{$message.payload.id}"]
	require.True(t, hasCond)
	assert.Equal(t, map[string]any{"type": "integer"}, cond)

	legacyOnly := NewExampleValue(map[string]any{"id": 1}, nil, map[string]any{
		"x-mock-params-match": map[string]any{"{$message.payload.id}": map[string]any{"type": "string"}},
	})
	m, ok := ValueMatch(legacyOnly)
	require.True(t, ok)
	cond2, hasCond2 := m["{$message.payload.id}"]
	require.True(t, hasCond2)
	assert.Equal(t, map[string]any{"type": "string"}, cond2)
}
