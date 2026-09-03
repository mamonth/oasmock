package server

import (
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const messageTemplatingWsDoc = `asyncapi: 3.0.0
info:
  title: Echo
  version: 1.0.0
channels:
  echo:
    address: /echo
    bindings:
      ws:
        method: GET
    messages:
      echoMsg:
        examples:
          - name: repl
            payload:
              echoed: "{$message.payload.id}"
              fromChan: "{$channel.sid}"
              counter: "{$state.counter}"
operations:
  sendEcho:
    action: send
    channel:
      $ref: '#/channels/echo'
`

/*
Scenario: Message and channel expressions evaluate for AsyncAPI traffic
Given a client message with a payload and channel parameters
When the message is rendered
Then {$message.*} and {$channel.*} resolve against the inbound traffic

Related spec scenarios: RS.ATM.1, RS.ATM.3
*/
func TestTemplateParity_MessageAndChannel(t *testing.T) {
	t.Parallel()

	srv, _, stateStore, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	stateStore.EXPECT().GetNamespace(gomock.Any()).Return(map[string]any{}).AnyTimes()

	mapping := &RouteMapping{
		Protocol: asyncWSProtocol,
		Action:   "send",
		Path:     "/echo",
		Pattern:  "/echo",
		Messages: mustMessageSpecs(t, messageTemplatingWsDoc),
	}

	count, out, err := srv.renderAsyncMessage(mapping, InboundMessage{
		Payload:    []byte(`{"id":"u-1"}`),
		PathParams: map[string]string{"sid": "conn-9"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, count)

	var body map[string]any
	require.NoError(t, json.Unmarshal(out, &body))
	assert.Equal(t, "u-1", body["echoed"])
	assert.Equal(t, "conn-9", body["fromChan"])
}

/*
Scenario: AsyncAPI state writes land in the schema namespace
Given a message example setting state via x-mock-set-state
When rendered against a prefixed schema
Then the state write goes to that schema's namespace

Related spec scenarios: RS.ATM.11, RS.ATM.16
*/
func TestTemplateParity_StateNamespaceIsolation(t *testing.T) {
	t.Parallel()

	srv, _, stateStore, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	stateStore.EXPECT().GetNamespace(gomock.Any()).Return(map[string]any{}).AnyTimes()

	var setNamespace string
	stateStore.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(namespace, key string, value any) { setNamespace = namespace }).AnyTimes()

	mapping := &RouteMapping{
		Protocol: asyncWSProtocol,
		Action:   "send",
		Path:     "/tenant/ch",
		Pattern:  "/ch",
		Prefix:   "/tenant",
		Messages: mustMessageSpecs(t, statefulMsgDoc),
	}

	_, _, err := srv.renderAsyncMessage(mapping, InboundMessage{})
	require.NoError(t, err)
	assert.Equal(t, "/tenant", setNamespace)
}

const headerTemplatingDoc = `asyncapi: 3.0.0
info:
  title: Echo
  version: 1.0.0
channels:
  echo:
    address: /echo
    messages:
      echoMsg:
        examples:
          - name: repl
            payload:
              trace: "{$message.headers.x-request-id}"
operations:
  sendEcho:
    action: send
    channel:
      $ref: '#/channels/echo'
`

/*
Scenario: Header expression evaluates from inbound message headers
Given a message example referencing {$message.headers.x-request-id}
When rendered with inbound headers
Then the expression resolves to the header value

Related spec scenarios: RS.ATM.2
*/
func TestTemplateParity_MessageHeader(t *testing.T) {
	t.Parallel()

	srv, _, stateStore, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	stateStore.EXPECT().GetNamespace(gomock.Any()).Return(map[string]any{}).AnyTimes()

	mapping := &RouteMapping{
		Protocol: asyncWSProtocol,
		Action:   "send",
		Path:     "/echo",
		Pattern:  "/echo",
		Messages: mustMessageSpecs(t, headerTemplatingDoc),
	}

	count, out, err := srv.renderAsyncMessage(mapping, InboundMessage{
		Headers: map[string]string{"x-request-id": "req-42"},
	})
	require.NoError(t, err)
	require.Equal(t, 1, count)

	var body map[string]any
	require.NoError(t, json.Unmarshal(out, &body))
	assert.Equal(t, "req-42", body["trace"])
}

const stateTemplatingDoc = `asyncapi: 3.0.0
info:
  title: State
  version: 1.0.0
channels:
  ch:
    address: /ch
    messages:
      msg:
        examples:
          - name: ex
            payload:
              counter: "{$state.counter}"
operations:
  send:
    action: send
    channel:
      $ref: '#/channels/ch'
`

/*
Scenario: State expression evaluates from the schema state store
Given a message example referencing {$state.counter}
When rendered against state containing counter
Then the expression resolves to the stored value

Related spec scenarios: RS.ATM.4
*/
func TestTemplateParity_StateExpression(t *testing.T) {
	t.Parallel()

	srv, _, stateStore, _, _, _, _, _, _ := newMockedServerWithGeneratedMocks(t, Config{HistorySize: DefaultHistorySize})
	stateStore.EXPECT().GetNamespace(gomock.Any()).Return(map[string]any{"counter": 7}).AnyTimes()

	mapping := &RouteMapping{
		Protocol: asyncWSProtocol,
		Action:   "send",
		Path:     "/ch",
		Pattern:  "/ch",
		Messages: mustMessageSpecs(t, stateTemplatingDoc),
	}

	count, out, err := srv.renderAsyncMessage(mapping, InboundMessage{})
	require.NoError(t, err)
	require.Equal(t, 1, count)

	var body map[string]any
	require.NoError(t, json.Unmarshal(out, &body))
	assert.Equal(t, float64(7), body["counter"])
}

const statefulMsgDoc = `asyncapi: 3.0.0
info:
  title: State
  version: 1.0.0
channels:
  ch:
    address: /ch
    messages:
      msg:
        examples:
          - name: ex
            payload: {}
            x-mock-set-state:
              counter: 5
operations:
  send:
    action: send
    channel:
      $ref: '#/channels/ch'
`

func mustMessageSpecs(t *testing.T, raw string) []*loader.MessageSpec {
	t.Helper()
	doc, err := asyncapi.Parse([]byte(raw))
	require.NoError(t, err)
	require.NotZero(t, len(doc.Channels))
	return loader.MessageSpecsFromAsync(doc.Channels[0].Messages)
}
