package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/mamonth/oasmock/internal/loader"
)

type RpcHandler struct {
	protocol     RpcProtocol
	procedureMap map[string]*RouteMapping
	server       *Server
}

func NewRpcHandler(protocol RpcProtocol, procedureMap map[string]*RouteMapping, server *Server) *RpcHandler {
	return &RpcHandler{
		protocol:     protocol,
		procedureMap: procedureMap,
		server:       server,
	}
}

// writeBody writes the response body, logging a debug message on failure.
func writeBody(w http.ResponseWriter, body []byte) {
	if _, err := w.Write(body); err != nil {
		slog.Debug("Failed to write RPC response body", "err", err)
	}
}

func (h *RpcHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize))
	if err != nil {
		errBody := h.protocol.ErrorResponse(-32700, "Parse error", nil)
		w.Header().Set("Content-Type", h.protocol.ContentType())
		w.WriteHeader(http.StatusOK)
		writeBody(w, errBody)
		return
	}

	isBatch := isBatchRequest(bodyBytes)

	calls, err := h.protocol.ParseBody(bodyBytes)
	if err != nil {
		errBody := h.protocol.ErrorResponse(-32700, "Parse error", nil)
		w.Header().Set("Content-Type", h.protocol.ContentType())
		w.WriteHeader(http.StatusOK)
		writeBody(w, errBody)
		return
	}

	pathParams := h.server.extractPathParams(r, &RouteMapping{ChiPattern: r.URL.Path})

	results := make([]json.RawMessage, 0, len(calls))
	var singleStatusCode string
	var singleHeaders map[string]string
	for _, call := range calls {
		mapping, ok := h.procedureMap[call.Procedure]
		if !ok {
			if call.HasID {
				errBody := h.protocol.ErrorResponse(-32601, "Method not found", call.ID)
				results = append(results, json.RawMessage(errBody))
			}
			continue
		}

		if !call.HasID {
			_, _, _, _, err := h.server.selectAndGenerateResponse(r, mapping, pathParams, call.Raw)
			if err != nil {
				slog.Debug("RPC notification pipeline error", "procedure", call.Procedure, "err", err)
			}
			continue
		}

		body, headers, statusCode, _, err := h.server.selectAndGenerateResponse(r, mapping, pathParams, call.Raw)
		if err != nil {
			errBody := h.protocol.ErrorResponse(-32603, "Internal error", call.ID)
			results = append(results, json.RawMessage(errBody))
			continue
		}

		singleStatusCode = statusCode
		singleHeaders = headers
		results = append(results, json.RawMessage(body))
	}

	if isBatch {
		// Batch: write array response
		if len(results) == 0 {
			w.Header().Set("Content-Type", h.protocol.ContentType())
			w.WriteHeader(http.StatusOK)
			writeBody(w, []byte("[]"))
			return
		}
		out, _ := json.Marshal(results)
		w.Header().Set("Content-Type", h.protocol.ContentType())
		w.WriteHeader(http.StatusOK)
		writeBody(w, out)
		return
	}

	// Single call
	if len(calls) == 1 && !calls[0].HasID {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if len(results) == 0 {
		errBody := h.protocol.ErrorResponse(-32603, "Internal error", nil)
		w.Header().Set("Content-Type", h.protocol.ContentType())
		w.WriteHeader(http.StatusOK)
		writeBody(w, errBody)
		return
	}

	for k, v := range singleHeaders {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", h.protocol.ContentType())
	sc := parseStatusCode(singleStatusCode)
	if sc <= 0 {
		sc = http.StatusOK
	}
	w.WriteHeader(sc)
	writeBody(w, results[0])
}

func isBatchRequest(body []byte) bool {
	s := strings.TrimSpace(string(body))
	return len(s) > 0 && s[0] == '['
}

func newRpcProtocol(cfg *loader.RpcConfig) (RpcProtocol, error) {
	switch cfg.ProtocolType {
	case loader.ProtocolTypeJsonRpc:
		return NewJsonRpcProtocol(cfg), nil
	default:
		return nil, strconv.ErrSyntax
	}
}
