package assertions

import (
	"context"
	"net"
	"time"

	"github.com/onsi/ginkgo/v2"

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
		ginkgo.GinkgoHelper()

		// Before opening a port-forward, we assert that there is at least one Pod that is ready
		p.RunningReplicas(envoyDeployment, BeNumerically(">=", 1))(ctx)

		portForwarder, err := p.clusterContext.Cli.StartPortForward(ctx,
			portforward.WithDeployment(envoyDeployment.GetName(), envoyDeployment.GetNamespace()),
			portforward.WithPorts(admincli.DefaultAdminPort, admincli.DefaultAdminPort),
		)
		Expect(err).NotTo(HaveOccurred(), "can open port-forward")
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

		adminClient := admincli.NewClient().
			WithReceiver(p.progressWriter).
			WithCurlOptions(
				curl.WithRetries(3, 0, 10),
				curl.WithPort(admincli.DefaultAdminPort),
			)
		adminAssertion(ctx, adminClient)
	}
}
