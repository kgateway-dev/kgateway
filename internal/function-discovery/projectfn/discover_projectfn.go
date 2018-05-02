package projectfn

import (
	"errors"

	"github.com/solo-io/gloo/internal/function-discovery"
	"github.com/solo-io/gloo/internal/function-discovery/detector"
	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/plugins/rest"

	updatefn "github.com/solo-io/gloo/internal/function-discovery/updater/projectfn"
)

type projectFnDetector struct {
}

func NewProjectFnDetector() detector.Interface {
	return &projectFnDetector{}
}

func (d *projectFnDetector) DetectFunctionalService(us *v1.Upstream, addr string) (*v1.ServiceInfo, map[string]string, error) {
	if updatefn.IsFnUpstream(us) {
		svcInfo := &v1.ServiceInfo{
			Type: rest.ServiceTypeREST,
		}

		annotations := make(map[string]string)
		annotations[functiondiscovery.DiscoveryTypeAnnotationKey] = "projectfn"
		return svcInfo, annotations, nil
	}

	return nil, nil, errors.New("not a projectfn upstream")
}
