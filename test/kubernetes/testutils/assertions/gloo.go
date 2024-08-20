package assertions

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/pkg/utils/glooadminutils/admincli"
	"github.com/solo-io/gloo/pkg/utils/kubeutils/portforward"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

func containElementMatcher(gvk schema.GroupVersionKind, meta metav1.ObjectMeta) types.GomegaMatcher {
	return gomega.ContainElement(
		gomega.And(
			gomega.HaveKeyWithValue("kind", gomega.Equal(gvk.Kind)),
			gomega.HaveKeyWithValue("apiVersion", gomega.Equal(gvk.GroupVersion().String())),
			gomega.HaveKeyWithValue("metadata", gomega.And(
				gomega.HaveKeyWithValue("name", meta.GetName()),
				gomega.HaveKeyWithValue("namespace", meta.GetNamespace()),
			)),
		),
	)
}
func (p *Provider) InputSnapshotContainsElement(gvk schema.GroupVersionKind, meta metav1.ObjectMeta) func(ctx context.Context, adminClient *admincli.Client) {
	return p.InputSnapshotMatches(containElementMatcher(gvk, meta))
}

func (p *Provider) InputSnapshotDoesNotContainElement(gvk schema.GroupVersionKind, meta metav1.ObjectMeta) func(ctx context.Context, adminClient *admincli.Client) {
	return p.InputSnapshotMatches(gomega.Not(containElementMatcher(gvk, meta)))
}

func (p *Provider) InputSnapshotMatches(inputSnapshotMatcher types.GomegaMatcher) func(ctx context.Context, adminClient *admincli.Client) {
	return func(ctx context.Context, adminClient *admincli.Client) {
		p.Gomega.Eventually(func(g gomega.Gomega) {
			inputSnapshot, err := adminClient.GetInputSnapshot(ctx)
			g.Expect(err).NotTo(gomega.HaveOccurred(), "error getting input snapshot")
			g.Expect(inputSnapshot).NotTo(gomega.BeEmpty(), "objects are returned")
			g.Expect(inputSnapshot).To(inputSnapshotMatcher)
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(gomega.Succeed())
	}
}
