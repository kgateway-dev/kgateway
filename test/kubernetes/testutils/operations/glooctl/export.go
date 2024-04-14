package glooctl

import (
	"context"

	"github.com/solo-io/gloo/test/kubernetes/testutils/operations"
)

func (p *operationProviderImpl) ExportReport() operations.Operation {
	p.requiresGlooGatewayContext()

	return &operations.BasicOperation{
		OpName: "glooctl-export-report",
		OpAction: func(ctx context.Context) error {
			p.testingFramework.Logf("invoking `glooctl export report` for Gloo Gateway installation in %s", p.glooGatewayContext.InstallNamespace)

			// TODO: implement `glooctl export report`
			// This would be useful for developers debugging tests and administrators inspecting running installations

			return nil
		},
		// This operation is performed against a cluster when a test fails, and so we do not perform any assertions against it
		OpAssertions: nil,
	}
}
