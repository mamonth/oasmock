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

// addExampleRequestSchema is the oneOf two-branch request schema for
// POST /_mock/examples (design D2). Branch A is the sync (OpenAPI) target:
// required path+response and no async-only fields. Branch B is the async
// (AsyncAPI) target: required channel+response and no path.
var addExampleRequestSchema = gojsonschema.NewGoLoader(map[string]any{
	"type":     "object",
	"required": []string{"response"},
	"oneOf": []any{
		map[string]any{
			"required": []string{"path", "response"},
			"not": map[string]any{
				"anyOf": []any{
					map[string]any{"required": []string{"protocol"}},
					map[string]any{"required": []string{"channel"}},
					map[string]any{"required": []string{"match"}},
					map[string]any{"required": []string{"interval"}},
					map[string]any{"required": []string{"delay"}},
				},
			},
		},
		map[string]any{
			"required": []string{"channel", "response"},
			"not": map[string]any{
				"anyOf": []any{
					map[string]any{"required": []string{"path"}},
				},
			},
		},
	},
	"properties": map[string]any{
		"path": map[string]any{"type": "string"},
		"protocol": map[string]any{
			"type": "string",
			"enum": []string{"http", "ws"},
		},
		"channel": map[string]any{"type": "string"},
		"method": map[string]any{
			"type":    "string",
			"enum":    []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
			"default": "GET",
		},
		"match": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"interval": map[string]any{"type": "integer", "minimum": 1},
		"delay":    map[string]any{"type": "integer", "minimum": 0},
		"once":     map[string]any{"type": "boolean"},
		"validate": map[string]any{"type": "boolean"},
		"ttl":      map[string]any{"type": "integer", "minimum": 0},
		"conditions": map[string]any{
			"type": "object",
			"additionalProperties": map[string]any{
				"oneOf": []any{
					map[string]any{"type": "string"},
					map[string]any{"type": "number"},
					map[string]any{"type": "boolean"},
					map[string]any{"type": "object"},
				},
			},
		},
		"response": map[string]any{
			"type":     "object",
			"required": []string{"code"},
			"properties": map[string]any{
				"code": map[string]any{"type": "integer"},
				"headers": map[string]any{
					"type":                 "object",
					"additionalProperties": map[string]any{"type": "string"},
				},
				"body": map[string]any{
					"type": []any{"string", "number", "boolean", "object", "array"},
				},
			},
		},
	},
})

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
	Match      map[string]any `json:"match"`
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
	if matchesEventContext(req.Match) && req.Interval > 0 {
		return nil, fmt.Errorf("'interval' and an event-based 'match' are mutually exclusive")
	}
	return &req, nil
}

// resolveExampleTarget maps an add-example request to its route: AsyncAPI
// targets resolve by protocol/channel, OpenAPI targets by path/method. A
// runtime match on an async target must drive emission, so only an
// {$event.*}-based match is accepted; a connection-only or literal match has
// no trigger and is rejected rather than silently registered nowhere
// (RS.MAPI.24-26, RS.MAPI.33).
func (s *Server) resolveExampleTarget(req *addExampleRequest) (*RouteMapping, error) {
	if req.Protocol != "" || req.Channel != "" {
		mapping := s.findAsyncRouteMapping(req.Protocol, req.Channel, req.Method)
		if mapping == nil {
			return nil, fmt.Errorf("no matching route found")
		}
		if mapping.Protocol != "" && req.Match != nil && !matchesEventContext(req.Match) {
			return nil, fmt.Errorf("async target 'match' must reference the event context ({$event.*}); use 'interval' for periodic emission")
		}
		return mapping, nil
	}
	for i := range s.mappings {
		if m := &s.mappings[i]; m.Pattern == req.Path && m.Method == req.Method {
			return m, nil
		}
	}
	return nil, fmt.Errorf("no matching route found")
}

// needsRuntimeRegistration reports whether an async target carries a trigger
// (match or interval) that registers through the event broker / scheduler.
func needsRuntimeRegistration(mapping *RouteMapping, req *addExampleRequest) bool {
	return mapping.Protocol != "" && (req.Match != nil || req.Interval > 0)
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
	if req.Match != nil {
		ext["x-mock-match"] = req.Match
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
