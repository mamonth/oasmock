package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/mamonth/oasmock/internal/history"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/state"
)

const (
	maxRequestBodySize = 1 << 20 // 1MB
	DefaultHistorySize = 1000
	DefaultStatusCode  = 200
	DefaultMethod      = "GET"
)

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
	copy(serverSchemas, schemas)
	rpcConfig := (*loader.RpcConfig)(nil)
	for _, schema := range schemas {
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
		RouteProvider: routeProvider,
		StateStore:    stateStore,
		HistoryStore:  historyStore,
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
	s.hubMgr = newHubManager(s.engine, s.protocolAdapters[asyncapi.ProtocolWS].(*wsProtocolAdapter), schemas)
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
			gwPath = loader.PrefixPath(schema.Prefix, rpcConfig.Gateway)
			break
		}

		procMap := make(map[string]*RouteMapping)
		for i := range rpcMappings {
			m := rpcMappings[i]
			procMap[m.Procedure] = &m.RouteMapping
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
