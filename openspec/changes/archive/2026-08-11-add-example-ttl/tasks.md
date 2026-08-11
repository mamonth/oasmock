## 1. Data Model

- [x] 1.1 Add `addedAt time.Time` field to `dynamicExample` struct in `internal/server/server_example.go`
- [x] 1.2 Add `sweepCtx context.Context` and `sweepCancel context.CancelFunc` fields to `Server` struct in `internal/server/server.go` for goroutine lifecycle control

## 2. API Schema

- [x] 2.1 Add `ttl` field (integer, non-negative, optional, description) to `AddExampleRequest` schema in `api/openapi.yaml`
- [x] 2.2 Update `addExampleRequestSchema` JSON Schema in `internal/server/server_management.go` to include optional `ttl` property

## 3. Add Example Handler

- [x] 3.1 Parse `ttl` field from request body in `handleAddExample` (`internal/server/server_management.go`)
- [x] 3.2 Validate `ttl >= 0`; reject negative values with HTTP 400
- [x] 3.3 Set `example.addedAt = time.Now()` when TTL is specified (> 0); use zero value when TTL is 0 or omitted

## 4. Lazy Expiry Check

- [x] 4.1 Implement `isExpired(dynamicExample) bool` helper in `internal/server/server_example.go`
- [x] 4.2 Add expired check in `selectDynamicExample` loop — skip expired examples same as `once`-consumed ones
- [x] 4.3 Log skipped expired examples at debug level (consistent with existing verbose logging pattern)

## 5. Background Sweep

- [x] 5.1 Implement `sweepExpiredExamples()` method on `Server` — iterates `dynamicExamples` under `dyMu.Lock()`, removes expired entries, cleans `onceExamples` under `onceMu.Lock()`
- [x] 5.2 Implement `startTTLSweep()` — launches a goroutine with `time.Ticker` (1s interval), selects on ticker and `sweepCtx.Done()`
- [x] 5.3 Log each removed example at debug level with route key, index, and TTL
- [x] 5.4 Compact remaining slice after removing expired entries (fresh-slice allocation, not in-place — in-place compaction races with `selectDynamicExample`'s slice iteration; covered by `TestConcurrentSelectAndSweepNoDataRace`)

## 6. Server Lifecycle

- [x] 6.1 Initialize sweep context/cancel in `NewWithDependencies` (`internal/server/server.go`)
- [x] 6.2 Call `startTTLSweep()` at the end of `NewWithDependencies` after `setupRouter`
- [x] 6.3 Call `sweepCancel()` in `Server.Shutdown()` before HTTP server shutdown
- [x] 6.4 Make `Server.Shutdown()` idempotent (`shutdownOnce`) and race-safe with `Start()` (guard `httpServer` with `httpMu`); add race-regression tests (`TestConcurrentStartAndShutdownNoDataRace`, `TestShutdownIsIdempotent`) and call `Shutdown` via `t.Cleanup` in `newMockedServerWithGeneratedMocks`

## 7. Unit Tests

- [x] 7.1 Test `selectDynamicExample` skips expired examples and returns non-expired ones (scenarios RS.MSC.40, RS.MSC.41, RS.MSC.42)
- [x] 7.2 Test TTL and `once` flag interaction — consumed example skipped regardless of TTL (scenario RS.MSC.43)
- [x] 7.3 Test `sweepExpiredExamples` removes only expired entries, preserves non-expired and no-TTL examples (scenarios RS.MSC.44, RS.MSC.46)
- [x] 7.4 Test `sweepExpiredExamples` cleans `onceExamples` entries for swept examples (scenario RS.MSC.45)
- [x] 7.5 Test `handleAddExample` rejects negative `ttl` (scenario RS.MAPI.17)
- [x] 7.6 Test `handleAddExample` accepts positive `ttl`, zero `ttl`, and omitted `ttl` (scenarios RS.MAPI.16, RS.MAPI.18)
- [x] 7.7 Test sweep goroutine starts on server creation and stops on shutdown (scenarios RS.MSC.48, RS.MSC.49)

## 8. Integration Tests

- [x] 8.1 Test full flow: add example with TTL via API, verify it's returned before expiry, verify it's not returned after expiry
- [x] 8.2 Test that debug logs contain TTL expiry messages when verbose mode is enabled (scenario RS.MSC.47)
- [x] 8.3 Update existing integration tests if `AddExampleRequest` validation logic changes
