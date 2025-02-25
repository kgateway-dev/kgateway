package helm

import (
	"embed"
)

//go:embed all:kgateway
var KGatewayHelmChart embed.FS
