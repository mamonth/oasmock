package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/websocket"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/test/_shared/tlshelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tlsTestServer struct {
	server   *Server
	httpPort int
	tlsPort  int
}

func loadTLSTestSchemas(t *testing.T, paths ...string) []loader.SchemaInfo {
	t.Helper()
	schemas, err := loader.LoadSchemas(paths, make([]string, len(paths)))
	require.NoError(t, err)
	return schemas
}

func startTLSTestServer(t *testing.T, config Config, schemaPaths ...string) *tlsTestServer {
	t.Helper()
	certFile, keyFile := tlshelper.GenerateSelfSigned(t)
	config.Port = 0
	config.TLS = &TLSConfig{CertFile: certFile, KeyFile: keyFile, Port: 0}
	srv, err := New(config, loadTLSTestSchemas(t, schemaPaths...))
	require.NoError(t, err)

	httpLn, httpPort, err := srv.Listen()
	require.NoError(t, err)
	go func() { _ = srv.Serve(httpLn) }()
	tlsLn, tlsPort, err := srv.ListenTLS()
	require.NoError(t, err)
	go func() { _ = srv.ServeTLS(tlsLn) }()

	waitForTCP(t, httpPort)
	waitForTCP(t, tlsPort)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	return &tlsTestServer{server: srv, httpPort: httpPort, tlsPort: tlsPort}
}

func waitForTCP(t *testing.T, port int) {
	t.Helper()
	require.Eventuallyf(t, func() bool {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 100*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 5*time.Second, 20*time.Millisecond, "port %d did not start listening", port)
}

func httpsGet(t *testing.T, port int, path string) (*http.Response, []byte) {
	t.Helper()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: tlshelper.ClientConfig()}}
	defer client.CloseIdleConnections()
	resp, err := client.Get(fmt.Sprintf("https://localhost:%d%s", port, path))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp, body
}

/*
Scenario: Serving the same routes over HTTP and HTTPS
Given a server with a valid certificate and both listeners bound
When the same mock and management routes are requested over HTTP and HTTPS
Then both listeners return successful responses

Related spec scenarios: RS.TLS.1, RS.TLS.2
*/
func TestTLSListenerServesHTTPS(t *testing.T) {
	t.Parallel()

	rt := startTLSTestServer(t, Config{EnableControlAPI: true},
		"../../test/_shared/resources/test.yaml")

	httpResp, err := http.Get(fmt.Sprintf("http://localhost:%d/users", rt.httpPort))
	require.NoError(t, err)
	defer httpResp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, httpResp.StatusCode)

	tlsResp, body := httpsGet(t, rt.tlsPort, "/users")
	assert.Equal(t, http.StatusOK, tlsResp.StatusCode)
	assert.Contains(t, string(body), "Alice")

	mgmtResp, _ := httpsGet(t, rt.tlsPort, "/_mock/requests")
	assert.Equal(t, http.StatusOK, mgmtResp.StatusCode)
}

/*
Scenario: Serving an AsyncAPI HTTP channel over HTTPS
Given a server with an AsyncAPI HTTP channel and a valid certificate
When the channel route is requested over HTTPS
Then the channel message is rendered with the same status as the HTTP listener

Related spec scenarios: RS.TLS.2
*/
func TestTLSServesAsyncAPIHTTPChannel(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "asyncapi.yaml")
	require.NoError(t, os.WriteFile(schemaPath, []byte(`asyncapi: 3.0.0
info:
  title: HTTP Events
  version: 1.0.0
channels:
  employees:
    address: /employees
    messages:
      emplMsg:
        examples:
          - payload:
              id: 1
              name: Ada
operations:
  getEmployees:
    action: send
    channel:
      $ref: '#/channels/employees'
    bindings:
      http:
        method: GET
`), 0644))

	rt := startTLSTestServer(t, Config{}, schemaPath)

	httpResp, err := http.Get(fmt.Sprintf("http://localhost:%d/employees", rt.httpPort))
	require.NoError(t, err)
	defer httpResp.Body.Close() //nolint:errcheck
	require.Equal(t, http.StatusOK, httpResp.StatusCode)

	tlsResp, body := httpsGet(t, rt.tlsPort, "/employees")
	assert.Equal(t, httpResp.StatusCode, tlsResp.StatusCode)
	assert.Contains(t, string(body), `"name":"Ada"`)
}

/*
Scenario: Negotiating TLS protocol versions
Given a server with TLS 1.2 as the minimum and TLS 1.3 as the maximum
When clients connect with TLS 1.3, TLS 1.2, and TLS 1.1
Then TLS 1.3 and TLS 1.2 succeed and TLS 1.1 is rejected

Related spec scenarios: RS.TLS.6, RS.TLS.7, RS.TLS.8
*/
func TestTLSProtocolVersions(t *testing.T) {
	t.Parallel()

	rt := startTLSTestServer(t, Config{}, "../../test/_shared/resources/test.yaml")

	dial := func(min, max uint16) (*tls.Conn, error) {
		return tls.DialWithDialer(
			&net.Dialer{Timeout: 2 * time.Second},
			"tcp",
			fmt.Sprintf("localhost:%d", rt.tlsPort),
			&tls.Config{InsecureSkipVerify: true, MinVersion: min, MaxVersion: max}, //nolint:gosec
		)
	}

	conn13, err := dial(tls.VersionTLS13, tls.VersionTLS13)
	require.NoError(t, err)
	assert.Equal(t, uint16(tls.VersionTLS13), conn13.ConnectionState().Version)
	_ = conn13.Close()

	conn12, err := dial(tls.VersionTLS12, tls.VersionTLS12)
	require.NoError(t, err)
	assert.Equal(t, uint16(tls.VersionTLS12), conn12.ConnectionState().Version)
	_ = conn12.Close()

	conn11, err := dial(tls.VersionTLS11, tls.VersionTLS11)
	if err == nil {
		_ = conn11.Close()
	}
	assert.Error(t, err, "TLS 1.1 handshake should be rejected")
}

/*
Scenario: Sharing state and history across HTTP and HTTPS
Given a server whose HTTP and HTTPS listeners share one runtime
When state is changed over HTTP and read over HTTPS
Then the HTTPS response observes the state and request history contains both requests

Related spec scenarios: RS.TLS.9, RS.TLS.3
*/
func TestTLSListenersShareStateAndHistory(t *testing.T) {
	t.Parallel()

	rt := startTLSTestServer(t, Config{EnableControlAPI: true},
		"../../test/_shared/resources/test-extensions.yaml")

	stateResp, err := http.Get(fmt.Sprintf("http://localhost:%d/state", rt.httpPort))
	require.NoError(t, err)
	stateResp.Body.Close() //nolint:errcheck
	require.Equal(t, http.StatusOK, stateResp.StatusCode)

	readResp, body := httpsGet(t, rt.tlsPort, "/read")
	require.Equal(t, http.StatusOK, readResp.StatusCode)
	assert.Contains(t, string(body), `"counter":1`)

	historyResp, historyBody := httpsGet(t, rt.tlsPort, "/_mock/requests")
	require.Equal(t, http.StatusOK, historyResp.StatusCode)
	assert.Contains(t, string(historyBody), "/state")
	assert.Contains(t, string(historyBody), "/read")
}

/*
Scenario: Upgrading a secure WebSocket connection
Given a server with an AsyncAPI WebSocket channel and a valid certificate
When a client connects with wss://
Then the upgrade succeeds and the channel message is delivered

Related spec scenarios: RS.TLS.10
*/
func TestTLSWebSocketUpgrade(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "asyncapi.yaml")
	require.NoError(t, os.WriteFile(schemaPath, []byte(`asyncapi: 3.0.0
info:
  title: Alerts
  version: 1.0.0
channels:
  alerts:
    address: /alerts
    bindings:
      ws:
        method: GET
    messages:
      alertMsg:
        examples:
          - name: snap
            payload:
              level: info
              msg: hello-wss
operations:
  receiveAlerts:
    action: receive
    channel:
      $ref: '#/channels/alerts'
`), 0644))

	rt := startTLSTestServer(t, Config{}, schemaPath)

	dialer := websocket.Dialer{TLSClientConfig: tlshelper.ClientConfig()}
	conn, _, err := dialer.Dial(fmt.Sprintf("wss://localhost:%d/alerts", rt.tlsPort), nil)
	require.NoError(t, err)
	defer conn.Close() //nolint:errcheck
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg), "hello-wss")
}

/*
Scenario: Shutting down both listeners
Given a server with both HTTP and HTTPS listeners running
When the server shuts down gracefully
Then both listeners stop accepting connections

Related spec scenarios: RS.TLS.11
*/
func TestTLSShutdownStopsBothListeners(t *testing.T) {
	t.Parallel()

	rt := startTLSTestServer(t, Config{}, "../../test/_shared/resources/test.yaml")
	httpPort, tlsPort := rt.httpPort, rt.tlsPort

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, rt.server.Shutdown(ctx))

	for _, port := range []int{httpPort, tlsPort} {
		_, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 100*time.Millisecond)
		assert.Error(t, err, "port %d should stop accepting connections", port)
	}
}

/*
Scenario: Stopping both listeners before draining in-flight requests
Given a server with both HTTP and HTTPS listeners and a blocked HTTP request
When graceful shutdown begins
Then the TLS listener stops accepting before the HTTP request completes

Related spec scenarios: RS.TLS.11
*/
func TestTLSShutdownStopsBothListenersBeforeDraining(t *testing.T) {
	t.Parallel()

	certFile, keyFile := tlshelper.GenerateSelfSigned(t)
	srv, err := New(Config{
		HistorySize: DefaultHistorySize,
		TLS:         &TLSConfig{CertFile: certFile, KeyFile: keyFile, Port: 0},
	}, nil)
	require.NoError(t, err)
	srv.router = chi.NewRouter()

	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseHandler := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(func() {
		releaseHandler()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	srv.router.Get("/block", func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})

	httpLn, httpPort, err := srv.Listen()
	require.NoError(t, err)
	go func() { _ = srv.Serve(httpLn) }()
	tlsLn, tlsPort, err := srv.ListenTLS()
	require.NoError(t, err)
	go func() { _ = srv.ServeTLS(tlsLn) }()
	waitForTCP(t, httpPort)
	waitForTCP(t, tlsPort)

	requestDone := make(chan error, 1)
	go func() {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/block", httpPort))
		if err == nil {
			_ = resp.Body.Close()
		}
		requestDone <- err
	}()

	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- srv.Shutdown(ctx) }()

	assertListenerClosed := func(port int, message string) {
		require.Eventually(t, func() bool {
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 50*time.Millisecond)
			if err != nil {
				return true
			}
			_ = conn.Close()
			return false
		}, 2*time.Second, 10*time.Millisecond, message)
	}
	assertListenerClosed(httpPort, "HTTP listener should stop accepting")
	assertListenerClosed(tlsPort, "TLS listener should stop accepting before HTTP drain completes")

	releaseHandler()
	require.NoError(t, <-requestDone)
	require.NoError(t, <-shutdownDone)
}

/*
Scenario: Disabling TLS without configuration
Given a server configuration without TLS settings
When the TLS listener is requested and the HTTP listener starts
Then no TLS listener is available and HTTPS traffic is not served

Related spec scenarios: RS.TLS.3
*/
func TestTLSDisabledWithoutConfig(t *testing.T) {
	t.Parallel()

	srv, err := New(Config{Port: 0}, loadTLSTestSchemas(t, "../../test/_shared/resources/test.yaml"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	_, _, err = srv.ListenTLS()
	assert.Error(t, err, "TLS listener must not bind without configuration")

	ln, port, err := srv.Listen()
	require.NoError(t, err)
	go func() { _ = srv.Serve(ln) }()
	waitForTCP(t, port)

	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/users", port))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

/*
Scenario: Cleaning up a partially started dual-listener server
Given a server with an unreadable TLS certificate pair
When StartTLS binds HTTP but fails to start the TLS listener
Then the async runtime is shut down before the error is returned

Related spec scenarios: RS.TLS.11
*/
func TestStartTLSCleansUpOnCertificateFailure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	routeProvider := NewMockRouteProvider(ctrl)
	routeProvider.EXPECT().BuildRouteMappings(gomock.Any()).Return([]RouteMapping{}, nil)
	deps := Dependencies{
		RouteProvider: routeProvider,
		StateStore:    NewMockStateStore(ctrl),
		HistoryStore:  NewMockHistoryStore(ctrl),
	}
	driver := &stubAsyncDriver{}
	dir := t.TempDir()
	srv, err := newServerWithDriver(Config{
		HistorySize: DefaultHistorySize,
		TLS: &TLSConfig{
			CertFile: filepath.Join(dir, "missing.crt"),
			KeyFile:  filepath.Join(dir, "missing.key"),
			Port:     0,
		},
	}, []SchemaInfo{}, deps, driver, nil, nil)
	require.NoError(t, err)

	err = srv.StartTLS()
	assert.ErrorIs(t, err, ErrTLSCertificate)
	assert.Equal(t, 1, driver.shutdownCalls, "partial startup must stop the async runtime")
}

/*
Scenario: Loading TLS certificate pairs
Given valid, missing, malformed, and mismatched certificate/key files
When ListenTLS loads the configured pair
Then valid pairs bind and invalid pairs fail with a certificate error before any listener is available

Related spec scenarios: RS.TLS.4, RS.TLS.5
*/
func TestTLSCertificateLoading(t *testing.T) {
	t.Parallel()

	certFile, keyFile := tlshelper.GenerateSelfSigned(t)
	_, otherKey := tlshelper.GenerateSelfSigned(t)
	dir := t.TempDir()
	malformed := filepath.Join(dir, "malformed.pem")
	require.NoError(t, os.WriteFile(malformed, []byte("not a certificate"), 0644))

	tests := []struct {
		name    string
		cert    string
		key     string
		wantErr bool
	}{
		{name: "valid pair", cert: certFile, key: keyFile},
		{name: "missing certificate", cert: filepath.Join(dir, "missing.pem"), key: keyFile, wantErr: true},
		{name: "malformed certificate", cert: malformed, key: keyFile, wantErr: true},
		{name: "mismatched pair", cert: certFile, key: otherKey, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := &Server{
				config: Config{TLS: &TLSConfig{CertFile: tt.cert, KeyFile: tt.key, Port: 0}},
				router: chi.NewRouter(),
			}
			ln, port, err := srv.ListenTLS()
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrTLSCertificate)
				assert.Nil(t, ln)
				return
			}
			require.NoError(t, err)
			assert.NotZero(t, port)
			require.NoError(t, ln.Close())
		})
	}
}
