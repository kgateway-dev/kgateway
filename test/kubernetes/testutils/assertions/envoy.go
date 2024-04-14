package assertions

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/envoyutils/admincli"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/portforward"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (p *Provider) EnvoyAdminApiAssertion(
	envoyDeployment v1.ObjectMeta,
	adminAssertion func(ctx context.Context, adminClient *admincli.Client),
) ClusterAssertion {
	return func(ctx context.Context) {
		p.testingFramework.Helper()

		portForwarder, err := p.clusterContext.Cli.StartPortForward(ctx,
			portforward.WithDeployment(envoyDeployment.GetName(), envoyDeployment.GetNamespace()),
			// TODO: Help Wanted
			// This always selects the DefaultAdminPort as the local port.
			// If we want to run tests in parallel, this will cause problems.
			// We should improve this to instead use the `portforward.WithPort` option,
			// which selects an open port, and then we can open a curl against the portForwarder.Address()
			portforward.WithPorts(admincli.DefaultAdminPort, admincli.DefaultAdminPort),
		)
		Expect(err).NotTo(HaveOccurred())
		defer func() {
			portForwarder.Close()
			portForwarder.WaitForStop()
		}()

		adminClient := admincli.NewClient().WithCurlOptions(curl.WithPort(admincli.DefaultAdminPort))
		adminAssertion(ctx, adminClient)
	}
}
