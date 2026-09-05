package server

import (
	"encoding/json"
	"testing"

	"github.com/mamonth/oasmock/internal/loader"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestProto(callPath string) *JsonRpcProtocol {
	return &JsonRpcProtocol{
		contentType: "application/json",
		callPath:    callPath,
	}
}

/*
Scenario: JsonRpcProtocol.ParseBody parses a valid single call
Given a JsonRpcProtocol with default callPath "method"
When ParseBody is called with a valid JSON-RPC 2.0 single call
Then it returns a 1-element slice with correct Procedure, ID, and HasID=true

Related spec scenarios: RS.JRP.10
*/
func TestJsonRpcProtocol_ParseBody_SingleCall(t *testing.T) {
	t.Parallel()

	proto := newTestProto("method")
	body := []byte(`{"jsonrpc":"2.0","method":"subtract","params":{"a":10},"id":1}`)

	entries, err := proto.ParseBody(body)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	call := entries[0].Call
	require.NotNil(t, call)

	assert.Equal(t, "subtract", call.Procedure)
	assert.Equal(t, float64(1), call.ID)
	assert.True(t, call.HasID)
}

/*
Scenario: JsonRpcProtocol.ParseBody parses a valid batch
Given a JsonRpcProtocol with default callPath "method"
When ParseBody is called with a batch array of 3 call objects
Then it returns a 3-element slice

Related spec scenarios: RS.JRP.11
*/
func TestJsonRpcProtocol_ParseBody_Batch(t *testing.T) {
	t.Parallel()

	proto := newTestProto("method")
	body := []byte(`[{"jsonrpc":"2.0","method":"add","params":{"a":1},"id":1},{"jsonrpc":"2.0","method":"sub","params":{"a":2},"id":2},{"jsonrpc":"2.0","method":"mul","params":{"a":3},"id":3}]`)

	entries, err := proto.ParseBody(body)
	require.NoError(t, err)
	assert.Len(t, entries, 3)
	for _, e := range entries {
		assert.NotNil(t, e.Call)
	}
}

/*
Scenario: JsonRpcProtocol.ParseBody handles notification (no id)
Given a JsonRpcProtocol with default callPath "method"
When ParseBody is called with a notification (no id field)
Then it returns a call with HasID=false but still in the slice

Related spec scenarios: RS.JRP.23
*/
func TestJsonRpcProtocol_ParseBody_Notification(t *testing.T) {
	t.Parallel()

	proto := newTestProto("method")
	body := []byte(`{"jsonrpc":"2.0","method":"log","params":{"msg":"hello"}}`)

	entries, err := proto.ParseBody(body)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	call := entries[0].Call
	require.NotNil(t, call)

	assert.False(t, call.HasID)
	assert.Nil(t, call.ID)
	assert.Equal(t, "log", call.Procedure)
}

/*
Scenario: JsonRpcProtocol.ParseBody handles null id
Given a JsonRpcProtocol with default callPath "method"
When ParseBody is called with id: null
Then HasID is false (per JSON-RPC 2.0 spec, null id means no response)

Related spec scenarios: RS.JRP.23
*/
func TestJsonRpcProtocol_ParseBody_NullId(t *testing.T) {
	t.Parallel()

	proto := newTestProto("method")
	body := []byte(`{"jsonrpc":"2.0","method":"notify","id":null}`)

	entries, err := proto.ParseBody(body)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	call := entries[0].Call
	require.NotNil(t, call)

	assert.False(t, call.HasID)
	assert.Nil(t, call.ID)
}

/*
Scenario: JsonRpcProtocol.ParseBody returns error on invalid JSON
Given a JsonRpcProtocol
When ParseBody is called with invalid JSON
Then it returns an error

Related spec scenarios: RS.JRP.12
*/
func TestJsonRpcProtocol_ParseBody_InvalidJSON(t *testing.T) {
	t.Parallel()

	proto := newTestProto("method")
	body := []byte(`not json`)

	_, err := proto.ParseBody(body)
	assert.Error(t, err)
}

/*
Scenario: JsonRpcProtocol.ParseBody reports error code -32600 for missing jsonrpc
Given a JsonRpcProtocol
When ParseBody is called with a valid JSON object but missing jsonrpc
Then it returns a typed error whose code is -32600 Invalid Request

Related spec scenarios: RS.JRP.13
*/
func TestJsonRpcProtocol_ParseBody_MissingJsonrpc_Code(t *testing.T) {
	t.Parallel()

	proto := newTestProto("method")
	body := []byte(`{"method":"sub","id":1}`)

	_, err := proto.ParseBody(body)
	require.Error(t, err)
	assert.Equal(t, -32600, rpcErrorCode(err))
}

/*
Scenario: JsonRpcProtocol.ParseBody reports -32600 for missing method
Given a JsonRpcProtocol
When ParseBody is called with jsonrpc: "2.0" but no method field
Then it returns a typed error whose code is -32600 Invalid Request

Related spec scenarios: RS.JRP.14
*/
func TestJsonRpcProtocol_ParseBody_MissingMethod_Code(t *testing.T) {
	t.Parallel()

	proto := newTestProto("method")
	body := []byte(`{"jsonrpc":"2.0","id":1}`)

	_, err := proto.ParseBody(body)
	require.Error(t, err)
	assert.Equal(t, -32600, rpcErrorCode(err))
}

/*
Scenario: JsonRpcProtocol.ParseBody reports -32600 for wrong version
Given a JsonRpcProtocol
When ParseBody is called with jsonrpc: "1.0"
Then it returns a typed error whose code is -32600 Invalid Request

Related spec scenarios: RS.JRP.15
*/
func TestJsonRpcProtocol_ParseBody_WrongVersion_Code(t *testing.T) {
	t.Parallel()

	proto := newTestProto("method")
	body := []byte(`{"jsonrpc":"1.0","method":"sub","id":1}`)

	_, err := proto.ParseBody(body)
	require.Error(t, err)
	assert.Equal(t, -32600, rpcErrorCode(err))
}

/*
Scenario: JsonRpcProtocol.ParseBody reports -32700 for malformed JSON
Given a JsonRpcProtocol
When ParseBody is called with invalid JSON
Then it returns a typed error whose code is -32700 Parse error

Related spec scenarios: RS.JRP.12
*/
func TestJsonRpcProtocol_ParseBody_InvalidJSON_Code(t *testing.T) {
	t.Parallel()

	proto := newTestProto("method")
	body := []byte(`not json`)

	_, err := proto.ParseBody(body)
	require.Error(t, err)
	assert.Equal(t, -32700, rpcErrorCode(err))
}

/*
Scenario: JsonRpcProtocol.ParseBody rejects a top-level non-object/array
Given a JsonRpcProtocol
When ParseBody is called with a valid JSON scalar (e.g. 42)
Then it returns a typed error whose code is -32600 Invalid Request

Related spec scenarios: RS.JRP.33
*/
func TestJsonRpcProtocol_ParseBody_ScalarBody_Code(t *testing.T) {
	t.Parallel()

	proto := newTestProto("method")
	body := []byte(`42`)

	_, err := proto.ParseBody(body)
	require.Error(t, err)
	assert.Equal(t, -32600, rpcErrorCode(err))
}

/*
Scenario: JsonRpcProtocol.ParseBody handles batch with a malformed element
Given a JsonRpcProtocol
When ParseBody is called with a batch containing one malformed element
Then the valid calls are returned and the malformed element yields an
RpcParsedError with code -32600 instead of failing the whole batch

Related spec scenarios: RS.JRP.33
*/
func TestJsonRpcProtocol_ParseBody_BatchMalformedElement(t *testing.T) {
	t.Parallel()

	proto := newTestProto("method")
	body := []byte(`[{"jsonrpc":"2.0","method":"add","id":1},"not-an-object",{"jsonrpc":"1.0","method":"sub","id":2}]`)

	entries, err := proto.ParseBody(body)
	require.NoError(t, err, "one malformed batch element must not fail the whole batch")
	require.Len(t, entries, 3, "batch order is preserved: 1 valid call + 2 errors")
	require.NotNil(t, entries[0].Call, "first element is a valid call")
	assert.Equal(t, "add", entries[0].Call.Procedure)
	require.NotNil(t, entries[1].Error, "second element is a parse error")
	assert.Equal(t, -32600, entries[1].Error.Code)
	require.NotNil(t, entries[2].Error, "third element is a parse error")
	assert.Equal(t, -32600, entries[2].Error.Code)
}

/*
Scenario: JsonRpcProtocol.ParseBody handles error on missing jsonrpc field
Given a JsonRpcProtocol
When ParseBody is called with a valid JSON object but missing jsonrpc
Then it returns an error

Related spec scenarios: RS.JRP.13
*/
func TestJsonRpcProtocol_ParseBody_MissingJsonrpc(t *testing.T) {
	t.Parallel()

	proto := newTestProto("method")
	body := []byte(`{"method":"sub","id":1}`)

	_, err := proto.ParseBody(body)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "jsonrpc")
}

/*
Scenario: JsonRpcProtocol.ParseBody returns error on missing method
Given a JsonRpcProtocol
When ParseBody is called with jsonrpc: "2.0" but no method field
Then it returns an error

Related spec scenarios: RS.JRP.14
*/
func TestJsonRpcProtocol_ParseBody_MissingMethod(t *testing.T) {
	t.Parallel()

	proto := newTestProto("method")
	body := []byte(`{"jsonrpc":"2.0","id":1}`)

	_, err := proto.ParseBody(body)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "method")
}

/*
Scenario: JsonRpcProtocol.ParseBody returns error on wrong jsonrpc version
Given a JsonRpcProtocol
When ParseBody is called with jsonrpc: "1.0"
Then it returns an error

Related spec scenarios: RS.JRP.15
*/
func TestJsonRpcProtocol_ParseBody_WrongVersion(t *testing.T) {
	t.Parallel()

	proto := newTestProto("method")
	body := []byte(`{"jsonrpc":"1.0","method":"sub","id":1}`)

	_, err := proto.ParseBody(body)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}

/*
Scenario: JsonRpcProtocol.ParseBody handles empty batch array
Given a JsonRpcProtocol
When ParseBody is called with an empty JSON array
Then it returns an empty slice with no error

Related spec scenarios: RS.JRP.11
*/
func TestJsonRpcProtocol_ParseBody_EmptyBatch(t *testing.T) {
	t.Parallel()

	proto := newTestProto("method")
	body := []byte(`[]`)

	_, err := proto.ParseBody(body)
	require.Error(t, err, "an empty batch is an Invalid Request per JSON-RPC 2.0")
	assert.Equal(t, -32600, rpcErrorCode(err))
}

/*
Scenario: JsonRpcProtocol.ParseBody uses configurable procedure.call path
Given a JsonRpcProtocol with callPath set to a custom field
When ParseBody is called with that field in the body
Then the procedure name is extracted from the custom field

Related spec scenarios: RS.JRP.16
*/
func TestJsonRpcProtocol_ParseBody_CustomCallPath(t *testing.T) {
	t.Parallel()

	proto := newTestProto("custom.proc")
	body := []byte(`{"jsonrpc":"2.0","method":"ignore","custom":{"proc":"subtract"},"id":1}`)

	entries, err := proto.ParseBody(body)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.NotNil(t, entries[0].Call)
	assert.Equal(t, "subtract", entries[0].Call.Procedure)
}

/*
Scenario: JsonRpcProtocol.ErrorResponse format for various error codes and ids
Given a JsonRpcProtocol
When ErrorResponse is called with error code -32700, -32600, -32601 and various ids
Then it returns correctly formatted JSON-RPC error objects

Related spec scenarios: RS.JRP.12, RS.JRP.13, RS.JRP.18
*/
func TestJsonRpcProtocol_ErrorResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		code    int
		message string
		id      any
	}{
		{
			name:    "parse error -32700 with null id",
			code:    -32700,
			message: "Parse error",
			id:      nil,
		},
		{
			name:    "invalid request -32600 with string id",
			code:    -32600,
			message: "Invalid Request",
			id:      "req-1",
		},
		{
			name:    "method not found -32601 with number id",
			code:    -32601,
			message: "Method not found",
			id:      float64(42),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			proto := newTestProto("method")
			data := proto.ErrorResponse(tt.code, tt.message, tt.id)

			var resp map[string]interface{}
			err := json.Unmarshal(data, &resp)
			require.NoError(t, err)

			assert.Equal(t, "2.0", resp["jsonrpc"])

			errObj, ok := resp["error"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, float64(tt.code), errObj["code"])
			assert.Equal(t, tt.message, errObj["message"])

			if tt.id == nil {
				assert.Nil(t, resp["id"])
			} else {
				assert.Equal(t, tt.id, resp["id"])
			}
		})
	}
}

/*
Scenario: JsonRpcProtocol.ContentType returns configured or default content type
Given a JsonRpcProtocol with a configured content type
When ContentType is called
Then it returns the configured value

Related spec scenarios: RS.JRP.1
*/
func TestJsonRpcProtocol_ContentType(t *testing.T) {
	t.Parallel()

	t.Run("default", func(t *testing.T) {
		t.Parallel()
		proto := &JsonRpcProtocol{contentType: "application/json"}
		assert.Equal(t, "application/json", proto.ContentType())
	})

	t.Run("configured", func(t *testing.T) {
		t.Parallel()
		proto := &JsonRpcProtocol{contentType: "application/json-rpc"}
		assert.Equal(t, "application/json-rpc", proto.ContentType())
	})
}

// Ensure JsonRpcProtocol implements RpcProtocol
var _ RpcProtocol = (*JsonRpcProtocol)(nil)

// Ensure loader.RpcConfig integration
/*
Scenario: Building a JSON-RPC protocol from a loader RpcConfig
Given a loader.RpcConfig with a content type and procedure call path
When NewJsonRpcProtocol is called
Then the protocol exposes the configured content type and extracts the
configured procedure path

Related spec scenarios: RS.JRP.1
*/
func TestNewJsonRpcProtocol_FromConfig(t *testing.T) {
	cfg := &loader.RpcConfig{
		ContentType: "application/json",
		Procedure: loader.ProcedureConfig{
			Call: "method",
		},
	}
	proto := NewJsonRpcProtocol(cfg)
	assert.Equal(t, "application/json", proto.ContentType())
	assert.Equal(t, "method", proto.callPath)
}
