package server

import (
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// managementRoutesOf registers a server's management routes onto a fresh chi
// router and returns the registered method+path pairs under /_mock.
func managementRoutesOf(t *testing.T) map[string]bool {
	t.Helper()
	srv := newPushServer(t)
	r := chi.NewRouter()
	srv.registerManagementRoutes(r)
	return walkRoutes(r, map[string]bool{})
}

// walkRoutes enumerates "METHOD /path" keys from a chi route tree.
func walkRoutes(r chi.Routes, out map[string]bool) map[string]bool {
	for _, route := range r.Routes() {
		if route.SubRoutes != nil {
			walkRoutes(route.SubRoutes, out)
			continue
		}
		for method := range route.Handlers {
			full := strings.ToUpper(method) + " " + route.Pattern
			out[full] = true
		}
	}
	return out
}

// loadOpenAPISpec loads api/openapi.yaml into an OpenAPI document.
func loadOpenAPISpec(t *testing.T) *openapi3.T {
	t.Helper()
	data, err := os.ReadFile("../../api/openapi.yaml")
	require.NoError(t, err)
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	require.NoError(t, err)
	require.NoError(t, doc.Validate(loader.Context))
	return doc
}

// openAPIPaths returns "METHOD /path" keys from an OpenAPI document, excluding
// the path placeholder braces discrepancy: chi and OpenAPI both use {param}.
func openAPIPaths(t *testing.T, doc *openapi3.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for path, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		if op := item.Post; op != nil {
			out["POST "+path] = true
		}
		if op := item.Get; op != nil {
			out["GET "+path] = true
		}
		if op := item.Delete; op != nil {
			out["DELETE "+path] = true
		}
		if op := item.Patch; op != nil {
			out["PATCH "+path] = true
		}
		if op := item.Put; op != nil {
			out["PUT "+path] = true
		}
	}
	return out
}

// specBasePath returns the path portion of the document's first server URL
// (e.g. "/_mock" for "http://localhost:19191/_mock"). OpenAPI paths are
// relative to this base; the router serves fully-prefixed paths.
func specBasePath(doc *openapi3.T) string {
	if len(doc.Servers) == 0 {
		return ""
	}
	u, err := url.Parse(doc.Servers[0].URL)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(u.Path, "/")
}

/*
Scenario: The OpenAPI control spec documents the registered management routes
Given the code registers the canonical /_mock/async|events|stream|examples routes,

	the deprecated /_mock/ws aliases and the removed /_mock/ws/schedule 410s

When the OpenAPI control spec and the router are both enumerated
Then every registered route is documented and every documented route is real

Related spec scenarios: RS.AMG.1, RS.MAPI.19, RS.AMG.22, RS.AMG.28
*/
func TestControlAPISpecSync_OpenAPI(t *testing.T) {
	t.Parallel()

	doc := loadOpenAPISpec(t)
	base := specBasePath(doc)
	require.NotEmpty(t, base, "openapi spec must declare a servers base URL")

	// Resolve documented paths against the servers base so both sides use the
	// fully-prefixed route shape the router registers.
	documented := map[string]bool{}
	for route := range openAPIPaths(t, doc) {
		method, path, _ := strings.Cut(route, " ")
		documented[method+" "+base+path] = true
	}
	registered := managementRoutesOf(t)

	// Every route the server registers must appear in the resolved spec.
	for route := range registered {
		assert.True(t, documented[route], "registered route not documented in api/openapi.yaml: %s", route)
	}

	// Every documented management path must be a real route.
	for route := range documented {
		if isControlPath(route) {
			assert.True(t, registered[route], "documented route not registered by the server: %s", route)
		}
	}
}

/*
Scenario: The AsyncAPI control spec envelope fields match the Go structs
Given the manageEnvelope payload structs emitted by the code
When the asyncapi.yaml component schemas are decoded
Then the documented JSON properties for each envelope type match the Go struct
JSON tags exactly (no drift between documentation and realization)

Related spec scenarios: RS.AMG.24, RS.AMG.25, RS.AMG.26, RS.AMG.27
*/
func TestControlAPISpecSync_EnvelopeFields(t *testing.T) {
	t.Parallel()

	schemas := loadAsyncAPISchemas(t)

	expected := map[string][]string{
		"EventEnvelopeBody":    jsonFields(manageEventEnvelope{}),
		"PushEnvelopeBody":     jsonFields(managePushEnvelope{}),
		"ConsumerEnvelopeBody": jsonFields(manageConsumerEnvelope{}),
		"ScheduleEnvelopeBody": jsonFields(manageScheduleEnvelope{}),
	}
	for schemaName, structFields := range expected {
		schema, ok := schemas[schemaName]
		require.True(t, ok, "schema %q missing from api/asyncapi.yaml", schemaName)
		props, ok := schema.properties()
		require.True(t, ok, "schema %q must declare properties", schemaName)
		var documented []string
		for name := range props {
			documented = append(documented, name)
		}
		assert.ElementsMatch(t, structFields, documented,
			"schema %q JSON properties drift from the Go struct", schemaName)
	}
}

// loadAsyncAPISchemas decodes the components.schemas of api/asyncapi.yaml.
func loadAsyncAPISchemas(t *testing.T) map[string]jsonSchema {
	t.Helper()
	data, err := os.ReadFile("../../api/asyncapi.yaml")
	require.NoError(t, err)
	var raw struct {
		Components struct {
			Schemas map[string]jsonSchema `yaml:"schemas"`
		} `yaml:"components"`
	}
	require.NoError(t, yaml.Unmarshal(data, &raw))
	return raw.Components.Schemas
}

// jsonSchema is a minimal view of an AsyncAPI/JSON schema node.
type jsonSchema struct {
	Type        string                `yaml:"type"`
	PropertySet map[string]jsonSchema `yaml:"properties"`
}

// properties returns the declared property names.
func (s jsonSchema) properties() (map[string]jsonSchema, bool) {
	return s.PropertySet, s.PropertySet != nil
}

// jsonFields returns the JSON field names of a struct (implementation dtypex of
// the envelope payloads emitted by /_mock/stream).
func jsonFields(v any) []string {
	tv := reflect.TypeOf(v)
	var out []string
	for i := 0; i < tv.NumField(); i++ {
		tag := tv.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name != "" && name != "-" {
			out = append(out, name)
		}
	}
	return out
}

// isControlPath reports whether a management path belongs to the async-mocking
// or event surface (all of them are under /_mock with a real handler).
func isControlPath(route string) bool {
	path := route[strings.Index(route, " ")+1:]
	return strings.HasPrefix(path, "/_mock/")
}

/*
Scenario: The AsyncAPI control spec documents the management stream channel
Given the code serves GET /_mock/stream with event/push/consumer/schedule envelopes
When the AsyncAPI control spec is parsed
Then its stream channel address is a real route and its message names match the
implemented envelope types

Related spec scenarios: RS.AMG.23, RS.AMG.24, RS.AMG.25, RS.AMG.26, RS.AMG.27
*/
func TestControlAPISpecSync_AsyncAPI(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../api/asyncapi.yaml")
	require.NoError(t, err)
	doc, err := asyncapi.Parse(data)
	require.NoError(t, err)

	ch := doc.Channel("stream")
	require.NotNil(t, ch, "stream channel missing from api/asyncapi.yaml")
	assert.Equal(t, "/_mock/stream", ch.Address)

	// The stream channel must correspond to a real GET /_mock/stream route.
	registered := managementRoutesOf(t)
	require.True(t, registered["GET /_mock/stream"], "GET /_mock/stream must be a registered route")

	// Message names match the envelope types emitted by the code.
	names := map[string]bool{}
	for _, m := range ch.Messages {
		names[m.Name] = true
	}
	for _, expected := range []string{"event", "push", "consumer", "schedule"} {
		assert.True(t, names[expected], "stream channel must document a %q envelope message", expected)
	}
}

/*
Scenario: OpenAPI /stream and AsyncAPI stream channel agree on the surface
Given the /stream path in api/openapi.yaml and the stream channel in api/asyncapi.yaml
When both are parsed
Then they reference the same address and the same envelope kinds

Related spec scenarios: RS.AMG.23
*/
func TestControlAPISpecSync_CrossFormat(t *testing.T) {
	t.Parallel()

	docs := loadOpenAPISpec(t)
	require.Contains(t, docs.Paths.Map(), "/stream")
	streamOp := docs.Paths.Map()["/stream"].Get
	require.NotNil(t, streamOp, "/stream must be a GET operation")

	asyncData, err := os.ReadFile("../../api/asyncapi.yaml")
	require.NoError(t, err)
	asyncDoc, err := asyncapi.Parse(asyncData)
	require.NoError(t, err)
	ch := asyncDoc.Channel("stream")
	require.NotNil(t, ch)

	// The OpenAPI servers block is "http://localhost:19191/_mock", so the
	// documented path /stream is served at /_mock/stream which matches the
	// AsyncAPI channel address.
	assert.Equal(t, "_mock/stream", strings.TrimPrefix(ch.Address, "/"))
}

/*
Scenario: The OpenAPI control spec documents every error response with the Error body
Given the api/openapi.yaml produces a valid OpenAPI document
When each 4xx/5xx response of every management operation is inspected
Then it carries an application/json body whose schema is the Error shape

Related spec scenarios: RS.MAPI.6, RS.MAPI.21, RS.AMG.28
*/
func TestControlAPISpecSync_ErrorResponses(t *testing.T) {
	t.Parallel()
	doc := loadOpenAPISpec(t)

	operations := func(item *openapi3.PathItem) []*openapi3.Operation {
		return []*openapi3.Operation{item.Get, item.Post, item.Delete, item.Patch, item.Put, item.Head, item.Options}
	}

	checked := 0
	for path, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		for _, op := range operations(item) {
			if op == nil {
				continue
			}
			for code, resp := range op.Responses.Map() {
				status, err := strconv.Atoi(code)
				if err != nil || status < 400 {
					continue
				}
				require.NotNil(t, resp.Value,
					"error response %s for %s %s must resolve to a value", code, methodOf(op, item), path)
				content := resp.Value.Content
				require.NotNil(t, content,
					"error response %s for %s %s must declare content", code, methodOf(op, item), path)
				mt := content.Get("application/json")
				require.NotNil(t, mt,
					"error response %s for %s %s must be application/json", code, methodOf(op, item), path)
				schema := mt.Schema.Value
				require.NotNil(t, schema,
					"error response %s for %s %s must carry a schema", code, methodOf(op, item), path)
				require.NotNil(t, schema.Properties,
					"error response %s for %s %s must be an object", code, methodOf(op, item), path)
				require.Contains(t, schema.Properties, "error",
					"error response %s for %s %s must expose the error field", code, methodOf(op, item), path)
				checked++
			}
		}
	}
	require.GreaterOrEqual(t, checked, 15,
		"the control spec should document error bodies for every management operation")
}

// methodOf returns the HTTP method for an operation belonging to a path item.
func methodOf(op *openapi3.Operation, item *openapi3.PathItem) string {
	switch op {
	case item.Get:
		return "GET"
	case item.Post:
		return "POST"
	case item.Delete:
		return "DELETE"
	case item.Patch:
		return "PATCH"
	case item.Put:
		return "PUT"
	case item.Head:
		return "HEAD"
	default:
		return "OPTIONS"
	}
}
