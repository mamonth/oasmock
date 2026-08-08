# JSON-RPC 2.0 Support

OASMock supports JSON-RPC 2.0 via the `x-rpc` root-level OpenAPI extension. All JSON-RPC calls are routed to a single gateway endpoint and dispatched by the procedure name (the `method` field in the request body).

## x-rpc Extension

Add the `x-rpc` extension at the document root of your OpenAPI spec:

```yaml
openapi: "3.0.3"
x-rpc:
  gateway: /rpc
  protocolType: json-rpc
  contentType: application/json   # optional, defaults per protocol
  procedure:
    call: method                   # body property containing the procedure name
    match: post.operationId        # spec operation field to match against
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `gateway` | Yes | Path prefix for the RPC endpoint. All RPC operations should be defined under this path. |
| `protocolType` | Yes | RPC protocol to use. Currently only `json-rpc` is supported. |
| `contentType` | No | Content type for responses. Defaults to `application/json` for JSON-RPC. |
| `procedure.call` | Yes | Dot-separated path in the request body to extract the procedure name. For JSON-RPC, this is `method`. |
| `procedure.match` | Yes | How procedure names are derived from spec operations. Format: `{httpMethod}.{field}`. Only `post.operationId` is currently supported. |

## Defining Procedures

Procedures are defined as POST operations under the gateway path. The procedure name is derived from the operation's `operationId`:

```yaml
paths:
  /rpc/subtract:
    post:
      operationId: subtract
      requestBody:
        content:
          application/json:
            schema:
              type: object
      responses:
        "200":
          description: OK
          content:
            application/json:
              examples:
                default:
                  value:
                    jsonrpc: "2.0"
                    result: "{$request.body.params.a} - {$request.body.params.b}"
                    id: "{$request.body.id}"
```

## Runtime Expressions in RPC Context

All standard runtime expressions work within RPC examples. The `{$request.body.*}` source references the **individual call object** (not the batch array), enabling per-call expression resolution:

- `{$request.body.id}` — the request id
- `{$request.body.method}` — the procedure name
- `{$request.body.params.*}` — call parameters (supports nested paths)
- `{$request.body.jsonrpc}` — protocol version

## Batch Support

JSON-RPC batch requests (array body) are supported. Each call is processed individually through the example selection pipeline, and responses are collected into an array:

**Request:**
```json
[
  {"jsonrpc":"2.0","method":"subtract","params":{"a":10,"b":3},"id":1},
  {"jsonrpc":"2.0","method":"add","params":{"a":1,"b":2},"id":2}
]
```

**Response:**
```json
[
  {"jsonrpc":"2.0","result":"10 - 3","id":1},
  {"jsonrpc":"2.0","result":"1 + 2 = sum","id":2}
]
```

## Notifications

JSON-RPC notifications (calls without an `id` field) are processed through the pipeline to trigger side effects (`x-mock-set-state`, history recording), but generate no response entry:

- Single notification → HTTP 204 No Content
- Notification in a batch → excluded from the response array
- All-notification batch → empty JSON array `[]`

## Error Responses

Standard JSON-RPC 2.0 error codes are returned for protocol-level errors:

| Code | Message | Condition |
|------|---------|-----------|
| -32700 | Parse error | Request body is not valid JSON |
| -32600 | Invalid Request | Missing or invalid `jsonrpc`/`method` fields |
| -32601 | Method not found | Procedure name not found in the operation map |
| -32603 | Internal error | Pipeline execution error |

## Coexistence with HTTP Routes

A single OpenAPI spec can contain both RPC procedures and normal HTTP routes. Paths under the gateway are served by the RPC handler; all other paths are served by the normal HTTP handler.

## CLI Usage

Start the server with a spec containing `x-rpc`:

```bash
oasmock mock --schema spec.yaml
```

With a schema prefix:

```bash
oasmock mock --schema spec.yaml --prefix /api
# Gateway available at POST /api/rpc
```

All existing flags (`--port`, `--delay`, `--verbose`, `--cors`, `--history-size`, `--control-api`) work identically with RPC-enabled specs.
