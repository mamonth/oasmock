package asyncapi

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return data
}

/*
Scenario: Parsing a valid AsyncAPI 3.0.0 spec
Given YAML data with asyncapi: 3.0.0, a channel and an operation
When Parse is called
Then it returns a document with version, channel, operation, message and extensions

Related spec scenarios: RS.AAL.2, RS.AAL.5, RS.AAL.11
*/
func TestParse_Valid30(t *testing.T) {
	t.Parallel()

	doc, err := Parse(readFixture(t, "test-30.yaml"))
	require.NoError(t, err)
	require.Equal(t, "3.0.0", doc.Version)
	require.Len(t, doc.Channels, 1)
	require.Len(t, doc.Operations, 1)

	ch := doc.Channels[0]
	assert.Equal(t, "userSignedUp", ch.ID)
	assert.Equal(t, "user/signedup", ch.Address)
	require.Len(t, ch.Messages, 1)
	assert.Len(t, ch.Messages[0].Examples, 2)
	assert.NotEmpty(t, ch.Messages[0].Examples[0].Extensions["x-mock-match"])
	assert.Equal(t, true, ch.Messages[0].Examples[0].Extensions["x-mock-once"])
	assert.Equal(t, true, ch.Messages[0].Examples[1].Extensions["x-mock-set-state"] != nil)

	op := doc.Operations[0]
	assert.Equal(t, "receiveUserSignedUp", op.ID)
	assert.Equal(t, ActionReceive, op.Action)
	assert.Equal(t, "userSignedUp", op.Channel.ID)
	require.Len(t, op.Messages, 1)
	assert.Equal(t, "auserSignedUp", op.Messages[0].Name)
}

/*
Scenario: Parsing a valid AsyncAPI 3.1.0 spec with components
Given YAML data with asyncapi: 3.1.0 and a components section
When Parse is called
Then it returns a document without error

Related spec scenarios: RS.AAL.3, RS.AAL.7
*/
func TestParse_Valid31(t *testing.T) {
	t.Parallel()

	doc, err := Parse(readFixture(t, "test-31.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "3.1.0", doc.Version)
	require.Len(t, doc.Channels, 1)
	assert.Equal(t, "user/signedup", doc.Channels[0].Address)
}

/*
Scenario: Rejecting an AMQP binding as unsupported
Given YAML data with an amqp channel binding declaring an exchange
When Parse is called
Then it reports a validation error naming the unsupported protocol (amqp)

Related spec scenarios: RS.AAL.8, RS.ASP.4
*/
func TestParse_AMQPBindingRejected(t *testing.T) {
	t.Parallel()

	data := []byte(`asyncapi: 3.0.0
info:
  title: AMQP Events
  version: 1.0.0
channels:
  userSignup:
    address: 'user/signup'
    bindings:
      amqp:
        is: routingKey
        exchange:
          name: myExchange
          type: topic
    messages:
      msg:
        examples:
          - payload:
              event: signup
operations:
  receiveSignup:
    action: receive
    channel:
      $ref: '#/channels/userSignup'
`)
	_, err := Parse(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "amqp")
	assert.Contains(t, err.Error(), "unsupported protocol")
}

/*
Scenario: Rejecting an unsupported AsyncAPI major version
Given YAML data with asyncapi: 2.6.0
When Parse is called
Then it returns an error stating the version is unsupported

Related spec scenarios: RS.AAL.12
*/
func TestParse_UnsupportedVersion(t *testing.T) {
	t.Parallel()

	data := []byte("asyncapi: 2.6.0\ninfo:\n  title: x\n  version: 1.0.0\nchannels: {}\n")
	_, err := Parse(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported AsyncAPI version")
}

/*
Scenario: Rejecting missing mandatory channels
Given YAML data with asyncapi: 3.0.0 but no channels
When Parse is called
Then it reports a schema validation error

Related spec scenarios: RS.AAL.6
*/
func TestParse_MissingChannels(t *testing.T) {
	t.Parallel()

	data := []byte("asyncapi: 3.0.0\ninfo:\n  title: x\n  version: 1.0.0\noperations: {}\n")
	_, err := Parse(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "channels")
}

/*
Scenario: Rejecting missing operations in a 3.0.0 spec
Given YAML data with asyncapi: 3.0.0, channels but no operations
When Parse is called
Then it reports a schema validation error

Related spec scenarios: RS.AAL.6
*/
func TestParse_MissingOperations(t *testing.T) {
	t.Parallel()

	data := []byte("asyncapi: 3.0.0\ninfo:\n  title: x\n  version: 1.0.0\nchannels:\n  c:\n    address: a\n")
	_, err := Parse(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operations")
}

/*
Scenario: Detecting an unsupported protocol binding
Given YAML data with a channel declaring a kafka binding
When Parse is called
Then it reports a validation error naming the unsupported protocol

Related spec scenarios: RS.AAL.8, RS.ASP.4
*/
func TestParse_UnsupportedProtocol(t *testing.T) {
	t.Parallel()

	data := []byte(`asyncapi: 3.0.0
info:
  title: x
  version: 1.0.0
channels:
  c:
    address: topic
    bindings:
      kafka:
        topic: events
operations:
  o:
    action: receive
    channel:
      $ref: '#/channels/c'
`)
	_, err := Parse(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kafka")
	assert.Contains(t, err.Error(), "unsupported protocol")
}

/*
Scenario: File with neither version key is not handled by the AsyncAPI parser
Given data that is not an AsyncAPI document
When Parse is called
Then it returns an error

Related spec scenarios: RS.AAL.4
*/
func TestParse_NonAsyncAPI(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte("openapi: 3.0.0\ninfo:\n  title: x\n  version: 1.0.0\n"))
	require.Error(t, err)
}

/*
Scenario: Capturing a root-level x-signalr hub declaration
Given an AsyncAPI document with a root x-signalr extension carrying a hub path
When Parse is called
Then the neutral Document view exposes the SignalR config with the hub path

Related spec scenarios: RS.SHR.1, RS.SHR.2
*/
func TestParse_RootSignalR(t *testing.T) {
	t.Parallel()

	data := []byte(`
asyncapi: 3.0.0
info:
  title: SignalR Hub
  version: 1.0.0
x-signalr:
  path: /hub
channels:
  priceFeed:
    address: priceFeed
    bindings:
      ws:
        method: GET
    messages:
      priceMsg:
        examples:
          - name: snap
            payload:
              symbol: ETH
operations:
  receivePrice:
    action: receive
    channel:
      $ref: '#/channels/priceFeed'
`)
	doc, err := Parse(data)
	require.NoError(t, err)
	require.NotNil(t, doc.SignalR)
	assert.Equal(t, "/hub", doc.SignalR.Path)
}

/*
Scenario: Absence of root x-signalr yields a nil SignalR config
Given an AsyncAPI document without the x-signalr extension
When Parse is called
Then the neutral Document view has a nil SignalR config

Related spec scenarios: RS.SHR.1, RS.SHR.2
*/
func TestParse_NoRootSignalR(t *testing.T) {
	t.Parallel()

	doc, err := Parse(readFixture(t, "test-30.yaml"))
	require.NoError(t, err)
	assert.Nil(t, doc.SignalR)
}

/*
Scenario: Capturing x-* extensions on a message example
Given an AsyncAPI message example with an arbitrary vendor extension
When Parse is called
Then the neutral example view surfaces the extension generically under x-*
*/
func TestParse_MessageExampleVendorExtensions(t *testing.T) {
	t.Parallel()

	data := []byte(`
asyncapi: 3.0.0
info:
  title: Events
  version: 1.0.0
channels:
  alerts:
    address: alerts
    bindings:
      ws:
        method: GET
    messages:
      alertMsg:
        examples:
          - name: ex1
            payload:
              level: info
            x-mock-once: true
operations:
  receiveAlerts:
    action: receive
    channel:
      $ref: '#/channels/alerts'
`)
	doc, err := Parse(data)
	require.NoError(t, err)
	require.Len(t, doc.Channels, 1)
	require.Len(t, doc.Channels[0].Messages, 1)
	examples := doc.Channels[0].Messages[0].Examples
	require.Len(t, examples, 1)
	assert.Contains(t, examples[0].Extensions, "x-mock-once")
}
