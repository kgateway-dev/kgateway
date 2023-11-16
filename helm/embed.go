package helm

import (
	"embed"
)

//go:embed all:gloo-gateway
var GlooGatewayHelmChart embed.FS
