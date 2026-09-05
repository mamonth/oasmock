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

const signalrReceiveBuiltInDoc = `asyncapi: 3.0.0
info:
  title: Hub
  version: 1.0.0
x-signalr:
  path: /hub
channels:
  chat:
    address: chat
    bindings:
      ws:
        method: GET
    messages:
      echo:
        examples:
          - name: snap
            payload:
              ok: true
          - name: e1
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
Scenario: receive built-in fires on a SignalR inbound invocation
Given a message example matching the receive built-in on a SignalR hub channel
When a SignalR client sends an invocation carrying a payload
Then the templated message is broadcast to the channel (open stream)

Related spec scenarios: RS.EVT.25
*/
func TestSignalRReceiveBuiltIn_FiresOnInbound(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(signalrReceiveBuiltInDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	hubURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/hub"

	conn, _, err := websocket.DefaultDialer.Dial(hubURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"protocol":"json","version":1}`)))
	_, _, err = conn.ReadMessage()
	require.NoError(t, err) // handshake reply

	// Open a stream on the chat channel so delivery has a target.
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":4,"invocationId":"s1","target":"chat"}`+"\x1e")))
	_, _, err = conn.ReadMessage()
	require.NoError(t, err) // snapshot/completion

	// Send an invocation carrying a payload → receive built-in fires.
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":1,"invocationId":"c1","target":"sendChat","arguments":[{"text":"hi"}]}`+"\x1e")))

	deadline := time.Now().Add(3 * time.Second)
	got := ""
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(deadline)
		_, msg, rerr := conn.ReadMessage()
		if rerr != nil {
			break
		}
		for _, frame := range splitSignalRFrames(msg) {
			var env signalREnvelope
			if err := json.Unmarshal(frame, &env); err != nil {
				continue
			}
			if env.Type != signalRTypeStreamItem {
				continue
			}
			raw, err := json.Marshal(env.Item)
			if err == nil {
				got = string(raw)
			}
		}
		if strings.Contains(got, `"echoed"`) {
			break
		}
	}
	assert.Contains(t, got, `"echoed":"hi"`)
}

const signalrConnectQueryBuiltInDoc = `asyncapi: 3.0.0
info:
  title: Hub
  version: 1.0.0
x-signalr:
  path: /hub
channels:
  chat:
    address: chat
    bindings:
      ws:
        method: GET
    messages:
      welcome:
        examples:
          - name: w
            payload:
              msg: "welcome"
            x-mock-match:
              '{$event.name}': "connect"
              '{$connection.query.tid}': "abc"
operations:
  sendChat:
    action: send
    channel:
      $ref: '#/channels/chat'
`

/*
Scenario: connect built-in resolves {$connection.query.*} on a SignalR upgrade
Given a connect example matching {$connection.query.tid} = abc
When a SignalR client connects through the hub with tid=abc
Then the welcome message is delivered to that single connection

Related spec scenarios: RS.EXT.27, RS.EVT.24
*/
func TestSignalRConnectBuiltIn_ResolvesConnectionQuery(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(signalrConnectQueryBuiltInDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	hubURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/hub?tid=abc"

	conn, _, err := websocket.DefaultDialer.Dial(hubURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"protocol":"json","version":1}`)))
	// consume any handshake reply and the connect-built-in invitation.
	deadline := time.Now().Add(3 * time.Second)
	got := ""
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(deadline)
		_, msg, rerr := conn.ReadMessage()
		if rerr != nil {
			break
		}
		for _, frame := range splitSignalRFrames(msg) {
			var env signalREnvelope
			if err := json.Unmarshal(frame, &env); err != nil {
				continue
			}
			if env.Type != signalRTypeInvocation {
				continue
			}
			raw, err := json.Marshal(env.Arguments)
			if err == nil {
				got = string(raw)
			}
		}
		if strings.Contains(got, `"welcome"`) {
			break
		}
	}
	assert.Contains(t, got, `"welcome"`)
}

/*
Scenario: Default hub channel selection is deterministic across map order
Given a SignalR hub serving several channels with a schema prefix
When the default channel address is computed
Then it resolves to the lexicographically smallest prefixed address regardless
of map iteration order (built-ins and recipient metadata become deterministic)

Related spec scenarios: RS.EVT.24, RS.EXT.27
*/
func TestHubDefaultChannel_Deterministic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prefix   string
		channels map[string]*asyncapi.Channel
		wantAddr string
	}{
		{name: "two channels picks lexicographic min", prefix: "/v1", channels: map[string]*asyncapi.Channel{
			"zeta":  {Address: "/zeta"},
			"alpha": {Address: "/alpha"},
		}, wantAddr: "/v1/alpha"},
		{name: "no prefix", prefix: "", channels: map[string]*asyncapi.Channel{
			"alerts": {Address: "alerts"},
		}, wantAddr: "/alerts"},
		{name: "empty address ignored", prefix: "/v1", channels: map[string]*asyncapi.Channel{
			"noaddr": {Address: ""},
			"b":      {Address: "/b"},
		}, wantAddr: "/v1/b"},
		{name: "no channels yields empty", prefix: "/v1", channels: map[string]*asyncapi.Channel{}, wantAddr: ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			hub := &signalRHub{prefix: tt.prefix, channels: tt.channels}
			assert.Equal(t, tt.wantAddr, hubDefaultChannel(hub))
		})
	}
}
