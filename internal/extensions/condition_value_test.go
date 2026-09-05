package extensions

import (
	"testing"

	"github.com/mamonth/oasmock/internal/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Reply-path condition values are compared literally
Given an x-mock-match whose key references the reply context and whose value is
a full runtime expression string
When EvaluateParamsMatch runs on the reply path
Then the value is compared as a literal string, not pre-resolved

Related spec scenarios: RS.EXT.30
*/
func TestEvaluateParamsMatch_ReplyConditionValueStaysLiteral(t *testing.T) {
	t.Parallel()

	eval := newMatchEvaluator(&runtime.RequestSource{
		QueryParams: map[string][]string{"id": {"abc"}},
	}, nil, nil, map[string]any{"token": "abc"})

	pm := ParamsMatch{"{$request.query.id}": "{$state.token}"}
	ok, err := EvaluateParamsMatch(pm, eval)
	require.NoError(t, err)
	assert.False(t, ok, "a reply-path condition value must be compared literally, never resolved")
}

/*
Scenario: Event-context condition values are pre-resolved
Given an x-mock-match whose key and value both reference the event context
When EvaluateParamsMatch runs against the event context
Then the value expression is resolved before comparison

Related spec scenarios: RS.EXT.18, RS.EXT.19
*/
func TestEvaluateParamsMatch_EventContextValueResolves(t *testing.T) {
	t.Parallel()

	eval := newMatchEvaluator(nil, &runtime.EventSource{
		Name: "orderCreated",
		Data: map[string]any{"accountId": "acc-1", "expectedAccount": "acc-1"},
	}, nil, nil)

	pm := ParamsMatch{"{$event.accountId}": "{$event.expectedAccount}"}
	ok, err := EvaluateParamsMatch(pm, eval)
	require.NoError(t, err)
	assert.True(t, ok)
}

/*
Scenario: Connection-context condition values are pre-resolved
Given an x-mock-match with '{$connection.id}': '{$event.connectionId}'
When EvaluateParamsMatch runs with the matching connection and event contexts
Then the condition matches

Related spec scenarios: RS.EXT.24, RS.EXT.27
*/
func TestEvaluateParamsMatch_ConnectionValueResolves(t *testing.T) {
	t.Parallel()

	eval := newMatchEvaluator(nil, &runtime.EventSource{
		Name: "orderCreated",
		Data: map[string]any{"connectionId": "c1"},
	}, &runtime.ConnectionSource{ID: "c1"}, nil)

	pm := ParamsMatch{"{$connection.id}": "{$event.connectionId}"}
	ok, err := EvaluateParamsMatch(pm, eval)
	require.NoError(t, err)
	assert.True(t, ok)
}
