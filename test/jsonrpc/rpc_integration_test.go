package jsonrpc_test

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
Scenario: Single JSON-RPC call returns correct example with body.id echoed
Given a schema with x-rpc gateway and subtract procedure
When a JSON-RPC single call is sent to the gateway
Then the response contains the correct result with {$request.body.id} evaluated

Related spec scenarios: RS.JRP.17, RS.JRP.25
*/
func TestRpcSingleCall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test-rpc.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	body := `{"jsonrpc":"2.0","method":"subtract","params":{"a":"10","b":"3"},"id":1}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/rpc", port), "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "2.0", result["jsonrpc"])
	assert.Equal(t, float64(1), result["id"])
	assert.Equal(t, "10 - 3", result["result"])

	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
	}
}

/*
Scenario: JSON-RPC batch with mixed success and error
Given a schema with multiple procedures
When a batch with one valid method, one unknown method, and one notification is sent
Then the response array contains entries for valid and error, notification is absent

Related spec scenarios: RS.JRP.19, RS.JRP.20, RS.JRP.21
*/
func TestRpcBatchMixed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test-rpc.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	body := `[{"jsonrpc":"2.0","method":"add","params":{"a":1,"b":2},"id":1},{"jsonrpc":"2.0","method":"unknown","id":2},{"jsonrpc":"2.0","method":"add","params":{"a":3,"b":4}}]`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/rpc", port), "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var results []map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&results)
	require.NoError(t, err)
	assert.Len(t, results, 2)

	var foundSuccess, foundError bool
	for _, r := range results {
		if r["id"] == float64(1) {
			foundSuccess = true
			assert.NotNil(t, r["result"])
		}
		if r["id"] == float64(2) {
			foundError = true
			assert.NotNil(t, r["error"])
			assert.Equal(t, float64(-32601), r["error"].(map[string]interface{})["code"])
		}
	}
	assert.True(t, foundSuccess)
	assert.True(t, foundError)

	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
	}
}

/*
Scenario: JSON-RPC notification applies state changes and returns 204
Given a schema with setState procedure that uses x-mock-set-state
When a notification is sent
Then the state is updated and the server returns HTTP 204

Related spec scenarios: RS.JRP.23, RS.JRP.24, RS.JRP.27
*/
func TestRpcNotificationState(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	// Cannot run in parallel since state is shared across tests
	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test-rpc.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	// Send notification to set state
	notifyBody := `{"jsonrpc":"2.0","method":"setState","params":{"value":"42"}}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/rpc", port), "application/json", strings.NewReader(notifyBody))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Verify state via another procedure
	checkBody := `{"jsonrpc":"2.0","method":"getState","id":1}`
	resp2, err := http.Post(fmt.Sprintf("http://localhost:%d/rpc", port), "application/json", strings.NewReader(checkBody))
	require.NoError(t, err)
	defer resp2.Body.Close() //nolint:errcheck

	var result map[string]interface{}
	err = json.NewDecoder(resp2.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "42", result["result"])

	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
	}
}

/*
Scenario: JSON-RPC method not found returns -32601 error
Given a schema with limited procedures
When a method that doesn't exist is called
Then the server returns a -32601 Method not found error

Related spec scenarios: RS.JRP.18
*/
func TestRpcMethodNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test-rpc.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	body := `{"jsonrpc":"2.0","method":"nonexistent","id":99}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/rpc", port), "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, float64(99), result["id"])
	assert.Equal(t, float64(-32601), result["error"].(map[string]interface{})["code"])
	assert.Contains(t, result["error"].(map[string]interface{})["message"], "not found")

	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
	}
}

/*
Scenario: RPC gateway with no POST operations starts with empty procedure map
Given a schema with x-rpc gateway but only GET operations under it
When a JSON-RPC call is sent to the gateway
Then the server starts successfully and returns method-not-found for any procedure

Related spec scenarios: RS.JRP.9
*/
func TestRpcNoProcedures(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test-rpc-empty.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	body := `{"jsonrpc":"2.0","method":"anything","id":1}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/rpc", port), "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, float64(1), result["id"])
	assert.Equal(t, float64(-32601), result["error"].(map[string]interface{})["code"])
	assert.Contains(t, result["error"].(map[string]interface{})["message"], "not found")

	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
	}
}

/*
Scenario: JSON-RPC parse error returns -32700
Given an RPC gateway
When invalid JSON is sent
Then the server returns -32700 Parse error

Related spec scenarios: RS.JRP.12
*/
func TestRpcParseError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test-rpc.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	body := `not json`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/rpc", port), "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	raw, _ := io.ReadAll(resp.Body)
	t.Logf("parse error response: %s", string(raw))

	var result map[string]interface{}
	err = json.Unmarshal(raw, &result)
	require.NoError(t, err)
	assert.Equal(t, float64(-32700), result["error"].(map[string]interface{})["code"])

	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
	}
}

/*
Scenario: RPC and HTTP routes coexist in the same spec
Given a schema with both RPC gateway and normal HTTP routes
When both endpoints are called
Then both respond correctly

Related spec scenarios: RS.JRP.31
*/
func TestRpcCoexistence(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test-rpc-coexistence.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	// Test RPC endpoint
	rpcBody := `{"jsonrpc":"2.0","method":"hello","params":{"name":"World"},"id":1}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/rpc", port), "application/json", strings.NewReader(rpcBody))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	var rpcResult map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&rpcResult)
	require.NoError(t, err)
	assert.Equal(t, "hello World", rpcResult["result"])

	// Test HTTP endpoint
	httpResp, err := http.Get(fmt.Sprintf("http://localhost:%d/users", port))
	require.NoError(t, err)
	defer httpResp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, httpResp.StatusCode)

	var httpResult map[string]interface{}
	err = json.NewDecoder(httpResp.Body).Decode(&httpResult)
	require.NoError(t, err)
	assert.Contains(t, httpResult["users"], "alice")

	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
	}
}

/*
Scenario: Schema prefix applied to RPC gateway
Given a schema with gateway /rpc and CLI --prefix /api
When a POST is sent to /api/rpc
Then the RPC handler responds correctly

Related spec scenarios: RS.JRP.32
*/
func TestRpcWithPrefix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test-rpc-coexistence.yaml", "/api").Run()
	defer clihelper.StopServer(t, cmd)

	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	body := `{"jsonrpc":"2.0","method":"hello","params":{"name":"Prefix"},"id":1}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/rpc", port), "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "hello Prefix", result["result"])

	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
	}
}

/*
Scenario: x-mock-once with RPC disposes example after first use
Given a schema with an RPC procedure having x-mock-once example
When the procedure is called twice
Then the first call returns the once example and the second returns the fallback

Related spec scenarios: RS.JRP.28
*/
func TestRpcOnceExample(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test-rpc.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	body := `{"jsonrpc":"2.0","method":"once","id":1}`
	url := fmt.Sprintf("http://localhost:%d/rpc", port)

	// First call
	resp1, err := http.Post(url, "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp1.Body.Close() //nolint:errcheck

	var r1 map[string]interface{}
	err = json.NewDecoder(resp1.Body).Decode(&r1)
	require.NoError(t, err)
	assert.Equal(t, "once-only", r1["result"])

	// Second call
	resp2, err := http.Post(url, "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp2.Body.Close() //nolint:errcheck

	var r2 map[string]interface{}
	err = json.NewDecoder(resp2.Body).Decode(&r2)
	require.NoError(t, err)
	assert.Equal(t, "fallback", r2["result"])

	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
	}
}

/*
Scenario: x-mock-skip with RPC never uses skipped example
Given a schema with an RPC procedure having x-mock-skip example
When the procedure is called
Then the skipped example is never returned

Related spec scenarios: RS.JRP.30
*/
func TestRpcSkipExample(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test-rpc.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	body := `{"jsonrpc":"2.0","method":"skip","id":1}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/rpc", port), "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)
	assert.Equal(t, "used", result["result"])
	assert.NotEqual(t, "skipped", result["result"])

	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
	}
}

/*
Scenario: x-mock-headers with RPC includes evaluated headers
Given a schema with an RPC procedure having x-mock-headers
When the procedure is called
Then the response includes the evaluated headers

Related spec scenarios: RS.JRP.29
*/
func TestRpcHeaders(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test-rpc.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	body := `{"jsonrpc":"2.0","method":"headers","id":42}`
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/rpc", port), "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	assert.Equal(t, "test-value-42", resp.Header.Get("X-Custom-Header"))
	assert.Equal(t, "static-value", resp.Header.Get("X-Static"))

	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
	}
}

/*
Scenario: x-mock-params-match with RPC evaluates against per-call params
Given a schema with an RPC procedure having x-mock-params-match
When calls with different params are made
Then the matching example is returned per-call

Related spec scenarios: RS.JRP.26
*/
func TestRpcParamsMatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cmd, errCh, port := clihelper.Cmd(t).SetSchema("../_shared/resources/test-rpc.yaml", "").Run()
	defer clihelper.StopServer(t, cmd)

	if !clihelper.WaitForServer(t, port, 2*time.Second) {
		t.Fatal("server did not start within timeout")
	}

	url := fmt.Sprintf("http://localhost:%d/rpc", port)

	// Admin call
	adminBody := `{"jsonrpc":"2.0","method":"paramsMatch","params":{"role":"admin"},"id":1}`
	resp1, err := http.Post(url, "application/json", strings.NewReader(adminBody))
	require.NoError(t, err)
	defer resp1.Body.Close() //nolint:errcheck

	var r1 map[string]interface{}
	err = json.NewDecoder(resp1.Body).Decode(&r1)
	require.NoError(t, err)
	assert.Equal(t, "admin-response", r1["result"])

	// User call
	userBody := `{"jsonrpc":"2.0","method":"paramsMatch","params":{"role":"user"},"id":2}`
	resp2, err := http.Post(url, "application/json", strings.NewReader(userBody))
	require.NoError(t, err)
	defer resp2.Body.Close() //nolint:errcheck

	var r2 map[string]interface{}
	err = json.NewDecoder(resp2.Body).Decode(&r2)
	require.NoError(t, err)
	assert.Equal(t, "user-response", r2["result"])

	select {
	case err := <-errCh:
		if err != nil && err.Error() != "signal: terminated" {
			t.Logf("server process exited with error: %v", err)
		}
	default:
	}
}
