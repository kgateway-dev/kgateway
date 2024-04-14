package assertions

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
)

func (p *Provider) RunningReplicas(deploymentMeta metav1.ObjectMeta, expectedReplicas int) ClusterAssertion {
	return func(ctx context.Context) {
		p.testingFramework.Helper()

		Eventually(func(g Gomega) {
			pods, err := kubeutils.GetPodsForDeployment(ctx, p.clusterContext.RestConfig, deploymentMeta.GetName(), deploymentMeta.GetNamespace())
			g.Expect(err).NotTo(HaveOccurred(), "can get pods for deployment")
			g.Expect(pods).To(HaveLen(expectedReplicas), "running pods matches expected count")
		}).
			WithContext(ctx).
			// It may take some time for pods to initialize and pull images from remote registries.
			// Therefore, we set a longer timeout, to account for latency that may exist.
			WithTimeout(time.Second * 30).
			WithPolling(time.Millisecond * 200).
			Should(Succeed())
	}
}
