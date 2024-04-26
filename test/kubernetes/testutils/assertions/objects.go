package assertions

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (p *Provider) EventuallyObjectsExist(ctx context.Context, objects ...client.Object) {
	for _, o := range objects {
		p.Gomega.Eventually(ctx, func(innerG Gomega) {
			err := p.clusterContext.Client.Get(ctx, client.ObjectKeyFromObject(o), o)
			innerG.Expect(err).NotTo(HaveOccurred(), "object should be available in cluster")
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 20).
			WithPolling(time.Millisecond * 200).
			Should(Succeed())
	}
}

func (p *Provider) EventuallyObjectsNotExist(ctx context.Context, objects ...client.Object) {
	for _, o := range objects {
		p.Gomega.Eventually(ctx, func(innerG Gomega) {
			err := p.clusterContext.Client.Get(ctx, client.ObjectKeyFromObject(o), o)
			innerG.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "object should not be found in cluster")
		}).
			WithContext(ctx).
			WithTimeout(time.Second * 10).
			WithPolling(time.Millisecond * 200).
			Should(Succeed())
	}
}

func (p *Provider) ExpectNamespaceNotExist(ctx context.Context, ns string) {
	_, err := p.clusterContext.Clientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	p.Gomega.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "namespace should not be found in cluster")
}
