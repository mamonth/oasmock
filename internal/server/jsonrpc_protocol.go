package server

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mamonth/oasmock/internal/loader"
)

// rpcProtocolError is a fatal JSON-RPC body error carrying the response code
// to emit (-32700 parse error, -32600 invalid request).
type rpcProtocolError struct {
	code int
	msg  string
}

func (e *rpcProtocolError) Error() string { return e.msg }

// rpcErrorCode extracts the JSON-RPC code from an error returned by
// RpcProtocol.ParseBody. It returns -32700 for an error without a code.
func rpcErrorCode(err error) int {
	if pe, ok := err.(*rpcProtocolError); ok {
		return pe.code
	}
	return -32700
}

func rpcError(code int, format string, args ...any) error {
	return &rpcProtocolError{code: code, msg: fmt.Sprintf(format, args...)}
}

type JsonRpcProtocol struct {
	contentType string
	callPath    string
}

func NewJsonRpcProtocol(cfg *loader.RpcConfig) *JsonRpcProtocol {
	ct := cfg.ContentType
	if ct == "" {
		ct = "application/json"
	}
	return &JsonRpcProtocol{
		contentType: ct,
		callPath:    cfg.Procedure.Call,
	}
}

func (p *JsonRpcProtocol) ParseBody(body []byte) ([]RpcEntry, error) {
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, rpcError(-32700, "parse error")
	}

	switch v := raw.(type) {
	case []interface{}:
		return p.parseBatch(v)
	case map[string]interface{}:
		call, err := p.parseSingle(v)
		if err != nil {
			return nil, err
		}
		return []RpcEntry{{Call: &call}}, nil
	default:
		return nil, rpcError(-32600, "invalid request: body must be an object or array")
	}
}

func (p *JsonRpcProtocol) parseBatch(items []interface{}) ([]RpcEntry, error) {
	// An empty [[ ]] is an Invalid Request per JSON-RPC 2.0 spec 7.
	if len(items) == 0 {
		return nil, rpcError(-32600, "invalid request: empty batch")
	}
	entries := make([]RpcEntry, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			entries = append(entries, RpcEntry{Error: &RpcParsedError{Code: -32600}})
			continue
		}
		call, err := p.parseSingle(obj)
		if err != nil {
			id, _ := obj["id"]
			entries = append(entries, RpcEntry{Error: &RpcParsedError{Code: rpcErrorCode(err), ID: id}})
			continue
		}
		calls := call
		entries = append(entries, RpcEntry{Call: &calls})
	}
	return entries, nil
}

func (p *JsonRpcProtocol) parseSingle(obj map[string]interface{}) (RpcCall, error) {
	version, ok := obj["jsonrpc"].(string)
	if !ok {
		return RpcCall{}, rpcError(-32600, "invalid request: missing or invalid jsonrpc field")
	}
	if version != "2.0" {
		return RpcCall{}, rpcError(-32600, "invalid request: unsupported jsonrpc version %q", version)
	}

	method, ok := obj["method"].(string)
	if !ok || method == "" {
		return RpcCall{}, rpcError(-32600, "invalid request: missing or invalid method field")
	}

	procedureName, err := p.extractProcedureName(obj)
	if err != nil {
		return RpcCall{}, err
	}

	id, hasID := obj["id"]
	if !hasID {
		return RpcCall{
			Procedure: procedureName,
			Raw:       obj,
			ID:        nil,
			HasID:     false,
		}, nil
	}

	if id == nil {
		return RpcCall{
			Procedure: procedureName,
			Raw:       obj,
			ID:        nil,
			HasID:     false,
		}, nil
	}

	return RpcCall{
		Procedure: procedureName,
		Raw:       obj,
		ID:        id,
		HasID:     true,
	}, nil
}

func (p *JsonRpcProtocol) extractProcedureName(obj map[string]interface{}) (string, error) {
	if p.callPath == "" {
		method, _ := obj["method"].(string)
		return method, nil
	}

	parts := strings.Split(p.callPath, ".")
	current := any(obj)
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("cannot traverse path %q", p.callPath)
		}
		val, exists := m[part]
		if !exists {
			return "", fmt.Errorf("path %q not found in request", p.callPath)
		}
		current = val
	}

	if s, ok := current.(string); ok {
		return s, nil
	}
	return "", fmt.Errorf("value at path %q is not a string", p.callPath)
}

func (p *JsonRpcProtocol) ErrorResponse(code int, message string, id any) []byte {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	if id != nil {
		resp["id"] = id
	} else {
		resp["id"] = nil
	}
	data, _ := json.Marshal(resp)
	return data
}

func (p *JsonRpcProtocol) ContentType() string {
	return p.contentType
}
