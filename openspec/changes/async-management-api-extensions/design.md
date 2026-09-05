# Design: Async management API extensions

## Context

The management surface today (see `api/openapi.yaml` and `internal/server/management_async.go`, `server_management.go`, `event_broker.go`, `scheduler.go`) is split under `/_mock/ws/*` even though its operations drive both `ws` and `http` AsyncAPI channels. Example selection is split across two vocabularies: `x-mock-match` (`extensions/match.go`, `example_value.go`) selects examples against HTTP/message contexts, while `x-send-events on:` (`send_events.go`) is a coarse event-name equality on top of the same matcher — a genuine duplicate. The built-ins `cron`/`connect`/`receive` are parsed and registered in the broker but never fired — the only `fire` callers are `x-event-trigger` (`server.go:533`) and `/_mock/events/fire`. The schedule endpoint is a one-variant bound duplicate of the `cron` built-in and marshals its payload once at registration (no per-delivery templating). Delivery is broadcast to all consumers of a channel with no server-side recipient decision. There is no reactive channel for test harnesses.

See `proposal.md` for the why; this document covers how. Requirements are in `specs/` (deltas against `extensions`, `event-driver`, `management-api`, `asyncapi-management`).

## Goals / Non-Goals

**Goals:**
- One selection extension — `x-mock-match` — across sync (request) and async (event) contexts, plus a per-connection recipient filter (`{$connection.*}`), so sync and async examples share selection semantics.
- Event-driven emission expressed declaratively (`{$event.name}` match) rather than via a parallel `x-send-events` subscription key; recurrence/delay are timing-only sibling extensions (`x-mock-interval`, `x-mock-delay`) that keep the matcher pure.
- One unified `POST /_mock/examples` surface for sync and async injection with runtime `match`/`interval`/`delay` mirroring the extensions.
- Protocol-neutral async management prefix (`/_mock/async/*`) with legacy `/_mock/ws/*` aliases kept.
- `POST /_mock/events` with a `type` discriminator replacing `/_mock/events/fire`.
- Consumers listable across all channels (`channel` optional).
- A general management WebSocket stream at `WS /_mock/stream` (V1 notifications-only).
- Actually-fired built-ins (`connect`, `receive`) and scheduler-driven periodic emission.

**Non-Goals:**
- Client→server commands on the management stream (V2, on the same socket).
- `once`/`ttl`/`conditions` selection semantics on runtime examples beyond the synchronous set already supported (payload/headers + match/interval/delay only).
- New AsyncAPI protocols or changes to channel serving.
- Auth on the management surface (mock tool premise, unchanged).

## Decisions

### D1: Path surface — protocol-neutral `/_mock/async/*`, general `/_mock/stream`
Management endpoints that act on both `ws` and `http` channels move under `/_mock/async/*`; `ws//` is a protocol artifact of the original MVP. The management WebSocket is deliberately *not* under `/async` — it is a general cross-cutting control channel — so it lives at `WS /_mock/stream`. Legacy `/_mock/ws/{push,consumers,disconnect}` and `/_mock/events/fire` are kept as deprecated aliases (identical handler registrations), and `/_mock/ws/schedule{,/{pushId}}` become `410 Gone` bodies pointing at `POST /_mock/examples` / `DELETE /_mock/examples/{id}`.
- **Alternative considered**: naming the stream `/_mock/ws/events` or `/_mock/async/ws` — rejected: locks a general control channel to the async domain and re-couples it to the ws protocol.

### D2: Unified `AddExampleRequest` — `oneOf` JSON Schema branches + single-trigger validation
Replace the single loose `addExampleRequestSchema` (`server_management.go:40`) with a `oneOf` two-branch schema so a request is rejected in a declarative way when targeting is mixed:
- branch A (sync): `required: [path, response]`, `not: {anyOf: [{required:[protocol]},{required:[channel]},{required:[match]},{required:[interval]},{required:[delay]}]}`;
- branch B (async): `required: [channel, response]`, `not: {anyOf: [{required:[path]}]}`.
`match` mirrors `x-mock-match` (object), `interval` (positive integer ms), `delay` (integer ms). A Go-side check enforces the single-trigger rule (`interval` xor event-based `match`), since cross-field-dependency is not expressable in draft-07. Route resolution reuses `findAsyncRouteMapping` (`server_management.go:20`) so unknown channels still 400 (keeps RS.MAPI.21).
- **Alternative considered**: a Go-only checker over the raw JSON — rejected: drops the data-driven declarative validation style and duplicates the field rules in two places.

### D3: Unified match model — event context, trigger classification, `x-send-events` shim
Emit decisions reuse the shared selection pipeline instead of bespoke subscription grouping:

- **Event context**: `EventSource` (`runtime/expression.go`) gains identity `{$event.name}` (named-event name or built-in `connect`/`receive`) and whole-payload `{$event.data}`, alongside unchanged `{$event.<field>}` payload access. `name`/`data` are reserved metadata.
- **Trigger classification at load**: an AsyncAPI message example is *event-driven* iff its `x-mock-match` references `{$event.*}`; *periodically driven* iff it declares `x-mock-interval`; otherwise it is a sync/async reply. Mixed match contexts (`{$event.*}` with `{$request|$message|$channel.*}`) or both triggers are load errors (RS.EXT.20, RS.EXT.28). The old subscription grouping (`event_server.go:collectSchemaSubscriptions`, `messageDeliverable`) is replaced by registration of each event-driven example keyed by its match identity + schema scope.
- **`x-send-events` shim**: on load, translate `{on:<x>, wait:N}` → `{$event.name}` match (or `x-mock-interval` for `cron`) with a verbose-mode deprecation note (RS.EVT.18).
- **Runtime path**: `handleAddExample` with `match`/`interval` registers the example through the same machinery (RS.MAPI.24-26, RS.MAPI.33); `Server` keeps `map[exampleID]→{trigger, jobID}` so `DELETE /_mock/examples/{id}` unregisters and cancels.
- **Alternative considered**: keeping `x-send-events` alongside `x-mock-match` as an explicit subscription key — rejected: duplicates the matcher (the spray of this change) and forces reconciling two "which example" systems.

### D4: Scheduler — `x-mock-interval` jobs with per-delivery templating
Retire the raw `pushScheduler.push(channel,payload)` model. `scheduler.go` becomes a job runner: job = `{id, interval, deliver func()}`; each tick calls the delivery pipeline (render + recipient partition + push) for that example. Spec `x-mock-interval` and runtime `interval` create such jobs per example (per-example cadence, so differing intervals don't intermix), and start/stop emits `schedule` envelopes. The legacy schedule endpoint is gone; one-shot `push` with `delay` keeps its existing `time.Sleep` path (`management_async.go:61`).
- **Alternative considered**: firing a synthetic named event per interval — rejected: keeps an event concept where none is needed and would merge same-schema intervals.

### D5: Built-in trigger firing (`connect`, `receive`)
`cron` is no longer an event (D4 covers periodicity). The two remaining built-ins are fired from lifecycle/inbound hooks, gated by a cheap `broker.hasSubscribers(name, schema)` check to keep hot loops cheap:
- `connect`: fired schema-local in the ws adapter after `registry.register` (`ws_adapter.go:177`) and in `signalr_hub.go` on connection; recipient set = the single connecting connection.
- `receive`: fired schema-local in the ws adapter read loop on an inbound message and in the SignalR `dispatch`/stream path, with the inbound message exposed via the event context.
Both are no-ops when nothing matches. The existing receive-operation snapshot emission (`ws_adapter.go:183`) is independent and unchanged.
- **Alternative considered**: no gating (always fire) — rejected for per-frame overhead; **Alternative**: deriving `receive` from the operation snapshot — rejected: snapshot is spec-shaped, built-in needs the inbound message payload.

### D6: Per-connection recipient partition (two-phase evaluation)
Server-side recipient selection replaces "broadcast and let the client filter" when an example asks for it. At delivery time `x-mock-match` is partitioned (either side referencing `{$connection.*}`):

```
0. fire → candidate set for the channel: all consumers (byChan[address] / hub
     streams); connect → just the connecting connection.
1. split conditions: connectionBucket (any {$connection.*} ref on either side)
     vs commonBucket (everything else).
2. evaluate commonBucket once (event/state/env context). Pre-evaluate
     {$event.*}/{$state.*} subexpressions inside connectionBucket values once.
     Fail → no delivery.
3. per candidate connection: evaluate only connectionBucket ({$connection.id},
     {$connection.channel}, {$connection.query.*}, {$connection.header.*}).
4. deliver the rendered-once payload to satisfied connections.
     connectionBucket empty → broadcast to all candidates (today's behavior).
```

This needs a `{$connection.*}` data source and connection metadata: `wsConnection` (`ws_adapter.go:90`) captures `r.URL.Query()`/headers at upgrade; SignalR connections mirror it. No new extension key and no load-time classification for connection references — the partition is a runtime concern. Condition evaluation reuses `evaluateParamsMatch`, so literal equality and JSON-schema conditions both work per connection; the schema cache (`match.go:getCachedSchema`) keeps the only heavier case bounded.
- **Alternatives considered**: (a) dedicated `x-mock-target` expression targeting one connection (O(1)) — rejected as a special case of the partition (`'{$connection.id}':'{$event.connectionId}'`); (b) client-side filtering only — rejected: the mock server must be able to assert per-consumer delivery in tests.

### D7: Management stream `/_mock/stream`
A dedicated `manageWSRegistry` (separate from `connectionRegistry` so management sockets never appear under `/_mock/async/consumers`). Upgrade only: check `Connection: Upgrade` first, else `405` (RS.AMG.28). Filters parsed at connect from `?events=` and `?channels=` (comma-separated, `*` glob). Envelopes:
`{type: event|push|consumer|schedule, ts, …}` with per-type payloads (event: name/payload/schema/global; push: channel/connectionId/payload; consumer: action/connectionId/channel/streams; schedule: action/exampleId/channel/interval).
Sources: `eventBus` observer hook (`addObserver(func(EventObservation))`, invoked in `fire` and `deliver`), ws-adapter/hub lifecycle hooks, scheduler start/stop. Reads serve pings/pongs only (V1 notifications-only; RS.AMG.23-28).
- **Alternative considered**: reusing the channel `connectionRegistry` — rejected: would pollute consumer discovery.

### D8: Consumers get-all
`handleAsyncConsumers` treats `channel == ""` as "all": iterate `connectionRegistry.byChan` (each `wsConnection` already carries its channel) plus every hub channel's `openStreamsForChannel`. Response shape unchanged (`{consumers:[{connectionId, channel, streams?}]}`), so the get-all list is a flat union (RS.AMG.22).

### D9: `POST /_mock/events` with `type` discriminator
`handleFireEvent` becomes `handleEvents` over a `type`-discriminated request. V1 enum `["fire"]`; `fire` reuses the existing `eventBus.fire(event, payload, "", global, triggerDelay(delay))` semantics (RS.MAPI.22-23). Missing/unknown `type` → 400 (RS.MAPI.32). The `/events/fire` route registers the same handler as a deprecated alias.

## Risks / Trade-offs

- **Management fire is effectively global-only for prefixed schemas**: `_mock/events` fires with `firingSchema == ""`; non-`global` delivery matches only empty-prefix subscriptions. Runtime examples on a `/v1` channel (prefix `/v1`) won't receive a schema-local management fire → mitigate with openapi/docs guidance (“use `global: true` from the management endpoint”) and noted as an open question (schema-universal fire later).
- **Per-connection evaluation cost** → bounded: common conditions evaluate once, connection conditions are map lookups, JSON-schema per connection cached and scoped to the candidate set; fast path skips entirely when no `{$connection.*}` refs (identical to today's broadcast).
- **Backward compatibility of `x-send-events`** → loader mapping shim with verbose deprecation note; removal deferred one release.
- **Event context reserved keys** (`{$event.name}`/`{$event.data}`) shadow payload fields of the same name → documented; aliased access via `{$event.data}` keeps the whole payload reachable.
- **Breaking removal of schedule** → mitigated by `410 Gone` bodies pointing at `/examples` + deprecated aliases; no client is silently broken (explicit error).
- **Built-in firing on hot loops** → gated by `broker.hasSubscribers`; negligible overhead when no examples match.
- **Recurring jobs leak on shutdown** → scheduler shutdown (existing `shutdownSchedules`) now also cancels interval jobs; `DELETE /_mock/examples/{id}` cancels individual ones.

## Migration Plan

1. Add all new endpoints (`/_mock/async/*`, `/_mock/events`, `/_mock/stream`, examples `match`/`interval`/`delay` + `DELETE /examples/{id}`) and the match/timing extension pipeline alongside the existing surface; register deprecated aliases and the `x-send-events` mapping shim.
2. Update `api/openapi.yaml`, `docs/extensions.md`, `docs/architecture.md`, `CHANGELOG.md`.
3. Switch internal tests to the new paths/model; add new unit + integration coverage.
4. Ship aliases for one release; schedule paths answer `410` only after their replacement is live.
Rollback = revert the change commit; legacy aliases + the `x-send-events` shim keep pre-change clients functional.

## Open Questions

- Whether management `fire` (no auto-schema) should become schema-universal by default in a later change (affects prefixed-schema consumption of runtime examples).
- Whether the management stream should gain client→server commands (bidirectional) in a follow-up.
- Whether connection metadata beyond id/channel/query/headers (e.g., negotiated protocol) is needed by real mocks (add later without spec changes).