package loader

type ProcedureConfig struct {
	Call  string `json:"call"`
	Match string `json:"match"`
}

type RpcConfig struct {
	Gateway      string          `json:"gateway"`
	ProtocolType string          `json:"protocolType"`
	ContentType  string          `json:"contentType"`
	Procedure    ProcedureConfig `json:"procedure"`
}

type RpcRouteMapping struct {
	Procedure string
	RouteMapping
}
