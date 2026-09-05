package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const connectBuiltInDoc = `asyncapi: 3.0.0
info:
  title: Connect
  version: 1.0.0
channels:
  alerts:
    address: /alerts
    bindings:
      ws:
        method: GET
    messages:
      welcome:
        examples:
          - name: welcome1
            payload:
              msg: "hello {$connection.id}"
            x-mock-match:
              '{$event.name}': "connect"
operations:
  receiveAlerts:
    action: receive
    channel:
      $ref: '#/channels/alerts'
`

/*
Scenario: connect built-in fires on consumer connection
Given a message example matching the connect built-in
When a ws consumer connects
Then the templated message is delivered to the just-connected consumer

Related spec scenarios: RS.EVT.24, RS.EXT.21, RS.EXT.26
*/
func TestBuiltInConnect_FiresOnConnect(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(connectBuiltInDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg), `"msg":"hello conn-1"`)
}

/*
Scenario: connect built-in is a no-op without subscribers
Given a server without any connect subscription
When a ws consumer connects
Then no connect-driven message is delivered (only the receive snapshot)

Related spec scenarios: RS.EVT.24, RS.EXT.26
*/
func TestBuiltInConnect_NoSubscriberNoOp(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(pushChannelDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, `{"level":"info","msg":"default"}`, strings.TrimSpace(string(msg)))
}

const receiveBuiltInDoc = `asyncapi: 3.0.0
info:
  title: Receive
  version: 1.0.0
channels:
  chat:
    address: /chat
    bindings:
      ws:
        method: GET
    messages:
      reply:
        examples:
          - name: echo1
            payload:
              echoed: "{$event.text}"
            x-mock-match:
              '{$event.name}': "receive"
operations:
  sendChat:
    action: send
    channel:
      $ref: '#/channels/chat'
`

/*
Scenario: receive built-in fires on inbound client traffic
Given a message example matching the receive built-in
When a client sends a message on the channel
Then the templated message is emitted with the inbound message in the context

Related spec scenarios: RS.EVT.25, RS.EVT.23, RS.EXT.21
*/
func TestBuiltInReceive_FiresOnInbound(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(receiveBuiltInDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/chat"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"text":"hi"}`)))

	// The receive-built-in delivery is broadcast to channel consumers; read
	// the first frame carrying the echoed payload.
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	deadline := time.Now().Add(3 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		_, msg, rerr := conn.ReadMessage()
		if rerr != nil {
			break
		}
		got = string(msg)
		if strings.Contains(got, `"echoed"`) {
			break
		}
	}
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(got), &payload))
	assert.Equal(t, "hi", payload["echoed"])
}
