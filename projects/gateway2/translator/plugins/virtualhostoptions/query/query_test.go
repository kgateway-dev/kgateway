package query_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/wrapperspb"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	gwscheme "github.com/solo-io/gloo/projects/gateway2/controller/scheme"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/virtualhostoptions/query"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	corev1 "github.com/solo-io/skv2/pkg/api/core.skv2.solo.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Query", func() {

	var builder *fake.ClientBuilder

	BeforeEach(func() {
		builder = fake.NewClientBuilder().WithScheme(gwscheme.NewScheme())
		query.IterateIndices(func(o client.Object, f string, fun client.IndexerFunc) error {
			builder.WithIndex(o, f, fun)
			return nil
		})
	})

	Describe("Get VirtualHostOptions", func() {
		It("should find the only attached option with a full targetRef", func() {
			ctx := context.Background()

			hr := httpRoute()
			deps := []client.Object{
				hr,
				attachedVirtualHostOption(),
				diffNamespaceVirtualHostOption(),
			}
			fakeClient := builder.WithObjects(deps...).Build()

			query := query.NewQuery(fakeClient)
			var virtualHostOptionList solokubev1.VirtualHostOptionList
			err := query.GetVirtualHostOptionsForGateway(ctx, hr, &virtualHostOptionList)
			items := virtualHostOptionList.Items

			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(HaveLen(1))
			rtOpt := &items[0]
			Expect(rtOpt.GetName()).To(Equal("good-policy"))
			Expect(rtOpt.GetNamespace()).To(Equal("default"))
		})

		It("should not find an attached option when none are in the same namespace as route", func() {
			ctx := context.Background()

			hr := httpRoute()
			deps := []client.Object{
				hr,
				diffNamespaceVirtualHostOption(),
			}
			fakeClient := builder.WithObjects(deps...).Build()

			query := query.NewQuery(fakeClient)
			var virtualHostOptionList solokubev1.VirtualHostOptionList
			err := query.GetVirtualHostOptionsForGateway(ctx, hr, &virtualHostOptionList)
			items := virtualHostOptionList.Items

			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(BeEmpty())
		})

		It("should find the only attached option with a targetRef with omitted namespace", func() {
			ctx := context.Background()

			hr := httpRoute()
			deps := []client.Object{
				hr,
				attachedVirtualHostOptionOmitNamespace(),
				diffNamespaceVirtualHostOption(),
			}
			fakeClient := builder.WithObjects(deps...).Build()

			query := query.NewQuery(fakeClient)
			var virtualHostOptionList solokubev1.VirtualHostOptionList
			err := query.GetVirtualHostOptionsForGateway(ctx, hr, &virtualHostOptionList)
			items := virtualHostOptionList.Items

			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(HaveLen(1))
			rtOpt := &items[0]
			Expect(rtOpt.GetName()).To(Equal("good-policy-no-ns"))
			Expect(rtOpt.GetNamespace()).To(Equal("default"))
		})

		It("should not find an attached option when none are in the same namespace as route with omitted namespace", func() {
			ctx := context.Background()

			hr := httpRoute()
			deps := []client.Object{
				hr,
				diffNamespaceVirtualHostOptionOmitNamespace(),
			}
			fakeClient := builder.WithObjects(deps...).Build()

			query := query.NewQuery(fakeClient)
			var virtualHostOptionList solokubev1.VirtualHostOptionList
			err := query.GetVirtualHostOptionsForGateway(ctx, hr, &virtualHostOptionList)
			items := virtualHostOptionList.Items

			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(BeEmpty())
		})
	})
})

func httpRoute() *gwv1.HTTPRoute {
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
	}
}

func attachedVirtualHostOption() *solokubev1.VirtualHostOption {
	now := metav1.Now()
	return &solokubev1.VirtualHostOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "good-policy",
			Namespace:         "default",
			CreationTimestamp: now,
		},
		Spec: sologatewayv1.VirtualHostOption{
			TargetRef: &corev1.PolicyTargetReferenceWithSectionName{
				Group:     gwv1.GroupVersion.Group,
				Kind:      wellknown.HTTPRouteKind,
				Name:      "test",
				Namespace: wrapperspb.String("default"),
			},
			Options: &v1.VirtualHostOptions{
				// TODO(jbohanon) put a real option here
			},
		},
	}
}

func attachedVirtualHostOptionOmitNamespace() *solokubev1.VirtualHostOption {
	now := metav1.Now()
	return &solokubev1.VirtualHostOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "good-policy-no-ns",
			Namespace:         "default",
			CreationTimestamp: now,
		},
		Spec: sologatewayv1.VirtualHostOption{
			TargetRef: &corev1.PolicyTargetReferenceWithSectionName{
				Group: gwv1.GroupVersion.Group,
				Kind:  wellknown.HTTPRouteKind,
				Name:  "test",
			},
			Options: &v1.VirtualHostOptions{
				// TODO(jbohanon) put a real option here
			},
		},
	}
}

func diffNamespaceVirtualHostOption() *solokubev1.VirtualHostOption {
	now := metav1.Now()
	return &solokubev1.VirtualHostOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "bad-policy",
			Namespace:         "non-default",
			CreationTimestamp: now,
		},
		Spec: sologatewayv1.VirtualHostOption{
			TargetRef: &corev1.PolicyTargetReferenceWithSectionName{
				Group:     gwv1.GroupVersion.Group,
				Kind:      wellknown.HTTPRouteKind,
				Name:      "test",
				Namespace: wrapperspb.String("default"),
			},
			Options: &v1.VirtualHostOptions{
				// TODO(jbohanon) put a real option here
			},
		},
	}
}

func diffNamespaceVirtualHostOptionOmitNamespace() *solokubev1.VirtualHostOption {
	now := metav1.Now()
	return &solokubev1.VirtualHostOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "bad-policy",
			Namespace:         "non-default",
			CreationTimestamp: now,
		},
		Spec: sologatewayv1.VirtualHostOption{
			TargetRef: &corev1.PolicyTargetReferenceWithSectionName{
				Group: gwv1.GroupVersion.Group,
				Kind:  wellknown.HTTPRouteKind,
				Name:  "test",
			},
			Options: &v1.VirtualHostOptions{
				// TODO(jbohanon) put a real option here
			},
		},
	}
}
