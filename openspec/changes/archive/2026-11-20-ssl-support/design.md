# Design

## Context

See `proposal.md` for motivation and `specs/tls-serving/spec.md` + `specs/cli/spec.md` for the behavior contract.

Current state constraints:

- `internal/server.Server` owns exactly one `*http.Server`, created in `Listen()` and served by `Serve(ln)`. `Shutdown()` closes that single server.
- `cmd/oasmock` resolves configuration through Viper, validates it, then calls `srv.Listen()` and `srv.Serve(ln)` from `runMock`.
- HTTP and WebSocket handlers are mounted on one `chi.Mux` stored on the server; there is no separate handler per protocol or listener.
- The project already depends on the Go standard library for HTTP and has no TLS dependency. Certificates are therefore handled with `crypto/tls`.
- Docker runs the distroless image as non-root UID 65532, so mounted certificate files must be readable by that UID.

## Goals / Non-Goals

**Goals:**

- Add an optional second listener that terminates TLS and reuses the existing router and runtime.
- Keep the plain HTTP listener and all existing behavior unchanged when TLS is not configured.
- Fail fast on invalid TLS configuration, with distinguishable exit codes.
- Cover the feature with unit, integration, and Docker smoke tests.

**Non-Goals:**

- No client-certificate authentication (mTLS), no CA bundle configuration.
- No automatic certificate reload or ACME integration.
- No HTTPS-only mode that replaces the plain HTTP listener.
- No password-protected PEM key support (Go's `tls.LoadX509KeyPair` does not handle encrypted keys).
- No change to the management API or schema loading.

## Decisions

### 1. A second `http.Server` instead of wrapping the HTTP listener

Use a dedicated `*http.Server` for TLS, sharing `s.router` with the existing HTTP server.

- `ListenTLS()` binds the TCP socket and creates the TLS server.
- `ServeTLS(ln)` calls `ServeTLS(ln, "", "")` after the certificate pair has been loaded into `TLSConfig.Certificates`.
- The pre-load in `ListenTLS()` gives fail-fast validation before any "started" log; passing empty file arguments to `ServeTLS` avoids reading the files twice and keeps Go's automatic HTTP/2 setup.

Alternative considered: wrap the existing listener in `tls.NewListener` and call `Serve`. Rejected because it bypasses `http.Server.ServeTLS`'s HTTP/2 and ALPN setup.

Alternative considered: start a separate `Server` instance for TLS. Rejected because it would duplicate state, history, and the event bus.

### 2. TLS configuration as an optional pointer on `server.Config`

```go
type TLSConfig struct {
    CertFile string
    KeyFile  string
    Port     int
}

type Config struct {
    ...
    TLS *TLSConfig
}
```

`TLS == nil` means disabled. The server does not apply CLI defaults; `cmd/oasmock` supplies `8443` and resolves configuration sources.

### 3. TLS version policy

Set `MinVersion: tls.VersionTLS12` and `MaxVersion: tls.VersionTLS13`. Go negotiates the highest mutually supported version, so TLS 1.3 is selected whenever the client supports it, while TLS 1.2 remains available for older clients.

Alternative considered: `MinVersion: tls.VersionTLS13`. Rejected because the requirement keeps TLS 1.2 clients working.

### 4. CLI configuration and validation

Add three flags bound to Viper keys `tls_cert`, `tls_key`, `tls_port`; `AutomaticEnv` provides `OASMOCK_TLS_CERT`, `OASMOCK_TLS_KEY`, `OASMOCK_TLS_PORT`. The `.oasmock.yaml` keys are flat and match the existing `history_size` style.

Detecting an explicitly configured TLS port needs a helper rather than `viper.IsSet`, because the bound flag default makes `IsSet` true even when the user wrote nothing. The helper checks all three sources:

- `cmd.Flags().Changed("tls-port")`
- `os.LookupEnv("OASMOCK_TLS_PORT")`
- `viper.InConfig("tls_port")`

Validation rules:

- certificate and key must be provided together;
- an explicitly configured TLS port requires the certificate pair;
- TLS port must be `0` or in `1..65535`;
- TLS port must differ from the plain HTTP port.

Exit code mapping:

- `2`: invalid flag/configuration combinations and ranges;
- `4`: TLS port already in use;
- `5`: certificate/key load or match failure.

### 5. Dual-listener startup and shutdown

`runMock` binds both listeners before logging success:

1. `Listen()` for HTTP.
2. If TLS is configured, `ListenTLS()`.
3. If the TLS bind or certificate load fails, close/shutdown the HTTP listener and return the mapped error.
4. Start both `Serve` goroutines with an error channel sized for two.
5. Log both actual ports (`Mock server started port=...`, `TLS listener started port=...`).
6. On signal, call `Shutdown` once; the server shuts down both `http.Server` values and joins errors with `errors.Join`.

`Start()` stays HTTP-only to avoid changing its contract for existing callers. TLS is started through `StartTLS()` or the `ListenTLS`/`ServeTLS` pair.

### 6. Docker

Expose `19191` and `8443`, and document read-only certificate mounts plus the non-root permission requirement. TLS is enabled through the existing flags/environment variables, so no image-specific entrypoint argument is needed.

The Docker smoke test generates a self-signed certificate, mounts it, starts the container with both ports, and verifies `https://` while also checking the plain HTTP control API.

## Risks / Trade-offs

- **Mounted key permissions** → document that UID 65532 must be able to read it; recommend `644` for development and an appropriately owned file for stricter setups.
- **Two listeners diverge in behavior** → both servers share the same router and runtime; integration tests assert state/history visibility across HTTP and HTTPS and exercise `wss://`.
- **TLS port loaded but unused** → validation rejects an explicitly configured TLS port without a certificate pair, avoiding silent misconfiguration.
- **Invalid certificates discovered late** → the certificate pair is loaded in `ListenTLS()` before serving and before success is logged.
- **Port collision between HTTP and TLS** → validation rejects equal ports before binding.
- **Test flakiness from fixed ports** → `--tls-port 0` is supported and the actual port is parsed from startup logs, matching the existing ephemeral HTTP-port approach.
- **HTTP/2 behavior** → using `ServeTLS` with preloaded certificates preserves HTTP/2 and ALPN; a unit test verifies HTTPS serving.

## Migration Plan

No migration is required. The feature is opt-in: existing deployments that do not set TLS options keep a single plain HTTP listener and unchanged exit behavior. To adopt TLS, operators provide a certificate and key via flags, environment, or the configuration file, publish the TLS port, and mount certificate files read-only in Docker. Rollback is removing the TLS options and the extra published/mounted resources.

## Open Questions

None. Scope decisions were resolved before implementation: no mTLS, flat configuration keys, new exit code 5, explicit TLS port without a certificate pair is an error, ephemeral TLS ports are allowed, and TLS 1.2 remains accepted with TLS 1.3 preferred.
