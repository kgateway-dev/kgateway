package kubernetes

import (
	"fmt"
	"time"

	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	kubeutils "github.com/solo-io/gloo/pkg/utils/kube"
)

func NewEndpointDiscovery(masterUrl, kubeconfigPath string, resyncDuration time.Duration) (endpointdiscovery.Interface, error) {
	cfg, err := kubeutils.GetConfig(masterUrl, kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build rest config: %v", err)
	}

	ctl, err := newEndpointController(cfg, resyncDuration)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize endpoint controller: %v", err)
	}

	return ctl, nil
}
