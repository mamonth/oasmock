package extensions

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Extracting event triggers from an OpenAPI example
Given an OpenAPI example with an x-event-trigger list
When ExtractEventTriggers is called
Then the parsed triggers carry name, payload, delay and global flags

Related spec scenarios: RS.EVT.1, RS.EVT.2, RS.EVT.3, RS.EVT.4
*/
func TestExtractEventTriggers(t *testing.T) {
	t.Parallel()

	ex := &openapi3.Example{
		Extensions: map[string]any{
			"x-event-trigger": []any{
				map[string]any{
					"name":    "orderCreated",
					"payload": map[string]any{"orderId": "1"},
					"delay":   float64(500),
					"global":  true,
				},
				map[string]any{
					"name": "notified",
				},
			},
		},
	}

	triggers, ok := ExtractEventTriggers(ex)
	require.True(t, ok)
	require.Len(t, triggers, 2)
	assert.Equal(t, "orderCreated", triggers[0].Name)
	assert.Equal(t, 500, triggers[0].Delay)
	assert.True(t, triggers[0].Global)
	assert.Equal(t, "1", triggers[0].Payload["orderId"])
	assert.Equal(t, "notified", triggers[1].Name)
	assert.False(t, triggers[1].Global)
}

/*
Scenario: No event triggers present
Given an OpenAPI example without x-event-trigger
When ExtractEventTriggers is called
Then it returns nil and false

Related spec scenarios: RS.EVT.1
*/
func TestExtractEventTriggers_Absent(t *testing.T) {
	t.Parallel()

	ex := &openapi3.Example{Extensions: map[string]any{"x-mock-once": true}}
	triggers, ok := ExtractEventTriggers(ex)
	assert.False(t, ok)
	assert.Nil(t, triggers)
}

/*
Scenario: Event trigger in short form is rejected
Given an OpenAPI example with a non-list x-event-trigger
When ExtractEventTriggers is called
Then it returns false

Related spec scenarios: RS.EVT.1
*/
func TestExtractEventTriggers_NonList(t *testing.T) {
	t.Parallel()

	ex := &openapi3.Example{
		Extensions: map[string]any{
			"x-event-trigger": map[string]any{"name": "x"},
		},
	}
	triggers, ok := ExtractEventTriggers(ex)
	assert.False(t, ok)
	assert.Nil(t, triggers)
}
