package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const paramOpenAPISpec = `
openapi: 3.0.3
info:
  title: Param API
  version: 1.0.0
paths:
  /users/{userId}:
    get:
      operationId: getUser
      parameters:
        - name: userId
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: OK
          content:
            application/json:
              examples:
                default:
                  value:
                    id: "{$request.path.userId}"
`

/*
Scenario: OpenAPI path parameters are captured end-to-end via the router
Given an OpenAPI spec with GET /users/{userId} and an example referencing
{$request.path.userId}
When a request arrives at /users/123 through the real router
Then the response body contains the captured parameter value 123

Related spec scenarios: RS.MSC.5
*/
func TestOpenAPIParam_CapturedEndToEnd(t *testing.T) {
	t.Parallel()

	ldr := openapi3.NewLoader()
	spec, err := ldr.LoadFromData([]byte(paramOpenAPISpec))
	require.NoError(t, err)
	require.NoError(t, spec.Validate(ldr.Context))

	schemas := []loader.SchemaInfo{{Kind: loader.KindOpenAPI, Spec: spec, Prefix: ""}}
	srv, err := New(Config{HistorySize: DefaultHistorySize}, schemas)
	require.NoError(t, err)
	defer func() { _ = srv.Shutdown(context.Background()) }()

	ts := httptest.NewServer(srv.router)
	defer ts.Close() //nolint:errcheck

	resp, err := http.Get(ts.URL + "/users/123")
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body := make([]byte, 256)
	n, _ := resp.Body.Read(body)
	assert.Contains(t, string(body[:n]), `"id":"123"`)
}
