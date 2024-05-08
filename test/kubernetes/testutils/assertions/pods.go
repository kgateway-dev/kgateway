package assertions

import (
	"context"
	"github.com/solo-io/k8s-utils/testutils/kube"
	"time"
)

func (p *Provider) EventuallyPodsAreReady(ctx context.Context, namespace string, timeout time.Duration, podNames ...string) error {
	return kube.WaitUntilPodsRunning(ctx, timeout, namespace, podNames...)
}

func (p *Provider) GetPodsInNamespace(ctx context.Context, namespace, labelSelector string) string {
	return kube.FindPodNameByLabel(p.clusterContext.RestConfig, ctx, namespace, labelSelector)
}
