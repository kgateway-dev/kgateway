package assertions

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/envoyutils/admincli"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/portforward"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (p *Provider) EnvoyAdminApiAssertion(
	envoyDeployment metav1.ObjectMeta,
	adminAssertion func(ctx context.Context, adminClient *admincli.Client),
) ClusterAssertion {
	return func(ctx context.Context) {
		p.testingFramework.Helper()

		portForwarder, err := p.clusterContext.Cli.StartPortForward(ctx,
			portforward.WithDeployment(envoyDeployment.GetName(), envoyDeployment.GetNamespace()),
			portforward.WithRemotePort(admincli.DefaultAdminPort),
		)
		Expect(err).NotTo(HaveOccurred(), "can open port-forward")
		defer func() {
			portForwarder.Close()
			portForwarder.WaitForStop()
		}()

		adminClient := admincli.NewClient().
			WithReceiver(p.testingProgressWriter).
			WithCurlOptions(
				curl.WithRetries(3, 0, 10),
				curl.WithHostPort(portForwarder.Address()),
			)
		adminAssertion(ctx, adminClient)
	}
}
