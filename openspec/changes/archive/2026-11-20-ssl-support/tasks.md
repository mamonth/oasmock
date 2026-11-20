# Tasks

## 1. Server TLS listener

- [x] 1.1 Add `TLSConfig` and an optional `TLS *TLSConfig` field to `server.Config`; verify existing server tests still compile and pass with `TLS == nil`
- [x] 1.2 Implement certificate loading in `internal/server/tls.go` using `tls.LoadX509KeyPair`; verify unit tests reject missing, malformed, and mismatched cert/key files (RS.TLS.4, RS.TLS.5)
- [x] 1.3 Implement `ListenTLS()` and `ServeTLS()` with `MinVersion` TLS 1.2 and `MaxVersion` TLS 1.3, sharing `s.router`; verify HTTPS requests succeed and TLS 1.2/TLS 1.3 are accepted while TLS 1.1 is rejected (RS.TLS.1, RS.TLS.2, RS.TLS.6, RS.TLS.7, RS.TLS.8)
- [x] 1.4 Extend `Shutdown()` to stop both HTTP and TLS `http.Server` values idempotently with `errors.Join`; verify both listeners refuse connections after shutdown (RS.TLS.11)
- [x] 1.5 Add server unit tests for HTTPS routing, shared state/history across listeners, `wss://` upgrade, and disabled TLS when `Config.TLS` is nil; verify with `go test ./internal/server` (RS.TLS.1, RS.TLS.2, RS.TLS.3, RS.TLS.9, RS.TLS.10)
- [x] 1.6 Update `docs/architecture.md` to describe the optional second listener; verify the document matches the implemented `ListenTLS`/`ServeTLS` lifecycle

## 2. CLI TLS configuration

- [x] 2.1 Add `--tls-cert`, `--tls-key`, and `--tls-port` flags (default `8443`) bound to Viper keys `tls_cert`, `tls_key`, `tls_port`; verify `oasmock mock --help` lists the new options
- [x] 2.2 Implement explicit TLS-port detection across CLI, environment, and config file, then validate certificate/key pairing, port range, and HTTP/TLS port inequality; verify unit tests return code 2 for each invalid case (RS.CLI.33, RS.CLI.34, RS.CLI.35, RS.CLI.39, RS.CLI.40, RS.CLI.41)
- [x] 2.3 Map TLS listener failures to exit codes: certificate/key errors to 5 and an occupied TLS port to 4; verify unit or integration tests assert both codes (RS.CLI.42, RS.CLI.43)
- [x] 2.4 Add CLI unit tests for flag/env/config precedence and ephemeral TLS-port acceptance; verify with `go test ./cmd/oasmock` (RS.CLI.37, RS.CLI.38, RS.CLI.36)
- [x] 2.5 Document the flags, environment variables, flat configuration keys, examples, and exit codes in `docs/cli.md`; verify the documented commands match the implementation
- [x] 2.6 Update `CHANGELOG.md` under `Unreleased -> Added`; verify the entry describes the optional TLS listener and exit code 5

## 3. Dual-listener lifecycle and integration

- [x] 3.1 Implement dual-listener startup in `runMock`: bind HTTP then TLS, roll back HTTP on TLS failure, serve both concurrently, log both actual ports, and handle either server error (RS.CLI.33, RS.CLI.34, RS.CLI.36, RS.CLI.44)
- [x] 3.2 Add a self-signed certificate helper under `test/_shared` and extend the CLI test builder with TLS cert/key/port options; verify the helper is reused by integration tests without committing certificate fixtures
- [x] 3.3 Add CLI integration tests for HTTP plus HTTPS serving, management API over HTTPS, `wss://`, ephemeral TLS port logging, and graceful shutdown; verify with `go test ./test/cli` (RS.TLS.1, RS.TLS.2, RS.TLS.10, RS.TLS.11, RS.CLI.44)
- [x] 3.4 Add CLI integration tests for invalid certificate exit code 5, incomplete pair exit code 2, TLS port without certificate pair exit code 2, and occupied TLS port exit code 4 (RS.CLI.39, RS.CLI.40, RS.CLI.42, RS.CLI.43)

## 4. Docker support and smoke test

- [x] 4.1 Add `EXPOSE 19191 8443` to `Dockerfile`; verify the built image reports both exposed ports
- [x] 4.2 Extend `.github/actions/smoke-test-image/action.yml` to generate a self-signed certificate, mount it read-only, start the container with `OASMOCK_TLS_CERT`/`OASMOCK_TLS_KEY`, and verify HTTPS plus plain HTTP responses (RS.TLS.1)
- [x] 4.3 Document certificate mounting, non-root UID 65532 permissions, port publishing, environment variables, and a Compose example in `docs/docker.md`; verify the examples run as written
- [x] 4.4 Add a short TLS example to `README.md`; verify it links to the detailed Docker and CLI documentation

## 5. Integration verification

- [x] 5.1 Run `make test-unit` and confirm all unit tests, including new TLS and CLI tests, pass
- [x] 5.2 Run `make test-integration` and confirm HTTP plus HTTPS/`wss://` integration tests pass
- [x] 5.3 Run `make lint` and `make test-headers`; verify lint is clean and every new test has a Gherkin header with related scenario codes
- [x] 5.4 Run `make spec-coverage` and `make coverage-unit`; verify all new scenarios are covered and coverage does not regress below 70%

## Workflow follow-up

- Archive the change with `openspec-archive-change` after all tasks and verification pass.
- Re-run `openspec validate --change ssl-support` before archiving and confirm no warnings.
