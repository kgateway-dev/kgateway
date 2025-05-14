package helm

import (
	"embed"
)

var (
	//go:embed all:kgateway
	KgatewayHelmChart embed.FS

	//go:embed all:inference-extension
	InferenceExtensionHelmChart embed.FS
)
