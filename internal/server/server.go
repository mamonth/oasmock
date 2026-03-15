package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/mamonth/oasmock/internal/history"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/runtime"
	"github.com/mamonth/oasmock/internal/state"
)

const (
	maxRequestBodySize = 1 << 20 // 1MB
	DefaultHistorySize = 1000
	DefaultStatusCode  = 200
	DefaultMethod      = "GET"
)

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

// routeKey generates a unique key for a route mapping.
func routeKey(method, pattern string) string {
	return method + " " + pattern
}

// responseRecorder wraps http.ResponseWriter to capture status code and body.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       []byte
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

// Config holds server configuration.
type Config struct {
	Port             int
	Delay            time.Duration
	Verbose          bool
	EnableCORS       bool
	HistorySize      int
	EnableControlAPI bool
}

// Server represents the mock HTTP server.
type Server struct {
	config       Config
	router       *chi.Mux
	httpServer   *http.Server
	mappings     []RouteMapping
	stateStore   StateStore
	historyStore HistoryStore
	// mapping from method+chiPattern to RouteMapping for quick lookup
	routeMap map[string]*RouteMapping
	// track once examples that have been used
	onceExamples map[string]bool
	onceMu       sync.RWMutex
	// dynamic examples added via management API
	dynamicExamples map[string][]dynamicExample
	dyMu            sync.RWMutex
	// dependencies
	deps Dependencies
}

// New creates a new mock server with the given configuration and loaded schemas.
func New(config Config, schemas []loader.SchemaInfo) (*Server, error) {
	// Convert loader.SchemaInfo to server.SchemaInfo
	serverSchemas := make([]SchemaInfo, len(schemas))
	for i, schema := range schemas {
		serverSchemas[i] = SchemaInfo{
			Spec:   schema.Spec,
			Prefix: schema.Prefix,
		}
	}

	// Create default dependencies using wrappers
	routeProvider := &loaderRouteProvider{}
	stateStore := newStateManagerStore(state.NewManager())
	historySize := config.HistorySize
	if historySize <= 0 {
		historySize = DefaultHistorySize
	}
	config.HistorySize = historySize
	historyStore := newHistoryRingBufferStore(history.NewRingBuffer(historySize))

	deps := Dependencies{
		RouteProvider:        routeProvider,
		StateStore:           stateStore,
		HistoryStore:         historyStore,
		RequestSourceFactory: &runtimeRequestSourceFactory{},
		StateSourceFactory:   newRuntimeStateSourceFactory(stateStore),
		EnvSourceFactory:     &runtimeEnvSourceFactory{},
		ExpressionEvaluator:  newRuntimeExpressionEvaluatorWrapper(runtime.NewEvaluator()),
		ExtensionProcessor:   &extensionsProcessorWrapper{},
	}

	return NewWithDependencies(config, serverSchemas, deps)
}

// NewWithDependencies creates a new mock server with explicit dependencies.
func NewWithDependencies(config Config, schemas []SchemaInfo, deps Dependencies) (*Server, error) {
	// Build route mappings
	mappings, err := deps.RouteProvider.BuildRouteMappings(schemas)
	if err != nil {
		return nil, fmt.Errorf("failed to build route mappings: %w", err)
	}

	// Ensure history size has a sensible default
	if config.HistorySize <= 0 {
		config.HistorySize = DefaultHistorySize
	}

	s := &Server{
		config:          config,
		mappings:        mappings,
		stateStore:      deps.StateStore,
		historyStore:    deps.HistoryStore,
		deps:            deps,
		routeMap:        make(map[string]*RouteMapping),
		onceExamples:    make(map[string]bool),
		dynamicExamples: make(map[string][]dynamicExample),
	}
	s.setupRouter()
	return s, nil
}

func (s *Server) setupRouter() {
	r := chi.NewRouter()

	// Basic middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Request delay middleware
	if s.config.Delay > 0 {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(s.config.Delay)
				next.ServeHTTP(w, r)
			})
		})
	}

	// CORS middleware
	if s.config.EnableCORS {
		corsMiddleware := cors.New(cors.Options{
			AllowedOrigins:   []string{"*"},
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
			AllowedHeaders:   []string{"*"},
			AllowCredentials: false,
			MaxAge:           300,
		})
		r.Use(corsMiddleware.Handler)
	}

	// Request history recording middleware
	r.Use(s.requestHistoryMiddleware)

	// Verbose logging middleware
	r.Use(s.verboseLoggingMiddleware)

	// Register mock routes
	s.registerMockRoutes(r)

	// Register management API routes
	if s.config.EnableControlAPI {
		s.registerManagementRoutes(r)
	} else {
		slog.Debug("Management control API disabled")
	}

	s.router = r
}

func (s *Server) registerMockRoutes(r chi.Router) {
	slog.Info("registerMockRoutes called", "verbose", s.config.Verbose, "numMappings", len(s.mappings))
	for i := range s.mappings {
		mapping := &s.mappings[i]
		key := routeKey(mapping.Method, mapping.ChiPattern)
		s.routeMap[key] = mapping

		// Register route with chi using Method function
		// chi.Method registers the route for the specified HTTP method
		if s.config.Verbose {
			slog.Info("XXXRegistering route", "method", mapping.Method, "chiPattern", mapping.ChiPattern, "fullPath", mapping.Path, "prefix", mapping.Prefix, "pattern", mapping.Pattern, "responses", mapping.Responses != nil)
		}
		r.Method(mapping.Method, mapping.ChiPattern, s.makeMockHandler(mapping))

		if s.config.Verbose {
			slog.Debug("Registered route", "method", mapping.Method, "pattern", mapping.ChiPattern)
		}
	}

}

func (s *Server) registerManagementRoutes(r chi.Router) {
	r.Post("/_mock/examples", s.handleAddExample)
	r.Get("/_mock/requests", s.handleGetRequests)
}

func (s *Server) newRequestSource(r *http.Request, pathParams map[string]string) *runtime.RequestSource {
	// Parse query parameters
	query := r.URL.Query()
	queryMap := make(map[string][]string)
	for k, v := range query {
		queryMap[k] = v
	}
	// Parse headers (lowercase keys)
	headers := make(map[string][]string)
	for k, v := range r.Header {
		headers[strings.ToLower(k)] = v
	}
	// Parse cookies
	cookies := make(map[string]string)
	for _, c := range r.Cookies() {
		cookies[c.Name] = c.Value
	}
	// Parse body (JSON only for now) - note: body already read by requestHistoryMiddleware
	var body any
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		if err == nil && len(bodyBytes) > 0 {
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
	data := s.stateStore.GetNamespace(prefix)
	if data == nil {
		data = make(map[string]any)
	}
	return &runtime.StateSource{Data: data}
}

func (s *Server) newEnvSource() *runtime.EnvSource {
	env := make(map[string]string)
	for _, e := range os.Environ() {
		if key, val, found := strings.Cut(e, "="); found {
			env[key] = val
		}
	}
	return &runtime.EnvSource{Env: env}
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
	// 1. Extract path parameters
	pathParams := s.extractPathParams(r, mapping)

	// 2. Build runtime data sources
	evaluator := runtime.NewEvaluator()
	evaluator.AddSource("request", s.newRequestSource(r, pathParams))
	evaluator.AddSource("state", s.newStateSource(mapping.Prefix))
	evaluator.AddSource("env", s.newEnvSource())

	// 3. Select response status code
	if s.config.Verbose {
		slog.Debug("handleMockRequestWithMapping: selecting response", "mappingResponses", mapping.Responses != nil)
	}
	statusCode, response := s.selectResponse(mapping, evaluator)
	if s.config.Verbose {
		slog.Debug("handleMockRequestWithMapping: selected response", "statusCode", statusCode, "response", response != nil)
	}
	if response == nil {
		writeJSONError(w, http.StatusInternalServerError, "No response defined for operation")
		return
	}

	// 4. Select media type (for now, pick first)
	var mediaType string
	var mediaTypeObj *openapi3.MediaType
	if response.Content != nil {
		if s.config.Verbose {
			slog.Debug("handleMockRequestWithMapping: selecting media type", "responseContent", true)
		}
		var mtErr error
		mediaType, mediaTypeObj, mtErr = s.selectMediaType(response)
		if s.config.Verbose {
			slog.Debug("handleMockRequestWithMapping: selected media type", "mediaType", mediaType, "mtErr", mtErr)
		}
		if mtErr != nil {
			writeJSONError(w, http.StatusNotImplemented, mtErr.Error())
			return
		}
	} else {
		// No content defined in schema; we'll use default media type if we have a dynamic example
		mediaType = "application/json" // default
		if s.config.Verbose {
			slog.Debug("handleMockRequestWithMapping: no content in response, using default media type", "mediaType", mediaType)
		}
	}

	// Generate operation ID for once-example tracking
	opID := mapping.Prefix + ":" + mapping.Method + ":" + mapping.Pattern
	if s.config.Verbose {
		slog.Debug("handleMockRequestWithMapping: selecting example",
			"method", mapping.Method,
			"pattern", mapping.Pattern,
			"chiPattern", mapping.ChiPattern,
			"key", routeKey(mapping.Method, mapping.ChiPattern))
	}
	// 5. Select example (dynamic first, then built‑in)
	var example *openapi3.Example
	var dynExample *dynamicExample
	var exampleKey string
	dynExample, exampleKey = s.selectDynamicExample(mapping, evaluator)
	if s.config.Verbose {
		slog.Debug("handleMockRequestWithMapping: after selectDynamicExample",
			"dynExample", dynExample != nil,
			"exampleKey", exampleKey)
	}
	if dynExample == nil {
		if mediaTypeObj == nil {
			// No content in schema and no dynamic example matched
			writeJSONError(w, http.StatusNotImplemented, "No example available")
			return
		}
		example, exampleKey = s.selectExample(mediaTypeObj, evaluator, opID)
		if s.config.Verbose {
			slog.Debug("handleMockRequestWithMapping: after selectExample",
				"example", example != nil,
				"exampleKey", exampleKey)
		}
		if example == nil {
			writeJSONError(w, http.StatusNotImplemented, "No example available")
			return
		}
	}
	// Log selected example if verbose
	if s.config.Verbose && exampleKey != "" {
		slog.Debug("Selected example", "example", exampleKey)
	}

	// 6. Apply extensions (x-mock-set-state, x-mock-headers, x-mock-once)
	if example != nil {
		s.applyExtensions(example, evaluator, mapping.Prefix)
	}

	// 7. Generate response body and headers
	body, headers, finalStatusCode, err := s.generateResponse(example, dynExample, evaluator, statusCode)
	if err != nil {
		writeJSONErrorf(w, http.StatusInternalServerError, err.Error())
		return
	}
	statusCode = finalStatusCode

	// 8. Set response headers
	for k, v := range headers {
		w.Header().Set(k, v)
	}

	// 9. Send response
	w.Header().Set("Content-Type", mediaType)
	w.WriteHeader(parseStatusCode(statusCode))
	if _, err := w.Write(body); err != nil && s.config.Verbose {
		slog.Debug("Failed to write response body", "err", err)
	}
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

// extractPathParams extracts path parameters from the request using chi URL params.
func (s *Server) extractPathParams(r *http.Request, mapping *RouteMapping) map[string]string {
	params := make(map[string]string)
	// Get chi route context
	ctx := chi.RouteContext(r.Context())
	if s.config.Verbose {
		slog.Debug("extractPathParams", "ctxNil", ctx == nil, "method", r.Method, "path", r.URL.Path, "chiPattern", mapping.ChiPattern)
	}
	if ctx == nil {
		return params
	}
	// URLParams are stored in ctx.URLParams.Keys and Values
	for i, key := range ctx.URLParams.Keys {
		if i < len(ctx.URLParams.Values) {
			params[key] = ctx.URLParams.Values[i]
		}
	}
	return params
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.config.Port)
	slog.Info("Starting mock server", "address", addr)
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}
