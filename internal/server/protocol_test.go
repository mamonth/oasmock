package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRecorder() *httptest.ResponseRecorder {
	return httptest.NewRecorder()
}

func newTestRequest() *http.Request {
	return httptest.NewRequest("GET", "/", nil)
}

/*
Scenario: Registering protocol adapters keyed by protocol
Given a server with default dependencies
When the adapter registry is queried for the http and ws protocols
Then both adapters are registered and expose their protocol name

Related spec scenarios: RS.ASP.1, RS.ASP.2, RS.ASP.4
*/
func TestServer_ProtocolAdaptersRegistered(t *testing.T) {
	t.Parallel()

	srv, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})

	httpAdapter := srv.adapterForProtocol("http")
	require.NotNil(t, httpAdapter)
	assert.Equal(t, defaultProtocol("http"), httpAdapter.Protocol())

	wsAdapter := srv.adapterForProtocol("ws")
	require.NotNil(t, wsAdapter)
	assert.Equal(t, defaultProtocol("ws"), wsAdapter.Protocol())
}

func defaultProtocol(p string) string { return p }

/*
Scenario: No adapter for an unsupported protocol
Given an AsyncAPI route with a protocol binding no adapter serves
When the adapter registry is queried
Then no adapter is returned

Related spec scenarios: RS.ASP.4
*/
func TestServer_NoAdapterForUnsupportedProtocol(t *testing.T) {
	t.Parallel()

	srv, _, _, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})

	assert.Nil(t, srv.adapterForProtocol("amqp"))
	assert.Nil(t, srv.adapterForProtocol("kafka"))
}

/*
Scenario: ProtocolAdapter contract via a stub adapter
Given a stub ProtocolAdapter registered under a protocol
When a route handler is built
Then the adapter is invoked with the message handler and returns an HTTP handler

Related spec scenarios: RS.ASP.1, RS.ASP.2
*/
func TestProtocolAdapter_HandlerBuilderContract(t *testing.T) {
	t.Parallel()

	var gotHandler MessageHandler
	adapter := &stubAdapter{protocol: "stub", onHandler: func(h MessageHandler) {
		gotHandler = h
	}}

	handler := MessageHandlerFunc(func(ctx context.Context, in InboundMessage) ([]byte, error) {
		return []byte("ok"), nil
	})
	built := adapter.Handler(&RouteMapping{}, handler)
	require.NotNil(t, built)

	rec := newTestRecorder()
	built(rec, newTestRequest())
	assert.Equal(t, http.StatusOK, rec.Code)

	assert.NotNil(t, gotHandler)
}

type stubAdapter struct {
	protocol  string
	onHandler func(h MessageHandler)
}

func (a *stubAdapter) Protocol() string { return a.protocol }

func (a *stubAdapter) Handler(_ *RouteMapping, h MessageHandler) http.HandlerFunc {
	if a.onHandler != nil {
		a.onHandler(h)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("served"))
	})
}
