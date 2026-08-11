## Why

Dynamic examples added via the management API currently live indefinitely or until matched once (`once: true`). There is no time-based expiration, which limits testing scenarios where mock data should only be valid for a specific window — e.g., simulating short-lived tokens, time-limited resources, or eventual consistency patterns.

## What Changes

- Add optional `ttl` (time-to-live) field to `AddExampleRequest` — a non-negative integer in seconds. Zero or omitted means no expiration.
- Store creation timestamp alongside dynamic examples on addition.
- Expired examples are skipped during request matching (excluded from selection, same as `once`-consumed examples).
- A background goroutine periodically sweeps expired examples from the `dynamicExamples` map and cleans up their `onceExamples` entries, logging each removal at debug level.
- The sweep interval is fixed (e.g., 1 second) with low-resource design: a single timer tick, coarse-grained locking per route key, no per-request overhead.

## Capabilities

### New Capabilities
None

### Modified Capabilities
- `management-api`: `AddExampleRequest` gains optional `ttl` field (non-negative integer seconds, 0 = no expiration)
- `mock-server-core`: Dynamic example TTL expiration (lazy skip during selection) and background cleanup goroutine that removes expired entries

## Impact

- **API**: `POST /_mock/examples` accepts new optional `ttl` field; `AddExampleRequest` schema updated
- **Data structures**: `dynamicExample` struct gains `addedAt` timestamp; `Server` struct gains cleanup goroutine control
- **Concurrency**: Existing `dyMu` (RWMutex) and `onceMu` (RWMutex) protect cleanup; no new mutex needed
- **Performance**: Per-request path unaffected (expiry check is a single `time.Now().After()` comparison, same cost as existing `once` check); background sweep is lightweight with configurable tick
- **Memory**: Cleanup ensures `dynamicExamples` slices and `onceExamples` map don't grow unbounded for TTL examples
