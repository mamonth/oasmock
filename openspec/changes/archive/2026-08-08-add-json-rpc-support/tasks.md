## 1. Interface and type definitions (design first)

- [x] 1.1 Define `loader.RpcConfig`, `loader.ProcedureConfig` structs in `internal/loader/rpc_config.go`
- [x] 1.2 Define `loader.RpcRouteMapping{Procedure string, RouteMapping}` type in `internal/loader/rpc_config.go`
- [x] 1.3 Define `server.RpcCall` and `server.RpcProtocol` interfaces in `internal/server/interfaces.go`
- [x] 1.4 Regenerate mocks: run `go generate ./internal/server/...` for `RpcProtocol` mock
- [x] 1.5 Define `server.RpcHandler` struct in `internal/server/jsonrpc.go` (holds protocol, procedure map, server ref)

## 2. Unit tests for loader RPC config parsing (TDD: test first)

- [x] 2.1 Test `ParseRpcConfig`: valid full config with all optional fields → correctly parsed struct
- [x] 2.2 Test `ParseRpcConfig`: missing gateway → error
- [x] 2.3 Test `ParseRpcConfig`: unsupported protocolType → error
- [x] 2.4 Test `ParseRpcConfig`: missing procedure field → error
- [x] 2.5 Test `ParseRpcConfig`: default contentType when not specified (should be "application/json" for json-rpc)
- [x] 2.6 Test `ParseRpcConfig`: returns nil when `x-rpc` extension is absent
- [x] 2.7 Test `ParseRpcConfig`: malformed x-rpc (not a map) → error
- [x] 2.8 Test `BuildRpcMappings`: paths under gateway with POST → mapped by operationId
- [x] 2.9 Test `BuildRpcMappings`: paths not under gateway → excluded from RPC mappings
- [x] 2.10 Test `BuildRpcMappings`: paths under gateway without POST operation → excluded
- [x] 2.11 Test `BuildRpcMappings`: duplicate operationId under gateway → error
- [x] 2.12 Test `BuildRpcMappings`: schema prefix applied to gateway path
- [x] 2.13 Test coexistence: one spec with both RPC (paths under gateway) and non-RPC paths → RpcMappings and regular RouteMappings both populated

## 3. Unit tests for JSON-RPC protocol implementation (TDD: test first)

- [x] 3.1 Test `JsonRpcProtocol.ParseBody`: valid single call → 1-element slice with correct Procedure, ID, HasID=true
- [x] 3.2 Test `JsonRpcProtocol.ParseBody`: valid batch (3 calls) → 3-element slice
- [x] 3.3 Test `JsonRpcProtocol.ParseBody`: notification (no id) → HasID=false, call still in slice
- [x] 3.4 Test `JsonRpcProtocol.ParseBody`: call with null id → HasID=false (null means no response per spec)
- [x] 3.5 Test `JsonRpcProtocol.ParseBody`: invalid JSON body → error
- [x] 3.6 Test `JsonRpcProtocol.ParseBody`: missing `jsonrpc` → error
- [x] 3.7 Test `JsonRpcProtocol.ParseBody`: missing `method` → error
- [x] 3.8 Test `JsonRpcProtocol.ParseBody`: wrong `jsonrpc` version ("1.0") → error
- [x] 3.9 Test `JsonRpcProtocol.ParseBody`: empty batch array → empty slice, no error
- [x] 3.10 Test `JsonRpcProtocol.ParseBody`: uses configurable `procedure.call` path to extract procedure name
- [x] 3.11 Test `JsonRpcProtocol.ErrorResponse`: format for -32700, -32600, -32601 with various ids (number, string, null)
- [x] 3.12 Test `JsonRpcProtocol.ContentType`: returns configured or default "application/json"

## 4. Unit tests for RPC server handler (TDD: test first)

- [x] 4.1 Test handler single call: protocol parses, dispatches to correct procedure, pipeline returns body, response written with correct Content-Type
- [x] 4.2 Test handler: method not found → calls ErrorResponse(-32601), writes error body
- [x] 4.3 Test handler: parse error → writes error body without calling pipeline
- [x] 4.4 Test handler: batch (3 calls) → 3 pipeline calls with per-call bodies, array response written
- [x] 4.5 Test handler: batch with notification → notification runs pipeline (side effects), not in response array
- [x] 4.6 Test handler: all-notification batch → empty JSON array written
- [x] 4.7 Test handler: per-call RequestSource Body is the individual call object, not the batch array
- [x] 4.8 Test handler: single notification → pipeline runs, HTTP 204 No Content
- [x] 4.9 Test handler: response headers from example are included in HTTP response
- [x] 4.10 Test handler: response status code from example is propagated

## 5. Implement loader RPC config parsing and mapping

- [x] 5.1 Implement `ParseRpcConfig(spec *openapi3.T) (*RpcConfig, error)` in `internal/loader/rpc.go`
- [x] 5.2 Implement `BuildRpcMappings(infos []SchemaInfo) ([]*RpcRouteMapping, error)` in `internal/loader/rpc.go`
- [x] 5.3 Implement `procedure.match` resolver: split on first dot → HTTP method + field name; validate method is POST; read `operation.OperationID`
- [x] 5.4 Wire into server initialization: call `ParseRpcConfig` + `BuildRpcMappings` during `New`/`NewWithDependencies`
- [x] 5.5 Skip HTTP route registration for paths under gateway (coexistence: only register RPC handler for those paths)
- [x] 5.6 Verify loader unit tests pass (phase 2 tests)

## 6. Implement JSON-RPC protocol

- [x] 6.1 Implement `NewJsonRpcProtocol(cfg *RpcConfig) server.RpcProtocol` in `internal/server/jsonrpc_protocol.go`
- [x] 6.2 Implement `ParseBody`: JSON decode, detect array vs object, validate jsonrpc, extract method via `procedure.call`, return `[]RpcCall`
- [x] 6.3 Implement `ErrorResponse`: format `{"jsonrpc":"2.0","error":{"code":-32601,"message":"..."},"id":...}`
- [x] 6.4 Implement `ContentType`: return configured or "application/json"
- [x] 6.5 Implement `procedure.call` resolver: split on dots, traverse body map to extract procedure name
- [x] 6.6 Verify protocol unit tests pass (phase 3 tests)

## 7. Refactor server response generation

- [x] 7.1 Extract method `selectAndGenerateResponse(r *http.Request, mapping *RouteMapping, pathParams map[string]string, callBody any) (body []byte, headers map[string]string, statusCode string, mediaType string, err error)` from `handleMockRequestWithMapping`
- [x] 7.2 Rewrite `handleMockRequestWithMapping` to call extracted method, then write to `http.ResponseWriter`
- [x] 7.3 Run existing `internal/server/server_test.go` — all must pass with no changes

## 8. Implement RPC server handler

- [x] 8.1 Implement `RpcHandler.ServeHTTP` in `internal/server/jsonrpc.go`
- [x] 8.2 Body read + protocol.ParseBody + per-call dispatch loop
- [x] 8.3 Per-call: look up procedure → mapping; if not found → protocol.ErrorResponse(-32601); if found → selectAndGenerateResponse
- [x] 8.4 Per-call: construct `runtime.RequestSource` with `Body: call.Raw`
- [x] 8.5 Result collection: skip calls with HasID=false
- [x] 8.6 Final assembly: batch (detected in handler) → `[...]` array; single → object; all notifications → `[]` or 204
- [x] 8.7 Register gateway route in `setupRouter`: `r.Post(gatewayPath, server.makeRpcHandler(cfg))`
- [x] 8.8 Select `RpcProtocol` implementation based on `protocolType` (factory function)
- [x] 8.9 Verify server handler unit tests pass (phase 4 tests)

## 9. Integration tests

- [x] 9.1 Create test fixture OAS with `x-rpc` and multiple operationId under gateway in `test/_shared/resources/`
- [x] 9.2 Create test fixture OAS with both RPC and non-RPC paths in `test/_shared/resources/`
- [x] 9.3 Test: start server with RPC spec → single JSON-RPC call → correct example returned with `{$request.body.id}` echoed
- [x] 9.4 Test: batch of 3 calls (1 valid, 1 unknown method, 1 notification) → correct mixed response array
- [x] 9.5 Test: notification → state update via `x-mock-set-state` fires, HTTP 204 returned
- [x] 9.6 Test: method not found → -32601 error object with correct id
- [x] 9.7 Test: parse error (invalid JSON) → -32700 error
- [x] 9.8 Test: coexistence — same spec has RPC gateway + regular HTTP `/users` → both endpoints work independently
- [x] 9.9 Test: schema prefix (`--prefix /api`) + `gateway: /rpc` → gateway at `/api/rpc`
- [x] 9.10 Test: `x-mock-once` with RPC → example disposed after first matching call
- [x] 9.11 Test: `x-mock-skip` with RPC → skipped example never used
- [x] 9.12 Test: `x-mock-headers` with RPC → response includes evaluated headers
- [x] 9.13 Test: `x-mock-params-match` with RPC → per-call conditions evaluated against call params

## 10. Documentation and coverage

- [x] 10.1 Create `docs/json-rpc.md` with `x-rpc` schema reference, config fields, and usage examples
- [x] 10.2 Update `docs/extensions.md` with `x-rpc` entry and JSON-RPC example patterns
- [x] 10.3 Update `docs/architecture.md` to show RPC routing in the request flow
- [x] 10.4 Run full test suite and confirm 70% coverage threshold is maintained
- [x] 10.5 Verify all RS.JRP.* scenarios from `specs/json-rpc/spec.md` have corresponding tests
