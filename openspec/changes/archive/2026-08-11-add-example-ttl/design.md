## Context

The dynamic example system (`internal/server/server_example.go`) stores examples in `map[string][]dynamicExample` keyed by route. Examples are matched against incoming requests in `selectDynamicExample` with optional `once` flag tracking via `onceExamples map[string]bool`. Currently, there is no time-based expiration — examples live until consumed (once) or indefinitely.

The user requires a CPU-friendly TTL mechanism with zero per-request overhead beyond a timestamp comparison, and memory usage may spike temporarily but must eventually be reclaimed.

## Goals / Non-Goals

**Goals:**
- Allow users to specify `ttl` (seconds) when adding a dynamic example via `POST /_mock/examples`
- Expired examples SHALL be skipped during request matching (lazy check)
- A background goroutine SHALL periodically sweep and remove expired examples from storage
- Each removed example SHALL be logged at debug level

**Non-Goals:**
- TTL for static (OpenAPI spec-defined) examples
- Per-example configurable sweep intervals
- Persistent storage of TTL values
- Configurable sweep tick via CLI
- A `DELETE` endpoint for manual example removal

## Decisions

### 1. Two-phase expiry: lazy check + periodic sweep

**Choice**: Check `addedAt + ttl > now` in `selectDynamicExample` (lazy), AND run a background goroutine with a 1-second `time.Ticker` to sweep expired entries.

**Rationale**: Lazy check prevents an expired example from ever being returned to a client (correctness). The sweep ensures memory is reclaimed for expired+unused examples (memory hygiene). A 1-second tick is low-frequency enough to avoid CPU overhead while keeping stale data lifetime bounded.

**Alternatives considered**:
- Timer-per-example (`time.AfterFunc`): O(n) goroutines for n examples, high memory and scheduler pressure.
- Sweep-only (no lazy check): A just-expired example could be returned between sweeps — violates correctness.
- Lazy-only (no sweep): `dynamicExamples` slices and `onceExamples` entries never cleaned up — unbounded memory growth.

### 2. Single background goroutine with coarse-grained iteration

**Choice**: One goroutine per `Server` instance. Each tick: acquire `dyMu.Lock()`, iterate route keys, filter expired entries, compact slices, hold `onceMu.Lock()` briefly to clean `onceExamples`, then release.

**Rationale**: Simplifies lifecycle management (start/stop via context cancellation). Coarse-grained locking means the sweep holds the write lock for a few milliseconds while iterating — negligible impact on request-serving goroutines that use `RLock()` for both `dyMu` and `onceMu`.

**Alternatives considered**:
- Per-key goroutine: Overkill for typical usage (tens to hundreds of examples).
- `sync.Map` with atomic deletion: No ordering guarantee for slices — iteration order matters for example precedence.

### 3. Timestamp stored per example, not per slice

**Choice**: `dynamicExample` gains `addedAt time.Time` field set at creation time.

**Rationale**: Each example has its own TTL; storing per-example is the natural granularity. No shared expiration timestamp needed.

### 4. `onceExamples` cleanup on TTL sweep

**Choice**: When an expired dynamic example is swept, also delete its corresponding `onceExamples` entry.

**Rationale**: Prevents `onceExamples` from accumulating entries for examples that no longer exist, keeping the map bounded.

### 5. Debug logging

**Choice**: Use `slog.Debug` (existing `log/slog` package) to log each removed example with route key, index, and original TTL.

**Rationale**: Consistent with existing verbose logging pattern in `selectDynamicExample` and `handleAddExample`. No new logging dependency.

## Risks / Trade-offs

- **[Risk]**: Sweep holds `dyMu.Lock()` while iterating routes — could block write operations for the duration of the sweep.
  - **Mitigation**: Sweep iterates only populated keys (missed keys in `dynamicExamples` map are skipped). Under typical loads (hundreds of entries), sweep completes in microseconds. If `dynamicExamples` grows to thousands, the lazy check still prevents expired examples from being served.

- **[Risk]**: TTL grain is 1 second (tick interval); an example added with ttl=1 could live for up to 2 seconds before sweep removes it.
  - **Mitigation**: Lazy check handles correctness (never returns expired). Sweep handles memory cleanup (bounded lateness is acceptable).

- **[Trade-off]**: Memory usage from expired-but-not-yet-swept examples is temporary but bounded to max 1 tick interval worth of additions per route.
