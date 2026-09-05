package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

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

// maxRequestsPage is the upper bound for GET /_mock/requests pagination
// (RS.MAPI.7, RS.MAPI.12).
const maxRequestsPage = 1000

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
	if limit <= 0 || limit > maxRequestsPage {
		limit = maxRequestsPage
	}
	paginated := paginateRecords(filtered, offset, limit)

	// Convert to API response
	items := recordsToAPIResponse(paginated)
	writeJSON(w, http.StatusOK, map[string]any{
		"data": items,
	})
}

// newExampleID returns a time-unique example id in the given namespace. The
// namespace prefix keeps runtime-async ids ("rtex-") disjoint from sync
// dynamic-example ids ("dynex-"), so DELETE /_mock/examples/{id} never has to
