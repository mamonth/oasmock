package loader

import (
	"fmt"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func specWithRpcExt(xrpc string) *openapi3.T {
	yaml := xrpcSpecBase + "\n" + xrpc
	return mustLoad([]byte(yaml))
}

const xrpcSpecBase = `openapi: "3.0.3"
info:
  title: Test
  version: "1.0"
paths:
  /rpc/subtract:
    post:
      operationId: subtract
      responses:
        "200":
          description: OK
  /rpc/add:
    post:
      operationId: add
      responses:
        "200":
          description: OK
`

func specFromYAML(yaml string) *openapi3.T {
	return mustLoad([]byte(yaml))
}

func mustLoad(data []byte) *openapi3.T {
	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData(data)
	if err != nil {
		panic(fmt.Sprintf("failed to load spec: %v", err))
	}
	return spec
}

/*
Scenario: ParseRpcConfig with valid full config returns correctly parsed struct
Given an OpenAPI spec with x-rpc containing all optional fields
When ParseRpcConfig is called
Then it returns a populated RpcConfig with all fields correctly parsed

Related spec scenarios: RS.JRP.1
*/
func TestParseRpcConfig_Valid(t *testing.T) {
	t.Parallel()

	spec := specWithRpcExt(`
x-rpc:
  gateway: /rpc
  protocolType: json-rpc
  contentType: application/json
  procedure:
    call: method
    match: post.operationId
`)

	cfg, err := ParseRpcConfig(spec)
	require.NoError(t, err)
	assert.Equal(t, "/rpc", cfg.Gateway)
	assert.Equal(t, "json-rpc", cfg.ProtocolType)
	assert.Equal(t, "application/json", cfg.ContentType)
	assert.Equal(t, "method", cfg.Procedure.Call)
	assert.Equal(t, "post.operationId", cfg.Procedure.Match)
}

/*
Scenario: ParseRpcConfig with missing gateway returns error
Given an OpenAPI spec with x-rpc but no gateway field
When ParseRpcConfig is called
Then it returns an error

Related spec scenarios: RS.JRP.3
*/
func TestParseRpcConfig_MissingGateway(t *testing.T) {
	t.Parallel()

	spec := specWithRpcExt(`
x-rpc:
  protocolType: json-rpc
  procedure:
    call: method
    match: post.operationId
`)

	_, err := ParseRpcConfig(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gateway")
}

/*
Scenario: ParseRpcConfig with unsupported protocolType returns error
Given an OpenAPI spec with x-rpc specifying an unsupported protocol type
When ParseRpcConfig is called
Then it returns an error

Related spec scenarios: RS.JRP.5
*/
func TestParseRpcConfig_UnsupportedProtocol(t *testing.T) {
	t.Parallel()

	spec := specWithRpcExt(`
x-rpc:
  gateway: /rpc
  protocolType: xml-rpc
  procedure:
    call: method
    match: post.operationId
`)

	_, err := ParseRpcConfig(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

/*
Scenario: ParseRpcConfig with missing procedure field returns error
Given an OpenAPI spec with x-rpc but no procedure field
When ParseRpcConfig is called
Then it returns an error

Related spec scenarios: RS.JRP.4
*/
func TestParseRpcConfig_MissingProcedure(t *testing.T) {
	t.Parallel()

	spec := specWithRpcExt(`
x-rpc:
  gateway: /rpc
  protocolType: json-rpc
`)

	_, err := ParseRpcConfig(spec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "procedure")
}

/*
Scenario: ParseRpcConfig uses default contentType for json-rpc when not specified
Given an OpenAPI spec with x-rpc but no contentType field
When ParseRpcConfig is called
Then contentType defaults to "application/json"

Related spec scenarios: RS.JRP.1
*/
func TestParseRpcConfig_DefaultContentType(t *testing.T) {
	t.Parallel()

	spec := specWithRpcExt(`
x-rpc:
  gateway: /rpc
  protocolType: json-rpc
  procedure:
    call: method
    match: post.operationId
`)

	cfg, err := ParseRpcConfig(spec)
	require.NoError(t, err)
	assert.Equal(t, "application/json", cfg.ContentType)
}

/*
Scenario: ParseRpcConfig returns nil when x-rpc extension is absent
Given an OpenAPI spec without x-rpc extension
When ParseRpcConfig is called
Then it returns nil, nil

Related spec scenarios: RS.JRP.2
*/
func TestParseRpcConfig_NoExtension(t *testing.T) {
	t.Parallel()

	spec := specWithRpcExt("") // no x-rpc in base spec

	cfg, err := ParseRpcConfig(spec)
	assert.NoError(t, err)
	assert.Nil(t, cfg)
}

/*
Scenario: ParseRpcConfig with malformed x-rpc (not a map) returns error
Given an OpenAPI spec where x-rpc is a scalar value instead of a map
When ParseRpcConfig is called
Then it returns an error

Related spec scenarios: RS.JRP.1
*/
func TestParseRpcConfig_Malformed(t *testing.T) {
	t.Parallel()

	yaml := xrpcSpecBase + `
x-rpc: "invalid"
`
	spec := specFromYAML(yaml)

	_, err := ParseRpcConfig(spec)
	assert.Error(t, err)
}

/*
Scenario: BuildRpcMappings maps paths under gateway with POST by operationId
Given schema infos with gateway /rpc and POST operations with operationIds
When BuildRpcMappings is called
Then it returns RpcRouteMappings keyed by operationId

Related spec scenarios: RS.JRP.6
*/
func TestBuildRpcMappings_ProceduresUnderGateway(t *testing.T) {
	t.Parallel()

	spec := specWithRpcExt(`
x-rpc:
  gateway: /rpc
  protocolType: json-rpc
  procedure:
    call: method
    match: post.operationId
`)

	cfg, err := ParseRpcConfig(spec)
	require.NoError(t, err)

	infos := []SchemaInfo{{Spec: spec, Prefix: ""}}
	mappings, err := BuildRpcMappings(infos, cfg)
	require.NoError(t, err)

	procMap := make(map[string]string)
	for _, m := range mappings {
		procMap[m.Procedure] = m.Path
	}

	assert.Equal(t, "/rpc/subtract", procMap["subtract"])
	assert.Equal(t, "/rpc/add", procMap["add"])
	assert.Len(t, mappings, 2)
}

/*
Scenario: BuildRpcMappings excludes paths not under gateway
Given schema infos with gateway /rpc and paths outside /rpc
When BuildRpcMappings is called
Then paths outside gateway are not in RPC mappings

Related spec scenarios: RS.JRP.7
*/
func TestBuildRpcMappings_PathsNotUnderGateway(t *testing.T) {
	t.Parallel()

	yaml := `openapi: "3.0.3"
info:
  title: Test
  version: "1.0"
paths:
  /rpc/subtract:
    post:
      operationId: subtract
      responses:
        "200":
          description: OK
  /users:
    post:
      operationId: createUser
      responses:
        "200":
          description: OK
    get:
      operationId: listUsers
      responses:
        "200":
          description: OK
x-rpc:
  gateway: /rpc
  protocolType: json-rpc
  procedure:
    call: method
    match: post.operationId
`
	spec := specFromYAML(yaml)

	cfg, err := ParseRpcConfig(spec)
	require.NoError(t, err)

	infos := []SchemaInfo{{Spec: spec, Prefix: ""}}
	mappings, err := BuildRpcMappings(infos, cfg)
	require.NoError(t, err)

	procedureNames := make(map[string]bool)
	for _, m := range mappings {
		procedureNames[m.Procedure] = true
	}

	assert.True(t, procedureNames["subtract"])
	assert.False(t, procedureNames["createUser"], "non-gateway operations should be excluded")
	assert.False(t, procedureNames["listUsers"], "non-gateway GET should be excluded")
	assert.Len(t, mappings, 1)
}

/*
Scenario: BuildRpcMappings excludes non-POST operations under gateway
Given schema infos with gateway /rpc and a GET operation under it
When BuildRpcMappings is called
Then GET operations are excluded from RPC mappings

Related spec scenarios: RS.JRP.6
*/
func TestBuildRpcMappings_NonPostExcluded(t *testing.T) {
	t.Parallel()

	yaml := `openapi: "3.0.3"
info:
  title: Test
  version: "1.0"
paths:
  /rpc/subtract:
    get:
      operationId: getSubtract
      responses:
        "200":
          description: OK
    post:
      operationId: subtract
      responses:
        "200":
          description: OK
x-rpc:
  gateway: /rpc
  protocolType: json-rpc
  procedure:
    call: method
    match: post.operationId
`
	spec := specFromYAML(yaml)

	cfg, err := ParseRpcConfig(spec)
	require.NoError(t, err)

	infos := []SchemaInfo{{Spec: spec, Prefix: ""}}
	mappings, err := BuildRpcMappings(infos, cfg)
	require.NoError(t, err)

	assert.Len(t, mappings, 1)
	assert.Equal(t, "subtract", mappings[0].Procedure)
}

/*
Scenario: BuildRpcMappings returns error on duplicate operationId under gateway
Given schema infos with two POST operations under gateway sharing the same operationId
When BuildRpcMappings is called
Then it returns an error

Related spec scenarios: RS.JRP.8
*/
func TestBuildRpcMappings_DuplicateOperationId(t *testing.T) {
	t.Parallel()

	yaml := `openapi: "3.0.3"
info:
  title: Test
  version: "1.0"
paths:
  /rpc/subtract:
    post:
      operationId: sub
      responses:
        "200":
          description: OK
  /rpc/minus:
    post:
      operationId: sub
      responses:
        "200":
          description: OK
x-rpc:
  gateway: /rpc
  protocolType: json-rpc
  procedure:
    call: method
    match: post.operationId
`
	spec := specFromYAML(yaml)

	cfg, err := ParseRpcConfig(spec)
	require.NoError(t, err)

	infos := []SchemaInfo{{Spec: spec, Prefix: ""}}
	_, err = BuildRpcMappings(infos, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

/*
Scenario: BuildRpcMappings returns empty when no POST operations under gateway
Given schema infos with gateway /rpc but only GET operations under it
When BuildRpcMappings is called
Then it returns no mappings and no error

Related spec scenarios: RS.JRP.9
*/
func TestBuildRpcMappings_NoPostUnderGateway(t *testing.T) {
	t.Parallel()

	yaml := `openapi: "3.0.3"
info:
  title: Test
  version: "1.0"
paths:
  /rpc/status:
    get:
      operationId: status
      responses:
        "200":
          description: OK
x-rpc:
  gateway: /rpc
  protocolType: json-rpc
  procedure:
    call: method
    match: post.operationId
`
	spec := specFromYAML(yaml)

	cfg, err := ParseRpcConfig(spec)
	require.NoError(t, err)

	infos := []SchemaInfo{{Spec: spec, Prefix: ""}}
	mappings, err := BuildRpcMappings(infos, cfg)
	require.NoError(t, err)
	assert.Empty(t, mappings)
}

/*
Scenario: BuildRpcMappings applies schema prefix to gateway path
Given schema infos with prefix /api and gateway /rpc
When BuildRpcMappings is called
Then gateway paths include the prefix

Related spec scenarios: RS.JRP.32
*/
func TestBuildRpcMappings_WithPrefix(t *testing.T) {
	t.Parallel()

	spec := specWithRpcExt(`
x-rpc:
  gateway: /rpc
  protocolType: json-rpc
  procedure:
    call: method
    match: post.operationId
`)

	cfg, err := ParseRpcConfig(spec)
	require.NoError(t, err)

	infos := []SchemaInfo{{Spec: spec, Prefix: "/api"}}
	mappings, err := BuildRpcMappings(infos, cfg)
	require.NoError(t, err)

	for _, m := range mappings {
		assert.Contains(t, m.Path, "/api/")
	}
}

/*
Scenario: Coexistence of RPC and normal HTTP route mappings
Given a spec with both RPC gateway and non-RPC paths
When both BuildRpcMappings and BuildRouteMappings are called
Then RPC mappings contain only gateway operations and RouteMappings contain all operations

Related spec scenarios: RS.JRP.31
*/
func TestBuildRpcMappings_Coexistence(t *testing.T) {
	t.Parallel()

	yaml := `openapi: "3.0.3"
info:
  title: Test
  version: "1.0"
paths:
  /rpc/subtract:
    post:
      operationId: subtract
      responses:
        "200":
          description: OK
  /rpc/add:
    post:
      operationId: add
      responses:
        "200":
          description: OK
  /users:
    get:
      operationId: listUsers
      responses:
        "200":
          description: OK
x-rpc:
  gateway: /rpc
  protocolType: json-rpc
  procedure:
    call: method
    match: post.operationId
`
	spec := specFromYAML(yaml)

	cfg, err := ParseRpcConfig(spec)
	require.NoError(t, err)

	infos := []SchemaInfo{{Spec: spec, Prefix: ""}}

	// Regular route mappings include everything
	routeMappings, err := BuildRouteMappings(infos)
	require.NoError(t, err)
	assert.Len(t, routeMappings, 3) // GET /users, POST /rpc/subtract, POST /rpc/add

	// RPC mappings include only gateway POST ops
	rpcMappings, err := BuildRpcMappings(infos, cfg)
	require.NoError(t, err)
	assert.Len(t, rpcMappings, 2) // subtract, add
}
