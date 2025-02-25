package helm

import (
	"embed"
)

//go:embed all:kgateway
var KgatewayHelmChart embed.FS
