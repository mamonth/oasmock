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

const targetedDeliveryDoc = `asyncapi: 3.0.0
info:
  title: Targeting
  version: 1.0.0
channels:
  alerts:
    address: /alerts
    bindings:
      ws:
        method: GET
    messages:
      ring:
        examples:
          - name: targeted
            payload:
              ring: "{$event.data}"
            x-mock-match:
              '{$event.name}': "orderCreated"
              '{$connection.id}': '{$event.connectionId}'
operations:
  receiveAlerts:
    action: receive
    channel:
      $ref: '#/channels/alerts'
`

/*
Scenario: Targeted event delivery by connection id
Given an event-driven example with a {$connection.id} = {$event.connectionId}
condition and two connected consumers
When an event fires with a connectionId payload for one consumer
Then only the matching consumer receives the message

Related spec scenarios: RS.EVT.19, RS.EXT.24
*/
func TestEventDelivery_TargetedByConnection(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(targetedDeliveryDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"

	conn1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn1.Close() //nolint:errcheck
	conn2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn2.Close() //nolint:errcheck
	waitForConnections(srv, "/alerts", 2)

	// Target conn-1 via the fired event's connectionId payload.
	body := `{"type":"fire","event":"orderCreated","payload":{"connectionId":"conn-1"}}`
	resp, err := http.Post(ts.URL+"/_mock/events", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	_ = conn1.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg1, err := conn1.ReadMessage()
	require.NoError(t, err)
	var payload1 map[string]any
	require.NoError(t, json.Unmarshal(msg1, &payload1))
	assert.Equal(t, map[string]any{"connectionId": "conn-1"}, payload1["ring"])

	// conn-2 must not receive the targeted message.
	_ = conn2.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, err2 := conn2.ReadMessage()
	require.Error(t, err2)
}

const broadcastFastPathDoc = `asyncapi: 3.0.0
info:
  title: Broadcast
  version: 1.0.0
channels:
  alerts:
    address: /alerts
    bindings:
      ws:
        method: GET
    messages:
      ring:
        examples:
          - name: broad
            payload:
              ring: "yes"
            x-mock-match:
              '{$event.name}': "orderCreated"
operations:
  receiveAlerts:
    action: receive
    channel:
      $ref: '#/channels/alerts'
`

/*
Scenario: Broadcast fast path with no connection conditions
Given an event-driven example without {$connection.*} conditions
When an event fires with two connected consumers
Then the message is broadcast to all consumers of the channel

Related spec scenarios: RS.EXT.25
*/
func TestEventDelivery_BroadcastFastPath(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(broadcastFastPathDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"

	conn1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn1.Close() //nolint:errcheck
	conn2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn2.Close() //nolint:errcheck
	waitForConnections(srv, "/alerts", 2)

	body := `{"type":"fire","event":"orderCreated","payload":{"x":1}}`
	resp, err := http.Post(ts.URL+"/_mock/events", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	for i, conn := range []*websocket.Conn{conn1, conn2} {
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, msg, rerr := conn.ReadMessage()
		require.NoError(t, rerr, "conn %d", i+1)
		assert.Contains(t, string(msg), `"ring":"yes"`)
	}
}

const connectTargetedDoc = `asyncapi: 3.0.0
info:
  title: Welcome
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
              msg: "welcome"
            x-mock-match:
              '{$event.name}': "connect"
              '{$connection.channel}': "/alerts"
operations:
  receiveAlerts:
    action: receive
    channel:
      $ref: '#/channels/alerts'
`

/*
Scenario: Single-recipient connect built-in with a connection condition
Given a connect example with a {$connection.channel} condition
When a consumer connects to the matching channel
Then the message is delivered to that single consumer only

Related spec scenarios: RS.EXT.26
*/
func TestEventDelivery_ConnectTargeted(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(connectTargetedDoc))
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
	waitForConnections(srv, "/alerts", 1)

	// The connect-built-in welcomes the connecting consumer.
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	deadline := time.Now().Add(3 * time.Second)
	got := ""
	for time.Now().Before(deadline) {
		_, msg, rerr := conn.ReadMessage()
		if rerr != nil {
			break
		}
		got = string(msg)
		if strings.Contains(got, `"msg":"welcome"`) {
			break
		}
	}
	assert.Contains(t, got, `"msg":"welcome"`)
}

/*
Scenario: registerRuntimeExample on an uninitialized bus reports an error
Given a nil eventBus
When registerRuntimeExample is called
Then it returns an error instead of a silent empty success

Related spec scenarios: RS.MAPI.24, RS.MAPI.25, RS.MAPI.26
*/
func TestEventBus_RegisterRuntimeExampleNilBusErrors(t *testing.T) {
	t.Parallel()

	var b *eventBus
	_, _, err := b.registerRuntimeExample("ex-1", "/alerts", "", &loader.MessageExampleSpec{
		Extensions: map[string]any{"x-mock-interval": 100},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

const partiallyInvalidSchemaDoc = `asyncapi: 3.0.0
info:
  title: Atomic
  version: 1.0.0
channels:
  alerts:
    address: /alerts
    messages:
      tick:
        examples:
          - name: good
            payload:
              seq: 1
            x-mock-interval: 500
          - name: bad
            payload:
              seq: 2
            x-mock-match:
              '{$event.name}': orderCreated
              '{$message.payload.kind}': order
operations:
  receiveAlerts:
    action: receive
    channel:
      $ref: '#/channels/alerts'
`

/*
Scenario: Schema registration is atomic under a late classification error
Given a schema whose first message example is periodically driven and whose
second example is invalid (mixed match contexts)
When registerSchema runs
Then it returns the load error and has scheduled no periodic job for the valid
example (no partial registration leaks a running interval goroutine)

Related spec scenarios: RS.EXT.20, RS.EXT.22
*/
func TestSchemaRegistration_AtomicOnError(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(partiallyInvalidSchemaDoc))
	require.NoError(t, err)

	bus := newEventBus(&stubMessageRenderer{}, &stubConsumerBus{}, false)
	err = bus.registerSchema("", doc)
	require.Error(t, err)

	// The valid periodic example ("good") must not have been scheduled, because
	// the later classification error aborts the whole schema registration.
	assert.False(t, bus.scheduler.started("interval---/alerts-good"),
		"no periodic job may be scheduled when schema registration fails")
}

const periodicSkipDoc = `asyncapi: 3.0.0
info:
  title: Skip
  version: 1.0.0
channels:
  alerts:
    address: /alerts
    bindings:
      ws:
        method: GET
    messages:
      tick:
        examples:
          - name: skipme
            payload:
              seq: 1
            x-mock-interval: 20
            x-mock-skip: true
operations:
  receiveAlerts:
    action: receive
    channel:
      $ref: '#/channels/alerts'
`

/*
Scenario: A periodically driven example honoring x-mock-skip is never emitted
Given a periodic message example declaring x-mock-skip
When the server runs the interval job with a connected consumer
Then no message is delivered to the channel

Related spec scenarios: RS.EXT.37
*/
func TestEventDelivery_PeriodicSkipsSkippedExample(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(periodicSkipDoc))
	require.NoError(t, err)
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: doc, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck
	waitForConnections(srv, "/alerts", 1)

	// Give several 20ms cadences time to fire; the skipped example must stay
	// silent on the wire.
	_ = conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
	_, _, rerr := conn.ReadMessage()
	require.Error(t, rerr, "a skipped periodic example must not be emitted")
}

const legacyXSendEventsDoc = `asyncapi: 3.0.0
info:
  title: Legacy
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
          - name: named
            payload:
              level: "{$event.level}"
            x-send-events:
              - on: legacyAlert
          - name: cron
            payload:
              seq: 1
            x-send-events:
              - on: cron
                wait: 1000
operations:
  receiveAlerts:
    action: receive
    channel:
      $ref: '#/channels/alerts'
`

/*
Scenario: A legacy x-send-events key is silently ignored when classifying
Given message examples whose only extension is x-send-events (named and cron)
When registerSchema classifies them
Then no subscription, no interval job and no load error result (the key no longer
maps to the unified trigger form)

Related spec scenarios: RS.EVT.5
*/
func TestSchemaRegistration_XSendEventsSilentlyIgnored(t *testing.T) {
	t.Parallel()

	doc, err := asyncapi.Parse([]byte(legacyXSendEventsDoc))
	require.NoError(t, err)

	bus := newEventBus(&stubMessageRenderer{}, &stubConsumerBus{}, false)
	require.NoError(t, bus.registerSchema("", doc))

	// Neither legacy key may register anything: the named entry would surface
	// as a "legacyAlert" subscription, the cron entry as an interval job.
	assert.Len(t, bus.broker.byEvent, 0, "x-send-events must not register any event subscription")
	assert.False(t, bus.scheduler.started("interval---/alerts-cron"),
		"x-send-events cron must not schedule an interval job")
}
