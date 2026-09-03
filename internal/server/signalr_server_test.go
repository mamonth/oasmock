package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const signalRHubDoc = `asyncapi: 3.0.0
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
              price: 3000
operations:
  receivePrice:
    action: receive
    channel:
      $ref: '#/channels/priceFeed'
`

func parseSignalRDoc(t *testing.T) *asyncapi.Document {
	t.Helper()
	doc, err := asyncapi.Parse([]byte(signalRHubDoc))
	require.NoError(t, err)
	return doc
}

/*
Scenario: Declaring a SignalR hub document maps ws channels to the hub
Given an AsyncAPI document with root x-signalr declared on a ws channel
When the server is constructed
Then a SignalR hub is registered at the hub path with the ws channel available as a stream target

Related spec scenarios: RS.SHR.1, RS.SHR.2, RS.SHR.3
*/
func TestNew_RegistersSignalRHub(t *testing.T) {
	t.Parallel()

	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: parseSignalRDoc(t), Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize}, schemas)
	require.NoError(t, err)

	require.Len(t, srv.hubMgr.hubs, 1)
	hub := srv.hubMgr.hubs[0]
	assert.Equal(t, "/hub", hub.path)
	require.Len(t, hub.channels, 1)
	assert.Equal(t, "priceFeed", hub.channels["priceFeed"].ID)
}

/*
Scenario: Negotiate endpoint is served for a SignalR hub
Given a server with a SignalR hub document
When POST {hubPath}/negotiate is invoked
Then the response carries a connection token and available transports

Related spec scenarios: RS.SHR.8, RS.SHR.9
*/
func TestSignalR_ServerNegotiate(t *testing.T) {
	t.Parallel()

	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: parseSignalRDoc(t), Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize}, schemas)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hub/negotiate?negotiateVersion=1", nil)
	srv.router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, float64(1), resp["negotiateVersion"])
	assert.NotEmpty(t, resp["connectionToken"])
	assert.NotEmpty(t, resp["connectionId"])
}

/*
Scenario: Signaling hub prefix is applied to the hub path
Given a SignalR hub document with a schema prefix /v1
When the negotiate endpoint is requested
Then it is served under /v1/hub/negotiate

Related spec scenarios: RS.SHR.1
*/
func TestSignalR_ServerNegotiateWithPrefix(t *testing.T) {
	t.Parallel()

	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: parseSignalRDoc(t), Prefix: "/v1"}}
	srv, err := New(Config{HistorySize: DefaultHistorySize}, schemas)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/hub/negotiate?negotiateVersion=1", nil)
	srv.router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	_ = strings.TrimSpace(rec.Body.String())
}
