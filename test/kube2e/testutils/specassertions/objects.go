package specassertions

import (
	"context"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/kube2e/testutils/spec"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

func (p *Provider) ObjectsExist(objects ...client.Object) spec.ScenarioAssertion {
	return func(ctx context.Context) {
		GinkgoHelper()

		for _, o := range objects {
			Eventually(ctx, func(g Gomega) {
				err := p.clusterContext.Client.Get(ctx, client.ObjectKeyFromObject(o), o)
				g.Expect(err).NotTo(HaveOccurred())
			}).
				WithContext(ctx).
				WithTimeout(time.Second * 10).
				WithPolling(time.Millisecond * 200).
				Should(Succeed())
		}
	}
}

func (p *Provider) ObjectsNotExist(objects ...client.Object) spec.ScenarioAssertion {
	return func(ctx context.Context) {
		GinkgoHelper()

		for _, o := range objects {
			Eventually(ctx, func(g Gomega) {
				err := p.clusterContext.Client.Get(ctx, client.ObjectKeyFromObject(o), o)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).
				WithContext(ctx).
				WithTimeout(time.Second * 10).
				WithPolling(time.Millisecond * 200).
				Should(Succeed())
		}
	}
}
