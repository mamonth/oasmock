package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const twoChannelDoc = `asyncapi: 3.0.0
info:
  title: Alerts
  version: 1.0.0
channels:
  alerts:
    address: /alerts
    bindings:
      ws:
        method: GET
    messages:
      alertMsg:
        examples:
          - name: ex1
            payload:
              level: info
  watches:
    address: /watches
    bindings:
      ws:
        method: GET
    messages:
      watchMsg:
        examples:
          - name: ex1
            payload:
              kind: watch
operations:
  receiveAlerts:
    action: receive
    channel:
      $ref: '#/channels/alerts'
  receiveWatches:
    action: receive
    channel:
      $ref: '#/channels/watches'
`

func newTwoChannelServer(t *testing.T) *Server {
	t.Helper()
	doc, err := asyncapi.Parse([]byte(twoChannelDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)
	return srv
}

func consumeJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	return payload
}

/*
Scenario: Listing all consumers without a channel filter
Given raw ws consumers connected on multiple channels
When the consumers endpoint is queried without a channel parameter
Then the server returns a single flat list across all channels

Related spec scenarios: RS.AMG.22, RS.AMG.8, RS.AMG.9
*/
func TestAsyncConsumers_GetAll(t *testing.T) {
	t.Parallel()

	srv := newTwoChannelServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	base := "ws" + strings.TrimPrefix(ts.URL, "http")
	conn1, _, err := websocket.DefaultDialer.Dial(base+"/alerts", nil)
	require.NoError(t, err)
	defer conn1.Close() //nolint:errcheck
	conn2, _, err := websocket.DefaultDialer.Dial(base+"/watches", nil)
	require.NoError(t, err)
	defer conn2.Close() //nolint:errcheck

	waitForConnections(srv, "/alerts", 1)
	waitForConnections(srv, "/watches", 1)

	resp, err := http.Get(ts.URL + "/_mock/async/consumers")
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	payload := consumeJSON(t, resp)
	items, ok := payload["consumers"].([]any)
	require.True(t, ok)
	assert.Len(t, items, 2)

	channels := map[string]bool{}
	for _, item := range items {
		m, ok := item.(map[string]any)
		require.True(t, ok)
		channels[m["channel"].(string)] = true
	}
	assert.True(t, channels["/alerts"])
	assert.True(t, channels["/watches"])
}

/*
Scenario: Listing consumers with no connections returns empty
Given no connected consumers
When the consumers endpoint is queried without a channel filter
Then the server returns an empty list

Related spec scenarios: RS.AMG.22, RS.AMG.9
*/
func TestAsyncConsumers_GetAllEmpty(t *testing.T) {
	t.Parallel()

	srv := newTwoChannelServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	resp, err := http.Get(ts.URL + "/_mock/async/consumers")
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	payload := consumeJSON(t, resp)
	items, ok := payload["consumers"].([]any)
	require.True(t, ok)
	assert.Empty(t, items)
}

/*
Scenario: Listing signalr stream consumers without a channel filter
Given open SignalR streams across hub channels
When the consumers endpoint is queried without a channel
Then the stream consumers are included in the flat union

Related spec scenarios: RS.AMG.22, RS.AMG.8
*/
func TestAsyncConsumers_GetAllSignalR(t *testing.T) {
	t.Parallel()

	srv := newSignalRPushMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	hubURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/hub"

	conn, _, err := websocket.DefaultDialer.Dial(hubURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"protocol":"json","version":1}`)))
	_, _, err = conn.ReadMessage()
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":4,"invocationId":"s1","target":"priceFeed"}`+"\x1e")))
	_, _, err = conn.ReadMessage()
	require.NoError(t, err) // snapshot

	resp, err := http.Get(ts.URL + "/_mock/async/consumers")
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	payload := consumeJSON(t, resp)
	items, ok := payload["consumers"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, items)
	first, ok := items[0].(map[string]any)
	require.True(t, ok)
	assert.NotEmpty(t, first["streams"])
}
