package assertions

import (
	"context"
	"net"
	"time"

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

		// the port-forward returns before it completely starts up (https://github.com/solo-io/gloo/issues/9353),
		// so as a workaround we try to keep dialing the address until it succeeds
		Eventually(func(g Gomega) {
			_, err = net.Dial("tcp", portForwarder.Address())
			g.Expect(err).NotTo(HaveOccurred())
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 15).
			WithPolling(time.Second).
			Should(Succeed())

		adminClient := admincli.NewClient().WithCurlOptions(
			curl.WithRetries(3, 0, 10),
			curl.WithPort(admincli.DefaultAdminPort),
		)
		adminAssertion(ctx, adminClient)
	}
}
