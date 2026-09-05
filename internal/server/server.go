package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/mamonth/oasmock/internal/extensions"
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
	config           Config
	router           *chi.Mux
	httpServer       *http.Server
	httpMu           sync.Mutex
	shutdownOnce     sync.Once
	shutdownResult   error
	mappings         []RouteMapping
	stateStore       StateStore
	historyStore     HistoryStore
	routeMap         map[string]*RouteMapping
	registry         *exampleRegistry
	engine           *exampleEngine
	deps             Dependencies
	rpcHandler       *RpcHandler
	rpcMappings      []*loader.RpcRouteMapping
	gatewayPath      string
	protocolAdapters map[string]ProtocolAdapter
	routerSetupErr   error
	hubMgr           *hubManager
	eventBus         *eventBus
	manageStream     *manageStream
	runtimeExamples  *runtimeExampleRegistry
}

// New creates a new mock server with the given configuration and loaded schemas.
func New(config Config, schemas []loader.SchemaInfo) (*Server, error) {
	serverSchemas := make([]SchemaInfo, len(schemas))
	rpcConfig := (*loader.RpcConfig)(nil)
	for i, schema := range schemas {
		serverSchemas[i] = SchemaInfo{
			Spec:   schema.Spec,
			Kind:   schema.Kind,
			Async:  schema.Async,
			Prefix: schema.Prefix,
		}

		if schema.Kind == loader.KindAsyncAPI {
			continue
		}
		if rpcConfig == nil {
			var err error
			rpcConfig, err = loader.ParseRpcConfig(schema.Spec)
			if err != nil {
				return nil, fmt.Errorf("failed to parse RPC config: %w", err)
			}
		}
	}

	var rpcMappings []*loader.RpcRouteMapping
	if rpcConfig != nil {
		var err error
		rpcMappings, err = loader.BuildRpcMappings(schemas, rpcConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build RPC mappings: %w", err)
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

	return NewWithDependencies(config, serverSchemas, deps, rpcConfig, rpcMappings)
}

// NewWithDependencies creates a new mock server with explicit dependencies.
func NewWithDependencies(config Config, schemas []SchemaInfo, deps Dependencies, rpcConfig *loader.RpcConfig, rpcMappings []*loader.RpcRouteMapping) (*Server, error) {
	// Build route mappings
	mappings, err := deps.RouteProvider.BuildRouteMappings(schemas)
	if err != nil {
		return nil, fmt.Errorf("failed to build route mappings: %w", err)
	}

	// Ensure history size has a sensible default
	if config.HistorySize <= 0 {
		config.HistorySize = DefaultHistorySize
	}

	registry := newExampleRegistry(config.Verbose)
	s := &Server{
		config:           config,
		mappings:         mappings,
		stateStore:       deps.StateStore,
		historyStore:     deps.HistoryStore,
		deps:             deps,
		routeMap:         make(map[string]*RouteMapping),
		registry:         registry,
		engine:           newExampleEngine(config, deps, registry),
		rpcMappings:      rpcMappings,
		protocolAdapters: defaultProtocolAdapters(),
	}
	s.hubMgr = newHubManager(s.engine, s.protocolAdapters[asyncWSProtocol].(*wsProtocolAdapter), schemas)
	s.manageStream = newManageStream(config.Verbose)
	s.runtimeExamples = newRuntimeExampleRegistry()
	s.eventBus = newEventBus(s.engine, s.hubMgr, config.Verbose)
	s.eventBus.setObserver(func(env manageEnvelope) {
		if s.manageStream != nil {
			s.manageStream.broadcast(env)
		}
	})
	if err := s.eventBus.registerEventSubscriptions(schemas); err != nil {
		return nil, err
	}
	s.wireBuiltInHooks()

	if rpcConfig != nil {
		proto, err := newRpcProtocol(rpcConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create RPC protocol: %w", err)
		}

		gwPath := rpcConfig.Gateway
		for _, schema := range schemas {
			gwPath = applyPrefixRpc(schema.Prefix, rpcConfig.Gateway)
			break
		}

		procMap := make(map[string]*RouteMapping)
		for _, m := range rpcMappings {
			rm := &RouteMapping{
				Method:     m.Method,
				Path:       m.Path,
				Pattern:    m.Pattern,
				Prefix:     m.Prefix,
				ChiPattern: m.ChiPattern,
				Operation:  m.Operation,
				Parameters: m.Parameters,
				Responses:  m.Responses,
			}
			procMap[m.Procedure] = rm
		}
		s.rpcHandler = NewRpcHandler(proto, procMap, s)
		s.gatewayPath = gwPath
	}

	s.setupRouter()

	if err := s.routerSetupErr; err != nil {
		return nil, err
	}

	s.registry.startSweep()
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

	// Register SignalR hubs (negotiate + upgrade)
	s.registerSignalRHubs(r)

	// Register RPC gateway route if configured
	if s.rpcHandler != nil {
		r.Post(s.gatewayPath, s.rpcHandler.ServeHTTP)
		slog.Info("Registered RPC gateway", "path", s.gatewayPath, "procedures", len(s.rpcHandler.procedureMap))
	}

	// Register management API routes
	if s.config.EnableControlAPI {
		s.registerManagementRoutes(r)
	} else {
		slog.Debug("Management control API disabled")
	}

	s.router = r
}

func (s *Server) registerMockRoutes(r chi.Router) {
	slog.Debug("registerMockRoutes called", "verbose", s.config.Verbose, "numMappings", len(s.mappings))

	rpcChiPatterns := make(map[string]bool)
	for _, m := range s.rpcMappings {
		rpcChiPatterns[m.ChiPattern] = true
	}

	for i := range s.mappings {
		mapping := &s.mappings[i]
		if rpcChiPatterns[mapping.ChiPattern] {
			continue
		}
		key := routeKey(mapping.Method, mapping.ChiPattern)
		s.routeMap[key] = mapping

		handler, err := s.buildRouteHandler(mapping)
		if err != nil {
			s.routerSetupErr = err
			slog.Error("Failed to register route", "pattern", mapping.ChiPattern, "err", err)
			return
		}

		if s.config.Verbose {
			slog.Info("XXXRegistering route", "method", mapping.Method, "chiPattern", mapping.ChiPattern, "fullPath", mapping.Path, "prefix", mapping.Prefix, "pattern", mapping.Pattern, "responses", mapping.Responses != nil)
		}
		r.Method(mapping.Method, mapping.ChiPattern, handler)

		if s.config.Verbose {
			slog.Debug("Registered route", "method", mapping.Method, "pattern", mapping.ChiPattern)
		}
	}
}

// buildRouteHandler dispatches AsyncAPI routes to their protocol adapter and
// falls back to the OpenAPI pipeline for regular routes (design D4).
func (s *Server) buildRouteHandler(mapping *RouteMapping) (http.HandlerFunc, error) {
	if mapping.Protocol == "" {
		return s.makeMockHandler(mapping), nil
	}
	adapter := s.adapterForProtocol(mapping.Protocol)
	if adapter == nil {
		return nil, fmt.Errorf("channel protocol %q is not supported (supported: http, ws)", mapping.Protocol)
	}
	if s.config.Verbose {
		slog.Debug("Using protocol adapter", "protocol", mapping.Protocol, "pattern", mapping.ChiPattern)
	}
	return adapter.Handler(mapping, s.asyncMessageHandler(mapping)), nil
}

func (s *Server) registerManagementRoutes(r chi.Router) {
	r.Post("/_mock/examples", s.handleAddExample)
	r.Delete("/_mock/examples/{exampleId}", s.handleDeleteExample)
	r.Get("/_mock/requests", s.handleGetRequests)

	// Canonical protocol-neutral async surface (design D1).
	r.Post("/_mock/events", s.handleEvents)
	r.Post("/_mock/async/push", s.handleAsyncPush)
	r.Get("/_mock/async/consumers", s.handleAsyncConsumers)
	r.Post("/_mock/async/disconnect", s.handleAsyncDisconnect)
	r.Get("/_mock/stream", s.handleManageStream)

	// Deprecated aliases kept for one release (design D1). The events/fire
	// alias serves the legacy type-less contract (handleFireEventLegacy); the
	// ws aliases share the canonical handlers.
	r.Post("/_mock/events/fire", s.handleFireEventLegacy)
	r.Post("/_mock/ws/push", s.handleAsyncPush)
	r.Get("/_mock/ws/consumers", s.handleAsyncConsumers)
	r.Post("/_mock/ws/disconnect", s.handleAsyncDisconnect)

	// Removed schedule surface answers 410 Gone pointing at /_mock/examples.
	r.Post("/_mock/ws/schedule", s.handleGoneSchedule)
	r.Delete("/_mock/ws/schedule/{pushId}", s.handleGoneSchedule)
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
		if err == errNoResponse || err == errNoExample {
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
	errNoExample      = fmt.Errorf("no example")
)

func (s *Server) selectAndGenerateResponse(r *http.Request, mapping *RouteMapping, pathParams map[string]string, callBody any) (body []byte, headers map[string]string, statusCode string, mediaType string, err error) {
	evaluator := runtime.NewEvaluator()
	if callBody != nil {
		evaluator.AddSource("request", s.newRpcRequestSource(r, pathParams, callBody))
	} else {
		evaluator.AddSource("request", s.newRequestSource(r, pathParams))
	}
	evaluator.AddSource("state", s.newStateSource(mapping.Prefix))
	evaluator.AddSource("env", s.newEnvSource())

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

func (s *Server) newRpcRequestSource(r *http.Request, pathParams map[string]string, callBody any) *runtime.RequestSource {
	query := r.URL.Query()
	queryMap := make(map[string][]string)
	for k, v := range query {
		queryMap[k] = v
	}
	headers := make(map[string][]string)
	for k, v := range r.Header {
		headers[strings.ToLower(k)] = v
	}
	cookies := make(map[string]string)
	for _, c := range r.Cookies() {
		cookies[c.Name] = c.Value
	}
	return &runtime.RequestSource{
		PathParams:  pathParams,
		QueryParams: queryMap,
		Headers:     headers,
		Body:        callBody,
		Cookies:     cookies,
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

// Start starts the HTTP server, returning once the server is serving.
func (s *Server) Start() error {
	ln, _, err := s.Listen()
	if err != nil {
		return err
	}
	return s.Serve(ln)
}

// Listen binds the configured port (config.Port 0 picks an OS-assigned port)
// and returns the listener along with the actual bound port. The server is
// not serving until Serve is called. Binding before serving lets the CLI
// report a truthful "started" log and detect port collisions synchronously.
func (s *Server) Listen() (net.Listener, int, error) {
	addr := fmt.Sprintf(":%d", s.config.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, 0, err
	}
	bound := s.config.Port
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		bound = tcpAddr.Port
	}
	s.httpMu.Lock()
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.router,
	}
	s.httpMu.Unlock()
	return ln, bound, nil
}

// Serve serves requests on an already-bound listener until Shutdown.
func (s *Server) Serve(ln net.Listener) error {
	return s.httpServer.Serve(ln)
}

// BoundPort returns the port the server is listening on (config.Port, or the
// OS-assigned port when config.Port was 0). It is meaningful after Listen.
func (s *Server) BoundPort() int {
	s.httpMu.Lock()
	defer s.httpMu.Unlock()
	if s.httpServer != nil && s.httpServer.Addr != "" {
		if _, portStr, err := net.SplitHostPort(s.httpServer.Addr); err == nil {
			if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
				return p
			}
		}
	}
	return s.config.Port
}

// Shutdown gracefully shuts down the server. It is idempotent: subsequent
// calls are no-ops that return the result of the first shutdown.
func (s *Server) Shutdown(ctx context.Context) error {
	s.registry.stopSweep()
	if s.eventBus != nil {
		s.eventBus.shutdown()
	}
	s.httpMu.Lock()
	hs := s.httpServer
	s.httpMu.Unlock()
	if hs == nil {
		return nil
	}
	s.shutdownOnce.Do(func() {
		s.shutdownResult = hs.Shutdown(ctx)
	})
	return s.shutdownResult
}

func applyPrefixRpc(prefix, path string) string {
	if prefix == "" {
		return path
	}
	p := "/" + strings.Trim(prefix, "/")
	pp := "/" + strings.Trim(path, "/")
	if pp == "/" {
		return p
	}
	return p + pp
}
