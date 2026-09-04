package loader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const httpChannelSpec = `asyncapi: 3.0.0
info:
  title: HTTP Events
  version: 1.0.0
channels:
  employees:
    address: /employees
    messages:
      emplMsg:
        examples:
          - payload:
              id: 1
operations:
  getEmployees:
    action: send
    channel:
      $ref: '#/channels/employees'
    bindings:
      http:
        method: GET
`

const wsChannelSpec = `asyncapi: 3.0.0
info:
  title: WS Events
  version: 1.0.0
channels:
  socket:
    address: /socket
    bindings:
      ws:
        method: GET
    messages:
      msg:
        examples:
          - payload:
              event: hello
operations:
  receiveSocket:
    action: receive
    channel:
      $ref: '#/channels/socket'
`

const unsupportedChannelSpec = `asyncapi: 3.0.0
info:
  title: Kafka Events
  version: 1.0.0
channels:
  k:
    address: topic
    bindings:
      kafka:
        topic: events
    messages:
      msg:
        examples:
          - payload: {}
operations:
  receiveK:
    action: receive
    channel:
      $ref: '#/channels/k'
`

const noBindingChannelSpec = `asyncapi: 3.0.0
info:
  title: No Binding
  version: 1.0.0
channels:
  n:
    address: something
    messages:
      msg:
        examples:
          - payload: {}
operations:
  receiveN:
    action: receive
    channel:
      $ref: '#/channels/n'
`

/*
Scenario: Mapping an AsyncAPI HTTP channel to a route
Given an asyncapi spec with an http channel binding and a GET operation binding
When BuildRouteMappings is called
Then it produces an http route with the address and method, and the message spec

Related spec scenarios: RS.ASP.1, RS.ASP.10
*/
func TestBuildAsyncRouteMappings_HTTP(t *testing.T) {
	t.Parallel()

	info := mustAsyncInfo(t, httpChannelSpec, "/v1")
	mappings, err := BuildRouteMappings([]SchemaInfo{info})
	require.NoError(t, err)
	require.Len(t, mappings, 1)
	rm := mappings[0]
	assert.Equal(t, "GET", rm.Method)
	assert.Equal(t, "/v1/employees", rm.Path)
	assert.Equal(t, "/employees", rm.Pattern)
	assert.Equal(t, "/v1/employees", rm.ChiPattern)
	assert.Equal(t, asyncapi.ProtocolHTTP, rm.Protocol)
	assert.Equal(t, "send", rm.Action)
	require.Len(t, rm.Messages, 1)
	assert.Equal(t, "emplMsg", rm.Messages[0].Name)
	require.Len(t, rm.Messages[0].Examples, 1)
}

/*
Scenario: Mapping an AsyncAPI WebSocket channel to a route
Given an asyncapi spec with a ws channel binding
When BuildRouteMappings is called
Then it produces a ws route at the channel address

Related spec scenarios: RS.ASP.2
*/
func TestBuildAsyncRouteMappings_WS(t *testing.T) {
	t.Parallel()

	info := mustAsyncInfo(t, wsChannelSpec, "")
	mappings, err := BuildRouteMappings([]SchemaInfo{info})
	require.NoError(t, err)
	require.Len(t, mappings, 1)
	rm := mappings[0]
	assert.Equal(t, asyncapi.ProtocolWS, rm.Protocol)
	assert.Equal(t, "/socket", rm.Path)
	assert.Equal(t, "GET", rm.Method)
	assert.Equal(t, "receive", rm.Action)
}

/*
Scenario: Mapping an AsyncAPI AMQP channel is rejected as unsupported
Given an asyncapi spec with an amqp channel binding
When asyncapi.Parse is called
Then it reports a validation error naming the unsupported protocol

Related spec scenarios: RS.AAL.8, RS.ASP.4
*/
func TestBuildAsyncRouteMappings_AMQPRejected(t *testing.T) {
	t.Parallel()

	data := []byte(`asyncapi: 3.0.0
info:
  title: AMQP Events
  version: 1.0.0
channels:
  signup:
    address: 'user/signup'
    bindings:
      amqp:
        is: routingKey
        exchange:
          name: userExchange
    messages:
      msg:
        examples:
          - payload:
              event: signup
operations:
  receiveSignup:
    action: receive
    channel:
      $ref: '#/channels/signup'
`)
	_, err := asyncapi.Parse(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "amqp")
	assert.Contains(t, err.Error(), "unsupported protocol")
}

/*
Scenario: Rejecting a channel with an unknown protocol binding
Given an asyncapi spec with a kafka channel binding
When the loader parses the spec
Then it reports a validation error naming the unsupported protocol

Related spec scenarios: RS.AAL.8, RS.ASP.4
*/
func TestBuildAsyncRouteMappings_UnsupportedProtocol(t *testing.T) {
	t.Parallel()

	_, err := asyncapi.Parse([]byte(unsupportedChannelSpec))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kafka")
	assert.Contains(t, err.Error(), "unsupported protocol")
}

/*
Scenario: Rejecting a channel without binding information
Given an asyncapi spec with a channel that declares no protocol binding
When BuildRouteMappings is called
Then it reports the channel as invalid with a clear error

Related spec scenarios: RS.ASP.5
*/
func TestBuildAsyncRouteMappings_NoBinding(t *testing.T) {
	t.Parallel()

	info := mustAsyncInfo(t, noBindingChannelSpec, "")
	_, err := BuildRouteMappings([]SchemaInfo{info})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no binding information")
}

/*
Scenario: Applying schema prefix to AsyncAPI channel addresses
Given an asyncapi spec with prefix /v1 and a channel address user/signedup
When BuildRouteMappings is called
Then the route serves under the prefixed address

Related spec scenarios: RS.ASP.8, RS.MSC.51
*/
func TestBuildAsyncRouteMappings_Prefix(t *testing.T) {
	t.Parallel()

	info := mustAsyncInfo(t, wsChannelSpec, "/v1")
	mappings, err := BuildRouteMappings([]SchemaInfo{info})
	require.NoError(t, err)
	require.Len(t, mappings, 1)
	assert.Equal(t, "/v1/socket", mappings[0].Path)
}

/*
Scenario: Multiple AsyncAPI schemas each honor their own prefix
Given two AsyncAPI schemas loaded with prefixes /a1 and /a2
When BuildRouteMappings is called over both
Then routes are produced under each prefixed address

Related spec scenarios: RS.AAL.9
*/
func TestBuildAsyncRouteMappings_TwoSchemasPrefixed(t *testing.T) {
	t.Parallel()

	info1 := mustAsyncInfo(t, wsChannelSpec, "/a1")
	info2 := mustAsyncInfo(t, httpChannelSpec, "/a2")
	mappings, err := BuildRouteMappings([]SchemaInfo{info1, info2})
	require.NoError(t, err)
	require.Len(t, mappings, 2)

	pathSet := make(map[string]bool)
	for _, rm := range mappings {
		pathSet[rm.Path] = true
	}
	assert.True(t, pathSet["/a1/socket"], "expected /a1/socket among routes, got %v", pathSet)
	assert.True(t, pathSet["/a2/employees"], "expected /a2/employees among routes, got %v", pathSet)
}

const loaderOpenAPIDoc = `openapi: 3.0.0
info:
  title: REST
  version: 1.0.0
paths:
  /users:
    get:
      responses:
        '200':
          description: OK
`

/*
Scenario: Mixing OpenAPI and AsyncAPI sources yields routes of both kinds
Given an OpenAPI and an AsyncAPI schema file
When LoadSchemas and BuildRouteMappings are called
Then routes from both kinds are produced

Related spec scenarios: RS.AAL.10, RS.MSC.50
*/
func TestBuildRouteMappings_MixedOpenAPIAndAsyncAPI(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	openapiPath := filepath.Join(dir, "openapi.yaml")
	asyncPath := filepath.Join(dir, "asyncapi.yaml")
	require.NoError(t, os.WriteFile(openapiPath, []byte(loaderOpenAPIDoc), 0o644))
	require.NoError(t, os.WriteFile(asyncPath, []byte(wsChannelSpec), 0o644))

	infos, err := LoadSchemas([]string{openapiPath, asyncPath}, []string{"/v1", "/v2"})
	require.NoError(t, err)
	require.Len(t, infos, 2)

	mappings, err := BuildRouteMappings(infos)
	require.NoError(t, err)
	require.NotEmpty(t, mappings)

	var openapiMapped, asyncMapped bool
	for _, rm := range mappings {
		if rm.Protocol != "" {
			asyncMapped = true
		} else if rm.Operation != nil {
			openapiMapped = true
		}
	}
	assert.True(t, openapiMapped, "expected at least one OpenAPI mapping")
	assert.True(t, asyncMapped, "expected at least one AsyncAPI mapping")
}

func mustAsyncInfo(t *testing.T, spec, prefix string) SchemaInfo {
	t.Helper()
	doc, err := asyncapi.Parse([]byte(spec))
	require.NoError(t, err)
	return SchemaInfo{Kind: KindAsyncAPI, Async: doc, Prefix: prefix}
}
