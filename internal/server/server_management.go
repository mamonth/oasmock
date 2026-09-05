package server

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

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
			"type":                 "object",
			"additionalProperties": true,
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

// filterRecords filters request records based on query parameters.
func filterRecords(records []RequestRecord, query url.Values) []RequestRecord {
	filtered := make([]RequestRecord, 0, len(records))
	for _, rec := range records {
		// Filter by path
		if path := query.Get("path"); path != "" && rec.Path != path {
			continue
		}
		// Filter by method
		if method := query.Get("method"); method != "" && rec.Method != method {
			continue
		}
		// Filter by time_from (milliseconds since epoch)
		if timeFromStr := query.Get("time_from"); timeFromStr != "" {
			if timeFrom, err := strconv.ParseInt(timeFromStr, 10, 64); err == nil {
				if rec.Timestamp.UnixMilli() < timeFrom {
					continue
				}
			}
		}
		// Filter by time_till
		if timeTillStr := query.Get("time_till"); timeTillStr != "" {
			if timeTill, err := strconv.ParseInt(timeTillStr, 10, 64); err == nil {
				if rec.Timestamp.UnixMilli() > timeTill {
					continue
				}
			}
		}
		filtered = append(filtered, rec)
	}
	return filtered
}

// paginateRecords applies offset and limit pagination to records.
func paginateRecords(records []RequestRecord, offset, limit int) []RequestRecord {
	if offset < 0 {
		offset = 0
	}
	if offset > len(records) {
		offset = len(records)
	}
	if limit < 0 {
		limit = 0
	}
	end := offset + limit
	if end > len(records) {
		end = len(records)
	}
	return records[offset:end]
}

// recordsToAPIResponse converts request records to API response format.
func recordsToAPIResponse(records []RequestRecord) []map[string]any {
	items := make([]map[string]any, len(records))
	for i, rec := range records {
		var body any
		if len(rec.Body) > 0 {
			// Try to unmarshal as JSON, else keep as string
			var jsonBody any
			if err := json.Unmarshal(rec.Body, &jsonBody); err == nil {
				body = jsonBody
			} else {
				body = string(rec.Body)
			}
		}
		headers := make(map[string]string)
		for k, v := range rec.Headers {
			if len(v) > 0 {
				headers[k] = v[0]
			}
		}
		items[i] = map[string]any{
			"ts":      rec.Timestamp.UnixMilli(),
			"url":     rec.Path + "?" + rec.Query,
			"method":  rec.Method,
			"body":    body,
			"headers": headers,
		}
	}
	return items
}

func (s *Server) handleGetRequests(w http.ResponseWriter, r *http.Request) {
	records := s.historyStore.GetAll()
	query := r.URL.Query()

	// Filtering
	filtered := filterRecords(records, query)

	// Pagination
	offset, _ := strconv.Atoi(query.Get("offset"))
	if offset < 0 {
		offset = 0
	}
	limit, _ := strconv.Atoi(query.Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	paginated := paginateRecords(filtered, offset, limit)

	// Convert to API response
	items := recordsToAPIResponse(paginated)
	response := map[string]any{
		"data": items,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil && s.config.Verbose {
		slog.Debug("Failed to encode response", "err", err)
	}
}

// newExampleID returns a time-unique example id in the given namespace. The
// namespace prefix keeps runtime-async ids ("rtex-") disjoint from sync
// dynamic-example ids ("dynex-"), so DELETE /_mock/examples/{id} never has to
// disambiguate a collision between the two registries.
func newExampleID(namespace string) string {
	return fmt.Sprintf("%s-%d", namespace, time.Now().UnixNano())
}

func (s *Server) handleAddExample(w http.ResponseWriter, r *http.Request) {
	// Read the raw body for validation
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		if s.config.Verbose {
			slog.Debug("Failed to read request body", "err", err)
		}
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	// Validate against OpenAPI schema
	if err := validateAddExampleRequest(bodyBytes); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Decode into struct
	var req struct {
		Path       string         `json:"path"`
		Method     string         `json:"method"`
		Protocol   string         `json:"protocol"`
		Channel    string         `json:"channel"`
		Match      map[string]any `json:"match"`
		Interval   int            `json:"interval"`
		Delay      int            `json:"delay"`
		Once       bool           `json:"once"`
		Validate   bool           `json:"validate"`
		TTL        int            `json:"ttl"`
		Conditions map[string]any `json:"conditions"`
		Response   struct {
			Code    int               `json:"code"`
			Headers map[string]string `json:"headers"`
			Body    any               `json:"body"`
		} `json:"response"`
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.Response.Code == 0 || (req.Path == "" && req.Channel == "") {
		writeJSONError(w, http.StatusBadRequest, "missing required fields")
		return
	}
	req.Method = cmp.Or(req.Method, DefaultMethod)

	// Single-trigger rule (RS.MAPI.29): an async target has exactly one
	// trigger — interval OR an {$event.*}-based match, never both.
	if matchesEventContext(req.Match) && req.Interval > 0 {
		writeJSONError(w, http.StatusBadRequest, "'interval' and an event-based 'match' are mutually exclusive")
		return
	}

	// Resolve the target route: OpenAPI path/method or AsyncAPI channel.
	var targetMapping *RouteMapping
	if req.Protocol != "" || req.Channel != "" {
		targetMapping = s.findAsyncRouteMapping(req.Protocol, req.Channel, req.Method)
		if targetMapping == nil {
			writeJSONError(w, http.StatusBadRequest, "no matching route found")
			return
		}
	} else {
		for i := range s.mappings {
			mapping := &s.mappings[i]
			if mapping.Pattern == req.Path && mapping.Method == req.Method {
				targetMapping = mapping
				break
			}
		}
		if targetMapping == nil {
			writeJSONError(w, http.StatusBadRequest, "no matching route found")
			return
		}
	}
	// TODO: validate response body against OpenAPI schema if req.Validate is true
	// (skipped for now)

	// Runtime async-driven examples (match/interval) register through the
	// event broker / scheduler (RS.MAPI.24-26, RS.MAPI.33). A runtime match on
	// an async target must drive emission, so only an {$event.*}-based match is
	// accepted; a connection-only or literal match has no trigger and is
	// rejected rather than silently registered nowhere.
	if targetMapping.Protocol != "" && req.Match != nil && !matchesEventContext(req.Match) {
		writeJSONError(w, http.StatusBadRequest, "async target 'match' must reference the event context ({$event.*}); use 'interval' for periodic emission")
		return
	}
	if targetMapping.Protocol != "" && (req.Match != nil || req.Interval > 0) {
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
		kind, jobID, err := s.registerRuntimeExample(id, targetMapping, example)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"message": "Example added",
			"id":      id,
			"kind":    triggerKindString(kind),
			"jobID":   jobID,
		})
		return
	}

	// Create dynamic example
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
	key := routeKey(targetMapping.Method, targetMapping.ChiPattern)
	if s.config.Verbose {
		slog.Debug("handleAddExample: storing dynamic example",
			"key", key,
			"path", req.Path,
			"method", req.Method,
			"chiPattern", targetMapping.ChiPattern,
			"pattern", targetMapping.Pattern,
			"numExamples", len(s.registry.dynamicExamples[key])+1)
	}
	s.registry.addDynamic(key, example)
	// Respond with success
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"message": "Example added",
		"id":      id,
	}); err != nil && s.config.Verbose {
		slog.Debug("Failed to encode success response", "err", err)
	}
}
