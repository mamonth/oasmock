package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
)

// ErrTLSCertificate identifies certificate loading or matching failures so the
// CLI can map them to its dedicated exit code without inspecting error text.
var ErrTLSCertificate = errors.New("TLS certificate error")

// TLSConfig configures the optional HTTPS listener.
type TLSConfig struct {
	CertFile string
	KeyFile  string
	Port     int
}

// ListenTLS binds the configured TLS port (TLS.Port 0 picks an OS-assigned
// port), loads and validates the certificate pair, and returns the listener
// with the actual bound port. The server is not serving until ServeTLS is
// called.
func (s *Server) ListenTLS() (net.Listener, int, error) {
	if s.config.TLS == nil {
		return nil, 0, fmt.Errorf("TLS is not configured")
	}
	cert, err := tls.LoadX509KeyPair(s.config.TLS.CertFile, s.config.TLS.KeyFile)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrTLSCertificate, err)
	}
	addr := fmt.Sprintf(":%d", s.config.TLS.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, 0, err
	}
	bound := s.config.TLS.Port
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok {
		bound = tcpAddr.Port
	}
	tlsServer := &http.Server{
		Addr:    addr,
		Handler: s.router,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
			MaxVersion:   tls.VersionTLS13,
		},
	}
	s.httpMu.Lock()
	s.tlsServer = tlsServer
	s.httpMu.Unlock()
	return ln, bound, nil
}

// ServeTLS serves requests on an already-bound TLS listener until Shutdown.
// Certificates were loaded by ListenTLS, so no file arguments are needed.
func (s *Server) ServeTLS(ln net.Listener) error {
	s.httpMu.Lock()
	ts := s.tlsServer
	s.httpMu.Unlock()
	if ts == nil {
		return fmt.Errorf("TLS server is not initialized")
	}
	return ts.ServeTLS(ln, "", "")
}

// StartTLS binds and serves both the plain HTTP listener and the TLS listener.
// It blocks until either listener stops, then shuts both down.
func (s *Server) StartTLS() error {
	httpLn, _, err := s.Listen()
	if err != nil {
		return err
	}
	tlsLn, _, err := s.ListenTLS()
	if err != nil {
		_ = httpLn.Close()
		_ = s.Shutdown(context.Background())
		return err
	}
	errCh := make(chan error, 2)
	go func() { errCh <- s.Serve(httpLn) }()
	go func() { errCh <- s.ServeTLS(tlsLn) }()
	err = <-errCh
	_ = s.Shutdown(context.Background())
	return err
}
