//go:build ignore

package assertions

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/test/helpers"
)

func (p *Provider) EventuallyResourceExists(getter helpers.ResourceGetter, timeout ...time.Duration) {
	ginkgo.GinkgoHelper()

	currentTimeout, pollingInterval := helpers.GetTimeouts(timeout...)
	gomega.Eventually(func(g gomega.Gomega) {
		_, err := getter()
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get resource")
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}

func (p *Provider) ConsistentlyResourceExists(ctx context.Context, getter helpers.ResourceGetter) {
	p.Gomega.Consistently(ctx, func(innerG gomega.Gomega) {
		_, err := getter()
		innerG.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get resource")
	}).
		WithContext(ctx).
		WithTimeout(time.Second*5).
		WithPolling(time.Second*1).
		Should(gomega.Succeed(), "resource should be found in cluster")
}
