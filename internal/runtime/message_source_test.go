package runtime

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Message payload/headers exposed via the message data source
Given a message payload and headers map
When the MessageSource is queried
Then payload and header fields resolve via {$message.*}

Related spec scenarios: RS.ATM.1, RS.ATM.5
*/
func TestMessageSource_Get(t *testing.T) {
	t.Parallel()

	src := &MessageSource{
		Payload: map[string]any{"id": "1", "nested": map[string]any{"k": "v"}},
		Headers: map[string]string{"x-request-id": "req-1"},
	}

	v, ok := src.Get("payload.id")
	require.True(t, ok)
	assert.Equal(t, "1", v)

	v, ok = src.Get("payload.nested.k")
	require.True(t, ok)
	assert.Equal(t, "v", v)

	v, ok = src.Get("headers.x-request-id")
	require.True(t, ok)
	assert.Equal(t, "req-1", v)
}

/*
Scenario: Channel parameters exposed via the channel data source
Given a channel params map
When the ChannelSource is queried
Then params resolve via {$channel.*}

Related spec scenarios: RS.ATM.3
*/
func TestChannelSource_Get(t *testing.T) {
	t.Parallel()

	src := &ChannelSource{Params: map[string]string{"userId": "u-42"}}

	v, ok := src.Get("userId")
	require.True(t, ok)
	assert.Equal(t, "u-42", v)

	_, ok = src.Get("missing")
	assert.False(t, ok)
}

/*
Scenario: Message expressions evaluate through the evaluator
Given an evaluator with a message source registered as "message"
When an expression is evaluated
Then the message payload value is returned

Related spec scenarios: RS.ATM.1, RS.ATM.5
*/
func TestEvaluator_MessageExpression(t *testing.T) {
	t.Parallel()

	eval := NewEvaluator()
	eval.AddSource("message", &MessageSource{
		Payload: map[string]any{"id": "42"},
	})

	val, err := eval.Evaluate("{$message.payload.id}")
	require.NoError(t, err)
	assert.Equal(t, "42", val)
}
