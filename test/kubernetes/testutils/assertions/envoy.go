package assertions

import (
	"context"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/envoyutils/admincli"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/kubectl"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/portforward"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (p *Provider) EnvoyAdminApiAssertion(
	envoyDeployment v1.ObjectMeta,
	kubeClient *kubectl.Cli,
	adminAssertion func(ctx context.Context, adminClient *admincli.Client),
) DiscreteAssertion {
	return func(ctx context.Context) {
		portForwarder, err := kubeClient.StartPortForward(ctx,
			portforward.WithDeployment(envoyDeployment.GetName(), envoyDeployment.GetNamespace()),
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
