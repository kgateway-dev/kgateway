package assertions

import (
	"context"
	"time"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/test/gomega/assertions"

	"github.com/solo-io/go-utils/stats"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"go.uber.org/zap/zapcore"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (p *Provider) EventuallyRunningReplicas(ctx context.Context, deploymentMeta metav1.ObjectMeta, replicaMatcher types.GomegaMatcher) {
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

func (p *Provider) EventuallyGlooReachesConsistentState(installNamespace string) {
	// We port-forward the Gloo deployment stats port to inspect the metrics and log settings
	// TODO(jbohanon) clean this up with newer style portforwarder
	glooStatsForwardConfig := assertions.StatsPortFwd{
		ResourceName:      "deployment/gloo",
		ResourceNamespace: installNamespace,
		LocalPort:         stats.DefaultPort,
		TargetPort:        stats.DefaultPort,
	}

	// Gloo components are configured to log to the Info level by default
	logLevelAssertion := assertions.LogLevelAssertion(zapcore.InfoLevel)

	// The emitter at some point should stabilize and not continue to increase the number of snapshots produced
	// We choose 4 here as a bit of a magic number, but we feel comfortable that if 4 consecutive polls of the metrics
	// endpoint returns that same value, then we have stabilized
	identicalResultInARow := 4
	emitterMetricAssertion, _ := assertions.IntStatisticReachesConsistentValueAssertion("api_gloosnapshot_gloo_solo_io_emitter_snap_out", identicalResultInARow)

	assertions.EventuallyStatisticsMatchAssertions(glooStatsForwardConfig,
		logLevelAssertion,
		emitterMetricAssertion,
	)
}
