package clients

import (
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"k8s.io/client-go/rest"
)

// MustRestConfig returns MustRestConfigWithContext with an empty Kubernetes Context
func MustRestConfig() *rest.Config {
	restConfig, err := kubeutils.GetRestConfigWithKubeContext("")
	if err != nil {
		panic(err)
	}

	return restConfig
}
