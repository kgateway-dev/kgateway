package glooctl

import (
	"context"

	"github.com/solo-io/gloo/test/kubernetes/testutils/actions"
)

func (p *providerImpl) ExportReport() actions.ClusterAction {
	p.requiresGlooGatewayContext()

	return func(ctx context.Context) error {
		p.testingFramework.Logf("invoking `glooctl export report` for Gloo Gateway installation in %s", p.glooGatewayContext.InstallNamespace)

		// TODO: implement `glooctl export report`
		// This would be useful for developers debugging tests and administrators inspecting running installations

		panic("not implemented")
		return nil
	}
}
