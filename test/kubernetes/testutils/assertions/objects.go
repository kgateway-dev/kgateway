package assertions

import (
	"context"
	"time"

	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (p *Provider) ObjectsExist(objects ...client.Object) DiscreteAssertion {
	return func(ctx context.Context) {
		p.testingFramework.Helper()

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

func (p *Provider) ObjectsNotExist(objects ...client.Object) DiscreteAssertion {
	return func(ctx context.Context) {
		p.testingFramework.Helper()

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
