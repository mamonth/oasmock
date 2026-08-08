## Purpose

JSON-RPC 2.0 gateway support that configures an RPC endpoint via the root-level `x-rpc` extension, derives procedure names from gateway operations, parses single and batch JSON-RPC bodies, dispatches calls to matching operations, handles notifications, and reuses all existing `x-mock-*` extensions.

## Requirements

### Requirement: x-rpc root extension
The mock server SHALL detect the root-level `x-rpc` extension to configure an RPC gateway endpoint.

#### Scenario RS.JRP.1: Detecting x-rpc extension
- **WHEN** an OpenAPI spec has `x-rpc` with `gateway`, `protocolType`, and `procedure` fields at the document root
- **THEN** the server parses the configuration and registers a gateway handler at the specified path

#### Scenario RS.JRP.2: Spec without x-rpc
- **WHEN** an OpenAPI spec does NOT have `x-rpc`
- **THEN** no RPC handler is registered and routing continues as normal HTTP path-based

#### Scenario RS.JRP.3: Invalid x-rpc — missing gateway
- **WHEN** `x-rpc` is present but `gateway` field is missing or empty
- **THEN** the server fails to start with an error

#### Scenario RS.JRP.4: Invalid x-rpc — missing procedure
- **WHEN** `x-rpc` is present but `procedure` field is missing
- **THEN** the server fails to start with an error

#### Scenario RS.JRP.5: Invalid x-rpc — unsupported protocolType
- **WHEN** `x-rpc` specifies a `protocolType` that is not supported (e.g., "xml-rpc")
- **THEN** the server fails to start with an error

### Requirement: Procedure name extraction from spec
The mock server SHALL derive procedure names from OpenAPI operations under the gateway using `procedure.match`. The match field specifies `{httpMethod}.{operationProperty}`. Only `post.operationId` is supported initially.

#### Scenario RS.JRP.6: Building procedure mappings
- **WHEN** the gateway is `/rpc` and the spec defines POST operations at `/rpc/subtract` (operationId: "subtract") and `/rpc/add` (operationId: "add")
- **THEN** the server builds a procedure map: "subtract" → subtract RouteMapping, "add" → add RouteMapping

#### Scenario RS.JRP.7: Paths not under gateway excluded
- **WHEN** the gateway is `/rpc` and the spec has a POST operation at `/users` (not under the gateway)
- **THEN** that operation is NOT included in the RPC procedure map and is treated as a normal HTTP route

#### Scenario RS.JRP.8: Duplicate procedure names
- **WHEN** two POST operations under the gateway have the same `operationId`
- **THEN** the server fails to start with an error indicating the duplicate

#### Scenario RS.JRP.9: No POST operations under gateway
- **WHEN** the gateway path is valid but no POST operations exist under it
- **THEN** the server starts successfully with an empty procedure map (all calls receive method-not-found responses)

### Requirement: JSON-RPC 2.0 body parsing
The mock server SHALL parse incoming JSON-RPC request bodies according to the protocol configured by `protocolType: json-rpc` using the `procedure.call` field to extract procedure names.

#### Scenario RS.JRP.10: Single call parsing
- **WHEN** a POST body is `{"jsonrpc":"2.0","method":"subtract","params":{"a":10},"id":1}`
- **THEN** the server parses it as one call with procedure "subtract", params `{"a":10}`, id `1`, HasID=true

#### Scenario RS.JRP.11: Batch parsing
- **WHEN** a POST body is an array of two JSON-RPC call objects
- **THEN** the server parses it into two calls with correct fields per element

#### Scenario RS.JRP.12: Invalid JSON body
- **WHEN** the POST body is not valid JSON
- **THEN** the server responds with `{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"},"id":null}`

#### Scenario RS.JRP.13: Missing jsonrpc field
- **WHEN** the POST body is a valid JSON object but missing the `jsonrpc` field
- **THEN** the server responds with `{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid Request"},"id":null}`

#### Scenario RS.JRP.14: Missing method field
- **WHEN** the POST body has `jsonrpc: "2.0"` but missing `method`
- **THEN** the server responds with error -32600

#### Scenario RS.JRP.15: Wrong jsonrpc version
- **WHEN** the POST body has `jsonrpc: "1.0"`
- **THEN** the server responds with error -32600

#### Scenario RS.JRP.16: Procedure name extracted via procedure.call
- **WHEN** `procedure.call` is `"method"` and the request body has `"method": "subtract"`
- **THEN** the extracted procedure name is `"subtract"`

### Requirement: Single JSON-RPC call dispatch
The mock server SHALL route a single JSON-RPC call to the matching operation and return the example's response envelope.

#### Scenario RS.JRP.17: Dispatch by procedure name
- **WHEN** a POST arrives at the gateway with body `{"jsonrpc":"2.0","method":"subtract","params":{"a":10,"b":3},"id":1}` and the procedure map has "subtract"
- **THEN** the server dispatches to the matched operation, evaluates the example (including `{$request.body.id}`), and returns the example's full response envelope

#### Scenario RS.JRP.18: Method not found
- **WHEN** the `method` value does not match any `operationId` in the procedure map
- **THEN** the server responds with `{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":<request-id>}`

### Requirement: JSON-RPC batch processing
The mock server SHALL process batch JSON-RPC calls (array body) and return an array of responses.

#### Scenario RS.JRP.19: Batch processing
- **WHEN** a POST body is an array of two valid calls with different ids and methods
- **THEN** the response is a JSON array of two response envelopes with matching ids

#### Scenario RS.JRP.20: Batch with mixed success and error
- **WHEN** a batch contains one valid method and one unknown method
- **THEN** the response array contains one result envelope and one error envelope in the same order

#### Scenario RS.JRP.21: Batch with notification
- **WHEN** a batch contains one call with id and one call without id (notification)
- **THEN** the response array contains only the entry for the call with id

#### Scenario RS.JRP.22: All-notification batch
- **WHEN** all calls in a batch are notifications (no id)
- **THEN** the response body is an empty JSON array `[]`

### Requirement: JSON-RPC notification handling
The mock server SHALL process notification calls (no `id`) for side effects without returning a response entry.

#### Scenario RS.JRP.23: Single notification returns no content
- **WHEN** a POST body is `{"jsonrpc":"2.0","method":"log","params":{"message":"hello"}}` (no `id` field)
- **THEN** the server processes the call (applying `x-mock-set-state` etc.) and returns HTTP 204 No Content

#### Scenario RS.JRP.24: Notification applies side effects
- **WHEN** a notification matches an example with `x-mock-set-state`
- **THEN** the state update is applied even though no response body is returned

### Requirement: Per-call runtime expression resolution
The mock server SHALL evaluate `{$request.body.*}` against the individual call object (not the full batch array).

#### Scenario RS.JRP.25: Per-call body.id resolution in batch
- **WHEN** a batch includes two calls with ids 1 and 2
- **THEN** each response envelope echoes the correct id via `{$request.body.id}`

#### Scenario RS.JRP.26: Per-call body.params resolution
- **WHEN** a batch includes two calls with different params objects
- **THEN** each call's `x-mock-params-match` conditions evaluate against its own params object, not the batch array

### Requirement: Extension compatibility
All existing `x-mock-*` extensions SHALL work identically for JSON-RPC calls as for HTTP requests.

#### Scenario RS.JRP.27: x-mock-set-state with JSON-RPC
- **WHEN** a JSON-RPC call matches an example with `x-mock-set-state`
- **THEN** the server updates state as specified

#### Scenario RS.JRP.28: x-mock-once with JSON-RPC
- **WHEN** a JSON-RPC call matches an example with `x-mock-once: true`
- **THEN** the example is removed from future consideration for that procedure

#### Scenario RS.JRP.29: x-mock-headers with JSON-RPC
- **WHEN** a JSON-RPC call matches an example with `x-mock-headers`
- **THEN** the response includes those headers

#### Scenario RS.JRP.30: x-mock-skip with JSON-RPC
- **WHEN** an example has `x-mock-skip: true`
- **THEN** the server never uses that example for JSON-RPC responses

### Requirement: Coexistence with HTTP routes
The mock server SHALL serve RPC and non-RPC routes from the same spec simultaneously.

#### Scenario RS.JRP.31: RPC and HTTP routes coexist
- **WHEN** a spec has both `x-rpc.gateway: /rpc` with procedure operations AND a regular `/api/users` GET endpoint
- **THEN** POST to `/rpc` dispatches to RPC handler and GET to `/api/users` dispatches to normal HTTP handler

#### Scenario RS.JRP.32: Schema prefix applied to gateway
- **WHEN** the schema is loaded with `--prefix /api` and has `gateway: /rpc`
- **THEN** the gateway handler is registered at `/api/rpc`
