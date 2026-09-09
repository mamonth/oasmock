package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDynamicServer(t *testing.T) *Server {
	t.Helper()
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: parsePushDoc(t), Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)
	return srv
}

/*
Scenario: Adding a dynamic example for an AsyncAPI channel
Given an AsyncAPI ws channel and a POST to /_mock/examples with a channel identifier
When a matching message arrives
Then the dynamic example is selected by the shared pipeline

Related spec scenarios: RS.MAPI.19, RS.MAPI.20
*/
func TestAddExample_AsyncChannel(t *testing.T) {
	t.Parallel()

	srv := newDynamicServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	addBody := `{
		"protocol": "ws",
		"channel": "/alerts",
		"response": {"code": 200, "body": {"level": "dynamic", "msg": "injected"}}
	}`
	resp, err := http.Post(ts.URL+"/_mock/examples", "application/json", strings.NewReader(addBody))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg), `"injected"`)
}

/*
Scenario: Adding a dynamic example for an unmatched AsyncAPI route is rejected
Given a POST to /_mock/examples with an unknown channel
When it does not match any loaded channel
Then the server responds with HTTP 400

Related spec scenarios: RS.MAPI.21
*/
func TestAddExample_UnmatchedAsyncRoute(t *testing.T) {
	t.Parallel()

	srv := newDynamicServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	addBody := `{
		"protocol": "ws",
		"channel": "/missing",
		"response": {"code": 200, "body": {"a": 1}}
	}`
	resp, err := http.Post(ts.URL+"/_mock/examples", "application/json", strings.NewReader(addBody))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

/*
Scenario: Adding an async example for a SignalR hub channel
Given a SignalR hub channel address and an interval trigger
When a POST is sent to /_mock/examples targeting the hub channel
Then the server resolves the target to the hub channel and accepts the example

Related spec scenarios: RS.MAPI.38, RS.SHR.27
*/
func TestAddExample_SignalRHubChannel(t *testing.T) {
	t.Parallel()

	srv := newSignalRPushMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	addBody := `{
		"protocol": "signalr",
		"channel": "/priceFeed",
		"interval": 1000,
		"response": {"code": 200, "body": {"symbol": "BTC"}}
	}`
	resp := postExample(t, ts.URL, addBody)
	defer resp.Body.Close() //nolint:errcheck
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var payload struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	assert.NotEmpty(t, payload.ID)
	assert.Equal(t, "interval", payload.Kind)
	t.Cleanup(func() { srv.deleteExample(payload.ID) })
}

/*
Scenario: Event-triggered hub example delivers into an open stream
Given a SignalR consumer with an open stream on a hub channel
When an event-triggered example is registered for the hub channel and the event fires
Then the hub channel's open stream receives the delivered StreamItem

Related spec scenarios: RS.MAPI.38, RS.SHR.27
*/
func TestAddExample_SignalRHubEventTrigger(t *testing.T) {
	t.Parallel()

	srv := newSignalRPushMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	conn := dialSignalRHub(t, ts.URL, "/hub")
	_ = streamInvoke(t, conn, "priceFeed", "s1") // open stream, drain snapshot

	addBody := `{"channel":"/priceFeed","conditions":{"{$event.name}":"levelUp"},"response":{"code":200,"body":{"msg":"{$event.msg}"}}}`
	id := addExample(t, ts.URL, addBody)
	t.Cleanup(func() { srv.deleteExample(id) })

	fire, err := http.Post(ts.URL+"/_mock/events", "application/json",
		strings.NewReader(`{"name":"levelUp","payload":{"msg":"boom"}}`))
	require.NoError(t, err)
	_ = fire.Body.Close()
	require.Equal(t, http.StatusOK, fire.StatusCode)

	env := readSignalRFrame(t, conn)
	assert.Equal(t, signalRTypeStreamItem, env.Type)
	raw, _ := json.Marshal(env.Item)
	assert.JSONEq(t, `{"msg":"boom"}`, string(raw))
}

/*
Scenario: Interval-triggered hub example delivers into an open stream
Given a SignalR consumer with an open stream on a hub channel
When an interval example is registered for the hub channel
Then the hub channel's open stream receives recurring StreamItems

Related spec scenarios: RS.MAPI.38, RS.SHR.27
*/
func TestAddExample_SignalRHubIntervalTrigger(t *testing.T) {
	t.Parallel()

	srv := newSignalRPushMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	// Open the stream before registering the interval example: a tick that
	// lands before the stream is registered is dropped (or falls back to a
	// server Invocation), which would make the read order nondeterministic.
	conn := dialSignalRHub(t, ts.URL, "/hub")
	_ = streamInvoke(t, conn, "priceFeed", "s1") // open stream, drain snapshot

	addBody := `{"channel":"/priceFeed","interval":40,"response":{"code":200,"body":{"tick":true}}}`
	id := addExample(t, ts.URL, addBody)
	t.Cleanup(func() { srv.deleteExample(id) })

	env := readSignalRFrame(t, conn)
	assert.Equal(t, signalRTypeStreamItem, env.Type)
	raw, _ := json.Marshal(env.Item)
	assert.Contains(t, string(raw), `"tick"`)
}

/*
Scenario: Registering an example for an unknown hub channel is rejected
Given a POST to /_mock/examples targeting a SignalR address that matches no hub channel
When the registration is attempted
Then the server responds with an error and registers nothing

Related spec scenarios: RS.SHR.28, RS.MAPI.21
*/
func TestAddExample_SignalRHubUnknownChannel(t *testing.T) {
	t.Parallel()

	srv := newSignalRPushMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	addBody := `{
		"protocol": "signalr",
		"channel": "/missing",
		"interval": 40,
		"response": {"code": 200, "body": {"a": 1}}
	}`
	resp := postExample(t, ts.URL, addBody)
	defer resp.Body.Close() //nolint:errcheck
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var payload struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	assert.Contains(t, payload.Error, "no matching route")
	assert.Equal(t, 0, len(srv.runtimeExamples.byID))
}

/*
Scenario: Hub channel registration with a conflicting protocol is rejected
Given a POST to /_mock/examples targeting a hub channel address with protocol http
When the registration is attempted
Then the server rejects it (no raw route and no signalr hub route match)

Related spec scenarios: RS.MAPI.21, RS.SHR.28
*/
func TestAddExample_SignalRHubWrongProtocolRejected(t *testing.T) {
	t.Parallel()

	srv := newSignalRPushMgmtServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	addBody := `{
		"protocol": "http",
		"channel": "/priceFeed",
		"interval": 40,
		"response": {"code": 200, "body": {"a": 1}}
	}`
	resp := postExample(t, ts.URL, addBody)
	defer resp.Body.Close() //nolint:errcheck
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	assert.Equal(t, 0, len(srv.runtimeExamples.byID))
}
