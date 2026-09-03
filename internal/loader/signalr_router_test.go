package loader

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const signalRDocSpec = `asyncapi: 3.0.0
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
`

/*
Scenario: Declaring a SignalR hub document
Given an AsyncAPI document with root x-signalr and ws channels
When the loader parses it
Then the neutral document exposes the SignalR hub path

Related spec scenarios: RS.SHR.1
*/
func TestSignalR_DocumentParsing(t *testing.T) {
	t.Parallel()

	info := mustAsyncInfo(t, signalRDocSpec, "")
	require.NotNil(t, info.Async)
	require.NotNil(t, info.Async.SignalR)
	assert.Equal(t, "/hub", info.Async.SignalR.Path)
}

/*
Scenario: SignalR hub document keeps ws channels accessible by ID
Given an AsyncAPI document with root x-signalr
When the neutral document is inspected
Then ws channels remain addressable by ID for stream targets

Related spec scenarios: RS.SHR.3, RS.SHR.4
*/
func TestSignalR_ChannelsAccessible(t *testing.T) {
	t.Parallel()

	info := mustAsyncInfo(t, signalRDocSpec, "")
	ch := info.Async.Channel("priceFeed")
	require.NotNil(t, ch)
	assert.Equal(t, "priceFeed", ch.ID)
	require.Len(t, ch.Messages, 1)
	assert.Len(t, ch.Messages[0].Examples, 1)
}

/*
Scenario: SignalR hub document does not map ws channels to raw ws routes
Given an AsyncAPI document with root x-signalr and a ws channel
When BuildRouteMappings is called
Then no raw ws route mapping is produced (the hub serves ws channels)

Related spec scenarios: RS.SHR.1, RS.ASP.3
*/
func TestSignalR_NoRawWSRouteMappings(t *testing.T) {
	t.Parallel()

	info := mustAsyncInfo(t, signalRDocSpec, "")
	mappings, err := BuildRouteMappings([]SchemaInfo{info})
	require.NoError(t, err)
	for _, rm := range mappings {
		assert.NotEqual(t, "ws", rm.Protocol, "signalR hub channels must not map to raw ws routes")
	}
}
