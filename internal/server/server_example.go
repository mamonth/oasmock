package server

import (
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/mamonth/oasmock/internal/loader"
	"github.com/mamonth/oasmock/internal/runtime"
)

// The once/dynamic example stores and the TTL sweep live in exampleRegistry;
// the selection/templating/state core lives in exampleEngine. The Server
// methods below are thin forwarders preserved for the HTTP pipeline and the
// test surface.

func (s *Server) sweepExpiredExamples() { s.registry.sweepExpired() }

func (s *Server) selectDynamicExample(mapping *RouteMapping, eval runtime.Evaluator) (*dynamicExample, string) {
	return s.registry.selectDynamic(routeKey(mapping.Method, mapping.ChiPattern), eval)
}

func (s *Server) selectResponse(mapping *RouteMapping, eval runtime.Evaluator) (string, *openapi3.Response) {
	return s.engine.selectResponse(mapping, eval)
}

func (s *Server) selectMediaType(response *openapi3.Response) (string, *openapi3.MediaType, error) {
	return s.engine.selectMediaType(response)
}

func (s *Server) generateResponse(example *openapi3.Example, dynExample *dynamicExample, eval runtime.Evaluator, currentStatusCode string) (body []byte, headers map[string]string, statusCode string, err error) {
	return s.engine.generateResponse(example, dynExample, eval, currentStatusCode)
}

func (s *Server) selectExample(mediaType *openapi3.MediaType, eval runtime.Evaluator, opID string) (*openapi3.Example, string) {
	return s.engine.selectExample(mediaType, eval, opID)
}

func (s *Server) applyExtensions(example *openapi3.Example, eval runtime.Evaluator, prefix string) {
	s.engine.applyExtensions(example, eval, prefix)
}

// markOnceUsed marks an example as used (for x-mock-once).
func (s *Server) markOnceUsed(id string) { s.registry.markOnceUsed(id) }

// isOnceUsed checks if an example has been used.
func (s *Server) isOnceUsed(id string) bool { return s.registry.isOnceUsed(id) }

func getStatusCode(mapping *loader.RouteMapping, response *openapi3.Response) int {
	// TODO: parse status code from mapping (key in Responses map)
	// For now, default to 200
	return 200
}
