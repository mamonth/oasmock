# Tasks: AsyncAPI 3.x Support (MVP)

Reference: proposal.md (why), specs/** (what), design.md (how).

## 1. Dependencies & Spec-Type Detection

- [x] 1.1 Vendor AsyncAPI parser (`github.com/benelser/go-asyncapi`) under `third_party/` with corrected module path + `go 1.23`, wire via `replace` in `go.mod`; add `github.com/gorilla/websocket` (RS.AAL.1-4; design D1)
- [x] 1.2 Implement spec-type detection: read raw file bytes, dispatch on root key `openapi` vs `asyncapi`, else schema error (RS.AAL.1, RS.AAL.2, RS.AAL.4)
- [x] 1.3 Extend `SchemaInfo` with `Kind` (OpenAPI|AsyncAPI) and AsyncAPI document view; keep `Prefix` semantics (RS.AAL.9-12)
- [x] 1.4 Unit tests for loadSchemas dispatch covering OpenAPI, AsyncAPI 3.0.0/3.1.0, and non-spec files

## 2. AsyncAPI Loading & Validation

- [x] 2.1 Implement `internal/asyncapi` abstraction: neutral `Document` view + `Parse` from vendored benelser parser; reject non-3.x major versions with clear error (RS.AAL.3, RS.AAL.12; design D1)
- [x] 2.2 Validate AsyncAPI 3.0.0 specs structurally (mandatory channels/operations) in `internal/asyncapi`, surfacing validation failures as schema errors (RS.AAL.5, RS.AAL.6)
- [x] 2.3 Validate AsyncAPI 3.1.0 specs including `webhooks`/components handling (RS.AAL.7)
- [x] 2.4 Detect unknown/unsupported protocol bindings (including `amqp`) and report errors naming the unsupported protocol (RS.AAL.8); capture `x-mock-*` extensions on message examples (design D1)
- [x] 2.5 Unit tests for AsyncAPI 3.0.0/3.1.0 load+validate and coverage of scenario RS.AAL.5-8, 12

## 3. Unified Routing Model

- [x] 3.1 Introduce protocol-neutral `SpecRoute` (Protocol, Address, Method, Action, Messages) and adapt `RouteMapping` to carry AsyncAPI-backed message specs (design D3)
- [x] 3.2 Map AsyncAPI channels to routes: `http` bindings → method+path; `ws` bindings → address (RS.ASP.1-3)
- [x] 3.3 Report channels with unknown bindings (including `amqp`) or missing binding info as startup errors (RS.ASP.4, RS.ASP.5)
- [x] 3.4 Apply schema prefix to AsyncAPI channel addresses (RS.ASP.8, RS.MSC.51)
- [x] 3.5 Unit tests for channel→route mapping and error cases (RS.ASP.1-5, RS.ASP.8)
- [x] 3.6 Extend model capture for root/document-level `x-signalr` and message-example `x-send-events`; surface in the neutral views (design D1, D7-D8)

## 4. Protocol Adapters (MVP)

- [x] 4.1 Define `ProtocolAdapter` interface and `MessageHandler`; register adapters keyed by protocol (design D4)
- [x] 4.2 Implement `httpAdapter` reusing the existing HTTP pipeline for AsyncAPI http channels (RS.ASP.1, RS.ASP.10)
- [x] 4.3 Implement `wsAdapter` (WebSocket upgrade; accept/send; MVP echo + receive-operation emission; connection registration) (RS.ASP.2, RS.ASP.6-7, RS.ASP.9)
- [x] 4.4 Wire adapters into server startup; server fails to start on unsupported protocol (including `amqp`) with exit code 3 (RS.ASP.4)
- [x] 4.5 Unit tests per adapter: http, ws echo/ack; integration test connecting a ws client

## 5. SignalR Hub Runtime (root-scoped x-signalr)

- [x] 5.1 Implement root-level `x-signalr` parsing: hub path; streams map to channels, one-shot invocations map to operations (RS.SHR.1-7, design D7)
- [x] 5.2 Implement `negotiate` endpoint (`POST {hubPath}/negotiate`): connectionToken/connectionId/negotiateVersion/availableTransports; transport/version handling (RS.SHR.8-10)
- [x] 5.3 Implement token-correlated WebSocket upgrade (`?id=<token>`; 404 on unknown; fresh token when absent) (RS.SHR.11-13)
- [x] 5.4 Implement handshake + `\x1e` framing: first-frame handshake validation, JSON record-separator framing, frame splitting (RS.SHR.14-16)
- [x] 5.5 Implement `StreamInvocation` (type 4) by channel ID → snapshot `StreamItem`, held-open stream, cancel → `Completion` (RS.SHR.3-5, RS.SHR.17)
- [x] 5.6 Implement one-shot `Invocation` (type 1) by operation ID → `Completion` result (RS.SHR.6-7)
- [x] 5.7 Implement open-stream registry and event-driven item append into open streams; server `Invocation` when no stream open (RS.SHR.18-19, RS.EVT.13)
- [x] 5.8 Implement `Ping` (type 6) response (RS.SHR.20); register streams for discovery (RS.SHR.21)
- [x] 5.9 Unit tests per layer (negotiate, handshake, framing, stream/invocation lifecycle); integration test with a raw-frame SignalR client (handshake → StreamInvocation → snapshot → held-open → event-driven item → cancel)

## 6. Event Driver (x-event-trigger / x-send-events)

- [x] 6.1 Parse `x-event-trigger` (list form: name/payload/delay/global) on OpenAPI examples and `x-send-events` (named + `connect`/`cron`/`receive`) on AsyncAPI message examples (RS.EVT.1-11, design D8)
- [x] 6.2 Implement the event broker: schema-local vs `global: true` scoping, fire-on-example-selection, subscriber resolution, delay scheduling (RS.EVT.4-6, RS.EVT.14-15)
- [x] 6.3 Implement `{$event.*}` runtime data source evaluated at emission time alongside schema state/env (RS.EVT.8, RS.ATM.17)
- [x] 6.4 Implement broadcast emission to channel consumers and SignalR open-stream targeting (RS.EVT.12-13)
- [x] 6.5 Implement fire-event management endpoint with delay and payload templating (RS.EVT.16-17, RS.MAPI.22-23, RS.AMG.20-21)
- [x] 6.6 Unit tests for parsing, scoping, broker, templating, and fire-event; integration test covering REST fill → open SignalR stream

## 7. Templating Parity

- [x] 7.1 Add `MessageSource` and `EventSource` data sources (payload/headers/channel params/event payload) registered into `runtime.Evaluator`; wire `{$message.*}`, `{$channel.*}`, `{$event.*}` (RS.ATM.1-3, RS.ATM.5, RS.ATM.17)
- [x] 7.2 Introduce `ExampleValue` wrapper abstracting OpenAPI examples and AsyncAPI message examples; refactor extension extraction (`x-mock-match`, `skip`, `once`, `set-state`, `headers`) onto it (RS.ATM.6-14, design D5)
- [x] 7.3 Evaluate `{$state.*}` for AsyncAPI traffic in the schema's isolated namespace (RS.ATM.4, RS.ATM.16)
- [x] 7.4 Record AsyncAPI message exchanges in the request history store (RS.ATM.15)
- [x] 7.5 Unit tests covering RS.ATM.1-18; parity tests asserting identical selection behavior across OpenAPI and AsyncAPI examples

## 8. AsyncAPI + Async Mocking Management API

- [x] 8.1 Extend `/_mock/examples` route resolution to AsyncAPI route identifiers (protocol + address; method/action for http); return 400 for unmatched AsyncAPI routes (RS.MAPI.19, RS.MAPI.21)
- [x] 8.2 Implement dynamic-example selection for ws/http AsyncAPI traffic using the shared selection pipeline (RS.MAPI.20)
- [x] 8.3 Implement push endpoint with `delay` (ms): immediate (0/omitted) and delayed delivery to channel consumers; negative delay → 400; no-consumers accepted (RS.AMG.1-4)
- [x] 8.4 Implement targeted push (`connectionId`) and broadcast push (omitted); unknown `connectionId` → 404 (RS.AMG.5-7)
- [x] 8.5 Implement consumer discovery endpoint returning active connections per channel (with open streams for SignalR); empty list when none (RS.AMG.8-9)
- [x] 8.6 Evaluate runtime expressions in pushed payloads against schema state/env; unresolvable expression → 400 (RS.AMG.10-11)
- [x] 8.7 Implement recurring scheduled push by interval with cancellation by push ID (RS.AMG.12-13); recurrences may target SignalR streams
- [x] 8.8 Implement fire-event endpoint (payload/delay/global, templated) (RS.AMG.20-21, RS.EVT.16-17)
- [x] 8.9 Implement connection lifecycle control: force-disconnect by `connectionId` with optional close reason/code, and simulate abrupt drop (abort without close frame); unknown consumer → 404 (RS.AMG.14-17)
- [x] 8.10 Add async-mocking endpoints (push, consumers/streams, recurring, fire-event, lifecycle) and AsyncAPI route targeting to `api/openapi.yaml` contract
- [x] 8.11 Unit tests for RS.AMG.1-17, RS.AMG.20-21, RS.EVT.16-17 and RS.MAPI.19-23; integration tests driving push/schedule/consumers/disconnect/fire-event against live ws + SignalR connections

## 9. Server & CLI Wiring

- [x] 9.1 Route incoming traffic by protocol: HTTP asyncapi channels, ws upgrade endpoints (raw / SignalR) (RS.MSC.52, RS.MSC.53)
- [x] 9.2 Keep prefixing, CORS, delay, verbose logging, and management API working for AsyncAPI routes (RS.ATM.15, RS.MSC.1-3, RS.MSC.50-51)
- [x] 9.3 Ensure `--from`/config `schemas` accept AsyncAPI files with no flag changes; schema failure exits code 3 (RS.CLI.30-31, RS.CLI.16)
- [x] 9.4 Integration tests: start server with AsyncAPI spec mix (http + ws + SignalR + event-driven push), verify history/state/CORS/delay

## 10. Docs & Validation

- [x] 10.1 Update `docs/architecture.md` (loader autodetect, adapters, SchemaInfo, SignalR overlay, event broker), `docs/cli.md` (schema types), `docs/project.md` (structure)
- [x] 10.2 Update openspec coverage map: all new RS.* scenarios (AAL/ASP/SHR/EVT/ATM/AMG/MAPI/MSC/CLI) marked covered by tests
- [x] 10.3 Run `go vet`/lint, full test suite, and coverage threshold check; regenerate mocks

## 11. Post-Implementation Review

- [x] 11.1 Run `openspec verify` to confirm specs, design, and implementation coherence
- [x] 11.2 Confirm deferred scope recorded as non-goals (AMQP, Binance diff-depth book, SSE/LongPolling, MessagePack, Ack/Sequence, session/account routing)
- [x] 11.3 Archive the change per the archive workflow