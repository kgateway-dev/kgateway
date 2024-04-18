package glooctl

import (
	"context"

	"github.com/onsi/ginkgo/v2"

	"github.com/solo-io/gloo/test/kubernetes/testutils/actions"
)

func (p *providerImpl) ExportReport() actions.ClusterAction {
	p.requiresGlooGatewayContext()

	return func(ctx context.Context) error {
		ginkgo.GinkgoWriter.Print("invoking `glooctl export report` for Gloo Gateway installation in %s", p.glooGatewayContext.InstallNamespace)

		// TODO: implement `glooctl export report`
		// This would be useful for developers debugging tests and administrators inspecting running installations

		return nil
	}
}
