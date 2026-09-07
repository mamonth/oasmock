# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Protocol-neutral async management prefix `/_mock/async/{push,consumers,disconnect}`
- Unified example injection: `POST /_mock/examples` gains `match`/`interval`/`delay` for AsyncAPI targets (runtime mirror of `x-mock-match`/`x-mock-interval`/`x-mock-delay`), with strict context-aware validation, plus `DELETE /_mock/examples/{exampleId}` to remove and cancel recurrence
- Single event resource `POST /_mock/events` with a `type` discriminator (V1: `fire`)
- Management WebSocket stream `/_mock/stream` with connect-time `events`/`channels` filters; pushes `event`/`push`/`consumer`/`schedule` envelopes
- Event-context matching: `{$event.name}` (identity), `{$event.data}` (whole payload) alongside `{$event.<field>}`; `{$connection.*}` per-connection recipient partition (id/channel/query/header) with broadcast fast path
- Timing extensions `x-mock-interval` (periodic emission) and `x-mock-delay` (delayed emission); `cron` is no longer an event
- Actually-fired built-in triggers `connect` (on consumer connection) and `receive` (on inbound traffic), gated by a cheap `hasSubscribers` check
- Consumers listable without a `channel` filter — flat union across all channels (raw ws + SignalR streams)

### Changed
- Recurring delivery moved off the schedule endpoint onto `interval` on `/_mock/examples`
- `AddExampleRequest` is now a `oneOf` two-branch schema (sync `path` vs async `channel`) rejecting mixed targeting
- Delivered/scheduled messages are templated at emission time so `{$event.*}`/`{$state.*}`/`{$env.*}` resolve against current state

### Removed
- Deprecated alias endpoints `POST /_mock/ws/push`, `GET /_mock/ws/consumers`, `POST /_mock/ws/disconnect` and `POST /_mock/events/fire`; the canonical `/_mock/async/*` and `POST /_mock/events` surface is the only way to reach those behaviors and any legacy `/_mock/ws/*` path answers a plain 404
- The schedule 410 stubs `POST /_mock/ws/schedule` and `DELETE /_mock/ws/schedule/{pushId}`
- The `x-send-events` extension mapping shim — a message example is classified solely by its `x-mock-match`/`x-mock-interval`/reply trigger; a spec still carrying the key loads without error and the key is silently ignored

### Fixed
- `x-mock-delay` now actually delays an async emission (it was parsed but never applied); a `connect` welcome honors it too
- A runtime async `match` without an `{$event.*}` reference is rejected with 400 instead of silently registering nothing
- `DELETE /_mock/examples/{exampleId}` now also removes sync (OpenAPI) dynamic examples, not only async-driven ones
- The `/_mock/stream` ping keepalive goroutine no longer leaks past the connection's lifetime
- Schema registration is atomic: a load/classification error from any example aborts the whole schema without leaking already-started interval jobs
- A periodically driven example is single-trigger — declaring `x-mock-interval` together with any `x-mock-match` is rejected at load (previously the match was silently dropped at delivery), and period examples honor `x-mock-skip`
- An `{$event.name}` identity whose value is itself a runtime expression is rejected at load instead of registering a subscription key that could never match (matches without an identity pin stay wildcard)
- Timing extensions require integer milliseconds: fractional `x-mock-interval`/`x-mock-delay` values are load errors rather than silently truncated
- A panicking interval delivery is recovered: the job is unregistered and logged instead of silently losing its cadence
- SignalR built-in `connect`/`receive` use a deterministic default channel address instead of map-iteration order for multi-channel hubs
- Periodic deliveries now emit `push` envelopes and built-in `connect` fires emit `event` envelopes to `/_mock/stream` subscribers; `schedule` `started`/`stopped` envelopes carry the same example identity, channel and interval so clients can correlate them
- SignalR upgrades now capture query/headers so `{$connection.query.*}`/`{$connection.header.*}` resolve for hub connections too
- `{$event.*}`/`{$connection.*}` condition values pre-resolve at delivery; reply-path condition values stay literal (sync matching unchanged)
- Docker image `/app/oasmock` is now marked executable — GitHub artifact downloads strip exec bits, breaking `ENTRYPOINT` in the published image
- CI-built binaries are now statically linked (`CGO_ENABLED=0`) — previously `linux/amd64` was dynamically linked against glibc, causing `exec /app/oasmock: no such file or directory` in the `distroless/static` image
- Release Docker image is now smoke-tested (starts and serves the control API) before it is pushed to Docker Hub, via a shared `smoke-test-image` action also used by the PR `docker-build` check
- `api/openapi.yaml` was an invalid OpenAPI document (array schemas missing `items`), causing the server to exit at startup and the Docker smoke test to fail with connection refused; now fixed and covered by a loader test
- Docker smoke test now waits for server readiness with a retry loop and dumps container logs on failure for diagnosis

## [0.1.0] - Initial Release

### Added
- Initial release of OASMock - OpenAPI mock server
- Support for OpenAPI 3.0 schemas with custom extensions
- Runtime expression evaluation with modifiers
- CLI with environment variable support
- Management HTTP API for dynamic examples and request history
- State management and request history recording
- CORS support and configurable request delays