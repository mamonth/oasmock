package asyncapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Looking up a channel by ID
Given a parsed AsyncAPI document
When Channel is called with an existing and a missing ID
Then the existing channel is returned and the missing lookup yields nil

Related spec scenarios: RS.SHR.3, RS.SHR.5
*/
func TestDocument_ChannelLookup(t *testing.T) {
	t.Parallel()

	doc, err := Parse(readFixture(t, "test-30.yaml"))
	require.NoError(t, err)

	ch := doc.Channel("userSignedUp")
	require.NotNil(t, ch)
	assert.Equal(t, "user/signedup", ch.Address)

	assert.Nil(t, doc.Channel("missing"))
}

/*
Scenario: Looking up an operation by ID
Given a parsed AsyncAPI document
When Operation is called with an existing and a missing ID
Then the existing operation is returned and the missing lookup yields nil

Related spec scenarios: RS.SHR.6, RS.SHR.7
*/
func TestDocument_OperationLookup(t *testing.T) {
	t.Parallel()

	doc, err := Parse(readFixture(t, "test-30.yaml"))
	require.NoError(t, err)

	op := doc.Operation("receiveUserSignedUp")
	require.NotNil(t, op)
	assert.Equal(t, ActionReceive, op.Action)

	assert.Nil(t, doc.Operation("missing"))
}
