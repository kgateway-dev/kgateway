package query_test

import (
	"context"

	"github.com/solo-io/gloo/pkg/schemes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/wrapperspb"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
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
		ctx      context.Context
		deps     []client.Object
		gw       *gwv1.Gateway
		listener *gwv1.Listener
		qry      query.VirtualHostOptionQueries
	)

	BeforeEach(func() {
		ctx = context.Background()
		gw = &gwv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "test",
			},
		}
		listener = &gwv1.Listener{
			Name: "test-listener",
		}
	})

	JustBeforeEach(func() {
		builder := fake.NewClientBuilder().WithScheme(schemes.DefaultScheme())
		query.IterateIndices(func(o client.Object, f string, fun client.IndexerFunc) error {
			builder.WithIndex(o, f, fun)
			return nil
		})
		fakeClient := builder.WithObjects(deps...).Build()
		qry = query.NewQuery(fakeClient)
	})

	When("targetRef fully present without sectionName", func() {
		BeforeEach(func() {
			deps = []client.Object{
				gw,
				attachedVirtualHostOption(),
				diffNamespaceVirtualHostOption(),
			}
		})
		It("should find the only attached option", func() {
			virtualHostOptions, err := qry.GetVirtualHostOptionsForListener(ctx, listener, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(virtualHostOptions).NotTo(BeNil())
			Expect(virtualHostOptions).To(HaveLen(1))
			Expect(virtualHostOptions[0].GetName()).To(Equal("good-policy"))
			Expect(virtualHostOptions[0].GetNamespace()).To(Equal("default"))
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
			virtualHostOptions, err := qry.GetVirtualHostOptionsForListener(ctx, listener, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(virtualHostOptions).To(BeNil())
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
			virtualHostOptions, err := qry.GetVirtualHostOptionsForListener(ctx, listener, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(virtualHostOptions).NotTo(BeNil())
			Expect(virtualHostOptions).To(HaveLen(1))

			Expect(virtualHostOptions[0].GetName()).To(Equal("good-policy-no-ns"))
			Expect(virtualHostOptions[0].GetNamespace()).To(Equal("default"))
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
			virtualHostOptions, err := qry.GetVirtualHostOptionsForListener(ctx, listener, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(virtualHostOptions).To(BeNil())
		})
	})
	When("targetRef has section name matching listener", func() {
		When("no other options", func() {
			BeforeEach(func() {
				deps = []client.Object{
					gw,
					attachedVirtualHostOptionWithSectionName(),
				}
			})
			It("should find the attached option specified by section name", func() {
				virtualHostOptions, err := qry.GetVirtualHostOptionsForListener(ctx, listener, gw)
				Expect(err).NotTo(HaveOccurred())
				Expect(virtualHostOptions).NotTo(BeNil())
				Expect(virtualHostOptions).To(HaveLen(1))

				Expect(virtualHostOptions[0].GetName()).To(Equal("good-policy-with-section-name"))
				Expect(virtualHostOptions[0].GetNamespace()).To(Equal("default"))
			})
		})
		When("no other options with section name", func() {
			BeforeEach(func() {
				deps = []client.Object{
					gw,
					attachedVirtualHostOptionWithSectionName(),
					attachedVirtualHostOption(),
				}
			})
			It("should find the attached option with and without section name", func() {
				virtualHostOptions, err := qry.GetVirtualHostOptionsForListener(ctx, listener, gw)
				Expect(err).NotTo(HaveOccurred())
				Expect(virtualHostOptions).NotTo(BeNil())

				Expect(virtualHostOptions).To(HaveLen(2))
				Expect(virtualHostOptions[0].GetName()).To(Equal("good-policy-with-section-name"))
				Expect(virtualHostOptions[0].GetNamespace()).To(Equal("default"))

				Expect(virtualHostOptions[1].GetName()).To(Equal("good-policy"))
				Expect(virtualHostOptions[1].GetNamespace()).To(Equal("default"))
			})
		})
		When("targetRef has non-matching section name", func() {
			When("no other options", func() {
				BeforeEach(func() {
					deps = []client.Object{
						gw,
						attachedVirtualHostOptionWithDiffSectionName(),
					}
				})
				It("should not find any attached options", func() {
					virtualHostOptions, err := qry.GetVirtualHostOptionsForListener(ctx, listener, gw)
					Expect(err).NotTo(HaveOccurred())
					Expect(virtualHostOptions).To(BeNil())
				})
			})
			When("gateway-targeted options exist", func() {
				BeforeEach(func() {
					deps = []client.Object{
						gw,
						attachedVirtualHostOption(),
						attachedVirtualHostOptionWithDiffSectionName(),
					}
				})
				It("should find the gateway-level attached options", func() {
					virtualHostOptions, err := qry.GetVirtualHostOptionsForListener(ctx, listener, gw)
					Expect(err).NotTo(HaveOccurred())
					Expect(virtualHostOptions).NotTo(BeNil())
					Expect(virtualHostOptions).To(HaveLen(1))
					Expect(virtualHostOptions[0].GetName()).To(Equal("good-policy"))
					Expect(virtualHostOptions[0].GetNamespace()).To(Equal("default"))
				})
			})
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
			TargetRefs: []*corev1.PolicyTargetReferenceWithSectionName{
				{
					Group:     gwv1.GroupVersion.Group,
					Kind:      wellknown.GatewayKind,
					Name:      "test",
					Namespace: wrapperspb.String("default"),
				},
			},
			Options: &v1.VirtualHostOptions{},
		},
	}
}
func attachedVirtualHostOptionWithSectionName() *solokubev1.VirtualHostOption {
	vhOpt := attachedVirtualHostOption()
	vhOpt.ObjectMeta.Name = "good-policy-with-section-name"
	vhOpt.Spec.TargetRefs[0].SectionName = &wrapperspb.StringValue{
		Value: "test-listener",
	}
	return vhOpt
}

func attachedVirtualHostOptionWithDiffSectionName() *solokubev1.VirtualHostOption {
	vhOpt := attachedVirtualHostOption()
	vhOpt.ObjectMeta.Name = "bad-policy-with-section-name"
	vhOpt.Spec.TargetRefs[0].SectionName = &wrapperspb.StringValue{
		Value: "not-our-listener",
	}
	return vhOpt
}

func attachedVirtualHostOptionOmitNamespace() *solokubev1.VirtualHostOption {
	vhOpt := attachedVirtualHostOption()
	vhOpt.ObjectMeta.Name = "good-policy-no-ns"
	vhOpt.Spec.TargetRefs[0].Namespace = nil
	return vhOpt
}

func diffNamespaceVirtualHostOption() *solokubev1.VirtualHostOption {
	vhOpt := attachedVirtualHostOption()
	vhOpt.ObjectMeta.Name = "bad-policy"
	vhOpt.ObjectMeta.Namespace = "non-default"
	return vhOpt
}

func diffNamespaceVirtualHostOptionOmitNamespace() *solokubev1.VirtualHostOption {
	vhOpt := attachedVirtualHostOption()
	vhOpt.ObjectMeta.Name = "bad-policy"
	vhOpt.ObjectMeta.Namespace = "non-default"
	vhOpt.Spec.TargetRefs[0].Namespace = nil
	return vhOpt
}
