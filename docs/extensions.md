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

### Event-context matching (async-driven examples)

On an AsyncAPI message example, `x-mock-match` selects the example against the event context instead of a request context. The event context exposes:

| Expression         | Description                                                       |
|--------------------|-------------------------------------------------------------------|
| `{$event.name}`    | Event identity: the named-event name or the built-in kind (`connect`/`receive`) |
| `{$event.data}`    | The whole event payload                                           |
| `{$event.<field>}` | A single event payload field                                      |

`name` and `data` are reserved metadata names; payload fields with those names are shadowed (reachable via `{$event.data.<field>}`).

```yaml
examples:
  orderAlert:
    payload:
      severity: '{$event.priority}'
    x-mock-match:
      '{$event.name}': orderCreated
```

An example whose `x-mock-match` references `{$event.*}` is classified at load as event-driven. Mixing `{$event.*}` with `{$request.*}`/`{$message.*}`/`{$channel.*}` in the same match, or declaring both `x-mock-interval` and `x-mock-match`, is rejected at load.

When a match pins the identity (`{$event.name}`), the condition value **must be a literal string**: an expression value (e.g. `{$state.envName}`) is rejected at load, because the produced subscription key could never match a fired identity. A match that references the event context without an `{$event.name}` condition (e.g. `'{$connection.id}': '{$event.connectionId'}`) still registers as an event-driven example with a **wildcard identity** that evaluates against every fired event.

Condition **values** follow the context of their key: on an event/connection key a full-expression value (e.g. `'{$connection.id}': '{$event.connectionId}'`) is resolved before comparison, while values on the reply-path contexts (`{$request.*}`/`{$message.*}`/`{$channel.*}`) are always compared literally.

### Per-connection recipient matching (`{$connection.*}`)

For event-driven examples, conditions referencing the connection context become a **per-connection recipient filter** evaluated at delivery time in two phases: non-connection conditions are evaluated once per emission, and `{$connection.*}` conditions are evaluated against each candidate consumer. Only consumers satisfying both phases receive the message. When no condition references `{$connection.*}`, delivery broadcasts to all consumers of the channel.

| Expression                         | Description                                          |
|------------------------------------|------------------------------------------------------|
| `{$connection.id}`                 | Consumer connection id                               |
| `{$connection.channel}`            | Channel address the consumer connected to            |
| `{$connection.query.<key>}`        | Query parameter captured at upgrade                  |
| `{$connection.header.<key>}`       | Request header captured at upgrade (lower-cased)     |

```yaml
examples:
  targeted:
    payload:
      ring: '{$event.data}'
    x-mock-match:
      '{$event.name}': orderCreated
      '{$connection.id}': '{$event.connectionId}'
```

A condition referencing `{$connection.*}` on a reply-path example (no connection context available) never matches, and in verbose mode a warning is logged.

### x-mock-interval / x-mock-delay

**Location**: AsyncAPI message example object

Timing-only sibling extensions that keep `x-mock-match` pure. Neither is an event identity. A periodically driven example is single-trigger: declaring `x-mock-interval` together with any `x-mock-match` is **rejected at load** (a periodic emission has no match context to honor — periodicity and selection are mutually exclusive).

- `x-mock-interval`: positive integer milliseconds marking a **periodically driven** example — the message is emitted repeatedly at that cadence until removed or the server shuts down.

```yaml
examples:
  ticker:
    payload:
      seq: '{$state.counter}'
    x-mock-interval: 1000
```

- `x-mock-delay`: integer milliseconds (default 0) delaying emission after an event fire.

```yaml
examples:
  welcome:
    payload:
      msg: hello
    x-mock-match:
      '{$event.name}': connect
    x-mock-delay: 150
```

Timing values are integer milliseconds: a fractional value (e.g. `x-mock-interval: 2.5`) is rejected at load instead of being silently truncated. Periodically driven examples honor `x-mock-skip` like every other example and are not emitted while it is set.

### Runtime matches and timing (management API)

`POST /_mock/examples` mirrors the extensions for AsyncAPI targets with `match`, `interval` and `delay` fields; the same classification and delivery rules apply. See `api/openapi.yaml`.

> **Not idempotent**: every successful `POST /_mock/examples` registers a distinct example (and, for interval targets, a separate delivery job). Re-sending a request after a lost response creates a second subscription; keep the returned `id` and stop an interval example with `DELETE /_mock/examples/{id}`.

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
| `{$event.name}` / `{$event.data}` / `{$event.<field>}` | Event identity, whole payload, and payload fields (async-driven examples) |
| `{$connection.id}` / `{$connection.channel}` / `{$connection.query.<key>}` / `{$connection.header.<key>}` | Per-connection recipient matching (event delivery) |
