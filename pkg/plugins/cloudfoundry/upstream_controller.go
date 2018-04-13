package cloudfoundry

import (
	"context"
	"time"

	"code.cloudfoundry.org/copilot"
	"github.com/solo-io/gloo/pkg/api/types/v1"
)

const generatedBy = "cloudfoundry-upstream-discovery"

type ServiceController struct {
	resyncDuration time.Duration
	ctx            context.Context
	client         copilot.IstioClient
}

func (sc *ServiceController) Run(stop <-chan struct{}) {
	ResyncLoop(sc.ctx, stop, sc.resync, sc.resyncDuration)
}

func (sc *ServiceController) GetDesiredUpstreams() ([]*v1.Upstream, error) {
	return GetUpstreams(sc.ctx, sc.client)
}

func (sc *ServiceController) resync() {
	err := sc.resyncWithError()
	if err != nil {
		// sc.errors <- err
	}
}
func (sc *ServiceController) resyncWithError() error {
	return syncUpstreams()
}

func syncUpstreams() error {
	panic("replace me when merged")
}
