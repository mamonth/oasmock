package server

import (
	"testing"

	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Rejecting a legacy cron entry without a wait interval
Given an example whose x-send-events contains {on: cron} without a wait
When derivedExamples maps the entry
Then loading fails loudly with an error naming the missing interval

Related spec scenarios: RS.EVT.18
*/
func TestDerivedExamples_CronWithoutWaitRejected(t *testing.T) {
	t.Parallel()

	bus := newEventBus(nil, nil, false)
	ex := &asyncapi.Example{
		Name:    "bad",
		Payload: map[string]any{"x": 1},
		Extensions: map[string]any{
			"x-send-events": []any{map[string]any{"on": "cron"}},
		},
	}
	_, err := bus.derivedExamples(ex)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wait")
}

/*
Scenario: Mapping legacy {on: cron, wait: N} to the interval shim
Given an example whose x-send-events contains {on: cron, wait: 1000}
When derivedExamples maps the entry
Then the example becomes periodically driven at the given interval

Related spec scenarios: RS.EVT.18, RS.EXT.22
*/
func TestDerivedExamples_CronWithWaitMapsToInterval(t *testing.T) {
	t.Parallel()

	bus := newEventBus(nil, nil, false)
	ex := &asyncapi.Example{
		Name:    "tick",
		Payload: map[string]any{"seq": 1},
		Extensions: map[string]any{
			"x-send-events": []any{map[string]any{"on": "cron", "wait": float64(1000)}},
		},
	}
	derived, err := bus.derivedExamples(ex)
	require.NoError(t, err)
	require.Len(t, derived, 1)
	require.NotNil(t, derived[0].Extensions["x-mock-interval"])
	assert.Equal(t, 1000, derived[0].Extensions["x-mock-interval"])
}
