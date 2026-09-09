package server

import (
	"encoding/json"
	"net/http"
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

// accountHubDoc serves one channel (orders) over a parameterized per-account
// hub path, the topology the Qoden scheduler uses.
const accountHubDoc = `asyncapi: 3.0.0
info:
  title: Account Hub
  version: 1.0.0
x-signalr:
  path: /frontoffice/ws/account/{accountId}
channels:
  orders:
    address: orders
    bindings:
      ws:
        method: GET
    messages:
      orderMsg:
        examples:
          - name: snap
            payload:
              symbol: ETH
operations:
  receiveOrders:
    action: receive
    channel:
      $ref: '#/channels/orders'
`

func newAccountHubServer(t *testing.T) *Server {
	t.Helper()
	doc, err := asyncapi.Parse([]byte(accountHubDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)
	return srv
}

// dialAccountHub connects and opens a stream over the given per-account hub path.
func dialAccountHub(t *testing.T, ts *httptest.Server, account string) *websocket.Conn {
	t.Helper()
	conn := dialSignalRHub(t, ts.URL, "/frontoffice/ws/account/"+account)
	_ = streamInvoke(t, conn, "orders", "s1") // open stream, drain snapshot
	return conn
}

/*
Scenario: Per-account hub connections retain their captured upgrade path
Given two SignalR consumers connecting over different per-account hub paths
When their connection records are inspected
Then each retains a distinct captured path value

Related spec scenarios: RS.SHR.26
*/
func TestSignalR_ConnectionsRetainAccountPath(t *testing.T) {
	t.Parallel()

	srv := newAccountHubServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	dialAccountHub(t, ts, "qa-A")
	dialAccountHub(t, ts, "qa-B")

	candidates := srv.hubMgr.Candidates("/orders")
	require.Len(t, candidates, 2)

	paths := map[string]bool{}
	for _, c := range candidates {
		require.NotEmpty(t, c.Path, "consumer record must expose its upgrade path")
		paths[c.Path] = true
	}
	assert.True(t, paths["/frontoffice/ws/account/qa-A"], "qa-A path captured")
	assert.True(t, paths["/frontoffice/ws/account/qa-B"], "qa-B path captured")
}

/*
Scenario: Consumer discovery exposes the captured hub path
Given two SignalR consumers over distinct per-account hub paths
When the management consumers endpoint lists the channel's consumers
Then each consumer record carries the path value that distinguishes them

Related spec scenarios: RS.AMG.32, RS.MAPI.39
*/
func TestAsyncConsumers_ExposeHubPath(t *testing.T) {
	t.Parallel()

	srv := newAccountHubServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	dialAccountHub(t, ts, "qa-A")
	dialAccountHub(t, ts, "qa-B")

	resp, err := http.Get(ts.URL + "/_mock/async/consumers?channel=/orders")
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var payload struct {
		Consumers []map[string]any `json:"consumers"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Len(t, payload.Consumers, 2)

	paths := map[string]bool{}
	for _, c := range payload.Consumers {
		path, ok := c["path"].(string)
		require.True(t, ok, "consumer record must carry a path field")
		require.NotEmpty(t, path)
		paths[path] = true
	}
	assert.True(t, paths["/frontoffice/ws/account/qa-A"])
	assert.True(t, paths["/frontoffice/ws/account/qa-B"])
}

/*
Scenario: A push scoped to one account's captured path reaches only that account
Given two SignalR consumers with open streams over distinct per-account paths
When an event-triggered example scoped by {$connection.path} fires
Then only the matching account's stream receives the StreamItem

Related spec scenarios: RS.SHR.26, RS.MAPI.39
*/
func TestSignalR_PushScopedToAccountPath(t *testing.T) {
	t.Parallel()

	srv := newAccountHubServer(t)
	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	connA := dialAccountHub(t, ts, "qa-A")
	connB := dialAccountHub(t, ts, "qa-B")

	addBody := `{"channel":"/orders","conditions":{"{$event.name}":"levelUp","{$connection.path}":"/frontoffice/ws/account/qa-A"},"response":{"code":200,"body":{"msg":"only-a"}}}`
	resp := postExample(t, ts.URL, addBody)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close() //nolint:errcheck

	fire, err := http.Post(ts.URL+"/_mock/events", "application/json",
		strings.NewReader(`{"name":"levelUp","payload":{"msg":"boom"}}`))
	require.NoError(t, err)
	_ = fire.Body.Close()
	require.Equal(t, http.StatusOK, fire.StatusCode)

	// qa-A receives the scoped StreamItem.
	_ = connA.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := connA.ReadMessage()
	require.NoError(t, err)
	var env signalREnvelope
	require.NoError(t, json.Unmarshal(splitSignalRFrames(msg)[0], &env))
	assert.Equal(t, signalRTypeStreamItem, env.Type)
	raw, _ := json.Marshal(env.Item)
	assert.JSONEq(t, `{"msg":"only-a"}`, string(raw))

	// qa-B must not receive the scoped push.
	_ = connB.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, errB := connB.ReadMessage()
	require.Error(t, errB, "qa-B must not receive the qa-A-scoped push")
}
