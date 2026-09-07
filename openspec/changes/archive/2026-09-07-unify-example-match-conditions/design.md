## Context

See proposal.md — Why for motivation. Current state that shapes the approach:

- `POST /_mock/examples` is a `oneOf` discriminator over two branches
  (sync/async) that merge a shared base (`NewExampleRequestBase` via `allOf`).
  Selection currently splits across two fields: `conditions` (shared base,
  sync-only in practice, evaluated by `exampleRegistry.selectDynamic` /
  `exampleEligible`) and `match` (async-only, mapped onto the `x-mock-match`
  extension for the event/connection contexts).
- The async registration path (`registerAsyncRuntimeExample`) builds a
  `MessageExampleSpec` with `Extensions["x-mock-match"]` from `req.Match`; the
  sync path (`registerDynamicExample`) stores `req.Conditions` into
  `dynamicExample.conditions`.
- The Go request struct `addExampleRequest` (server_management.go) decodes
  both `Match` and `Conditions`; server-side cross-field rules
  (`decodeAddExampleRequest`, `resolveExampleTarget`, `needsRuntimeRegistration`)
  read `Match` to enforce single-trigger and event-context semantics.
- Internal selection everywhere already goes through one engine
  (`extensions.EvaluateParamsMatch` on `x-mock-match`) — the duplication exists
  only in the wire/request layer, not the matching engine itself.
- The runtime validation schema
  `internal/server/add_example_request_schema_gen.go` is generated from the
  OpenAPI document (single source of truth) via `gen-control-schema`.

## Goals / Non-Goals

**Goals:**
- A single `conditions` field selects examples for every target kind in
  `POST /_mock/examples`.
- Async targets honor `conditions` for event/connection/reply contexts; the
  `match` wire field is removed with a clean drop.
- The generated schema and OpenAPI contract stay in sync.

**Non-Goals:**
- Renaming the internal selection vocabulary (`conditions` stays a wire field
  that maps onto `x-mock-match` internally; `x-mock-match` extension is
  unchanged).
- Changing `interval`/`delay` semantics or placement (they remain async-only
  timing siblings).
- Changing the `+specs-extensions` matching engine
  (`extensions/match.go`, `classify.go`).
- Deprecation alias for `match` (decided: clean drop — field unreleased).

## Decisions

**D1: Keep `conditions` as the single field; remove `match`.**

The `conditions` field already exists on the shared base and is the 
evolutionary successor — it carries sync reply semantics today and can simply
gain async event/connection semantics. Alternative: unify on `match` — rejected
as it would break the existing sync `conditions` consumers and contradict the
stated goal of eliminating the duplicate rather than renaming it.

**D2: Map async `conditions` onto the existing `x-mock-match` extension.**

Async runtime examples are registered through the event broker / scheduler as
`MessageExampleSpec` with `Extensions["x-mock-match"]`. Repoint that mapping to
`req.Conditions`. No change to `internal/extensions` — the classification
(`MatchReferencesEvent`, `PartitionConnectionConditions`) and evaluation
(`EvaluateParamsMatch`) already handle all contexts. This keeps a single
matching engine and keeps the OpenAPI/doc gap (wire `conditions` ↔ extension
`x-mock-match`) as the only translation point.

**D3: Drive the async target checks off `conditions` in the handler.**

The single-trigger and event-context guards currently key off `req.Match`
(server_management.go: `decodeAddExampleRequest`, `resolveExampleTarget`,
`needsRuntimeRegistration`). These read `req.Conditions` instead, preserving
RS.MAPI.29 (interval xor event-based conditions) and RS.MAPI.35 (connection-only
conditions rejected on async) semantics. `needsRuntimeRegistration` becomes
`len(req.Conditions) > 0 || req.Interval > 0`.

**D4: Reject the removed field at the handler, not the schema.**

`internal/server/add_example_request_schema_gen.go` is `DO NOT EDIT` generated
and `validateAddExampleRequest` runs it. The `oneOf`/`allOf` request schema
leaves `additionalProperties` open (the base bag is explicitly "left open" for
`allOf` merging), so removing `match` from the YAML alone does NOT make the
schema reject a stale `match` field — the schema is permissive of unknown
top-level properties. Rejecting `match` at the schema would require closing
`additionalProperties`, which the `allOf` structure makes brittle (inner-branch
`additionalProperties: false` would not see base properties merged via
`allOf`, wrongly rejecting `conditions`/`method`/`response`). So the rejected
field and the misplaced-field rules are enforced in the handler:
`decodeAddExampleRequest` rejects a body that still carries `match`
(RS.MAPI.37), and `resolveExampleTarget` rejects `interval`/`delay` on an
OpenAPI (sync) target (RS.MAPI.28). The generated schema is still regenerated
from the OpenAPI contract (single source of truth) so `match` no longer
appears as a declared property.

**D5: Widen the `conditions` doc (not schema) to cover event/connection contexts.**

The current `conditions` `additionalProperties` already permits object (JSON
schema) values, which is all the event/connection forms need. Only the
description text is updated to document both context families. No typing
change required, keeping behavior identical for existing sync consumers.

## Risks / Trade-offs

- **Breaking wire change** (removal of `match`) → Clean drop is acceptable
  because the field was introduced in an unreleased change; no alias kept per
  decision. Mitigated by adding RS.MAPI.37 and a schema-level rejection test.
- **Silently-shifted async selection** — a prior async client sending
  `conditions` (hoping for event semantics) was ignored; now it is honored,
  which could change observed delivery. Mitigated by the async-conditions
  spec scenario (RS.MAPI.36) and adding a behavioral test.
- **Generated-schema drift** → Regeneration is a step in tasks.md and verified
  via `go generate` + build; the generated file is committed with the change.
