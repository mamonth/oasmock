# SDK Specification

The OASMock provides a TypeScript/JavaScript SDK for programmatic interaction with the mock server. The SDK communicates via the HTTP API described in [openapi.yaml](./openapi.yaml).

## Installation

```bash
npm install @oasmock/sdk
```

## Basic Usage

```typescript
import { MockSDK } from '@oasmock/sdk';

const mockSDK = new MockSDK('http://localhost:19191');

// One‑time mocking of a response
await mockSDK.onRequest('/some/url', 'POST').respondWithOnce({
  body: { some: 'data' },
});

// Mocking with additional conditions
await mockSDK.onRequest('/some/url')
  .withCookies({ uid: '1' })
  .withHeaders({ authorization: 'Bearer token' })
  .withSearchParams({ test: 'value' })
  .respondWith({
    body: {},
  });

// Retrieve request history
const lastRequest = await mockSDK.getLastRequest({
  path: '/some/url',
  method: 'POST',
});
```

## API Reference

### Class `MockSDK`

#### Constructor

```typescript
constructor(baseUrl: string)
```

Creates a new SDK instance pointing to the mock server at `baseUrl`.

#### Methods

##### `onRequest(path: string, method?: TMethod): MockSDKRequest`

Returns a `MockSDKRequest` builder for the given path and HTTP method (default `'GET'`).

##### `setExample(conditions: IRequestConditions, exampleData: IResponseData): Promise<string>`

Low‑level method to add a mock example. Returns a promise that resolves with the example ID.

##### `getRequestList(options?: IGetRequestHistoryOptions): Promise<IRequestHistoryItem[]>`

Retrieves the request history, optionally filtered by the provided options.

##### `getLastRequest(options?: IGetRequestHistoryOptions): Promise<IRequestHistoryItem | undefined>`

Retrieves the most recent matching request from the history.

##### Static `toConditionKey(type: string, value: string): string`

Utility to create a condition key from a condition type and value (e.g., `'query'`, `'id'` → `'{$request.query.id}'`).

### Class `MockSDKRequest`

Builder for defining a mock example with chainable condition methods.

#### Properties

- `conditions: IRequestConditions` – the accumulated conditions.
- `readonly sdk: MockSDK` – reference to the parent SDK instance.

#### Methods

##### `withHeaders(headers: Record<string, string | string[]>): this`

Adds header conditions.

##### `withSearchParams(params: URLSearchParams | string | Record<string, string | readonly string[]> | Iterable<[string, string]>): this`

Adds query parameter conditions.

##### `withCookies(cookies: Record<string, string>): this`

Adds cookie conditions.

##### `respondWith(responseData: IResponseData): Promise<string>`

Adds the example with the given response data. Returns the example ID.

##### `respondWithOnce(responseData: IResponseData): Promise<string>`

Adds a one‑time example (removed after first match). Returns the example ID.

## Type Definitions

```typescript
type TMethod = 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE' | 'HEAD' | 'OPTIONS';

interface IResponseData {
  once?: boolean;
  validate?: boolean;
  code?: number;
  body?: string | object;
  headers?: Record<string, string | string[]>;
}

interface IRequestConditions {
  path: string;
  method?: TMethod;
  [key: string]: any;
}

interface IRequestHistoryItem {
  ts: number;
  url: string;
  method: TMethod;
  body?: string | object;
  headers: Record<string, string | string[]>;
}

interface IGetRequestHistoryOptions {
  path?: string;
  method?: TMethod;
  limit?: number;
  offset?: number;
  since?: number | string;  // milliseconds or relative string (e.g. '-10s')
  till?: number | string;   // milliseconds or relative string (e.g. '+2h')
}
```

## Error Handling

All methods return promises that reject with an `Error` if the HTTP request fails or the server returns an error response.
