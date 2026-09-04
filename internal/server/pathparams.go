package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// addressParams returns the chi route URL parameters captured when the route
// pattern contains {param} placeholders (the channel address params). The chi
// route context is populated only when the request flowed through the router;
// direct handler invocations in tests get an empty map.
func addressParams(r *http.Request) map[string]string {
	params := make(map[string]string)
	if r == nil || r.URL == nil {
		return params
	}
	ctx := chi.RouteContext(r.Context())
	if ctx == nil {
		return params
	}
	for i, key := range ctx.URLParams.Keys {
		if i < len(ctx.URLParams.Values) {
			params[key] = ctx.URLParams.Values[i]
		}
	}
	return params
}
