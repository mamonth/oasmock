package servercore_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mamonth/oasmock/test/_shared/clihelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

/*
Scenario: Basic server startup and request handling
Given a simple OpenAPI schema
When server is started with that schema
Then it responds to defined endpoints with correct status codes

Related spec scenarios: RS.MSC.1, RS.MSC.4, RS.MSC.7
*/
func TestServerBasic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	// Start CLI server with auto port discovery
	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	// Wait for server to be ready
	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	// Make a request
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/users", port))
	require.NoError(t, err, "failed to make request")
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, resp.StatusCode, "expected status 200")
	// TODO: check response body

	// Stop server (deferred clihelper.StopServer will handle it)
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
Scenario: Server matches requests based on parameter conditions
Given an OpenAPI schema with x‑mock‑params‑match examples
When requests are made with matching and non‑matching query parameters
Then matching requests return specific example, others fall back to default

Related spec scenarios: RS.EXT.1, RS.MSC.10
*/
func TestServerParamsMatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	// Start CLI server with params match schema and auto port discovery
	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test-params.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	// Wait for server to be ready
	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	// Helper to make request and parse JSON response
	makeRequest := func(query string) map[string]any {
		url := fmt.Sprintf("http://localhost:%d/search%s", port, query)
		resp, err := http.Get(url)
		require.NoError(t, err, "failed to make request")
		defer resp.Body.Close() //nolint:errcheck
		assert.Equal(t, 200, resp.StatusCode, "expected status 200")
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "failed to read response body")
		var data map[string]any
		require.NoError(t, json.Unmarshal(body, &data), "failed to parse JSON response")
		return data
	}

	// Request with id=12 should match exactMatch example
	data := makeRequest("?id=12")
	assert.Equal(t, "matched id=12", data["message"], "expected message 'matched id=12'")

	// Request with id=13 should fall back to default example
	data = makeRequest("?id=13")
	assert.Equal(t, "default response", data["message"], "expected message 'default response'")

	// Request without id should also default
	data = makeRequest("")
	assert.Equal(t, "default response", data["message"], "expected message 'default response'")

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
Scenario: Adding dynamic examples via control API
Given a running server with a static OpenAPI schema
When a dynamic example is added via POST /_mock/examples
Then subsequent requests to that path return the dynamic example

Related spec scenarios: RS.MAPI.2
*/
func TestServerDynamicExample(t *testing.T) {
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

	// Add a dynamic example for /users path
	exampleJSON := `{
		"path": "/users",
		"method": "GET",
		"once": false,
		"conditions": {},
		"response": {
			"code": 200,
			"headers": { "X-Custom": "dynamic" },
			"body": { "message": "dynamic example" }
		}
	}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/_mock/examples", port), "application/json", strings.NewReader(exampleJSON))
	require.NoError(t, err, "failed to POST dynamic example")
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, resp.StatusCode, "expected status 200")
	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result), "failed to decode response")
	success, ok := result["success"].(bool)
	require.True(t, ok, "success field missing or not bool")
	assert.True(t, success, "expected success true")

	// Now make a request to /users and verify dynamic example is used
	req, err := http.Get(fmt.Sprintf("http://localhost:%d/users", port))
	require.NoError(t, err, "failed to make request")
	defer req.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, req.StatusCode, "expected status 200")
	assert.Equal(t, "dynamic", req.Header.Get("X-Custom"), "expected X-Custom header 'dynamic'")
	var data map[string]any
	require.NoError(t, json.NewDecoder(req.Body).Decode(&data), "failed to decode response")
	assert.Equal(t, "dynamic example", data["message"], "expected message 'dynamic example'")

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
Scenario: Dynamic examples with embedded runtime expressions
Given a running server
When a dynamic example with embedded runtime expressions is added
Then requests matching conditions produce responses with evaluated expressions

Related spec scenarios: RS.MSC.14
*/
func TestServerEmbeddedExpressions(t *testing.T) {
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

	// Add a dynamic example with embedded runtime expression in body
	exampleJSON := `{
		"path": "/users",
		"method": "GET",
		"once": false,
		"conditions": {
			"{$request.query.embed}": "yes"
		},
		"response": {
			"code": 200,
			"headers": { "X-Embed": "prefix-{$request.query.embed}" },
			"body": { "message": "embedded {$request.query.embed}" }
		}
	}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/_mock/examples", port), "application/json", strings.NewReader(exampleJSON))
	require.NoError(t, err, "failed to POST dynamic example")
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, resp.StatusCode, "expected status 200")
	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result), "failed to decode response")
	success, ok := result["success"].(bool)
	require.True(t, ok, "success field missing or not bool")
	assert.True(t, success, "expected success true")

	// Make request with query param embed=yes
	req, err := http.Get(fmt.Sprintf("http://localhost:%d/users?embed=yes", port))
	require.NoError(t, err, "failed to make request")
	defer req.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, req.StatusCode, "expected status 200")
	assert.Equal(t, "prefix-yes", req.Header.Get("X-Embed"), "expected X-Embed header 'prefix-yes'")
	var data map[string]any
	require.NoError(t, json.NewDecoder(req.Body).Decode(&data), "failed to decode response")
	assert.Equal(t, "embedded yes", data["message"], "expected message 'embedded yes'")

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
Scenario: Once‑only example execution
Given an OpenAPI schema with x‑mock‑once example
When multiple requests are made to the same endpoint
Then the once example is returned only for the first request, default for subsequent

Related spec scenarios: RS.EXT.8, RS.MSC.12
*/
func TestServerOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	// Start CLI server with once schema and auto port discovery
	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test-once.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	// Wait for server to be ready
	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	// Make a request
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/once", port))
	require.NoError(t, err, "failed to make request")
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, resp.StatusCode, "expected status 200")
	var data1 map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data1), "failed to decode response")
	assert.Equal(t, "first (once)", data1["message"], "first request expected message 'first (once)'")

	// Second request should return the default example
	resp2, err := http.Get(fmt.Sprintf("http://localhost:%d/once", port))
	require.NoError(t, err, "failed to make second request")
	defer resp2.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, resp2.StatusCode, "expected status 200")
	var data2 map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&data2), "failed to decode response")
	assert.Equal(t, "second (default)", data2["message"], "second request expected message 'second (default)'")

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
Scenario: Request history recording and filtering
Given a running server with request history enabled
When multiple requests are made
Then history API returns recorded requests, filterable by path and method

Related spec scenarios: RS.MAPI.7, RS.MAPI.8, RS.MAPI.9, RS.MAPI.13
*/
func TestServerRequestHistory(t *testing.T) {
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

	// Make a few requests
	req1, err := http.Get(fmt.Sprintf("http://localhost:%d/users", port))
	require.NoError(t, err, "failed to make first request")
	req1.Body.Close()                 //nolint:errcheck
	time.Sleep(10 * time.Millisecond) // ensure timestamps differ

	req2, err := http.Get(fmt.Sprintf("http://localhost:%d/users", port))
	require.NoError(t, err, "failed to make second request")
	req2.Body.Close() //nolint:errcheck

	// Retrieve request history
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/_mock/requests", port))
	require.NoError(t, err, "failed to retrieve request history")
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, resp.StatusCode, "expected status 200")
	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result), "failed to decode response")
	data, ok := result["data"].([]any)
	require.True(t, ok, "expected data array")
	assert.GreaterOrEqual(t, len(data), 2, "expected at least 2 history entries")

	// Filter by path
	resp2, err := http.Get(fmt.Sprintf("http://localhost:%d/_mock/requests?path=/users", port))
	require.NoError(t, err, "failed to retrieve filtered history")
	defer resp2.Body.Close() //nolint:errcheck
	var result2 map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&result2), "failed to decode filtered response")
	data2, ok := result2["data"].([]any)
	require.True(t, ok, "expected data array in filtered response")
	assert.GreaterOrEqual(t, len(data2), 2, "expected at least 2 filtered entries")

	// Filter by method (GET)
	resp3, err := http.Get(fmt.Sprintf("http://localhost:%d/_mock/requests?method=GET", port))
	require.NoError(t, err, "failed to retrieve method-filtered history")
	defer resp3.Body.Close() //nolint:errcheck
	var result3 map[string]any
	require.NoError(t, json.NewDecoder(resp3.Body).Decode(&result3), "failed to decode method-filtered response")
	data3, ok := result3["data"].([]any)
	require.True(t, ok, "expected data array in method-filtered response")
	assert.GreaterOrEqual(t, len(data3), 2, "expected at least 2 method-filtered entries")

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
Scenario: OpenAPI extensions (set‑state, headers) behavior
Given an OpenAPI schema with x‑mock‑set‑state and x‑mock‑headers extensions
When requests are made to endpoints using those extensions
Then state is updated, headers are added, and runtime expressions read state

Related spec scenarios: RS.EXT.4, RS.EXT.5, RS.EXT.7, RS.MSC.20, RS.MSC.28
*/
func TestServerExtensions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	// Start CLI server with extensions schema and auto port discovery
	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test-extensions.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	// Wait for server to be ready
	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	// Test x-mock-set-state increment
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/state", port))
	require.NoError(t, err, "failed to make state request")
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, resp.StatusCode, "expected status 200")
	var data1 map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&data1), "failed to decode response")
	assert.Equal(t, "counter incremented", data1["message"], "expected message 'counter incremented'")

	// Test reading state via runtime expression
	resp2, err := http.Get(fmt.Sprintf("http://localhost:%d/read", port))
	require.NoError(t, err, "failed to make read request")
	defer resp2.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, resp2.StatusCode, "expected status 200")
	var data2 map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&data2), "failed to decode response")
	counter, ok := data2["counter"].(float64)
	require.True(t, ok, "counter field missing or not number")
	assert.Equal(t, float64(1), counter, "expected counter = 1")

	// Test x-mock-headers
	resp3, err := http.Get(fmt.Sprintf("http://localhost:%d/headers", port))
	require.NoError(t, err, "failed to make headers request")
	defer resp3.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, resp3.StatusCode, "expected status 200")
	assert.Equal(t, "custom-value", resp3.Header.Get("X-Custom-Header"), "expected X-Custom-Header = 'custom-value'")
	var data3 map[string]any
	require.NoError(t, json.NewDecoder(resp3.Body).Decode(&data3), "failed to decode response")
	assert.Equal(t, "headers added", data3["message"], "expected message 'headers added'")

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
Scenario: CORS headers present by default
Given a running server with CORS not disabled
When a request is processed
Then the response includes appropriate CORS headers (Access-Control-Allow-Origin: * etc.)

Related spec scenarios: RS.MSC.33
*/
func TestServerCORSHeadersPresent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Start CLI server with test schema (control API enabled by default) and auto port discovery
	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	// Wait for server to be ready
	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	// Make a request with Origin header to trigger CORS
	req, err := http.NewRequest("GET", fmt.Sprintf("http://localhost:%d/users", port), nil)
	require.NoError(t, err, "failed to create request")
	req.Header.Set("Origin", "http://example.com")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "failed to make request")
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, resp.StatusCode, "expected status 200")
	t.Logf("Response headers: %v", resp.Header)
	// Check for CORS headers
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"), "expected CORS header Access-Control-Allow-Origin: *")
	// Additional CORS headers may be present; we can check for Vary: Origin
	assert.Contains(t, resp.Header.Get("Vary"), "Origin")

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
Scenario: CORS disabled
Given a running server started with --nocors or OASMOCK_NO_CORS=true
When a request is processed
Then responses do not include CORS headers

Related spec scenarios: RS.MSC.34
*/
func TestServerCORSDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	// Start CLI server with CORS disabled and auto port discovery
	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test.yaml", "").DisableCORS(true).Run()
	defer clihelper.StopServer(t, cmd)

	// Wait for server to be ready
	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	// Make a request
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/users", port))
	require.NoError(t, err, "failed to make request")
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, 200, resp.StatusCode, "expected status 200")
	// Verify no CORS headers
	assert.Empty(t, resp.Header.Get("Access-Control-Allow-Origin"), "CORS header should not be present when disabled")
	assert.Empty(t, resp.Header.Get("Access-Control-Allow-Methods"), "CORS header should not be present when disabled")

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
Scenario: Default delay
Given a running server without --delay option
When a request is made
Then the server responds immediately

Related spec scenarios: RS.MSC.35
*/
func TestServerDefaultDelay(t *testing.T) {
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

	// Measure response time (roughly)
	start := time.Now()
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/users", port))
	require.NoError(t, err, "failed to make request")
	defer resp.Body.Close() //nolint:errcheck
	elapsed := time.Since(start)
	assert.Equal(t, 200, resp.StatusCode, "expected status 200")
	// Default delay is 100ms per mock.go defaultDelay; we expect response within 200ms (allowing for overhead)
	assert.Less(t, elapsed, 200*time.Millisecond, "response should be within 200ms with default delay")

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
Scenario: Custom delay
Given a running server started with --delay 500
When a request is made
Then the server waits 500ms before responding

Related spec scenarios: RS.MSC.36
*/
func TestServerCustomDelay(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	// Start CLI server with custom delay and auto port discovery
	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test.yaml", "").SetDelay(500).Run()
	defer clihelper.StopServer(t, cmd)

	// Wait for server to be ready
	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	// Measure response time
	start := time.Now()
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/users", port))
	require.NoError(t, err, "failed to make request")
	defer resp.Body.Close() //nolint:errcheck
	elapsed := time.Since(start)
	assert.Equal(t, 200, resp.StatusCode, "expected status 200")
	// Should be at least 500ms (allowing some tolerance)
	assert.GreaterOrEqual(t, elapsed, 450*time.Millisecond, "response delay should be at least 450ms")
	// Should not be extremely long (max 1 second)
	assert.Less(t, elapsed, 1*time.Second, "response delay should be less than 1 second")

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
