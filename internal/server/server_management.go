package server

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/mamonth/oasmock/internal/asyncapi"
	"github.com/mamonth/oasmock/internal/extensions"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/xeipuuv/gojsonschema"
)

// matchesEventContext reports whether a match references the event context
// ({$event.*}), which makes it an event-driven runtime trigger.
func matchesEventContext(match map[string]any) bool {
	return extensions.MatchReferencesEvent(match)
}

// triggerKindString maps an extensions.TriggerKind to the wire value used in
// the POST /_mock/examples response "kind" field (OpenAPI enum: event|interval).
func triggerKindString(kind extensions.TriggerKind) string {
	switch kind {
	case extensions.TriggerEvent:
		return "event"
	case extensions.TriggerPeriodic:
		return "interval"
	default:
		return ""
	}
}

// findAsyncRouteMapping resolves an AsyncAPI route mapping by protocol and
// channel address (RS.MAPI.19, RS.MAPI.21).
func (s *Server) findAsyncRouteMapping(protocol, channel, method string) *RouteMapping {
	for i := range s.mappings {
		mapping := &s.mappings[i]
		if mapping.Protocol == "" {
			continue
		}
		if protocol != "" && mapping.Protocol != protocol {
			continue
		}
		if mapping.Path != channel {
			continue
		}
		if method != "" && mapping.Method != method && method != DefaultMethod {
			continue
		}
		return mapping
	}
	return nil
}

// addExampleRequestSchema is the runtime request-validation schema for
// POST /_mock/examples (design D2: oneOf two-branch target discriminator — the
// sync branch requires path+response and forbids every async-only field, the
// async branch requires channel+response and forbids path). It is not
// hand-written: it is regenerated from components.schemas.AddExampleRequest in
// api/openapi.yaml (see add_example_request_schema_gen.go), keeping the OpenAPI
// document the single source of truth for the management contract.
var addExampleRequestSchema = func() gojsonschema.JSONLoader {
	loader := gojsonschema.NewBytesLoader(AddExampleRequestSchemaJSON)
	if _, err := gojsonschema.NewSchemaLoader().Compile(loader); err != nil {
		panic("generated AddExampleRequest schema failed to compile: " + err.Error())
	}
	return loader
}()

func validateAddExampleRequest(rawJSON []byte) error {
	loader := gojsonschema.NewBytesLoader(rawJSON)
	result, err := gojsonschema.Validate(addExampleRequestSchema, loader)
	if err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}
	if !result.Valid() {
		var errStrs []string
		for _, desc := range result.Errors() {
			errStrs = append(errStrs, desc.String())
		}
		return fmt.Errorf("invalid request: %s", strings.Join(errStrs, "; "))
	}
	return nil
}

// newExampleID returns a time-unique example id in the given namespace. The
// namespace prefix keeps runtime-async ids ("rtex-") disjoint from sync
// dynamic-example ids ("dynex-"), so DELETE /_mock/examples/{id} never has to
// disambiguate a collision between the two registries.
func newExampleID(namespace string) string {
	return fmt.Sprintf("%s-%d", namespace, time.Now().UnixNano())
}

// addExampleRequest is the decoded body of POST /_mock/examples.
type addExampleRequest struct {
	Path       string         `json:"path"`
	Method     string         `json:"method"`
	Protocol   string         `json:"protocol"`
	Channel    string         `json:"channel"`
	Interval   int            `json:"interval"`
	Delay      int            `json:"delay"`
	Once       bool           `json:"once"`
	Validate   *bool          `json:"validate"`
	TTL        int            `json:"ttl"`
	Conditions map[string]any `json:"conditions"`
	Response   struct {
		Code    int               `json:"code"`
		Headers map[string]string `json:"headers"`
		Body    any               `json:"body"`
	} `json:"response"`
}

func (s *Server) handleAddExample(w http.ResponseWriter, r *http.Request) {
	req, err := decodeAddExampleRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	mapping, err := s.resolveExampleTarget(req)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Validate == nil || *req.Validate {
		if mapping.Operation != nil && mapping.Responses != nil {
			if verr := s.validateExampleResponse(req, mapping); verr != nil {
				writeJSONError(w, http.StatusBadRequest, verr.Error())
				return
			}
		}
	}
	if needsRuntimeRegistration(mapping, req) {
		s.registerAsyncRuntimeExample(w, req, mapping)
		return
	}
	s.registerDynamicExample(w, req, mapping)
}

// validateExampleResponse validates an add-example response body against the
// resolved route's OpenAPI response schema for the requested status code and
// media type. Async targets have no OpenAPI schema and skip validation.
func (s *Server) validateExampleResponse(req *addExampleRequest, mapping *RouteMapping) error {
	if req.Response.Body == nil {
		return nil
	}
	schema := responseSchemaFor(mapping.Responses, req.Response.Code)
	if schema == nil {
		return nil
	}
	if err := schema.VisitJSON(req.Response.Body); err != nil {
		return fmt.Errorf("response body does not match the OpenAPI schema for status %d: %w", req.Response.Code, err)
	}
	return nil
}

// responseSchemaFor returns the JSON schema of a response status code's first
// JSON media type, or nil when none is declared.
func responseSchemaFor(responses *openapi3.Responses, code int) *openapi3.Schema {
	if responses == nil {
		return nil
	}
	respMap := responses.Map()
	respRef := respMap[fmt.Sprintf("%d", code)]
	if respRef == nil || respRef.Value == nil || respRef.Value.Content == nil {
		return nil
	}
	for _, mt := range respRef.Value.Content {
		if mt == nil || mt.Schema == nil || mt.Schema.Value == nil {
			continue
		}
		return mt.Schema.Value
	}
	return nil
}

// rejectRemovedMatchField rejects a stale top-level `match` field (RS.MAPI.37):
// the async-only selector was removed in favor of the unified `conditions`, so
// a body still carrying it must never be silently registered without its
// selection conditions.
func rejectRemovedMatchField(bodyBytes []byte) error {
	var raw map[string]any
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return fmt.Errorf("invalid JSON")
	}
	if _, ok := raw["match"]; ok {
		return fmt.Errorf("'match' is removed; use 'conditions'")
	}
	return nil
}

// rejectSyncTimingFields rejects the async-only timing fields on an OpenAPI
// (sync) target (RS.MAPI.28): interval/delay are only meaningful for AsyncAPI
// routes.
func rejectSyncTimingFields(req *addExampleRequest) error {
	if req.Interval > 0 || req.Delay > 0 {
		return fmt.Errorf("'interval' and 'delay' are only valid on an AsyncAPI target")
	}
	return nil
}

// decodeAddExampleRequest reads, schema-validates and decodes an add-example
// body, applying the field checks that are independent of the resolved target.
func decodeAddExampleRequest(r *http.Request) (*addExampleRequest, error) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	if err := validateAddExampleRequest(bodyBytes); err != nil {
		return nil, err
	}
	if err := rejectRemovedMatchField(bodyBytes); err != nil {
		return nil, err
	}
	var req addExampleRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		return nil, fmt.Errorf("invalid JSON")
	}
	if req.Response.Code == 0 || (req.Path == "" && req.Channel == "") {
		return nil, fmt.Errorf("missing required fields")
	}
	req.Method = cmp.Or(req.Method, DefaultMethod)

	// Single-trigger rule (RS.MAPI.29): an async target has exactly one
	// trigger — interval OR an {$event.*}-based match, never both.
	if matchesEventContext(req.Conditions) && req.Interval > 0 {
		return nil, fmt.Errorf("'interval' and an event-based 'conditions' are mutually exclusive")
	}
	return &req, nil
}

// rejectNonEventAsyncConditions rejects an async target whose conditions
// reference only {$connection.*} or literal values (no {$event.*}): a runtime
// example needs a trigger, and a connection-only match has none (RS.MAPI.35).
func rejectNonEventAsyncConditions(mapping *RouteMapping, req *addExampleRequest) error {
	if mapping.Protocol != "" && len(req.Conditions) > 0 && !matchesEventContext(req.Conditions) {
		return fmt.Errorf("async target 'conditions' must reference the event context ({$event.*}); use 'interval' for periodic emission")
	}
	return nil
}

// resolveExampleTarget maps an add-example request to its route: AsyncAPI
// targets resolve by protocol/channel, OpenAPI targets by path/method. A
// runtime match on an async target must drive emission, so only an
// {$event.*}-based conditions is accepted; a connection-only or literal match
// has no trigger and is rejected rather than silently registered nowhere
// (RS.MAPI.24-26, RS.MAPI.33).
func (s *Server) resolveExampleTarget(req *addExampleRequest) (*RouteMapping, error) {
	if req.Protocol != "" || req.Channel != "" {
		mapping := s.findAsyncRouteMapping(req.Protocol, req.Channel, req.Method)
		if mapping == nil {
			// A SignalR hub channel is deliberately absent from the raw-route
			// mapping table (design D7), so resolve it directly against the
			// hubs when the raw-route scan misses (design D3). The requested
			// protocol must not contradict the hub target: only an explicit
			// "signalr" (or an omitted protocol) may fall back to a hub channel.
			mapping = s.hubRouteMapping(req.Protocol, req.Channel)
		}
		if mapping == nil {
			return nil, fmt.Errorf("no matching route found")
		}
		if err := rejectNonEventAsyncConditions(mapping, req); err != nil {
			return nil, err
		}
		return mapping, nil
	}
	for i := range s.mappings {
		if m := &s.mappings[i]; m.Pattern == req.Path && m.Method == req.Method {
			if err := rejectSyncTimingFields(req); err != nil {
				return nil, err
			}
			return m, nil
		}
	}
	return nil, fmt.Errorf("no matching route found")
}

// hubRouteMapping builds a hub-backed RouteMapping for an address served by a
// SignalR hub channel (design D3). It returns nil when the address matches no
// hub channel or the requested protocol contradicts a hub target (anything
// other than "signalr" or an omitted protocol).
func (s *Server) hubRouteMapping(protocol, channel string) *RouteMapping {
	if protocol != "" && protocol != asyncapi.ProtocolSignalR {
		return nil
	}
	hub, channelID := s.hubMgr.hubChannelForAddress(channel)
	if hub == nil {
		return nil
	}
	return &RouteMapping{
		Method:     http.MethodGet,
		Path:       channel,
		Pattern:    channel,
		Prefix:     hub.prefix,
		ChiPattern: channel,
		Protocol:   asyncapi.ProtocolSignalR,
		Messages:   loader.MessageSpecsFromAsync(hub.channels[channelID].Messages),
	}
}

// needsRuntimeRegistration reports whether an async target carries a trigger
// (conditions or interval) that registers through the event broker / scheduler.
func needsRuntimeRegistration(mapping *RouteMapping, req *addExampleRequest) bool {
	return mapping.Protocol != "" && (len(req.Conditions) > 0 || req.Interval > 0)
}

// registerAsyncRuntimeExample registers an event-driven or periodically driven
// example through the event broker / scheduler and responds with its runtime
// identity (RS.MAPI.24-26).
func (s *Server) registerAsyncRuntimeExample(w http.ResponseWriter, req *addExampleRequest, mapping *RouteMapping) {
	id := newExampleID("rtex")
	headers := make(map[string]any, len(req.Response.Headers))
	for k, v := range req.Response.Headers {
		headers[k] = v
	}
	ext := make(map[string]any)
	if len(req.Conditions) > 0 {
		ext["x-mock-match"] = req.Conditions
	}
	if req.Interval > 0 {
		ext["x-mock-interval"] = req.Interval
	}
	if req.Delay > 0 {
		ext["x-mock-delay"] = req.Delay
	}
	example := &loader.MessageExampleSpec{
		Name:       "runtime-" + id,
		Headers:    headers,
		Payload:    req.Response.Body,
		Extensions: ext,
	}
	kind, jobID, err := s.registerRuntimeExample(id, mapping, example)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Example added",
		"id":      id,
		"kind":    triggerKindString(kind),
		"jobID":   jobID,
	})
}

// registerDynamicExample stores a sync or async reply example in the example
// registry and responds with its id.
func (s *Server) registerDynamicExample(w http.ResponseWriter, req *addExampleRequest, mapping *RouteMapping) {
	id := newExampleID("dynex")
	example := dynamicExample{
		onceID:     id,
		once:       req.Once,
		conditions: req.Conditions,
		ttl:        req.TTL,
	}
	if req.TTL > 0 {
		example.addedAt = time.Now()
	}
	example.response.code = req.Response.Code
	example.response.headers = req.Response.Headers
	example.response.body = req.Response.Body
	// Store under mapping key
	key := routeKey(mapping.Method, mapping.ChiPattern)
	if s.config.Verbose {
		slog.Debug("handleAddExample: storing dynamic example",
			"key", key,
			"path", req.Path,
			"method", req.Method,
			"chiPattern", mapping.ChiPattern,
			"pattern", mapping.Pattern,
			"numExamples", len(s.registry.dynamicExamples[key])+1)
	}
	s.registry.addDynamic(key, example)
	// Respond with success
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Example added",
		"id":      id,
	})
}
