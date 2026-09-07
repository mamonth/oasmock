package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

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
			slog.Info("Registering route", "method", mapping.Method, "chiPattern", mapping.ChiPattern, "fullPath", mapping.Path, "prefix", mapping.Prefix, "pattern", mapping.Pattern, "responses", mapping.Responses != nil)
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
}

// buildRequestSource constructs the runtime request data source from an
// inbound HTTP request. When callBody is non-nil it is used verbatim (the RPC
// gateway passes the decoded call body); otherwise the request body is read,
// parsed as JSON when possible, and restored for downstream handlers.
// Start starts the HTTP server, returning once the server is serving.
