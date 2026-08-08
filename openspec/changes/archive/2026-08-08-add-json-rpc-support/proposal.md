## Why

OASMock currently routes exclusively by URL path + HTTP method. JSON-RPC 2.0 — widely used by Ethereum clients, LSP servers, and many other APIs — sends all calls to a single POST endpoint and dispatches by the `method` field in the request body. It also supports batch (array of calls → array of responses) and notifications (calls with no id, no response). Adding extensible RPC support closes this gap with a protocol-agnostic foundation.

## What Changes

- **New capability**: Root-level `x-rpc` extension that declares an RPC gateway endpoint with protocol type, content type, and procedure-to-spec matching rules. Initially supports `protocolType: json-rpc` (2.0).
- **New capability**: Catch-all POST handler at the gateway that parses the request body, dispatches by procedure name to the matching OpenAPI operation, and reuses the existing example-selection/extension/state pipeline.
- **New capability**: JSON-RPC batch (array body → array response), notifications (no id → no response entry), and standard JSON-RPC 2.0 error codes (-32700, -32600, -32601).
- **Modified capability**: `mock-server-core` — extract reusable response-body generation from the HTTP handler so both HTTP and RPC paths share the same example selection, expression evaluation, and extension processing pipeline.
- **Modified capability**: `extensions` — documentation-only; existing `x-mock-*` and `{$request.body.*}` expressions work unchanged with RPC calls.

## Capabilities

### New Capabilities

- `json-rpc`: JSON-RPC 2.0 over HTTP POST — structured `x-rpc` extension, single-call dispatch, batch, notifications, standard errors, per-call runtime expression evaluation.

### Modified Capabilities

- `mock-server-core`: Response generation extracted from `handleMockRequestWithMapping` into a reusable function. No changes to HTTP routing, example selection, or extension behavior — purely structural refactor to enable RPC handler reuse.
- `extensions`: No behavioral changes — documentation-only update with JSON-RPC example patterns to show usage of `{$request.body.id}`, `{$request.body.method}`, and `{$request.body.params.*}` in RPC contexts.

## Impact

- **Code**: New interfaces (`RpcProtocol`, `RpcCall`), new file `internal/server/jsonrpc.go` plus protocol implementation, new loader file `internal/loader/rpc.go`, and a small refactor in `internal/server/server.go`.
- **APIs**: No CLI or management API changes — `x-rpc` is a schema-level extension detected at load time.
- **Dependencies**: None — uses existing `kin-openapi` (root extensions preserved as `map[string]any`), standard library `encoding/json`, and existing `runtime.RequestSource`.
- **Testing**: Unit tests (TDD order — interfaces first, then implementation), integration tests under `test/jsonrpc/`. Existing HTTP tests must pass after the refactor.
- **Documentation**: New `docs/json-rpc.md`; update `docs/extensions.md` with `x-rpc` schema reference and RPC example patterns.
