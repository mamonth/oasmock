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

	entries, err := h.protocol.ParseBody(bodyBytes)
	if err != nil {
		errBody := h.protocol.ErrorResponse(rpcErrorCode(err), rpcErrorMessage(codeMessage(rpcErrorCode(err))), nil)
		w.Header().Set("Content-Type", h.protocol.ContentType())
		w.WriteHeader(http.StatusOK)
		writeBody(w, errBody)
		return
	}

	pathParamsCache := make(map[string]map[string]string)
	results := make([]json.RawMessage, 0, len(entries))
	var singleStatusCode string
	var singleHeaders map[string]string
	for _, entry := range entries {
		if entry.Error != nil {
			results = append(results, json.RawMessage(h.protocol.ErrorResponse(entry.Error.Code, codeMessage(entry.Error.Code), entry.Error.ID)))
			continue
		}
		if sc, headers := h.handleCall(entry.Call, r, pathParamsCache, &results); sc != "" {
			singleStatusCode = sc
			singleHeaders = headers
		}
	}

	if isBatch {
		// Batch: every slot is either a result or an error; notifications and
		// error slots with no id produce no response entry. An all-notification
		// (or all-error-without-id) batch answers 204 No Content, matching the
		// single-notification behavior.
		if len(results) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		out, _ := json.Marshal(results)
		w.Header().Set("Content-Type", h.protocol.ContentType())
		w.WriteHeader(http.StatusOK)
		writeBody(w, out)
		return
	}

	// Single call
	if len(entries) == 1 && entries[0].Call != nil && !entries[0].Call.HasID {
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

// codeMessage maps a JSON-RPC error code to its standard message.
func codeMessage(code int) string {
	switch code {
	case -32700:
		return "Parse error"
	case -32600:
		return "Invalid Request"
	case -32601:
		return "Method not found"
	case -32603:
		return "Internal error"
	default:
		return "Server error"
	}
}

// rpcErrorMessage is a compatibility alias so fatal parse-path callers read
// clearly; it simply returns the given message.
func rpcErrorMessage(msg string) string { return msg }

func isBatchRequest(body []byte) bool {
	s := strings.TrimSpace(string(body))
	return len(s) > 0 && s[0] == '['
}

// handleCall resolves the mapping for one JSON-RPC call and executes its mock
// pipeline, appending a response entry (a result body or a protocol error) to
// results for calls with an id. Notifications run without a response entry.
// It returns the status code and response headers of a successful call ("" and
// nil for notifications and errors), which the single-call path uses for the
// standard HTTP response.
func (h *RpcHandler) handleCall(call *RpcCall, r *http.Request, pathParamsCache map[string]map[string]string, results *[]json.RawMessage) (string, map[string]string) {
	if call == nil {
		return "", nil
	}
	mapping, ok := h.procedureMap[call.Procedure]
	if !ok {
		if call.HasID {
			*results = append(*results, json.RawMessage(h.protocol.ErrorResponse(-32601, "Method not found", call.ID)))
		}
		return "", nil
	}

	// Path parameters are extracted per procedure from the request against the
	// procedure's own brace-form ChiPattern (the gateway route itself has no
	// params); cached per pattern so a batch reuses one extraction.
	pathParams, ok := pathParamsCache[call.Procedure]
	if !ok {
		pathParams = h.server.extractPathParams(r, mapping)
		pathParamsCache[call.Procedure] = pathParams
	}

	if !call.HasID {
		_, _, _, _, err := h.server.selectAndGenerateResponse(r, mapping, pathParams, call.Raw)
		if err != nil {
			slog.Debug("RPC notification pipeline error", "procedure", call.Procedure, "err", err)
		}
		return "", nil
	}

	body, headers, statusCode, _, err := h.server.selectAndGenerateResponse(r, mapping, pathParams, call.Raw)
	if err != nil {
		*results = append(*results, json.RawMessage(h.protocol.ErrorResponse(-32603, "Internal error", call.ID)))
		return "", nil
	}
	*results = append(*results, json.RawMessage(body))
	return statusCode, headers
}

func newRpcProtocol(cfg *loader.RpcConfig) (RpcProtocol, error) {
	switch cfg.ProtocolType {
	case loader.ProtocolTypeJsonRpc:
		return NewJsonRpcProtocol(cfg), nil
	default:
		return nil, strconv.ErrSyntax
	}
}
