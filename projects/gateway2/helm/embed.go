package helm

import (
	"embed"
)

//go:embed all:gloo-gateway2
var GlooGateway2HelmChart embed.FS
