package assertions

import (
	"context"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
)

func (p *Provider) RunningReplicas(objectMeta v1.ObjectMeta, expectedReplicas int) ClusterAssertion {
	return func(ctx context.Context) {
		p.testingFramework.Helper()

		Eventually(func(g Gomega) {
			pods, err := kubeutils.GetPodsForDeployment(ctx, p.clusterContext.RestConfig, objectMeta.GetName(), objectMeta.GetNamespace())
			g.Expect(err).NotTo(HaveOccurred(), "can get pods for deployment")
			g.Expect(pods).To(HaveLen(expectedReplicas))
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 30).
			WithPolling(time.Millisecond * 200).
			Should(Succeed())
	}
}
