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

		Eventually(func(g Gomega) {
			for _, o := range objects {
				g.Eventually(ctx, func() error {
					return p.clusterContext.Client.Get(ctx, client.ObjectKeyFromObject(o), o)
				}).Should(Succeed())
			}
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(Succeed())
	}
}

func (p *Provider) ObjectsNotExist(objects ...client.Object) spec.ScenarioAssertion {
	return func(ctx context.Context) {
		GinkgoHelper()

		Eventually(func(g Gomega) {
			for _, o := range objects {
				g.Eventually(ctx, func() error {
					return p.clusterContext.Client.Get(ctx, client.ObjectKeyFromObject(o), o)
				}).Should(MatchError(apierrors.IsNotFound))
			}
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(Succeed())
	}
}
