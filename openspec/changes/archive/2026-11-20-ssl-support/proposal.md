# Proposal

## Why

OASMock currently serves only plain HTTP, so it cannot mock APIs whose clients require TLS (HTTPS endpoints, `wss://` WebSockets, SignalR over TLS, or HTTPS-only management tooling). Consumers are forced to put a reverse proxy in front of the mock server or relax client security settings, which adds setup friction and diverges from the real environment.

## What Changes

- Add an **additional** HTTPS listener to the mock server. The existing HTTP listener keeps serving on `--port`; when TLS is configured, both listeners serve the same router, state, history and async runtime.
- Add CLI flags `--tls-cert`, `--tls-key` and `--tls-port` (default `8443`; `0` selects an OS-assigned port).
- Add environment variables `OASMOCK_TLS_CERT`, `OASMOCK_TLS_KEY`, `OASMOCK_TLS_PORT`.
- Add flat `.oasmock.yaml` keys `tls_cert`, `tls_key`, `tls_port`, preserving the existing precedence: CLI > environment > config file > defaults.
- Enable TLS only when both certificate and key are provided. Supplying only one, or supplying `tls_port` without the certificate pair, is a configuration error (exit code `2`).
- Load and validate the PEM certificate/key pair before binding; invalid or mismatched files exit with a new exit code `5`. A busy TLS port keeps exit code `4`.
- Serve HTTPS with `MinVersion` TLS 1.2 and `MaxVersion` TLS 1.3, so TLS 1.3 is negotiated whenever the client supports it.
- Gracefully shut down both listeners.
- Docker: expose port `8443`, document mounting certificate files read-only, and extend the release smoke test to verify HTTPS.
- No mTLS/client-certificate authentication in this change.

## Capabilities

### New Capabilities

- `tls-serving`: HTTPS listener lifecycle and behavior: certificate loading, TLS version policy, shared routing/state/history, `wss://` support, and graceful shutdown of both HTTP and HTTPS listeners.

### Modified Capabilities

- `cli`: new TLS flags, environment variables, configuration-file keys, validation rules, startup logging and exit code `5`.

## Impact

- `internal/server`: `Config` gains TLS configuration; new `ListenTLS`/`ServeTLS`/`StartTLS` lifecycle; `Shutdown` covers both listeners.
- `cmd/oasmock`: new flags/env/config keys, validation, TLS listener startup and dual-listener error handling.
- `Dockerfile` and `.github/actions/smoke-test-image`: exposed TLS port and HTTPS smoke coverage.
- Documentation: `docs/cli.md`, `docs/docker.md`, `docs/architecture.md`, `README.md`, `CHANGELOG.md`.
- Tests: server TLS unit tests, CLI configuration unit tests, end-to-end CLI HTTPS/`wss://` integration tests, Docker TLS smoke test.
- No changes to the management API contract or schema loading.
