package specassertions

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/test/kube2e/testutils/spec"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"time"
)

func (p *Provider) RunningReplicas(ref *core.ResourceRef, expectedReplicas int) spec.ScenarioAssertion {
	return func(ctx context.Context) {
		GinkgoHelper()

		Eventually(func(g Gomega) {
			pods, err := kubeutils.GetPodsForDeployment(ctx, p.clusterContext.RestConfig, ref.GetName(), ref.GetNamespace())
			g.Expect(err).NotTo(HaveOccurred(), "can get pods for deployment")
			g.Expect(pods).To(HaveLen(expectedReplicas))
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(Succeed())
	}
}
