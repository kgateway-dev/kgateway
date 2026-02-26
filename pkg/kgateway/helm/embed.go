package helm

import (
	"embed"
)

//go:embed all:envoy
var EnvoyHelmChart embed.FS
