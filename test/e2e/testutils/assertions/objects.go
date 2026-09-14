//go:build e2e

package assertions

import (
	"context"
	"fmt"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

func (p *Provider) EventuallyObjectsExist(ctx context.Context, objects ...client.Object) {
	p.t.Helper()
	for _, o := range objects {
		p.Gomega.Eventually(ctx, func(innerG Gomega) {
			err := p.clusterContext.Client.Get(ctx, client.ObjectKeyFromObject(o), o)
			innerG.Expect(err).NotTo(HaveOccurred(), "object %s %s should be available in cluster", o.GetObjectKind().GroupVersionKind().String(), client.ObjectKeyFromObject(o).String())
		}).
			WithContext(ctx).
			WithTimeout(time.Second*20).
			WithPolling(time.Millisecond*200).
			Should(Succeed(), fmt.Sprintf("object %s %s should be available in cluster", o.GetObjectKind().GroupVersionKind().String(), client.ObjectKeyFromObject(o).String()))
	}
}

// EventuallyCRDsEstablished asserts that the named CustomResourceDefinitions exist and have
// their Established condition set to True. Helm returns as soon as it has applied the CRD
// manifests, before the API server has necessarily finished registering them, so callers that
// immediately create custom resources of those kinds should wait on this first to avoid racing
// the apiserver (surfaces as "no matches for kind" errors).
func (p *Provider) EventuallyCRDsEstablished(ctx context.Context, crdNames ...string) {
	p.t.Helper()
	for _, name := range crdNames {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		p.Gomega.Eventually(ctx, func(innerG Gomega) {
			innerG.Expect(p.clusterContext.Client.Get(ctx, client.ObjectKey{Name: name}, crd)).NotTo(HaveOccurred())
			innerG.Expect(crd.Status.Conditions).To(ContainElement(SatisfyAll(
				HaveField("Type", apiextensionsv1.Established),
				HaveField("Status", apiextensionsv1.ConditionTrue),
			)), fmt.Sprintf("CRD %s should be Established", name))
		}).
			WithContext(ctx).
			WithTimeout(time.Second*20).
			WithPolling(time.Millisecond*200).
			Should(Succeed(), fmt.Sprintf("CRD %s should become Established", name))
	}
}

func (p *Provider) EventuallyObjectsNotExist(ctx context.Context, objects ...client.Object) {
	p.t.Helper()
	for _, o := range objects {
		p.Gomega.Eventually(ctx, func(innerG Gomega) {
			err := p.clusterContext.Client.Get(ctx, client.ObjectKeyFromObject(o), o)
			innerG.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "object %s %s should not be found in cluster", o.GetObjectKind().GroupVersionKind().String(), client.ObjectKeyFromObject(o).String())
		}).
			WithContext(ctx).
			WithTimeout(time.Second*60).
			WithPolling(time.Millisecond*200).
			Should(Succeed(), fmt.Sprintf("object %s %s should not be found in cluster", o.GetObjectKind().GroupVersionKind().String(), client.ObjectKeyFromObject(o).String()))
	}
}

// EventuallyObjectTypesNotExist asserts that eventually no objects of the specified types exist on the cluster.
// The `objectLists` holds the object list types to check, e.g. to check that no HTTPRoutes exist on the cluster, pass in HTTPRouteList{}
func (p *Provider) EventuallyObjectTypesNotExist(ctx context.Context, objectLists ...client.ObjectList) {
	p.t.Helper()
	for _, o := range objectLists {
		p.Gomega.Eventually(ctx, func(innerG Gomega) {
			err := p.clusterContext.Client.List(ctx, o)
			p.Assert.NoError(err, "can list %T", o)
			innerG.Expect(o).To(HaveField("Items", HaveLen(0)))
		}).
			WithContext(ctx).
			WithTimeout(time.Second*20).
			WithPolling(time.Millisecond*200).
			Should(Succeed(), fmt.Sprintf("object type %T should not be found in cluster", o))
	}
}

func (p *Provider) ConsistentlyObjectsNotExist(ctx context.Context, objects ...client.Object) {
	p.t.Helper()
	for _, o := range objects {
		p.Gomega.Consistently(ctx, func(innerG Gomega) {
			err := p.clusterContext.Client.Get(ctx, client.ObjectKeyFromObject(o), o)
			innerG.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "object %s %s should not be found in cluster", o.GetObjectKind().GroupVersionKind().String(), client.ObjectKeyFromObject(o).String())
		}).
			WithContext(ctx).
			WithTimeout(time.Second*10).
			WithPolling(time.Second*1).
			Should(Succeed(), fmt.Sprintf("object %s %s should not be found in cluster", o.GetObjectKind().GroupVersionKind().String(), client.ObjectKeyFromObject(o).String()))
	}
}

func (p *Provider) ExpectNamespaceNotExist(ctx context.Context, ns string) {
	p.t.Helper()
	_, err := p.clusterContext.Clientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	p.Gomega.Expect(apierrors.IsNotFound(err)).To(BeTrue(), fmt.Sprintf("namespace %s should not be found in cluster", ns))
}

func (p *Provider) EventuallyNamespaceExists(ctx context.Context, ns string) {
	p.t.Helper()
	p.Gomega.Eventually(ctx, func(innerG Gomega) {
		_, err := p.clusterContext.Clientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
		innerG.Expect(err).NotTo(HaveOccurred(), "namespace %s should exist", ns)
	}).
		WithContext(ctx).
		WithTimeout(time.Second*20).
		WithPolling(time.Millisecond*200).
		Should(Succeed(), fmt.Sprintf("namespace %s should exist", ns))
}

func (p *Provider) ExpectObjectDeleted(manifest string, err error, actualOutput string) {
	p.t.Helper()
	p.Assert.NoError(err, "can delete "+manifest)
}
