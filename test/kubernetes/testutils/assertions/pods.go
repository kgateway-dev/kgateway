package assertions

import (
	"context"
	"github.com/solo-io/k8s-utils/testutils/kube"
	"k8s.io/apimachinery/pkg/labels"
	"time"
)

func (p *Provider) EventuallyPodsAreReady(ctx context.Context, namespace string, timeout time.Duration, podNames ...string) error {
	return kube.WaitUntilPodsRunning(ctx, timeout, namespace, podNames...)
}

func (p *Provider) FindPodNameByLabel(ctx context.Context, namespace string, labelSelector map[string]string) string {
	return kube.FindPodNameByLabel(p.clusterContext.RestConfig, ctx, namespace, labels.SelectorFromSet(labelSelector).String())
}
