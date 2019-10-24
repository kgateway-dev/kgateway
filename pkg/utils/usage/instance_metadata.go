package usage

import (
	"runtime"

	reportingapi "github.com/solo-io/reporting-client/pkg/api/v1"
)

func LoadInstanceMetadata(product, version string) *reportingapi.InstanceMetadata {
	return &reportingapi.InstanceMetadata{
		Product: product,
		Version: version,
		Arch:    runtime.GOARCH,
		Os:      runtime.GOOS,
	}
}
