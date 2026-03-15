package managementapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mamonth/oasmock/test/_shared/clihelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Adding a conditional example
Given a running server
When a POST request to /_mock/examples includes conditions object with runtime expressions
Then the server stores the example and will match it only when conditions are satisfied

Related spec scenarios: RS.MAPI.3
*/
func TestManagementAPIConditionalExample(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	// Start CLI server with test schema (control API enabled by default) and auto port discovery
	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	// Wait for server to be ready
	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	// Add a conditional example based on query parameter
	exampleJSON := `{
		"path": "/conditional",
		"method": "GET",
		"once": false,
		"conditions": {
			"{$request.query.mode}": "test"
		},
		"response": {
			"code": 200,
			"headers": { "X-Condition": "matched" },
			"body": { "status": "conditional" }
		}
	}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/_mock/examples", port), "application/json", strings.NewReader(exampleJSON))
	require.NoError(t, err, "failed to POST conditional example")
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, resp.StatusCode, "expected status 200")
	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result), "failed to decode response")
	success, ok := result["success"].(bool)
	require.True(t, ok, "success field missing or not bool")
	assert.True(t, success, "expected success true")

	// Make request with matching query parameter
	req, err := http.Get(fmt.Sprintf("http://localhost:%d/conditional?mode=test", port))
	require.NoError(t, err, "failed to make matching request")
	defer req.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, req.StatusCode, "expected status 200")
	assert.Equal(t, "matched", req.Header.Get("X-Condition"), "expected X-Condition header 'matched'")
	var data map[string]any
	require.NoError(t, json.NewDecoder(req.Body).Decode(&data), "failed to decode response")
	assert.Equal(t, "conditional", data["status"], "expected status 'conditional'")

	// Make request with non-matching query parameter (should not match conditional example, fallback to default)
	req2, err := http.Get(fmt.Sprintf("http://localhost:%d/conditional?mode=other", port))
	require.NoError(t, err, "failed to make non-matching request")
	defer req2.Body.Close() //nolint:errcheck
	// Expect 501 because no default example defined for /conditional (route exists but no example matches)
	assert.Equal(t, 501, req2.StatusCode, "expected status 501 for non-matching condition")

	// Check for any errors from the server process
	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
		// No error yet, process still running
	}
}

/*
Scenario: Adding a one-time example
Given a running server
When a POST request to /_mock/examples includes once: true
Then the server stores the example as one-time (disposed after first match)

Related spec scenarios: RS.MAPI.4
*/
func TestManagementAPIOneTimeExample(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	// Start CLI server with test schema (control API enabled by default) and auto port discovery
	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	// Wait for server to be ready
	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	// Add a one-time example
	exampleJSON := `{
		"path": "/once",
		"method": "GET",
		"once": true,
		"conditions": {},
		"response": {
			"code": 200,
			"headers": { "X-Once": "first" },
			"body": { "message": "first and only" }
		}
	}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/_mock/examples", port), "application/json", strings.NewReader(exampleJSON))
	require.NoError(t, err, "failed to POST one-time example")
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, resp.StatusCode, "expected status 200")
	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result), "failed to decode response")
	success, ok := result["success"].(bool)
	require.True(t, ok, "success field missing or not bool")
	assert.True(t, success, "expected success true")

	// First request should match the one-time example
	req1, err := http.Get(fmt.Sprintf("http://localhost:%d/once", port))
	require.NoError(t, err, "failed to make first request")
	defer req1.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, req1.StatusCode, "expected status 200")
	assert.Equal(t, "first", req1.Header.Get("X-Once"), "expected X-Once header 'first'")
	var data1 map[string]any
	require.NoError(t, json.NewDecoder(req1.Body).Decode(&data1), "failed to decode response")
	assert.Equal(t, "first and only", data1["message"], "expected message 'first and only'")

	// Second request should not match the one-time example (should be removed)
	req2, err := http.Get(fmt.Sprintf("http://localhost:%d/once", port))
	require.NoError(t, err, "failed to make second request")
	defer req2.Body.Close() //nolint:errcheck
	// Expect 501 because no default example defined for /once (route exists but no example matches)
	assert.Equal(t, 501, req2.StatusCode, "expected status 501 after one-time example consumed")

	// Check for any errors from the server process
	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
		// No error yet, process still running
	}
}

/*
Scenario: Adding an example with validation disabled
Given a running server
When a POST request to /_mock/examples includes validate: false
Then the server does not validate the example data against the OpenAPI schema

Related spec scenarios: RS.MAPI.5
*/
func TestManagementAPIValidationDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	// Start CLI server with test schema (control API enabled by default) and auto port discovery
	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	// Wait for server to be ready
	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	// Add an example with validation disabled (path not in schema, but should be accepted)
	exampleJSON := `{
		"path": "/not-in-schema",
		"method": "GET",
		"once": false,
		"validate": false,
		"conditions": {},
		"response": {
			"code": 200,
			"headers": {},
			"body": { "custom": "path" }
		}
	}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/_mock/examples", port), "application/json", strings.NewReader(exampleJSON))
	require.NoError(t, err, "failed to POST example with validation disabled")
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, resp.StatusCode, "expected status 200")
	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result), "failed to decode response")
	success, ok := result["success"].(bool)
	require.True(t, ok, "success field missing or not bool")
	assert.True(t, success, "expected success true")

	// Make request to the custom path
	req, err := http.Get(fmt.Sprintf("http://localhost:%d/not-in-schema", port))
	require.NoError(t, err, "failed to make request")
	defer req.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, req.StatusCode, "expected status 200")
	var data map[string]any
	require.NoError(t, json.NewDecoder(req.Body).Decode(&data), "failed to decode response")
	assert.Equal(t, "path", data["custom"], "expected custom field 'path'")

	// Check for any errors from the server process
	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
		// No error yet, process still running
	}
}

/*
Scenario: ExampleResponse structure
Given a running server
When an example response is provided
Then it includes code (integer), headers (object), and body (any JSON value)

Related spec scenarios: RS.MAPI.15
*/
func TestManagementAPIExampleResponseStructure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	// Start CLI server with test schema (control API enabled by default) and auto port discovery
	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	// Wait for server to be ready
	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	// Add an example with full response structure
	exampleJSON := `{
		"path": "/structured",
		"method": "GET",
		"once": false,
		"conditions": {},
		"response": {
			"code": 201,
			"headers": { "X-Custom": "value", "Content-Type": "application/json" },
			"body": { "nested": { "array": [1, 2, 3] }, "string": "text" }
		}
	}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/_mock/examples", port), "application/json", strings.NewReader(exampleJSON))
	require.NoError(t, err, "failed to POST example")
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, resp.StatusCode, "expected status 200")
	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result), "failed to decode response")
	success, ok := result["success"].(bool)
	require.True(t, ok, "success field missing or not bool")
	assert.True(t, success, "expected success true")

	// Make request and verify response structure matches
	req, err := http.Get(fmt.Sprintf("http://localhost:%d/structured", port))
	require.NoError(t, err, "failed to make request")
	defer req.Body.Close() //nolint:errcheck
	assert.Equal(t, 201, req.StatusCode, "expected status 201")
	assert.Equal(t, "value", req.Header.Get("X-Custom"), "expected X-Custom header 'value'")
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"), "expected Content-Type header 'application/json'")
	var data map[string]any
	require.NoError(t, json.NewDecoder(req.Body).Decode(&data), "failed to decode response")
	nested, ok := data["nested"].(map[string]any)
	require.True(t, ok, "nested field missing or not object")
	arr, ok := nested["array"].([]any)
	require.True(t, ok, "array field missing or not array")
	assert.Len(t, arr, 3, "expected array length 3")
	assert.Equal(t, "text", data["string"], "expected string field 'text'")

	// Check for any errors from the server process
	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
		// No error yet, process still running
	}
}

/*
Scenario: Management API endpoint accessible
Given a running server with control API enabled (default)
When a GET request is sent to /_mock/requests
Then the server responds with HTTP 200 and a JSON list of recent requests

Related spec scenarios: RS.MAPI.1
*/
func TestManagementAPIAccessible(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	// Start CLI server with test schema (control API enabled by default) and auto port discovery
	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test.yaml", "").SetVerbose(true).Run()
	defer clihelper.StopServer(t, cmd)

	// Wait for server to be ready
	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	// Make a request to management API endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/_mock/requests", port))
	require.NoError(t, err, "failed to GET /_mock/requests")
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, resp.StatusCode, "expected status 200")
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"), "expected JSON content type")
	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result), "failed to decode response")
	_, ok := result["data"].([]any)
	assert.True(t, ok, "response missing data array")

	// Check for any errors from the server process
	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
		// No error yet, process still running
	}
}
