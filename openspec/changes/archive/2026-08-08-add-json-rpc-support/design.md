## Context

OASMock is an HTTP-path router at its core. JSON-RPC 2.0 (and other RPC protocols) route by a body field instead, with a single gateway endpoint. This change introduces a protocol-agnostic `x-rpc` extension at the document root that declares routing configuration while keeping the existing OpenAPI operation model for examples, responses, and extensions.

The core insight: everything downstream of routing — example selection via `x-mock-params-match`, runtime expressions (`{$request.body.*}`), extensions (`x-mock-set-state`, `x-mock-headers`, `x-mock-once`), state, and history — already works with arbitrary JSON bodies. The only new code is routing.

## Goals / Non-Goals

**Goals:**

1. Support `x-rpc` root extension with modular protocol type (json-rpc first, extensible later).
2. Declare gateway path, procedure call source (body field), and procedure match target (spec operation field).
3. Route JSON-RPC calls by `method` body field to the correct OpenAPI operation's examples.
4. Batch: array body → per-call pipeline → array response.
5. Notifications: no `id` → run pipeline (side effects) but no response entry.
6. Standard JSON-RPC 2.0 errors: -32700 parse, -32600 invalid request, -32601 method not found.
7. Full JSON-RPC response envelope in examples (pass-through with `{$request.body.id}`).
8. RPC routes co-exist with normal HTTP routes in the same spec.
9. Reuse existing example selection, expression, extension, state, and history pipeline.

**Non-Goals:**

1. Protocols beyond JSON-RPC 2.0 (design is modular but only one implementation).
2. JSON-RPC 1.0 or over WebSocket.
3. Params-array positional dispatch (always treat params as arbitrary).
4. No CLI or management API changes.
5. No breaking changes to existing path-based routing.
6. No spec-level JSON-RPC schema generation or validation.

## Decisions

**1. x-rpc as a structured root extension**

```yaml
x-rpc:
  gateway: /rpc/single/endpoint
  protocolType: json-rpc
  contentType: application/json   # optional, defaults per protocol
  procedure:
    call: method                   # body property with procedure name
    match: post.operationId        # spec operation field to match against
```

- **Decision**: `x-rpc` is a `map[string]interface{}` at the spec root. kin-openapi v0.133.0 preserves top-level `x-*` fields in `T.Extensions`. Parse into a typed struct. Validate required fields (`gateway`, `protocolType`, `procedure`).
- **Rationale**: Structuring as an object (not a string) enables protocol-specific configuration, future extensibility, and self-documenting schema. Using the same kin-openapi extension mechanism that already works for example-level `x-mock-*`.
- **Alternative**: Flat string `x-json-rpc-base-path: /rpc` — simpler but not extensible to other protocols or configurations.

**2. Protocol abstraction — RpcProtocol interface**

```go
type RpcProtocol interface {
    ParseBody(body []byte) ([]RpcCall, error)
    ErrorResponse(code int, message string, id any) []byte
    ContentType() string
}
type RpcCall struct {
    Procedure string // extracted method/procedure name
    Raw       any    // raw call object for per-call RequestSource body
    ID        any    // call id (nil for notifications)
    HasID     bool   // true if id is present
}
```

- **Decision**: Server delegates body parsing and error formatting to a protocol implementation. `protocolType` selects the implementation via a registry or constructor.
- **Rationale**: Protocol-specific logic (batch detection, version validation, error format) is isolated behind an interface. Adding a new protocol (e.g., xml-rpc) means implementing three methods. Success responses need no wrapping — examples already contain the full envelope.
- **Alternative**: Embed protocol logic directly in the handler — not extensible, would require refactoring for each new protocol.

**3. Procedure matching — `procedure.call` and `procedure.match`**

- `procedure.call`: dot-separated path in the request body to extract the procedure name. For JSON-RPC: `"method"` → `body["method"]`.
- `procedure.match`: dot-separated path into the OpenAPI spec's operation to derive the procedure name. Format: `{httpMethod}.{field}`. Only POST is supported (JSON-RPC is POST-only). `post.operationId` → for each path under the gateway, the POST operation's `OperationID` is the procedure name.

- **Decision**: Two configurable paths — one body-side, one spec-side — that extract names for comparison. The loader builds `map[string]*RouteMapping` (procedure name → mapping) by evaluating `procedure.match` for each POST operation under the gateway. The server handler uses `procedure.call` at runtime to extract the procedure name from the request body.
- **Rationale**: Decouples protocol conventions from spec structure. A future xml-rpc protocol could use `call: methodCall.methodName` and the same `match: post.operationId`. The dot-path resolver handles nesting.
- **Alternative**: Hardcode `body.method` → `operationId` — simpler but not protocol-agnostic.

**4. Catch-all gateway registration, skip per-path registration under gateway**

- **Decision**: Register `POST {gateway}` as a single chi route. Do NOT register individual HTTP routes for paths under the gateway — they would conflict with the catch-all handler and are unreachable anyway.
- **Rationale**: All RPC calls target the same URL. Individual routes would add noise and potential chi routing priority conflicts. Paths NOT under the gateway continue as normal HTTP routes — coexistence is automatic.
- **Coexistence example**: A spec with both `GET /api/users` and `x-rpc.gateway: /rpc` serves both independently. The gateway handler only intercepts POST to `/rpc` and its sub-paths.

**5. Per-call RequestSource in batch**

- **Decision**: For each `RpcCall`, construct a `runtime.RequestSource` with `Body: call.Raw` (the individual call object), NOT the batch array. This ensures `{$request.body.id}` and `{$request.body.params.*}` resolve per-call.
- **Rationale**: `RequestSource.Body` is `any` — can hold individual call objects directly. Zero runtime changes needed. The history middleware records the full batch body (as it should), not per-call.
- **Alternative**: Add a separate `rpc-call` data source — adds complexity for no gain since `request.body` already carries the right data semantically.

**6. Response generation refactor**

- **Decision**: Extract `selectAndGenerateResponse(r *http.Request, mapping *RouteMapping, pathParams map[string]string, callBody any) (body []byte, headers map[string]string, statusCode int, mediaType string, err error)` from `handleMockRequestWithMapping` (server.go:771).
- **Rationale**: The core pipeline (selectResponse → selectMediaType → selectExample → applyExtensions → generateResponse) is identical for HTTP and RPC. Extraction avoids duplication and keeps refactor surface small. Existing `server_test.go` validates no regressions.

**7. Error handling**

- **Decision**: `RpcProtocol.ErrorResponse()` formats protocol-compliant error objects. For JSON-RPC: `{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":<request-id>}`.
  - Parse error (-32700): body is not valid JSON → null id.
  - Invalid request (-32600): missing `jsonrpc`/`method` or wrong version → null or request id.
  - Method not found (-32601): procedure name not in map → echo received id.
- **Rationale**: Standard RFC compliance is expected by JSON-RPC clients. Error objects with proper codes allow clients to distinguish transport errors from application errors.
- **Alternative**: Return plain HTTP 4xx — would confuse JSON-RPC clients expecting structured errors.

**8. Notification handling**

- **Decision**: Process notifications (no `id` field) through the full pipeline to trigger side effects (`x-mock-set-state`, history), but produce no response entry. Single notification → HTTP 204 No Content. Batch notification → skipped in response array.
- **Rationale**: Notifications are valid JSON-RPC and can carry meaningful state updates. Ignoring them would break stateful mock scenarios.
- **Alternative**: Skip notification processing entirely — breaks `x-mock-set-state` semantics.

**9. Content type defaults**

- `contentType` field in `x-rpc` is optional. For `protocolType: json-rpc`, default is `"application/json"`. The handler sets response `Content-Type` accordingly. If specified, overrides the per-protocol default.

**10. TDD approach**

Write interfaces first, then unit tests (using gomock-generated mocks), then implementation. This follows the project's design-first, test-first guidelines and ensures the `RpcProtocol` interface is testable by construction.

## Risks / Trade-offs

- **[Risk] Body consumed by history middleware before RPC handler sees it.**  
  **Mitigation**: Existing `requestHistoryMiddleware` (server.go:909) restores body via `io.NopCloser`. Verified — RPC handler receives the full body.

- **[Risk] Large batch arrays could cause OOM or slow responses.**  
  **Mitigation**: Existing `maxRequestBodySize = 1MB` limit. Per-call processing is sequential (no goroutine per call). Batch size is implicitly bounded by body size.

- **[Trade-off] `procedure.match` supports only `post.operationId` initially.**  
  **Justification**: JSON-RPC is POST-only. Adding `post.path`, `post.x-rpc-name`, etc. later is a simple extension of the field resolver.

- **[Trade-off] By-position params array routing is not supported (always treat params as arbitrary).**  
  **Justification**: Positional params are deprecated in JSON-RPC 2.0 spec. Named-object params are the recommended form and cover the vast majority of real-world usage.

- **[Risk] Refactoring `handleMockRequestWithMapping` may break existing HTTP tests.**  
  **Mitigation**: Run full test suite after extraction. The change is purely structural (extract method, delegate) with zero behavioral diff. All existing `server_test.go` must pass.

- **[Risk] Schema prefix (`--prefix /api`) combined with `gateway: /rpc` creates effective path `/api/rpc`.**  
  **Mitigation**: The loader's `applyPrefix` is already used for all paths. Gateway paths get prefixed identically. Integration test verifies prefix + gateway combination.

## Open Questions

1. Should `procedure.match` support HTTP methods other than POST in the initial implementation?  
   **Answer**: No — JSON-RPC is POST-only. Extend when needed.
2. Should `procedure.call` support nested paths (e.g., `jsonrpc.method`)?  
   **Answer**: Dot-separated support is included for forward compatibility. JSON-RPC uses flat `method`.
3. Should batch responses be parallelized?  
   **Answer**: No — sequential is safer, bounded by body size cap, and creates no goroutine leak risk.
