package assertions

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/helpers"
	"github.com/solo-io/gloo/test/kube2e/helper"
)

func (p *Provider) EventuallyResourceExists(getter helpers.ResourceGetter, timeout ...time.Duration) {
	ginkgo.GinkgoHelper()

	currentTimeout, pollingInterval := helper.GetTimeouts(timeout...)
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
