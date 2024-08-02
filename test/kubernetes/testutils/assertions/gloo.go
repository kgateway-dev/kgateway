package assertions

import (
	"context"
	"io"
	"net"
	"time"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/glooadminutils/admincli"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/portforward"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (p *Provider) AssertGlooAdminApi(
	ctx context.Context,
	glooDeployment metav1.ObjectMeta,
	adminAssertions ...func(ctx context.Context, adminClient *admincli.Client),
) {
	// Before opening a port-forward, we assert that there is at least one Pod that is ready
	p.EventuallyRunningReplicas(ctx, glooDeployment, BeNumerically(">=", 1))

	portForwarder, err := p.clusterContext.Cli.StartPortForward(ctx,
		portforward.WithDeployment(glooDeployment.GetName(), glooDeployment.GetNamespace()),
		portforward.WithPorts(admincli.DefaultAdminPort, admincli.DefaultAdminPort),
	)
	p.Require.NoError(err, "can open port-forward")
	defer func() {
		portForwarder.Close()
		portForwarder.WaitForStop()
	}()

	// the port-forward returns before it completely starts up (https://github.com/solo-io/gloo/issues/9353),
	// so as a workaround we try to keep dialing the address until it succeeds
	p.Gomega.Eventually(func(g Gomega) {
		_, err = net.Dial("tcp", portForwarder.Address())
		g.Expect(err).NotTo(HaveOccurred())
	}).
		WithContext(ctx).
		WithTimeout(time.Second * 15).
		WithPolling(time.Second).
		Should(Succeed())

	adminClient := admincli.NewClient().
		WithReceiver(io.Discard). // adminAssertion can overwrite this
		WithCurlOptions(
			curl.WithRetries(3, 0, 10),
			curl.WithPort(admincli.DefaultAdminPort),
		)

	for _, adminAssertion := range adminAssertions {
		adminAssertion(ctx, adminClient)
	}
}
