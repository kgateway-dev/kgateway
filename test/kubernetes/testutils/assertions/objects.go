package assertions

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (p *Provider) ObjectsExist(objects ...client.Object) ClusterAssertion {
	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()

		for _, o := range objects {
			Eventually(ctx, func(g Gomega) {
				err := p.clusterContext.Client.Get(ctx, client.ObjectKeyFromObject(o), o)
				g.Expect(err).NotTo(HaveOccurred(), "object should be available in cluster")
			}).
				WithContext(ctx).
				WithTimeout(time.Second * 10).
				WithPolling(time.Millisecond * 200).
				Should(Succeed())
		}
	}
}

func (p *Provider) ObjectsNotExist(objects ...client.Object) ClusterAssertion {
	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()

		for _, o := range objects {
			Eventually(ctx, func(g Gomega) {
				err := p.clusterContext.Client.Get(ctx, client.ObjectKeyFromObject(o), o)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "object should not be found in cluster")
			}).
				WithContext(ctx).
				WithTimeout(time.Second * 10).
				WithPolling(time.Millisecond * 200).
				Should(Succeed())
		}
	}
}

func (p *Provider) NamespaceNotExist(ns string) ClusterAssertion {
	return func(ctx context.Context) {
		ginkgo.GinkgoHelper()

		_, err := p.clusterContext.Clientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue(), "namespace should not be found in cluster")
	}
}
