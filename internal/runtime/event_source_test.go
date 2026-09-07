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

Related spec scenarios: RS.EVT.23, RS.ATM.17
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

Related spec scenarios: RS.EVT.23
*/
func TestEvaluator_EventExpression(t *testing.T) {
	t.Parallel()

	eval := NewEvaluator()
	eval.AddSource("event", &EventSource{Data: map[string]any{"level": "info"}})

	val, err := eval.Evaluate("{$event.level}")
	require.NoError(t, err)
	assert.Equal(t, "info", val)
}

/*
Scenario: Event identity is exposed via the reserved name accessor
Given an EventSource carrying an event name and a payload
When the name accessor is queried
Then {$event.name} resolves to the event identity (registered name or built-in kind)

Related spec scenarios: RS.EXT.18
*/
func TestEventSource_GetName(t *testing.T) {
	t.Parallel()

	src := &EventSource{Name: "orderCreated", Data: map[string]any{"accountId": "acc-1"}}

	v, ok := src.Get("name")
	require.True(t, ok)
	assert.Equal(t, "orderCreated", v)
}

/*
Scenario: Whole-payload access via the reserved data accessor
Given an EventSource carrying a payload
When the data accessor is queried
Then {$event.data} resolves to the whole payload object

Related spec scenarios: RS.EXT.19
*/
func TestEventSource_GetData(t *testing.T) {
	t.Parallel()

	payload := map[string]any{"accountId": "acc-1", "amount": 10}
	src := &EventSource{Name: "orderCreated", Data: payload}

	v, ok := src.Get("data")
	require.True(t, ok)
	assert.Equal(t, payload, v)
}

/*
Scenario: Payload field access is unchanged alongside reserved accessors
Given an EventSource with payload fields named and data
When the payload field accessors are queried
Then field names shadowed by reserved accessors are reachable only via data

Related spec scenarios: RS.EXT.18, RS.EXT.19
*/
func TestEventSource_ReservedFieldsShadowed(t *testing.T) {
	t.Parallel()

	src := &EventSource{Name: "orderCreated", Data: map[string]any{
		"name": "shadowed",
		"data": "shadowed",
	}}

	name, ok := src.Get("name")
	require.True(t, ok)
	assert.Equal(t, "orderCreated", name)

	data, ok := src.Get("data")
	require.True(t, ok)
	assert.Equal(t, map[string]any{"name": "shadowed", "data": "shadowed"}, data)
}
