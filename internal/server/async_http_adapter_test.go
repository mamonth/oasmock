package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: HTTP adapter renders an AsyncAPI http channel message
Given a route mapping with a message example payload
When the adapter handler is invoked
Then it responds 200 with the rendered JSON payload

Related spec scenarios: RS.ASP.1, RS.ASP.10
*/
func TestHTTPProtocolAdapter_RendersMessage(t *testing.T) {
	t.Parallel()

	srv, _, stateStore, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	stateStore.EXPECT().GetNamespace(gomock.Any()).Return(map[string]any{}).AnyTimes()

	mapping := &RouteMapping{
		Protocol: asyncapi.ProtocolHTTP,
		Method:   http.MethodPost,
		Prefix:   "",
		Pattern:  "/employees",
		Messages: []*loader.MessageSpec{
			{
				Name: "emplMsg",
				Examples: []*loader.MessageExampleSpec{
					{Payload: map[string]any{"id": 1, "name": "Ada"}},
				},
			},
		},
	}

	adapter := srv.adapterForProtocol(asyncapi.ProtocolHTTP)
	require.NotNil(t, adapter)

	mh := srv.asyncMessageHandler(mapping)
	handler := adapter.Handler(mapping, mh)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/employees", strings.NewReader(`{"name":"Ada"}`))
	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, float64(1), body["id"])
	assert.Equal(t, "Ada", body["name"])
}

/*
Scenario: Async reply-context conditions on a dynamic example are honored
Given an AsyncAPI http channel with a management-injected dynamic example whose
conditions reference the reply context ({$message.payload.*})
When a matching inbound message arrives
Then the dynamic example is selected and its body rendered; a non-matching
message falls through without selecting it

Related spec scenarios: RS.MAPI.36, RS.MAPI.20
*/
func TestAsyncDynamicExample_ReplyContextConditionsHonored(t *testing.T) {
	t.Parallel()

	srv, _, stateStore, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	stateStore.EXPECT().GetNamespace(gomock.Any()).Return(map[string]any{}).AnyTimes()

	mapping := &RouteMapping{
		Protocol:   asyncapi.ProtocolHTTP,
		Method:     http.MethodPost,
		ChiPattern: "/employees",
		Pattern:    "/employees",
		Messages:   nil, // no spec messages: the dynamic example is the only reply
	}

	// Register a management-injected dynamic example guarded by a reply-context
	// ({$message.*}) condition (RS.MAPI.36).
	key := routeKey(mapping.Method, mapping.ChiPattern)
	srv.registry.addDynamic(key, dynamicExample{
		onceID:     "dynex-1",
		conditions: map[string]any{"{$message.payload.kind}": "urgent"},
		response: struct {
			code    int
			headers map[string]string
			body    any
		}{code: 200, body: map[string]any{"dyn": "yes"}},
	})

	adapter := srv.adapterForProtocol(asyncapi.ProtocolHTTP)
	require.NotNil(t, adapter)
	handler := adapter.Handler(mapping, srv.asyncMessageHandler(mapping))

	// A matching message selects the dynamic example.
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodPost, "/employees", strings.NewReader(`{"kind":"urgent","id":7}`)))
	assert.Equal(t, http.StatusOK, rec.Code)
	var matched map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &matched))
	assert.Equal(t, "yes", matched["dyn"])

	// A non-matching message does not select the dynamic example (no spec
	// messages exist, so the reply is empty).
	rec2 := httptest.NewRecorder()
	handler(rec2, httptest.NewRequest(http.MethodPost, "/employees", strings.NewReader(`{"kind":"normal","id":7}`)))
	assert.Equal(t, http.StatusOK, rec2.Code)
	assert.Empty(t, rec2.Body.String())
}

/*
Scenario: HTTP adapter ack send with no reply message
Given an AsyncAPI http channel whose operation has no reply message
When the adapter handler is invoked
Then it responds 200 with an empty body

Related spec scenarios: RS.ASP.10
*/
func TestHTTPProtocolAdapter_SendNoReply(t *testing.T) {
	t.Parallel()

	srv, _, stateStore, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	stateStore.EXPECT().GetNamespace(gomock.Any()).Return(map[string]any{}).AnyTimes()

	mapping := &RouteMapping{
		Protocol: asyncapi.ProtocolHTTP,
		Method:   http.MethodPost,
		Pattern:  "/events",
		Messages: nil,
	}

	adapter := srv.adapterForProtocol(asyncapi.ProtocolHTTP)
	require.NotNil(t, adapter)

	handler := adapter.Handler(mapping, srv.asyncMessageHandler(mapping))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(`{"event":"signup"}`))
	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())
}

/*
Scenario: AsyncAPI message selection skips x-mock-skip examples
Given a message spec carrying one skipped and one active example
When selectAsyncExample is called
Then the active example is selected

Related spec scenarios: RS.ATM.6
*/
func TestSelectAsyncExample_Skip(t *testing.T) {
	t.Parallel()

	message := &loader.MessageSpec{
		Name: "m",
		Examples: []*loader.MessageExampleSpec{
			{Name: "skip", Extensions: map[string]any{"x-mock-skip": true}, Payload: map[string]any{"id": 1}},
			{Name: "active", Payload: map[string]any{"id": 2}},
		},
	}
	srv, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})

	view, key := srv.selectAsyncExample(message, runtime.NewEvaluator(), "op")
	require.NotNil(t, view)
	assert.Equal(t, "m-1", key)
	payload, ok := view.Payload().(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 2, payload["id"])
}

/*
Scenario: First example selected when no example has conditions
Given a message spec with two condition-free examples
When selectAsyncExample is called
Then the first example (by definition order) is selected

Related spec scenarios: RS.ATM.7
*/
func TestSelectAsyncExample_FirstNoConditions(t *testing.T) {
	t.Parallel()

	message := &loader.MessageSpec{
		Name: "m",
		Examples: []*loader.MessageExampleSpec{
			{Name: "first", Payload: map[string]any{"id": 1}},
			{Name: "second", Payload: map[string]any{"id": 2}},
		},
	}
	srv, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})

	view, key := srv.selectAsyncExample(message, runtime.NewEvaluator(), "op")
	require.NotNil(t, view)
	assert.Equal(t, "m-0", key)
	payload, ok := view.Payload().(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, payload["id"])
}

/*
Scenario: One-time example is removed from future selection
Given a message spec with an x-mock-once example
When selectAsyncExample is called twice
Then the first call returns it and the second returns nil

Related spec scenarios: RS.ATM.10
*/
func TestSelectAsyncExample_Once(t *testing.T) {
	t.Parallel()

	message := &loader.MessageSpec{
		Name: "m",
		Examples: []*loader.MessageExampleSpec{
			{Name: "once", Extensions: map[string]any{"x-mock-once": true}, Payload: map[string]any{"id": 1}},
		},
	}
	srv, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})

	view, key := srv.selectAsyncExample(message, runtime.NewEvaluator(), "op")
	require.NotNil(t, view)
	assert.Equal(t, "m-0", key)

	second, _ := srv.selectAsyncExample(message, runtime.NewEvaluator(), "op")
	assert.Nil(t, second)
}

/*
Scenario: Server fails to build a route with an unsupported protocol
Given a route mapping declaring an unsupported protocol
When buildRouteHandler is called
Then it returns an error naming the unsupported protocol

Related spec scenarios: RS.ASP.4
*/
func TestBuildRouteHandler_UnsupportedProtocol(t *testing.T) {
	t.Parallel()

	srv, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})

	mapping := &RouteMapping{
		Protocol: "amqp",
		Method:   http.MethodGet,
		Pattern:  "/q",
	}
	_, err := srv.buildRouteHandler(mapping)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "amqp")
	assert.Contains(t, err.Error(), "not supported")
}

/*
Scenario: Server builds a ws route via its protocol adapter
Given a route mapping declaring the ws protocol
When buildRouteHandler is called
Then it returns a non-nil handler
*/
func TestBuildRouteHandler_WSAssignsAdapter(t *testing.T) {
	t.Parallel()

	srv, _, stateStore, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	stateStore.EXPECT().GetNamespace(gomock.Any()).Return(map[string]any{}).AnyTimes()

	mapping := &RouteMapping{
		Protocol: asyncapi.ProtocolWS,
		Method:   http.MethodGet,
		Pattern:  "/socket",
	}
	handler, err := srv.buildRouteHandler(mapping)
	require.NoError(t, err)
	require.NotNil(t, handler)
}
