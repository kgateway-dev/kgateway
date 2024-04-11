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

var _ = Describe("Query Get VirtualHostOptions", func() {

	var (
		ctx  context.Context
		deps []client.Object
		gw   *gwv1.Gateway
		qry  query.VirtualHostOptionQueries
	)

	BeforeEach(func() {
		ctx = context.Background()
		gw = &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
		}
	})

	JustBeforeEach(func() {
		builder := fake.NewClientBuilder().WithScheme(gwscheme.NewScheme())
		query.IterateIndices(func(o client.Object, f string, fun client.IndexerFunc) error {
			builder.WithIndex(o, f, fun)
			return nil
		})
		fakeClient := builder.WithObjects(deps...).Build()
		qry = query.NewQuery(fakeClient)
	})

	When("targetRef fully present", func() {
		BeforeEach(func() {
			deps = []client.Object{
				gw,
				attachedVirtualHostOption(),
				diffNamespaceVirtualHostOption(),
			}
		})
		It("should find the only attached option", func() {
			virtualHostOptionList, err := qry.GetVirtualHostOptionsForGateway(ctx, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(virtualHostOptionList).NotTo(BeNil())
			items := virtualHostOptionList.Items

			Expect(items).To(HaveLen(1))
			vhOpt := &items[0]
			Expect(vhOpt.GetName()).To(Equal("good-policy"))
			Expect(vhOpt.GetNamespace()).To(Equal("default"))
		})
	})

	When("no options in same namespace as gateway", func() {
		BeforeEach(func() {
			deps = []client.Object{
				gw,
				diffNamespaceVirtualHostOption(),
			}
		})
		It("should not find an attached option", func() {
			virtualHostOptionList, err := qry.GetVirtualHostOptionsForGateway(ctx, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(virtualHostOptionList).NotTo(BeNil())
			items := virtualHostOptionList.Items

			Expect(items).To(BeEmpty())
		})

	})

	When("targetRef has omitted namespace", func() {
		BeforeEach(func() {
			deps = []client.Object{
				gw,
				attachedVirtualHostOptionOmitNamespace(),
				diffNamespaceVirtualHostOption(),
			}
		})
		It("should find the attached option", func() {
			virtualHostOptionList, err := qry.GetVirtualHostOptionsForGateway(ctx, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(virtualHostOptionList).NotTo(BeNil())
			items := virtualHostOptionList.Items

			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(HaveLen(1))
			vhOpt := &items[0]
			Expect(vhOpt.GetName()).To(Equal("good-policy-no-ns"))
			Expect(vhOpt.GetNamespace()).To(Equal("default"))
		})

	})

	When("no options in namespace as gateway with omitted namespace", func() {
		BeforeEach(func() {
			deps = []client.Object{
				gw,
				diffNamespaceVirtualHostOptionOmitNamespace(),
			}
		})
		It("should not find an attached option", func() {
			virtualHostOptionList, err := qry.GetVirtualHostOptionsForGateway(ctx, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(virtualHostOptionList).NotTo(BeNil())
			items := virtualHostOptionList.Items

			Expect(err).NotTo(HaveOccurred())
			Expect(items).To(BeEmpty())
		})
	})
})

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
				Kind:      wellknown.GatewayKind,
				Name:      "test",
				Namespace: wrapperspb.String("default"),
			},
			Options: &v1.VirtualHostOptions{},
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
				Kind:  wellknown.GatewayKind,
				Name:  "test",
			},
			Options: &v1.VirtualHostOptions{},
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
				Kind:      wellknown.GatewayKind,
				Name:      "test",
				Namespace: wrapperspb.String("default"),
			},
			Options: &v1.VirtualHostOptions{},
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
				Kind:  wellknown.GatewayKind,
				Name:  "test",
			},
			Options: &v1.VirtualHostOptions{},
		},
	}
}
