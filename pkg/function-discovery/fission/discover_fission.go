package fission

import (
	"errors"

	"github.com/solo-io/gloo/pkg/function-discovery/detector"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/rest"

	"github.com/solo-io/gloo/pkg/function-discovery"
	"github.com/solo-io/gloo/pkg/function-discovery/updater/fission"
)

type fissionDetector struct {
}

func NewFissionDetector() detector.Interface {
	return &fissionDetector{}
}

func (d *fissionDetector) DetectFunctionalService(us *v1.Upstream, addr string) (*v1.ServiceInfo, map[string]string, error) {
	if fission.IsFissionUpstream(us) {
		svcInfo := &v1.ServiceInfo{
			Type: rest.ServiceTypeREST,
		}
		annotations := make(map[string]string)
		annotations[functiondiscovery.DiscoveryTypeAnnotationKey] = "fission"
		return svcInfo, annotations, nil
	}

	return nil, nil, errors.New("not a fission upstream")
}
