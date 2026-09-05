package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/mamonth/oasmock/internal/extensions"
	"github.com/mamonth/oasmock/internal/runtime"
)

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       []byte
}

// writeJSONError writes a JSON error response with the given status code and message.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error": %q}`, message)
}

// writeJSONErrorf writes a formatted JSON error response.
func writeJSONErrorf(w http.ResponseWriter, status int, format string, args ...any) {
	writeJSONError(w, status, fmt.Sprintf(format, args...))
}

// writeJSON writes a JSON-encoded success/response body with the given status
// code. It is the single success-write point for the management handlers so
// they no longer duplicate the Content-Type + Encoder block per endpoint.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Debug("Failed to encode JSON response", "err", err)
	}
}

// decodeJSONBody reads a size-limited request body and unmarshals it into v.
// It returns an error on read failure, an empty body, or malformed JSON.
func decodeJSONBody(r *http.Request, v any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return fmt.Errorf("empty request body")
	}
	return json.Unmarshal(body, v)
}

// routeKey generates a unique key for a route mapping.
func routeKey(method, pattern string) string {
	return method + " " + pattern
}

// WriteHeader captures the status code before writing.
func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

// Write captures the written body.
func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return r.ResponseWriter.Write(b)
}

// Hijack preserves the underlying connection so WebSocket upgrades work even
// when request history recording is active.
func (r *responseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := r.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
}

// Flush forwards flush calls to the underlying writer when supported.
func (r *responseRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func buildRequestSource(r *http.Request, pathParams map[string]string, callBody any) *runtime.RequestSource {
	// Parse query parameters
	query := r.URL.Query()
	queryMap := make(map[string][]string, len(query))
	for k, v := range query {
		queryMap[k] = v
	}
	// Parse headers (lowercase keys)
	headers := make(map[string][]string, len(r.Header))
	for k, v := range r.Header {
		headers[strings.ToLower(k)] = v
	}
	// Parse cookies
	cookies := make(map[string]string)
	for _, c := range r.Cookies() {
		cookies[c.Name] = c.Value
	}
	// Parse body (JSON only for now) - note: body already read by
	// requestHistoryMiddleware; RPC bodies arrive pre-decoded in callBody.
	body := callBody
	if callBody == nil && r.Body != nil {
		if bodyBytes, err := io.ReadAll(r.Body); err == nil && len(bodyBytes) > 0 {
			var parsed any
			if err := json.Unmarshal(bodyBytes, &parsed); err == nil {
				body = parsed
			}
			// Restore body for downstream handlers
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}
	return &runtime.RequestSource{
		PathParams:  pathParams,
		QueryParams: queryMap,
		Headers:     headers,
		Body:        body,
		Cookies:     cookies,
	}
}

func (s *Server) newStateSource(prefix string) *runtime.StateSource {
	return s.engine.NewStateSource(prefix)
}

func (s *Server) newEnvSource() *runtime.EnvSource {
	return s.engine.NewEnvSource()
}

func (s *Server) makeMockHandler(mapping *RouteMapping) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.config.Verbose {
			slog.Debug("makeMockHandler invoked", "method", r.Method, "path", r.URL.Path, "pattern", mapping.Pattern, "chiPattern", mapping.ChiPattern)
		}
		s.handleMockRequestWithMapping(w, r, mapping)
	}
}

func (s *Server) handleMockRequestWithMapping(w http.ResponseWriter, r *http.Request, mapping *RouteMapping) {
	if s.config.Verbose {
		slog.Debug("handleMockRequestWithMapping called", "method", r.Method, "path", r.URL.Path, "mappingPattern", mapping.Pattern)
	}
	pathParams := s.extractPathParams(r, mapping)

	body, headers, statusCodeStr, mediaType, err := s.selectAndGenerateResponse(r, mapping, pathParams, nil)
	if err != nil {
		if err == errNoResponse {
			writeJSONError(w, http.StatusInternalServerError, "No response defined for operation")
			return
		}
		if err == errNotImplemented {
			writeJSONError(w, http.StatusNotImplemented, "No example available")
			return
		}
		writeJSONErrorf(w, http.StatusInternalServerError, err.Error())
		return
	}

	for k, v := range headers {
		w.Header().Set(k, v)
	}
	w.Header().Set("Content-Type", mediaType)
	w.WriteHeader(parseStatusCode(statusCodeStr))
	if _, writeErr := w.Write(body); writeErr != nil && s.config.Verbose {
		slog.Debug("Failed to write response body", "err", writeErr)
	}
}

var (
	errNoResponse     = fmt.Errorf("no response")
	errNotImplemented = fmt.Errorf("not implemented")
)

func (s *Server) selectAndGenerateResponse(r *http.Request, mapping *RouteMapping, pathParams map[string]string, callBody any) (body []byte, headers map[string]string, statusCode string, mediaType string, err error) {
	evaluator := runtime.NewEvaluator()
	evaluator.AddSource(runtime.SourceRequest, buildRequestSource(r, pathParams, callBody))
	evaluator.AddSource(runtime.SourceState, s.newStateSource(mapping.Prefix))
	evaluator.AddSource(runtime.SourceEnv, s.newEnvSource())

	statusCode, response := s.selectResponse(mapping, evaluator)
	if response == nil {
		return nil, nil, "", "", errNoResponse
	}

	var mediaTypeObj *openapi3.MediaType
	mediaType = "application/json"
	if len(response.Content) > 0 {
		var mtErr error
		mediaType, mediaTypeObj, mtErr = s.selectMediaType(response)
		if mtErr != nil {
			return nil, nil, "", "", mtErr
		}
	}

	opID := mapping.Prefix + ":" + mapping.Method + ":" + mapping.Pattern

	var example *openapi3.Example
	var dynExample *dynamicExample
	dynExample, _ = s.selectDynamicExample(mapping, evaluator)
	if dynExample == nil {
		if mediaTypeObj == nil {
			return nil, nil, "", "", errNotImplemented
		}
		example, _ = s.selectExample(mediaTypeObj, evaluator, opID)
		if example == nil {
			return nil, nil, "", "", errNotImplemented
		}
	}

	if example != nil {
		s.applyExtensions(example, evaluator, mapping.Prefix)
	}

	body, headers, statusCode, genErr := s.generateResponse(example, dynExample, evaluator, statusCode)
	if genErr != nil {
		return nil, nil, "", "", genErr
	}

	// Fire x-event-trigger events after the response is produced (RS.EVT.1-4).
	if example != nil {
		s.fireExampleTriggers(example, mapping.Prefix)
	}

	return body, headers, statusCode, mediaType, nil
}

// fireExampleTriggers dispatches the x-event-trigger events declared on an
// OpenAPI response example against the schema's event broker (design D8).
func (s *Server) fireExampleTriggers(example *openapi3.Example, prefix string) {
	if s.eventBus == nil || example == nil {
		return
	}
	triggers, ok := extensions.ExtractEventTriggers(example)
	if !ok {
		return
	}
	for _, trigger := range triggers {
		delay := triggerDelay(trigger.Delay)
		s.eventBus.fire(trigger.Name, trigger.Payload, prefix, trigger.Global, delay)
	}
}

// triggerDelay maps a trigger delay (ms) to a schedule.
func triggerDelay(ms int) *delaySchedule {
	if ms <= 0 {
		return nil
	}
	return &delaySchedule{ms: ms}
}

func parseStatusCode(codeStr string) int {
	if codeStr == "default" {
		return DefaultStatusCode
	}
	code, err := strconv.Atoi(codeStr)
	if err != nil {
		return DefaultStatusCode
	}
	return code
}

// requestHistoryMiddleware records incoming requests and responses.
func (s *Server) requestHistoryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Capture request body (up to 1MB)
		var requestBody []byte
		if r.Body != nil {
			body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize)) // 1MB
			if err == nil {
				requestBody = body
				// Restore body for downstream handlers
				r.Body = io.NopCloser(bytes.NewReader(body))
			} else if s.config.Verbose {
				slog.Debug("Failed to read request body", "err", err)
			}
		}

		// Create response recorder
		recorder := &responseRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // default if not set
		}

		start := time.Now()
		next.ServeHTTP(recorder, r)
		duration := time.Since(start)

		// Build request record
		record := RequestRecord{
			ID:        fmt.Sprintf("%d", start.UnixNano()),
			Timestamp: start,
			Method:    r.Method,
			Path:      r.URL.Path,
			Query:     r.URL.RawQuery,
			Headers:   r.Header.Clone(),
			Body:      requestBody,
			Response: &ResponseRecord{
				StatusCode: recorder.statusCode,
				Headers:    recorder.Header().Clone(),
				Body:       recorder.body,
				Duration:   duration,
			},
		}

		s.historyStore.Add(record)
	})
}

// verboseLoggingMiddleware logs request/response details when verbose is enabled.
func (s *Server) verboseLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.config.Verbose {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		slog.Info("Request started", "time", start.Format(time.RFC3339), "method", r.Method, "path", r.URL.Path)
		if ctx := chi.RouteContext(r.Context()); ctx != nil {
			slog.Debug("Route matched", "pattern", ctx.RoutePattern(), "keys", ctx.URLParams.Keys, "values", ctx.URLParams.Values)
		} else {
			slog.Debug("No route matched")
		}
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		slog.Info("Request completed", "time", start.Format(time.RFC3339), "duration", duration)
	})
}

// extractPathParams extracts path parameters from the request using chi URL
// params, falling back to pattern-matching the request path against the
// mapping's brace-form ChiPattern when chi has not populated them (the RPC
// gateway dispatch and direct handler invocations).
func (s *Server) extractPathParams(r *http.Request, mapping *RouteMapping) map[string]string {
	params := make(map[string]string)
	// Get chi route context
	ctx := chi.RouteContext(r.Context())
	if s.config.Verbose {
		slog.Debug("extractPathParams", "ctxNil", ctx == nil, "method", r.Method, "path", r.URL.Path, "chiPattern", mapping.ChiPattern)
	}
	if ctx != nil {
		// URLParams are stored in ctx.URLParams.Keys and Values
		for i, key := range ctx.URLParams.Keys {
			if i < len(ctx.URLParams.Values) {
				params[key] = ctx.URLParams.Values[i]
			}
		}
		if len(params) > 0 {
			return params
		}
	}
	if mapping == nil || r == nil || r.URL == nil {
		return params
	}
	matchPathParams(params, r.URL.Path, mapping.ChiPattern)
	return params
}

// matchPathParams extracts {param} captures by matching each segment of the
// actual request path against the corresponding segment of the brace-form chi
// pattern. Only well-formed brace segments capture; literal and malformed
// segments must match exactly.
func matchPathParams(params map[string]string, path, pattern string) {
	if pattern == "" || path == "" {
		return
	}
	patSegs := splitPathSegments(pattern)
	pathSegs := splitPathSegments(path)
	if len(patSegs) != len(pathSegs) {
		return
	}
	for i, seg := range patSegs {
		if len(seg) > 2 && seg[0] == '{' && seg[len(seg)-1] == '}' {
			name := seg[1 : len(seg)-1]
			if name != "" && !strings.ContainsAny(name, " \t") {
				params[name] = pathSegs[i]
			}
		}
	}
}

// splitPathSegments splits an absolute path on "/" preserving empty segments
// for exact length comparison.
func splitPathSegments(p string) []string {
	return strings.Split(p, "/")
}
