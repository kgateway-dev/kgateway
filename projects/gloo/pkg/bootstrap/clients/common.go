package clients

import (
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
)

const (
	DefaultK8sQPS   = 50     // 10x the k8s-recommended default; gloo gets busy writing status updates
	DefaultK8sBurst = 100    // 10x the k8s-recommended default; gloo gets busy writing status updates
	DefaultRootKey  = "gloo" // used for vault and consul key-value storage
)

func GetWriteNamespace(settings *v1.Settings) string {
	writeNamespace := settings.GetDiscoveryNamespace()
	if writeNamespace == "" {
		writeNamespace = defaults.GlooSystem
	}

	return writeNamespace
}
