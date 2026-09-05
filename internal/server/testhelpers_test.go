package server

// Shared test helpers and fixtures for the async-management server tests.
// Keeping them in one file gives package-level helpers a canonical home so a
// fixture or stub edited here does not silently drift from a near-twin in
// another _test.go file.

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/runtime"
	"github.com/stretchr/testify/require"
)

// pushChannelDoc is the minimal AsyncAPI ws fixture used by most management
// control-API tests: one /alerts receive channel with a default example.
const pushChannelDoc = `asyncapi: 3.0.0
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
              msg: default
operations:
  receiveAlerts:
    action: receive
    channel:
      $ref: '#/channels/alerts'
`

func parsePushDoc(t *testing.T) *asyncapi.Document {
	t.Helper()
	doc, err := asyncapi.Parse([]byte(pushChannelDoc))
	require.NoError(t, err)
	return doc
}

// newPushServer builds a server with the control API enabled and one ws
// channel (/alerts). It is the de-facto default test server for the
// async-management surface.
func newPushServer(t *testing.T) *Server {
	t.Helper()
	schemas := []loader.SchemaInfo{{Kind: loader.KindAsyncAPI, Async: parsePushDoc(t), Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize, EnableControlAPI: true}, schemas)
	require.NoError(t, err)
	return srv
}

// newAsyncMgmtServer is a discoverability alias of newPushServer for tests that
// conceptually drive the async management surface.
func newAsyncMgmtServer(t *testing.T) *Server {
	t.Helper()
	return newPushServer(t)
}

// postExample POSTs a body to /_mock/examples and returns the response.
func postExample(t *testing.T, tsURL, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(tsURL+"/_mock/examples", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	return resp
}

// addExample POSTs a body to /_mock/examples, asserts a 200 and returns the
// returned example id.
func addExample(t *testing.T, tsURL, body string) string {
	t.Helper()
	resp := postExample(t, tsURL, body)
	defer resp.Body.Close() //nolint:errcheck
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var payload map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	id, _ := payload["id"].(string)
	require.NotEmpty(t, id)
	return id
}

// waitForConnections blocks until the ws registry holds at least n connections
// for the channel (avoiding the gorilla sticky read-timeout on empty pre-reads).
func waitForConnections(srv *Server, channel string, n int) {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(srv.wsRegistry().connections(channel)) >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// waitForPush blocks until the observer records a push envelope or the
// deadline elapses, returning the elapsed time from start. It reports whether
// the push arrived within the deadline so callers can distinguish a missing
// delivery from a merely late one.
func waitForPush(t *testing.T, got *atomic.Int64, start time.Time) (time.Duration, bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for got.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	return time.Since(start), got.Load() > 0
}

// stubConsumerBus is an in-memory ConsumerBus capturing delivery via wsPush.
type stubConsumerBus struct {
	candidates []ConsumerInfo
	wsPush     func(consumer ConsumerInfo, payload []byte)
}

func (s *stubConsumerBus) SignalRPush(address string, payload []byte) {}
func (s *stubConsumerBus) WSBroadcast(address string, payload []byte) {
	if s.wsPush != nil {
		s.wsPush(ConsumerInfo{Channel: address}, payload)
	}
}
func (s *stubConsumerBus) Candidates(address string) []ConsumerInfo { return s.candidates }
func (s *stubConsumerBus) PushTo(consumer ConsumerInfo, address string, payload []byte) {
	if s.wsPush != nil {
		s.wsPush(consumer, payload)
	}
}

// stubMessageRenderer is an in-memory MessageRenderer; render overrides the
// default JSON-marshal of the example payload.
type stubMessageRenderer struct {
	render func(example *MessageExampleView, evaluator runtime.Evaluator) ([]byte, error)
}

func (s *stubMessageRenderer) SelectAsyncExample(message *loader.MessageSpec, evaluator runtime.Evaluator, opID string) (*MessageExampleView, string) {
	return nil, ""
}
func (s *stubMessageRenderer) RenderMessageSpecs(messages []*loader.MessageSpec, prefix, opID string, in InboundMessage) (int, []byte, error) {
	return 0, nil, nil
}
func (s *stubMessageRenderer) RenderAsyncPayload(example *MessageExampleView, evaluator runtime.Evaluator) ([]byte, error) {
	if s.render != nil {
		return s.render(example, evaluator)
	}
	b, _ := json.Marshal(example.Payload())
	return b, nil
}
func (s *stubMessageRenderer) ApplySetState(stateMap map[string]any, eval runtime.Evaluator, prefix string) {
}
func (s *stubMessageRenderer) NewStateSource(prefix string) *runtime.StateSource {
	return &runtime.StateSource{Data: map[string]any{}}
}
func (s *stubMessageRenderer) NewEnvSource() *runtime.EnvSource {
	return &runtime.EnvSource{Env: map[string]string{}}
}

// loaderExampleSpecForTest builds a MessageExampleSpec for event-bus tests.
func loaderExampleSpecForTest(payload, ext map[string]any) *loader.MessageExampleSpec {
	return &loader.MessageExampleSpec{Payload: payload, Extensions: ext}
}
