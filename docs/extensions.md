# OASMock OpenAPI Extensions

This document describes the custom OpenAPI extensions used by the OpenApi Mock tool.

## x-mock-match

**Location**: OAS example object

**Purpose**: Provides ability to pick examples by a set of conditions. Keys can be runtime expressions, values can be literal values or JSON schemas.

**Example**:
```yaml
examples:
  first:
    x-mock-params-match:
      '{$request.query.id}': 12
      '{$request.query.limit}':
        type: number
        minimum: 1
        maximum: 3
    # ... example data
```

**Alias**: x-mock-params-match - deprecated, only for backward compatibility

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

**Purpose**: Sets server state that can be used later as a condition in `x-mock-params-match`. Runtime expressions are available in keys and values.

**Example**:
```yaml
examples:
  first:
    x-mock-set-state:
      state-plain-key: '{$request.body.param}'
      'state-mixed-{$request.cookie.some}': plain value,
      '{$request.cookie.some}': plain value
      state-obj-key:
        value:
          complex-param: complex value
      state-counter-key:
        increment: 1
      deleted-state-key: null
```

### x-mock-headers

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

### x-mock-once

**Location**: OAS example object

**Purpose**: Makes the marked example one‑time (disposed after first match).

**Example**:
```yaml
examples:
  first:
    x-mock-once: true
    # ... example data
```

## Runtime Expressions

Runtime expressions like `{$request.url}` are evaluated inside keys and values of mock extensions. Dot as part of a property name must be escaped: `{$request.cookie.dot\.dot}`.

Value modifiers can be specified after a `|` sign. Example: `{$request.path.param|encodeURIComponent}`.

### Custom Modifiers

| Modifier     | Description                                                                 | Example                                           |
|--------------|-----------------------------------------------------------------------------|---------------------------------------------------|
| `default`    | Returns a default value if the provided data is empty.                      | `{$request.path.param\|default:some default value}` |
| `getByPath`  | Returns part of an object or array by a dot‑separated path.                 | `{$state.someObject\|getByPath:some.example.array.last}` |
| `toJWT`      | Packs the provided object into JWT format (expires in 1h, aud="mock‑client"). | `{$state.someObject\|toJWT}`                      |
| `getJWKn`    | *(To be documented)*                                                        |                                                   |
| `getJWKe`    | *(To be documented)*                                                        |                                                   |

### Available Data

| Expression example            | Description                                         | Value Example                                  |
|-------------------------------|-----------------------------------------------------|------------------------------------------------|
| `$url`                        | Full request URL                                    | `https://example.org/api/pathParamValue/?param=value` |
| `$method`                     | Request method                                      | `POST`                                         |
| `$request.path.param`         | Path parameters (declared in routes)                | `pathParamValue`                               |
| `$request.query.param`        | Query string parameters                             | `value`                                        |
| `$request.header.header-name` | Request headers                                     | `application/json`                             |
| `$request.body.param`         | Data from request body (JSON or form)               | `some body data`                               |
| `$request.cookie.cookieName`  | Parsed request cookies                              | `value from cookie`                            |
| `$state.someSavedParam`       | State data (set previously with `x-mock-set-state`) | `param saved to state`                         |
| `$env.ENV_VAR`                | Runtime environment variables                       | `value from env`                           |

