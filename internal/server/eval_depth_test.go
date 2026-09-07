package server

import (
	"testing"

	"github.com/mamonth/oasmock/internal/runtime"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Bounding recursive templating depth
Given a value nested deeper than the evaluation limit
When evaluateValue is called
Then it returns an error instead of overflowing the stack

Related spec scenarios: RS.MSC.13
*/
func TestEvaluateValueBoundsRecursionDepth(t *testing.T) {
	eng := &exampleEngine{}

	var nested any
	inner := map[string]any{"leaf": "value"}
	for i := 0; i <= maxEvaluationDepth+1; i++ {
		nested = map[string]any{"next": inner}
		inner = nested.(map[string]any)
	}

	eval := runtime.NewEvaluator()
	_, err := eng.evaluateValue(inner, eval)
	require.Error(t, err)
	require.Contains(t, err.Error(), "maximum depth")

	// A shallow value still succeeds.
	got, err := eng.evaluateValue(map[string]any{"a": []any{"x", "y"}}, eval)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"a": []any{"x", "y"}}, got)
}
