package helm

import (
	"embed"
)

//go:embed all:gloo-gateway
var GlooGatewayHelmChart embed.FS

//go:embed all:inference-extension
var InferenceExtensionHelmChart embed.FS
