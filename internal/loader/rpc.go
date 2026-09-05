package loader

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

const (
	ProtocolTypeJsonRpc = "json-rpc"
)

var supportedProtocols = map[string]bool{
	ProtocolTypeJsonRpc: true,
}

func ParseRpcConfig(spec *openapi3.T) (*RpcConfig, error) {
	ext := spec.Extensions["x-rpc"]
	if ext == nil {
		return nil, nil
	}

	extMap, ok := ext.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("x-rpc must be a map")
	}

	cfg := &RpcConfig{}

	if v, ok := extMap["gateway"].(string); ok {
		cfg.Gateway = v
	} else {
		return nil, fmt.Errorf("x-rpc.gateway is required")
	}

	if v, ok := extMap["protocolType"].(string); ok {
		if !supportedProtocols[v] {
			return nil, fmt.Errorf("unsupported protocolType %q", v)
		}
		cfg.ProtocolType = v
	} else {
		return nil, fmt.Errorf("x-rpc.protocolType is required")
	}

	if v, ok := extMap["contentType"].(string); ok {
		cfg.ContentType = v
	} else if cfg.ProtocolType == ProtocolTypeJsonRpc {
		cfg.ContentType = "application/json"
	}

	procRaw, ok := extMap["procedure"]
	if !ok {
		return nil, fmt.Errorf("x-rpc.procedure is required")
	}
	procMap, ok := procRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("x-rpc.procedure must be a map")
	}

	if v, ok := procMap["call"].(string); ok {
		cfg.Procedure.Call = v
	}
	if v, ok := procMap["match"].(string); ok {
		cfg.Procedure.Match = v
	}

	return cfg, nil
}

func BuildRpcMappings(infos []SchemaInfo, cfg *RpcConfig) ([]*RpcRouteMapping, error) {
	if cfg == nil {
		return nil, nil
	}

	seen := make(map[string]bool)
	var mappings []*RpcRouteMapping

	for _, info := range infos {
		if info.Kind == KindAsyncAPI || info.Spec == nil {
			continue
		}
		spec := info.Spec
		prefix := info.Prefix

		pathMap := spec.Paths.Map()
		for path, pathItem := range pathMap {
			if pathItem == nil {
				continue
			}

			fullPath := PrefixPath(prefix, path)

			if !isUnderGateway(fullPath, cfg.Gateway, prefix) {
				continue
			}

			if pathItem.Post == nil {
				continue
			}

			if pathItem.Post.OperationID == "" {
				continue
			}

			procedureName := pathItem.Post.OperationID
			if seen[procedureName] {
				return nil, fmt.Errorf("duplicate procedure name %q under gateway", procedureName)
			}
			seen[procedureName] = true

			mapping := &RpcRouteMapping{
				Procedure: procedureName,
				RouteMapping: RouteMapping{
					Method:     "POST",
					Path:       fullPath,
					Pattern:    path,
					Prefix:     prefix,
					ChiPattern: OpenAPIPatternToChi(fullPath),
					Operation:  pathItem.Post,
					Parameters: pathItem.Parameters,
					Responses:  pathItem.Post.Responses,
				},
			}
			mappings = append(mappings, mapping)
		}
	}

	return mappings, nil
}

func isUnderGateway(path, gateway, prefix string) bool {
	gwPath := PrefixPath(prefix, gateway)
	return path == gwPath || strings.HasPrefix(path, gwPath+"/")
}
