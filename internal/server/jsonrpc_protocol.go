package server

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mamonth/oasmock/internal/loader"
)

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

func (p *JsonRpcProtocol) ParseBody(body []byte) ([]RpcCall, error) {
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	switch v := raw.(type) {
	case []interface{}:
		return p.parseBatch(v)
	case map[string]interface{}:
		call, err := p.parseSingle(v)
		if err != nil {
			return nil, err
		}
		return []RpcCall{call}, nil
	default:
		return nil, fmt.Errorf("invalid request: body must be object or array")
	}
}

func (p *JsonRpcProtocol) parseBatch(items []interface{}) ([]RpcCall, error) {
	calls := make([]RpcCall, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid request: batch element must be an object")
		}
		call, err := p.parseSingle(obj)
		if err != nil {
			return nil, err
		}
		calls = append(calls, call)
	}
	return calls, nil
}

func (p *JsonRpcProtocol) parseSingle(obj map[string]interface{}) (RpcCall, error) {
	version, ok := obj["jsonrpc"].(string)
	if !ok {
		return RpcCall{}, fmt.Errorf("invalid request: missing or invalid jsonrpc field")
	}
	if version != "2.0" {
		return RpcCall{}, fmt.Errorf("invalid request: unsupported jsonrpc version %q", version)
	}

	method, ok := obj["method"].(string)
	if !ok || method == "" {
		return RpcCall{}, fmt.Errorf("invalid request: missing or invalid method field")
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
