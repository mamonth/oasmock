package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Event payload is exposed via the event data source
Given an event payload map and a nested value
When the EventSource is queried
Then the payload fields resolve via {$event.*}

Related spec scenarios: RS.EVT.8, RS.ATM.17
*/
func TestEventSource_Get(t *testing.T) {
	t.Parallel()

	src := &EventSource{Data: map[string]any{
		"accountId": "acc-1",
		"order":     map[string]any{"price": 10},
	}}

	v, ok := src.Get("accountId")
	require.True(t, ok)
	assert.Equal(t, "acc-1", v)

	v, ok = src.Get("order.price")
	require.True(t, ok)
	assert.Equal(t, 10, v)

	_, ok = src.Get("missing")
	assert.False(t, ok)
}

/*
Scenario: Event expressions evaluate through the evaluator
Given an evaluator with an event source registered as "event"
When an expression is evaluated
Then the event payload value is returned

Related spec scenarios: RS.EVT.8
*/
func TestEvaluator_EventExpression(t *testing.T) {
	t.Parallel()

	eval := NewEvaluator()
	eval.AddSource("event", &EventSource{Data: map[string]any{"level": "info"}})

	val, err := eval.Evaluate("{$event.level}")
	require.NoError(t, err)
	assert.Equal(t, "info", val)
}
