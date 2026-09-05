package server

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/mamonth/oasmock/internal/extensions"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const incrementCronDoc = `asyncapi: 3.0.0
info:
  title: Cron
  version: 1.0.0
channels:
  feed:
    address: /feed
    bindings:
      ws:
        method: GET
    messages:
      tick:
        examples:
          - name: paced
            payload:
              seq: "{$state.counter}"
            x-mock-set-state:
              counter:
                increment: 1
            x-send-events:
              - on: cron
                wait: 1000
operations:
  receiveFeed:
    action: receive
    channel:
      $ref: '#/channels/feed'
`

/*
Scenario: Incrementing state from a message example
Given a message example whose x-mock-set-state increments a counter
When renderMessageSpecs runs
Then the state store Increment is applied with the delta

Related spec scenarios: RS.ATM.12
*/
func TestRenderMessageSpecs_Increment(t *testing.T) {
	t.Parallel()

	srv, _, stateStore, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	stateStore.EXPECT().GetNamespace(gomock.Any()).Return(map[string]any{}).AnyTimes()
	stateStore.EXPECT().Increment("/ns", "counter", 2.0).Return(9.0, nil)

	message := &loader.MessageSpec{
		Name: "m",
		Examples: []*loader.MessageExampleSpec{
			{
				Payload: map[string]any{"done": true},
				Extensions: map[string]any{
					"x-mock-set-state": map[string]any{"counter": map[string]any{"increment": 2}},
				},
			},
		},
	}
	count, out, err := srv.renderMessageSpecs([]*loader.MessageSpec{message}, "/ns", "op", InboundMessage{})
	require.NoError(t, err)
	require.Equal(t, 1, count)
	assert.Contains(t, string(out), `"done":true`)
}

/*
Scenario: Deleting a state key from a message example
Given a message example whose x-mock-set-state maps a key to null
When renderMessageSpecs runs
Then the state store Delete is applied for that key

Related spec scenarios: RS.ATM.13
*/
func TestRenderMessageSpecs_Delete(t *testing.T) {
	t.Parallel()

	srv, _, stateStore, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	stateStore.EXPECT().GetNamespace(gomock.Any()).Return(map[string]any{}).AnyTimes()
	stateStore.EXPECT().Delete("/ns", "key")

	message := &loader.MessageSpec{
		Name: "m",
		Examples: []*loader.MessageExampleSpec{
			{
				Payload: map[string]any{"done": true},
				Extensions: map[string]any{
					"x-mock-set-state": map[string]any{"key": nil},
				},
			},
		},
	}
	count, out, err := srv.renderMessageSpecs([]*loader.MessageSpec{message}, "/ns", "op", InboundMessage{})
	require.NoError(t, err)
	require.Equal(t, 1, count)
	assert.Contains(t, string(out), `"done":true`)
}

/*
Scenario: Cron subscriptions map to the periodic x-mock-interval shim
Given a message example subscribing to the cron built-in with a wait and a
state-backed sequence counter
When derivedExamples maps its x-send-events entry
Then the example becomes a periodically driven example with the wait interval,
keeping the pace-by-state-and-cron behavior of the templating spec

Related spec scenarios: RS.ATM.18, RS.EVT.18, RS.EXT.22
*/
func TestDerivedExamples_CronToPeriodic(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(incrementCronDoc))
	require.NoError(t, err)

	bus := newEventBus(nil, nil, false)
	ex := doc.Channels[0].Messages[0].Examples[0]
	derived, err := bus.derivedExamples(ex)
	require.NoError(t, err)
	require.Len(t, derived, 1)

	view := &MessageExampleView{spec: derived[0]}
	trig, err := extensions.ClassifyTrigger(view)
	require.NoError(t, err)
	assert.Equal(t, extensions.TriggerPeriodic, trig.Kind)
	assert.Equal(t, 1000, trig.Interval)
}
