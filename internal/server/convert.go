package server

import "github.com/mamonth/oasmock/internal/loader"

// ConvertRouteMappings converts loader route mappings into server route
// mappings. It is the single translation point between the loader's routing
// model and the server's, so both share one field mapping.
func ConvertRouteMappings(loaderMappings []loader.RouteMapping) []RouteMapping {
	mappings := make([]RouteMapping, len(loaderMappings))
	for i, lm := range loaderMappings {
		mappings[i] = RouteMapping{
			Method:     lm.Method,
			Path:       lm.Path,
			Pattern:    lm.Pattern,
			Prefix:     lm.Prefix,
			ChiPattern: lm.ChiPattern,
			Operation:  lm.Operation,
			Parameters: lm.Parameters,
			Responses:  lm.Responses,
			Protocol:   lm.Protocol,
			Action:     lm.Action,
			Messages:   lm.Messages,
		}
	}
	return mappings
}
