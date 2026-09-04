package server

import (
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

const tenantChannelDoc = `asyncapi: 3.0.0
info:
  title: Tenant
  version: 1.0.0
channels:
  tenant:
    address: /tenant/{tenantId}
    bindings:
      ws:
        method: GET
    parameters:
      tenantId:
        description: The tenant identifier
    messages:
      tenantMsg:
        examples:
          - name: ex1
            payload:
              tenant: "{$channel.tenantId}"
operations:
  sendTenant:
    action: send
    channel:
      $ref: '#/channels/tenant'
`

/*
Scenario: Channel address parameters are captured end-to-end via the router
Given an AsyncAPI ws channel address /tenant/{tenantId} with a send example
referencing {$channel.tenantId}
When a client dials /tenant/abc and sends a message through the real router
Then the echoed payload contains the captured parameter value abc

Related spec scenarios: RS.ATM.3
*/
func TestChannelParams_CapturedEndToEnd(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(tenantChannelDoc))
	require.NoError(t, err)
	require.Len(t, doc.Channels[0].Parameters, 1)
	assert.Equal(t, "tenantId", doc.Channels[0].Parameters[0].Name)

	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/tenant/abc"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"msg":"hello"}`)))
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, reply, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(reply), `"tenant":"abc"`)
}
