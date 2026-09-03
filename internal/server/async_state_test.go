package server

import (
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/mamonth/oasmock/internal/asyncapi"
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
Scenario: Cron subscriptions render an incrementing state counter per delivery
Given a message example subscribing to the cron built-in with an increment
When collectSchemaSubscriptions captures it and the spec is rendered
Then the subscription is retained and each delivery applies the increment

Related spec scenarios: RS.ATM.18, RS.EVT.10
*/
func TestCollectSchemaSubscriptions_CronIncrement(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(incrementCronDoc))
	require.NoError(t, err)

	subs := collectSchemaSubscriptions("", doc)
	require.Len(t, subs, 1)
	assert.Equal(t, "cron", subs[0].event)
	require.Len(t, subs[0].messages, 1)
	require.NotNil(t, subs[0].messages[0].spec)
	require.Len(t, subs[0].messages[0].spec.Examples, 1)

	srv, _, stateStore, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	stateStore.EXPECT().GetNamespace(gomock.Any()).Return(map[string]any{}).AnyTimes()
	stateStore.EXPECT().Increment("/ns", "counter", 1.0).Return(1.0, nil)

	count, out, err := srv.renderMessageSpecsWithEvent(
		[]*loader.MessageSpec{subs[0].messages[0].spec},
		"/ns",
		"op",
		map[string]any{},
	)
	require.NoError(t, err)
	require.Equal(t, 1, count)

	var body map[string]any
	require.NoError(t, json.Unmarshal(out, &body))
	assert.Contains(t, body, "seq")
}
