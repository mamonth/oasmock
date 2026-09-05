package server

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gorilla/websocket"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Acknowledging a ws send with no reply via echo
Given an AsyncAPI ws channel with a send operation and no reply message
When a ws client sends a message
Then the server acknowledges receipt with a frame

Related spec scenarios: RS.ASP.6, RS.ASP.9
*/
func TestWSProtocolAdapter_SendEchoAck(t *testing.T) {
	t.Parallel()

	srv, _, stateStore, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	stateStore.EXPECT().GetNamespace(gomock.Any()).Return(map[string]any{}).AnyTimes()

	mapping := &RouteMapping{
		Protocol: asyncWSProtocol,
		Action:   "send",
		Path:     "/socket",
		Pattern:  "/socket",
		Messages: nil,
	}

	adapter := srv.adapterForProtocol(asyncWSProtocol)
	require.NotNil(t, adapter)

	ts := httptest.NewServer(adapter.Handler(mapping, srv.asyncMessageHandler(mapping)))
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + ts.URL[4:] + "/socket"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"hello":"world"}`)))

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(msg))
}

/*
Scenario: Receive operation emits the message example on connect
Given an AsyncAPI ws channel with a receive operation and a message example
When a ws client connects
Then the server emits the operation's message example

Related spec scenarios: RS.ASP.2, RS.ASP.7
*/
func TestWSProtocolAdapter_ReceiveEmitsOnConnect(t *testing.T) {
	t.Parallel()

	srv, _, stateStore, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	stateStore.EXPECT().GetNamespace(gomock.Any()).Return(map[string]any{}).AnyTimes()

	mapping := &RouteMapping{
		Protocol: asyncWSProtocol,
		Action:   "receive",
		Path:     "/prices",
		Pattern:  "/prices",
		Messages: []*loader.MessageSpec{
			{
				Name: "priceMsg",
				Examples: []*loader.MessageExampleSpec{
					{Payload: map[string]any{"symbol": "ETH", "price": 3000}},
				},
			},
		},
	}

	adapter := srv.adapterForProtocol(asyncWSProtocol)
	require.NotNil(t, adapter)

	ts := httptest.NewServer(adapter.Handler(mapping, srv.asyncMessageHandler(mapping)))
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + ts.URL[4:] + "/prices"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.JSONEq(t, `{"symbol":"ETH","price":3000}`, string(msg))
}

/*
Scenario: Connection registry tracks and removes connections
Given a registry with a registered connection
When unregister is called
Then the connection is no longer listed

Related spec scenarios: RS.AMG.8
*/
func TestConnectionRegistry_Lifecycle(t *testing.T) {
	t.Parallel()

	registry := newConnectionRegistry()
	id := registry.register("/chan", "", nil, nil, nil)
	assert.Equal(t, "/chan", registry.connections("/chan")[0].channel)

	registry.unregister(id)
	assert.Empty(t, registry.connections("/chan"))
	assert.Nil(t, registry.byID[id])
}
