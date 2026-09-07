package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gorilla/websocket"
	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openAPIExampleTriggerDoc() *openapi3.T {
	const yamlSpec = `
openapi: 3.0.0
info:
  title: REST
  version: 1.0.0
paths:
  /orders:
    post:
      responses:
        '200':
          description: OK
          content:
            application/json:
              examples:
                trigger:
                  value:
                    status: created
                  x-event-trigger:
                    - name: orderCreated
                      payload:
                        accountId: acc-1
`
	ldr := openapi3.NewLoader()
	spec, err := ldr.LoadFromData([]byte(yamlSpec))
	if err != nil {
		panic(err)
	}
	return spec
}

func asyncAlertDoc() *asyncapi.Document {
	raw := `
asyncapi: 3.0.0
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
              account: "{$event.accountId}"
            x-mock-match:
              '{$event.name}': "orderCreated"
operations:
  receiveAlerts:
    action: receive
    channel:
      $ref: '#/channels/alerts'
`
	doc, _ := asyncapi.Parse([]byte(raw))
	return doc
}

/*
Scenario: REST fill triggers an event delivered to an open ws stream
Given a REST example with x-event-trigger and a subscribed AsyncAPI ws channel
When a REST request selects the trigger example with a connected consumer
Then the consumer receives the templated message with the event payload

Related spec scenarios: RS.EVT.1, RS.EVT.12
*/
func TestEventDriver_RESTToWS(t *testing.T) {
	t.Parallel()

	schemas := []loader.SchemaInfo{
		{Kind: loader.KindOpenAPI, Spec: openAPIExampleTriggerDoc(), Prefix: ""},
		{Kind: loader.KindAsyncAPI, Async: asyncAlertDoc(), Prefix: ""},
	}
	srv, err := New(Config{HistorySize: DefaultHistorySize}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	// Connect a ws consumer to the alerts channel.
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	// Fire the REST example → triggers "orderCreated" → alert delivered to ws.
	resp, err := http.Post(ts.URL+"/orders", "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(msg, &payload))
	assert.Equal(t, "acc-1", payload["account"])
	assert.Equal(t, "info", payload["level"])
}

const signalRTriggerRestDoc = `
openapi: 3.0.0
info:
  title: REST
  version: 1.0.0
paths:
  /orders:
    post:
      responses:
        '200':
          description: OK
          content:
            application/json:
              examples:
                trigger:
                  value:
                    status: created
                  x-event-trigger:
                    - name: orderCreated
                      payload:
                        symbol: ETH
`

const signalRTriggeredHubDoc = `asyncapi: 3.0.0
info:
  title: Prices
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
          - name: evented
            payload:
              symbol: "{$event.symbol}"
              price: 3100
            x-mock-match:
              '{$event.name}': "orderCreated"
operations:
  receivePrice:
    action: receive
    channel:
      $ref: '#/channels/priceFeed'
`

/*
Scenario: REST fill triggers an event delivered into an open SignalR stream
Given a REST example with x-event-trigger and a subscribed SignalR hub channel
When a REST request selects the trigger example with an open stream on the hub
Then the consumer receives the templated message as a StreamItem

Related spec scenarios: RS.EVT.1, RS.EVT.13, RS.SHR.18
*/
func TestEventDriver_RESTToSignalRStream(t *testing.T) {
	t.Parallel()

	restLoader := openapi3.NewLoader()
	restDoc, err := restLoader.LoadFromData([]byte(signalRTriggerRestDoc))
	require.NoError(t, err)
	hubDoc, err := asyncapi.Parse([]byte(signalRTriggeredHubDoc))
	require.NoError(t, err)

	schemas := []loader.SchemaInfo{
		{Kind: loader.KindOpenAPI, Spec: restDoc, Prefix: ""},
		{Kind: loader.KindAsyncAPI, Async: hubDoc, Prefix: ""},
	}
	srv, err := New(Config{HistorySize: DefaultHistorySize}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	// Connect a SignalR client and open a stream on the priceFeed channel.
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/hub"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"protocol":"json","version":1}`)))
	_, _, err = conn.ReadMessage()
	require.NoError(t, err)

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"type":4,"invocationId":"s1","target":"priceFeed"}`+"\x1e")))
	_, _, err = conn.ReadMessage() // snapshot
	require.NoError(t, err)

	// Fire the REST trigger; the event yields a StreamItem on the open stream.
	resp, err := http.Post(ts.URL+"/orders", "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	var env signalREnvelope
	require.NoError(t, json.Unmarshal(splitSignalRFrames(msg)[0], &env))
	assert.Equal(t, signalRTypeStreamItem, env.Type)
	assert.Equal(t, "s1", env.InvocationID)
	raw, _ := json.Marshal(env.Item)
	assert.Contains(t, string(raw), `"symbol":"ETH"`)
}
