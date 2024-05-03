package assertions

import (
	"context"
	"fmt"
	"github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (p *Provider) EventuallyObjectsExist(ctx context.Context, objects ...client.Object) {
	for _, o := range objects {
		p.Gomega.Eventually(ctx, func(innerG gomega.Gomega) {
			err := p.clusterContext.Client.Get(ctx, client.ObjectKeyFromObject(o), o)
			innerG.Expect(err).NotTo(gomega.HaveOccurred(), "object %s %s should be available in cluster", o.GetObjectKind().GroupVersionKind().String(), client.ObjectKeyFromObject(o).String())
		}).
			WithContext(ctx).
			WithTimeout(p.assertionOptions.Timeout).
			WithPolling(p.assertionOptions.Polling).
			Should(gomega.Succeed())
	}
}

func (p *Provider) EventuallyObjectsNotExist(ctx context.Context, objects ...client.Object) {
	for _, o := range objects {
		p.Gomega.Eventually(ctx, func(innerG gomega.Gomega) {
			err := p.clusterContext.Client.Get(ctx, client.ObjectKeyFromObject(o), o)
			innerG.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue(), "object %s %s should not be found in cluster", o.GetObjectKind().GroupVersionKind().String(), client.ObjectKeyFromObject(o).String())
		}).
			WithContext(ctx).
			WithTimeout(p.assertionOptions.Timeout).
			WithPolling(p.assertionOptions.Polling).
			Should(gomega.Succeed())
	}
}

func (p *Provider) ExpectNamespaceNotExist(ctx context.Context, ns string) {
	_, err := p.clusterContext.Clientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	p.Gomega.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue(), fmt.Sprintf("namespace %s should not be found in cluster", ns))
}
