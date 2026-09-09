# Tasks: Fix SignalR push and client compatibility

TDD workflow for every item below (per AGENTS.md design-first/TDD rule):
1. **Red** — write or edit the test for the parent/consumer first (mocking the new/edited interface), prove it fails.
2. **Red** — add the interface-level unit test (mock its dependencies), prove it fails.
3. **Green** — implement the interface until all tests pass (`go test ./...`).
4. **Refactor** — keep cognitive complexity low, cohesion high; re-run tests + lint.

## 1. SignalR handshake framing (RS.SHR.23)

- [x] 1.1 **Red**: add a failing test in `internal/server/signalr_integration_test.go` (or a hub unit test) that dials the hub and sends `{"protocol":"json","version":1}\x1e` as the first frame, asserting the server replies `{}\x1e` and accepts a subsequent `StreamInvocation`; verify it fails against the current code (`invalid character '\x1e' after top-level value`)
- [x] 1.2 **Green**: strip a single trailing `0x1E` from the first frame before parsing it in `signalr_conn.go` `runConnection` (keep accepting the bare form — RS.SHR.14 unchanged); verify 1.1 passes and `go build ./...` succeeds
- [x] 1.3 **Refactor**: run the new handshake test plus the existing SignalR hub tests (`go test ./internal/server/...`) and verify all pass (bare handshake still accepted); re-run lint

## 2. Arbitrary JSON push payloads (RS.SHR.24/25, RS.AMG.31)

- [x] 2.1 **Red**: write failing tests — a unit test that decoding `/_mock/async/messages` accepts an array/scalar body (the `asyncMessageRequest` decoder) and an HTTP test that pushes an array payload to a hub channel with an open stream, asserting the consumer receives it as the `StreamItem` `item` (RS.SHR.24, RS.AMG.31); verify they fail against the current `map[string]any` decode
- [x] 2.2 **Green**: change `asyncMessageRequest.Payload` from `map[string]any` to a raw-JSON/`any` type in `internal/server/management_async.go`, and update `handleAsyncMessage` to marshal the raw payload for delivery; verify 2.1 passes and `go build ./...` succeeds
- [x] 2.3 **Red**: write failing unit tests that runtime-expression evaluation (`evaluatePushPayload`) applies only when the payload is a JSON object, passing arrays/scalars through verbatim (expressions address object fields by path — design D2); verify they fail
- [x] 2.4 **Green**: make `evaluatePushPayload` object-gated, delivering non-object payloads untouched; verify 2.3 passes and the AMG.10/11 unit tests still pass
- [x] 2.5 Widen `MessagesRequest.payload` in `api/openapi.yaml` from `type: object` to any JSON value and regenerate the runtime request-validation schema (`make gen`); verify the generated file reflects the widened payload, `go build ./...` succeeds, and spectral lint / `openspec validate` passes

## 3. Async example targeting of SignalR hub channels (RS.MAPI.38, RS.SHR.27/28)

- [x] 3.1 **Red**: add a test that registers an async example against a hub channel address (e.g. `/qoden/OpenOrders`) via `POST /_mock/examples` and asserts it is accepted; verify it fails first (currently returns `no matching route found` — RS.SHR.27/RS.MAPI.38)
- [x] 3.2 **Green**: extend the async-target resolver (`resolveExampleTarget` / `findAsyncRouteMapping` in `internal/server/server_management.go`) to consult hub channels when the raw-route scan misses, returning a hub-backed target; verify 3.1 passes and existing management-api tests pass
- [x] 3.3 **Red**: write failing end-to-end tests — registration against a hub channel with an event/interval trigger delivers into the hub channel's open streams when the trigger fires (RS.MAPI.38/RS.SHR.27), and an unknown hub/ws address is still rejected with an error and nothing is registered (RS.SHR.28/RS.MAPI.21); verify they fail on the delivery leg
- [x] 3.4 **Green**: wire the hub-backed target through registration and the event/interval delivery pipeline (`message_delivery.go` already reaches hub streams via `SignalRPush`); verify 3.3 passes (including the RS.SHR.28 rejection) and `go build ./...` succeeds

## 4. Per-account hub connection identity (RS.SHR.26, RS.AMG.32, RS.MAPI.39)

- [x] 4.1 **Red**: write failing unit tests — two connections over `/frontoffice/ws/account/qa-A` and `/frontoffice/ws/account/qa-B` retain distinct captured path values, and `hubManager.Candidates` / `handleAsyncConsumers` surface the captured path in consumer records; verify they fail (the connection records only `sc.query`/`sc.headers`, not the path)
- [x] 4.2 **Green**: capture the concrete upgrade path (and chi path-parameter values, e.g. `accountId`) on `signalRConnection` in `signalr_hub.go` `serveUpgrade` and surface it in `hubManager.Candidates` / `handleAsyncConsumers`; verify 4.1 passes and `go build ./...` succeeds
- [x] 4.3 **Red**: write failing tests — a targeted push scoped to one account's captured path value reaches only that account's open stream (RS.SHR.26/RS.MAPI.39), and consumer-discovery responses expose the path for per-account selection (RS.AMG.32); verify they fail
- [x] 4.4 **Green**: match targeted pushes on the captured path value; verify 4.3 passes and existing targeted-push tests (by `connectionId`) still pass
- [ ] 4.5 **Red**: add a failing oasmock-sdk test that the consumer type carries the widened record (add the path field) — `oasmock-ts-sdk` `getConsumerList` returns it; verify the SDK type build fails on the missing field
- [ ] 4.6 **Green**: align the oasmock-sdk consumer type with the widened record and verify the SDK type build + tests pass

## 5. Specs, docs, API surface

- [x] 5.1 Reconcile `openspec/specs/signalr-hub-runtime/spec.md`, `openspec/specs/asyncapi-management/spec.md`, and `openspec/specs/management-api/spec.md` with the delta requirements and scenario numbers (RS.SHR.23-28, RS.AMG.31-32, RS.MAPI.38-39); verify `openspec validate` passes
