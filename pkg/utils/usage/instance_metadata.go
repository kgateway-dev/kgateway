package usage

import (
	"runtime"

	reportingapi "github.com/solo-io/reporting-client/pkg/api/v1"
)

func BuildProductMetadata(product, version string) *reportingapi.Product {
	return &reportingapi.Product{
		Product: product,
		Version: version,
		Arch:    runtime.GOARCH,
		Os:      runtime.GOOS,
	}
}
