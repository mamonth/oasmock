## Purpose

Serves the mock server's existing HTTP, WebSocket and management routes over an additional HTTPS listener when a certificate and key are configured, without changing the plain-HTTP listener.

## Requirements
### Requirement: Additional TLS listener
The server SHALL serve HTTPS on a separate listener when TLS certificate and key are configured, while continuing to serve plain HTTP on its existing port.

#### Scenario RS.TLS.1: Both listeners serve simultaneously
- **WHEN** the server starts with a valid certificate and key and with a plain HTTP port
- **THEN** mock requests succeed over both `http://` on the plain port and `https://` on the TLS port

#### Scenario RS.TLS.2: Same routes over HTTPS
- **WHEN** a request matches an OpenAPI operation, AsyncAPI HTTP channel, or management API route
- **THEN** that route behaves identically when reached over the TLS listener

#### Scenario RS.TLS.3: TLS disabled without configuration
- **WHEN** the server starts without a certificate and key
- **THEN** no TLS listener is bound and only the plain HTTP listener serves traffic

### Requirement: Certificate loading
The server SHALL load the configured PEM certificate and private key before binding the TLS listener and MUST reject a pair that cannot be read, parsed, or matched.

#### Scenario RS.TLS.4: Valid certificate pair
- **WHEN** the configured certificate and key are a matching valid PEM pair
- **THEN** the TLS listener binds successfully and serves requests

#### Scenario RS.TLS.5: Invalid certificate pair
- **WHEN** the certificate or key file is missing, malformed, or does not match the other
- **THEN** the TLS listener is not bound and server startup fails with an error identifying the certificate problem

### Requirement: TLS protocol versions
The TLS listener SHALL accept TLS 1.2 and TLS 1.3 connections and MUST reject protocol versions below TLS 1.2, with TLS 1.3 negotiated whenever the client supports it.

#### Scenario RS.TLS.6: TLS 1.3 negotiation
- **WHEN** a client that supports TLS 1.3 connects to the TLS listener
- **THEN** the connection uses TLS 1.3

#### Scenario RS.TLS.7: TLS 1.2 compatibility
- **WHEN** a client configured to use TLS 1.2 connects to the TLS listener
- **THEN** the connection succeeds using TLS 1.2

#### Scenario RS.TLS.8: Obsolete protocol rejection
- **WHEN** a client attempts to connect using TLS 1.1 or older
- **THEN** the TLS handshake is rejected

### Requirement: Shared runtime across listeners
The HTTP and HTTPS listeners SHALL share the same routing table, example state, request history, event bus and async consumer runtime.

#### Scenario RS.TLS.9: Shared state and history
- **WHEN** a request handled over HTTPS changes mock state or is recorded in request history
- **THEN** the same state and history are visible through requests served over HTTP and HTTPS

#### Scenario RS.TLS.10: Secure WebSocket upgrade
- **WHEN** a WebSocket or SignalR client connects with `wss://` to a configured channel or hub
- **THEN** the upgrade succeeds and messages are exchanged as they are over plain `ws://`

### Requirement: Dual-listener shutdown
On graceful shutdown the server SHALL stop accepting connections on both the HTTP and HTTPS listeners and complete in-flight requests before returning.

#### Scenario RS.TLS.11: Graceful shutdown of both listeners
- **WHEN** the server receives a shutdown request while both listeners are active
- **THEN** both listeners stop accepting new connections and the server returns without error
