package server

import (
	"io"
	"net/http"
	"strings"

	"github.com/mamonth/oasmock/internal/asyncapi"
)

// httpProtocolAdapter serves AsyncAPI http channels by reusing the HTTP mock
// response pipeline through the shared MessageHandler (RS.ASP.1, RS.ASP.10).
type httpProtocolAdapter struct{}

// Protocol implements ProtocolAdapter.
func (a *httpProtocolAdapter) Protocol() string { return asyncapi.ProtocolHTTP }

// Handler builds the HTTP handler that renders an AsyncAPI http channel route.
func (a *httpProtocolAdapter) Handler(mapping *RouteMapping, handler MessageHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodySize))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "failed to read request body")
			return
		}

		headers := make(map[string]string)
		for k := range r.Header {
			value := r.Header.Get(k)
			if value != "" {
				headers[strings.ToLower(k)] = value
			}
		}

		out, err := handler.HandleMessage(r.Context(), InboundMessage{
			Payload:    body,
			Headers:    headers,
			PathParams: addressParams(r),
		})
		if err != nil {
			writeJSONErrorf(w, http.StatusInternalServerError, "failed to render message: %v", err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if out == nil {
			// AsyncAPI http send with no reply message (RS.ASP.10).
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, werr := w.Write(out); werr != nil {
			// Best effort; connection may be gone.
			return
		}
	}
}
