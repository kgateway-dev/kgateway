//go:build e2e

package assertions

import (
	"context"
	"time"

	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
)

// EventuallyReadyReplicas asserts that given a Deployment, eventually the number of pods matching the replicaMatcher
// are in the ready state and able to receive traffic.
func (p *Provider) EventuallyReadyReplicas(ctx context.Context, deploymentMeta metav1.ObjectMeta, replicaMatcher types.GomegaMatcher) {
	p.Gomega.Eventually(func(innerG Gomega) {
		// We intentionally rely only on Pods that have marked themselves as ready as a way of defining more explicit assertions
		pods, err := kubeutils.GetReadyPodsForDeployment(ctx, p.clusterContext.Clientset, deploymentMeta)
		innerG.Expect(err).NotTo(HaveOccurred(), "can get pods for deployment")
		innerG.Expect(len(pods)).To(replicaMatcher, "running pods matches expected count")
	}).
		WithContext(ctx).
		// It may take some time for pods to initialize and pull images from remote registries.
		// Therefore, we set a longer timeout, to account for latency that may exist.
		WithTimeout(time.Second * 30).
		WithPolling(time.Millisecond * 200).
		Should(Succeed())
}
