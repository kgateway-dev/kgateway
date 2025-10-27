package helm

import (
	"embed"
)

var (
	//go:embed all:envoy
	EnvoyHelmChart embed.FS

	//go:embed all:agentgateway
	AgentgatewayHelmChart embed.FS

	//go:embed all:inference-extension
	InferenceExtensionHelmChart embed.FS
)
