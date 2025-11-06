//go:build e2e

package assertions

import (
	"context"
	"io"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/portforward"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/controllerutils/admincli"
)

func (p *Provider) AssertKgatewayAdminApi(
	ctx context.Context,
	kgatewayDeployment metav1.ObjectMeta,
	adminAssertions ...func(ctx context.Context, adminClient *admincli.Client),
) {
	// Before opening a port-forward, we assert that there is at least one Pod that is ready
	p.EventuallyReadyReplicas(ctx, kgatewayDeployment, BeNumerically(">=", 1))

	portForwarder, err := p.clusterContext.Cli.StartPortForward(ctx,
		portforward.WithDeployment(kgatewayDeployment.GetName(), kgatewayDeployment.GetNamespace()),
		portforward.WithRemotePort(int(wellknown.KgatewayAdminPort)),
	)
	p.Require.NoError(err, "can open port-forward")
	defer func() {
		portForwarder.Close()
		portForwarder.WaitForStop()
	}()

	adminClient := admincli.NewClient().
		WithReceiver(io.Discard). // adminAssertion can overwrite this
		WithCurlOptions(
			curl.WithRetries(3, 0, 10),
			curl.WithHostPort(portForwarder.Address()),
		)

	for _, adminAssertion := range adminAssertions {
		adminAssertion(ctx, adminClient)
	}
}
