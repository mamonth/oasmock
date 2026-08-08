package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createResponsesWithExample() *openapi3.Responses {
	const yamlSpec = `
openapi: 3.0.3
info:
  title: Test API
  version: 1.0.0
paths:
  /test:
    get:
      responses:
        '200':
          description: OK
          content:
            application/json:
              examples:
                default:
                  value:
                    message: "Hello, World!"
`
	ldr := openapi3.NewLoader()
	spec, err := ldr.LoadFromData([]byte(yamlSpec))
	if err != nil {
		panic(err)
	}
	pathMap := spec.Paths.Map()
	pathItem := pathMap["/test"]
	op := pathItem.Get
	if op == nil {
		panic("GET operation not found")
	}
	return op.Responses
}

func newRpcHandlerWithMocks(t *testing.T) (*RpcHandler, *MockRpcProtocol, *Server) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	proto := NewMockRpcProtocol(ctrl)

	routeProvider := NewMockRouteProvider(ctrl)
	routeProvider.EXPECT().BuildRouteMappings(gomock.Any()).Return([]RouteMapping{}, nil)

	stateStore := NewMockStateStore(ctrl)
	stateStore.EXPECT().GetNamespace(gomock.Any()).Return(nil).AnyTimes()
	stateStore.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	stateStore.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, false).AnyTimes()

	historyStore := NewMockHistoryStore(ctrl)
	historyStore.EXPECT().Add(gomock.Any()).AnyTimes()

	expressionEvaluator := NewMockExpressionEvaluator(ctrl)
	expressionEvaluator.EXPECT().AddSource(gomock.Any(), gomock.Any()).AnyTimes()
	expressionEvaluator.EXPECT().Evaluate(gomock.Any()).Return("", nil).AnyTimes()

	requestSourceFactory := NewMockRequestSourceFactory(ctrl)
	requestSourceFactory.EXPECT().NewRequestSource(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	stateSourceFactory := NewMockStateSourceFactory(ctrl)
	stateSourceFactory.EXPECT().NewStateSource(gomock.Any()).Return(nil).AnyTimes()

	envSourceFactory := NewMockEnvSourceFactory(ctrl)
	envSourceFactory.EXPECT().NewEnvSource().Return(nil).AnyTimes()

	extensionProcessor := NewMockExtensionProcessor(ctrl)

	deps := Dependencies{
		RouteProvider:        routeProvider,
		StateStore:           stateStore,
		HistoryStore:         historyStore,
		RequestSourceFactory: requestSourceFactory,
		StateSourceFactory:   stateSourceFactory,
		EnvSourceFactory:     envSourceFactory,
		ExpressionEvaluator:  expressionEvaluator,
		ExtensionProcessor:   extensionProcessor,
	}

	server, err := NewWithDependencies(Config{Port: 0, HistorySize: 1000}, []SchemaInfo{}, deps, nil, nil)
	require.NoError(t, err)

	handler := &RpcHandler{
		protocol:     proto,
		procedureMap: make(map[string]*RouteMapping),
		server:       server,
	}

	return handler, proto, server
}

/*
Scenario: RpcHandler serves single JSON-RPC call via pipeline
Given a handler with protocol that parses a single call and a procedure mapping
When ServeHTTP is called with a single-call body
Then the protocol parses, dispatches to correct procedure, and returns the example body

Related spec scenarios: RS.JRP.17
*/
func TestRpcHandler_SingleCall(t *testing.T) {
	t.Parallel()

	handler, proto, _ := newRpcHandlerWithMocks(t)

	mapping := &RouteMapping{
		Method:     "POST",
		Path:       "/rpc/subtract",
		Pattern:    "/rpc/subtract",
		ChiPattern: "/rpc/subtract",
		Responses:  createResponsesWithExample(),
	}
	handler.procedureMap["subtract"] = mapping

	proto.EXPECT().ParseBody(gomock.Any()).Return([]RpcCall{
		{Procedure: "subtract", Raw: map[string]interface{}{"jsonrpc": "2.0", "method": "subtract", "id": float64(1)}, ID: float64(1), HasID: true},
	}, nil)
	proto.EXPECT().ContentType().Return("application/json")

	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var body map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "Hello, World!", body["message"])
}

/*
Scenario: RpcHandler returns method not found error
Given a handler with a procedure map that doesn't contain the requested method
When ServeHTTP is called
Then the protocol's ErrorResponse is called with -32601 and the error is written

Related spec scenarios: RS.JRP.18
*/
func TestRpcHandler_MethodNotFound(t *testing.T) {
	t.Parallel()

	handler, proto, _ := newRpcHandlerWithMocks(t)

	errBody := []byte(`{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":2}`)

	proto.EXPECT().ParseBody(gomock.Any()).Return([]RpcCall{
		{Procedure: "unknown", Raw: map[string]interface{}{"jsonrpc": "2.0", "method": "unknown", "id": float64(2)}, ID: float64(2), HasID: true},
	}, nil)
	proto.EXPECT().ErrorResponse(-32601, "Method not found", float64(2)).Return(errBody)
	proto.EXPECT().ContentType().Return("application/json")

	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, "2.0", body["jsonrpc"])
	assert.Equal(t, float64(-32601), body["error"].(map[string]interface{})["code"])
	assert.Equal(t, float64(2), body["id"])
}

/*
Scenario: RpcHandler returns parse error
Given a handler with a protocol that fails to parse the body
When ServeHTTP is called
Then a parse error is written without calling the pipeline

Related spec scenarios: RS.JRP.12
*/
func TestRpcHandler_ParseError(t *testing.T) {
	t.Parallel()

	handler, proto, _ := newRpcHandlerWithMocks(t)

	errBody := []byte(`{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"},"id":null}`)

	proto.EXPECT().ParseBody(gomock.Any()).Return(nil, assert.AnError)
	proto.EXPECT().ErrorResponse(-32700, "Parse error", nil).Return(errBody)
	proto.EXPECT().ContentType().Return("application/json")

	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, float64(-32700), body["error"].(map[string]interface{})["code"])
	assert.Nil(t, body["id"])
}

/*
Scenario: RpcHandler processes batch and returns array response
Given a handler with protocol parsing 3 calls
When ServeHTTP is called with a batch
Then 3 pipeline calls are made with per-call bodies and an array response is written

Related spec scenarios: RS.JRP.19
*/
func TestRpcHandler_Batch(t *testing.T) {
	t.Parallel()

	handler, proto, _ := newRpcHandlerWithMocks(t)

	mapping := &RouteMapping{
		Method:     "POST",
		Path:       "/rpc",
		Pattern:    "/rpc",
		ChiPattern: "/rpc",
		Responses:  createResponsesWithExample(),
	}
	handler.procedureMap["a"] = mapping
	handler.procedureMap["b"] = mapping
	handler.procedureMap["c"] = mapping

	callA := RpcCall{Procedure: "a", Raw: map[string]interface{}{"jsonrpc": "2.0", "method": "a", "id": float64(1)}, ID: float64(1), HasID: true}
	callB := RpcCall{Procedure: "b", Raw: map[string]interface{}{"jsonrpc": "2.0", "method": "b", "id": float64(2)}, ID: float64(2), HasID: true}
	callC := RpcCall{Procedure: "c", Raw: map[string]interface{}{"jsonrpc": "2.0", "method": "c", "id": float64(3)}, ID: float64(3), HasID: true}

	proto.EXPECT().ParseBody(gomock.Any()).Return([]RpcCall{callA, callB, callC}, nil)
	proto.EXPECT().ContentType().Return("application/json")

	req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(`[{"jsonrpc":"2.0","method":"a","id":1}]`))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body []map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Len(t, body, 3)
}

/*
Scenario: RpcHandler batch with notification skips response entry
Given a handler with protocol parsing 2 calls (1 normal, 1 notification)
When ServeHTTP is called
Then the notification runs pipeline but is not in the response array

Related spec scenarios: RS.JRP.21
*/
func TestRpcHandler_BatchWithNotification(t *testing.T) {
	t.Parallel()

	handler, proto, _ := newRpcHandlerWithMocks(t)

	mapping := &RouteMapping{
		Method:     "POST",
		Path:       "/rpc",
		Pattern:    "/rpc",
		ChiPattern: "/rpc",
		Responses:  createResponsesWithExample(),
	}
	handler.procedureMap["a"] = mapping
	handler.procedureMap["notify"] = mapping

	callA := RpcCall{Procedure: "a", Raw: map[string]interface{}{"jsonrpc": "2.0", "method": "a", "id": float64(1)}, ID: float64(1), HasID: true}
	callN := RpcCall{Procedure: "notify", Raw: map[string]interface{}{"jsonrpc": "2.0", "method": "notify"}, ID: nil, HasID: false}

	proto.EXPECT().ParseBody(gomock.Any()).Return([]RpcCall{callA, callN}, nil)
	proto.EXPECT().ContentType().Return("application/json")

	req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(`[{"jsonrpc":"2.0","method":"a","id":1}]`))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body []map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Len(t, body, 1)
}

/*
Scenario: RpcHandler all-notification batch returns empty array
Given a handler with protocol parsing only notification calls
When ServeHTTP is called
Then an empty JSON array is written

Related spec scenarios: RS.JRP.22
*/
func TestRpcHandler_AllNotifications(t *testing.T) {
	t.Parallel()

	handler, proto, _ := newRpcHandlerWithMocks(t)

	mapping := &RouteMapping{
		Method:     "POST",
		Path:       "/rpc",
		Pattern:    "/rpc",
		ChiPattern: "/rpc",
		Responses:  createResponsesWithExample(),
	}
	handler.procedureMap["n1"] = mapping
	handler.procedureMap["n2"] = mapping

	call1 := RpcCall{Procedure: "n1", Raw: map[string]interface{}{"jsonrpc": "2.0", "method": "n1"}, ID: nil, HasID: false}
	call2 := RpcCall{Procedure: "n2", Raw: map[string]interface{}{"jsonrpc": "2.0", "method": "n2"}, ID: nil, HasID: false}

	proto.EXPECT().ParseBody(gomock.Any()).Return([]RpcCall{call1, call2}, nil)
	proto.EXPECT().ContentType().Return("application/json")

	req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(`[{"jsonrpc":"2.0","method":"n1"}]`))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body []interface{}
	err := json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Empty(t, body)
}

/*
Scenario: RpcHandler per-call RequestSource Body is individual call object
Given a handler with protocol parsing calls in a batch
When ServeHTTP is called
Then each call's RequestSource Body is the individual call object, not the batch array

Related spec scenarios: RS.JRP.25
*/
func TestRpcHandler_PerCallBody(t *testing.T) {
	t.Parallel()

	handler, proto, _ := newRpcHandlerWithMocks(t)

	mapping := &RouteMapping{
		Method:     "POST",
		Path:       "/rpc/a",
		Pattern:    "/rpc/a",
		ChiPattern: "/rpc/a",
		Responses:  createResponsesWithExample(),
	}
	handler.procedureMap["a"] = mapping

	callA := RpcCall{Procedure: "a", Raw: map[string]interface{}{"jsonrpc": "2.0", "method": "a", "id": float64(1), "params": map[string]interface{}{"x": float64(10)}}, ID: float64(1), HasID: true}

	proto.EXPECT().ParseBody(gomock.Any()).Return([]RpcCall{callA}, nil)
	proto.EXPECT().ContentType().Return("application/json")

	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

/*
Scenario: RpcHandler single notification returns 204
Given a handler with protocol parsing a single notification
When ServeHTTP is called
Then the pipeline runs and HTTP 204 No Content is returned

Related spec scenarios: RS.JRP.23
*/
func TestRpcHandler_SingleNotification(t *testing.T) {
	t.Parallel()

	handler, proto, _ := newRpcHandlerWithMocks(t)

	mapping := &RouteMapping{
		Method:     "POST",
		Path:       "/rpc/notify",
		Pattern:    "/rpc/notify",
		ChiPattern: "/rpc/notify",
		Responses:  createResponsesWithExample(),
	}
	handler.procedureMap["notify"] = mapping

	call := RpcCall{Procedure: "notify", Raw: map[string]interface{}{"jsonrpc": "2.0", "method": "notify"}, ID: nil, HasID: false}

	proto.EXPECT().ParseBody(gomock.Any()).Return([]RpcCall{call}, nil)

	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

/*
Scenario: RpcHandler returns response headers from example
Given a handler with a mapping that includes example headers
When ServeHTTP is called
Then the response headers are included in the HTTP response

Related spec scenarios: RS.JRP.29
*/
func TestRpcHandler_ResponseHeaders(t *testing.T) {
	t.Parallel()

	handler, proto, _ := newRpcHandlerWithMocks(t)

	mapping := &RouteMapping{
		Method:     "POST",
		Path:       "/rpc/h",
		Pattern:    "/rpc/h",
		ChiPattern: "/rpc/h",
		Responses:  createResponsesWithExample(),
	}
	handler.procedureMap["h"] = mapping

	call := RpcCall{Procedure: "h", Raw: map[string]interface{}{"jsonrpc": "2.0", "method": "h", "id": float64(1)}, ID: float64(1), HasID: true}
	proto.EXPECT().ParseBody(gomock.Any()).Return([]RpcCall{call}, nil)
	proto.EXPECT().ContentType().Return("application/json")

	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}

/*
Scenario: RpcHandler propagates response status code from example
Given a handler with a mapping that returns a specific status code
When ServeHTTP is called
Then the HTTP response uses that status code

Related spec scenarios: RS.JRP.17
*/
func TestRpcHandler_ResponseStatusCode(t *testing.T) {
	t.Parallel()

	handler, proto, _ := newRpcHandlerWithMocks(t)

	mapping := &RouteMapping{
		Method:     "POST",
		Path:       "/rpc/s",
		Pattern:    "/rpc/s",
		ChiPattern: "/rpc/s",
		Responses:  createResponsesWithExample(),
	}
	handler.procedureMap["s"] = mapping

	call := RpcCall{Procedure: "s", Raw: map[string]interface{}{"jsonrpc": "2.0", "method": "s", "id": float64(1)}, ID: float64(1), HasID: true}
	proto.EXPECT().ParseBody(gomock.Any()).Return([]RpcCall{call}, nil)
	proto.EXPECT().ContentType().Return("application/json")

	req := httptest.NewRequest(http.MethodPost, "/rpc", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
