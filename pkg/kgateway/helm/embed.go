package helm

import (
	"embed"
)

var (
	//go:embed all:envoy
	EnvoyHelmChart embed.FS
)
