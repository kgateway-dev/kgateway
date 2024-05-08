package assertions

import (
	"context"
	"time"
)

func (p *Provider) EventuallyPodsAreReady(ctx context.Context, namespace string, timeout time.Duration, podNames ...string) error {
	return p.clusterContext.Cli.WaitUntilPodsRunning(ctx, timeout, namespace, podNames...)
}
