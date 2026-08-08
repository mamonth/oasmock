# OASMock OpenAPI Extensions

This document describes the custom OpenAPI extensions used by OASMock.

## x-mock-match

**Location**: OAS example object

**Purpose**: Provides ability to pick examples by a set of conditions. Keys can be runtime expressions, values can be literal values or JSON schemas.

**Example**:
```yaml
examples:
  first:
    x-mock-match:
      '{$request.query.id}': 12
      '{$request.query.limit}':
        type: number
        minimum: 1
        maximum: 3
    # ... example data
```

**Alias**: x-mock-params-match — deprecated, kept for backward compatibility

## x-mock-skip

**Location**: OAS example object

**Purpose**: Skips the example from mocking.

**Example**:
```yaml
examples:
  first:
    x-mock-skip: true
    # ... example data
```

## x-mock-set-state

**Location**: OAS example object

**Purpose**: Sets server state that can be used later as a condition in `x-mock-match`. Runtime expressions are available in keys and values.

**Example**:
```yaml
examples:
  first:
    x-mock-set-state:
      state-plain-key: '{$request.body.param}'
      'state-mixed-{$request.cookie.some}': plain value
      '{$request.cookie.some}': plain value
      state-obj-key:
        value:
          complex-param: complex value
      state-counter-key:
        increment: 1
      deleted-state-key: null
```

## x-mock-headers

**Location**: OAS example object

**Purpose**: Sets headers for a concrete example. Runtime expressions are available in values.

**Example**:
```yaml
examples:
  first:
    x-mock-headers:
      Location: '{$request.query.backUrl}'
      Set-Cookie:
        - 'cookie-name={$request.body.param};'
        - 'cookie-name=second cookie;'
```

## x-mock-once

**Location**: OAS example object

**Purpose**: Makes the marked example one‑time (disposed after first match).

**Example**:
```yaml
examples:
  first:
    x-mock-once: true
    # ... example data
```

## x-rpc

**Location**: OpenAPI document root

**Purpose**: Configures an RPC gateway endpoint for routing calls by body field instead of URL path. Currently supports JSON-RPC 2.0.

**Minimal example**:
```yaml
x-rpc:
  gateway: /rpc
  protocolType: json-rpc
```

When `x-rpc` is present, all POST operations under the gateway path are treated as RPC procedures. The procedure name is derived from the field specified at `x-rpc.procedure.match`. Requests are dispatched by matching the field specified in `x-rpc.procedure.call` in the JSON-RPC request body against the procedure name.

For JSON-RPC contexts, `{$request.body.id}`, `{$request.body.method}`, and `{$request.body.params.*}` expressions evaluate against the individual call object (not the batch array), enabling per-call resolution in batch requests.

See [JSON-RPC Documentation](json-rpc.md) for full details.

# Runtime Expressions

Runtime expressions like `{$request.path.id}` are evaluated inside keys and values of mock extensions. Dot as part of a property name must be escaped: `{$request.cookie.dot\.dot}`.

Value modifiers can be specified after a `|` sign. Example: `{$request.path.param|default:not-found}`.

### Custom Modifiers

| Modifier     | Description                                                                 | Example                                           |
|--------------|-----------------------------------------------------------------------------|---------------------------------------------------|
| `default`    | Returns a default value if the provided data is empty.                      | `{$request.path.param\|default:some default value}` |
| `getByPath`  | Returns part of an object or array by a dot‑separated path.                 | `{$state.someObject\|getByPath:some.example.array.last}` |
| `toJWT`      | (stub) Returns a placeholder JWT‑like string.                               | `{$state.someObject\|toJWT}`                      |
### Available Data

| Expression example                     | Description                                         |
|----------------------------------------|-----------------------------------------------------|
| `{$request.path.param}`                | Path parameters (declared in routes)                |
| `{$request.query.param}`               | Query string parameters                             |
| `{$request.header.header-name}`        | Request headers                                     |
| `{$request.body.param}`                | Data from request body (JSON or form)               |
| `{$request.cookie.cookieName}`         | Parsed request cookies                              |
| `{$state.someSavedParam}`              | State data (set previously with `x-mock-set-state`) |
| `{$env.ENV_VAR}`                       | Runtime environment variables                       |
