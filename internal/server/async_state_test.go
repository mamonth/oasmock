package server

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Incrementing state from a message example
Given a message example whose x-mock-set-state increments a counter
When renderMessageSpecs runs
Then the state store Increment is applied with the delta

Related spec scenarios: RS.ATM.12
*/
func TestRenderMessageSpecs_Increment(t *testing.T) {
	t.Parallel()

	srv, _, stateStore, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
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

	srv, _, stateStore, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
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
