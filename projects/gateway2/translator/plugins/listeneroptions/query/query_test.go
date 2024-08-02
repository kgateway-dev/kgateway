package query_test

import (
	"context"

	"github.com/solo-io/gloo/pkg/schemes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/wrapperspb"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/listeneroptions/query"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	corev1 "github.com/solo-io/skv2/pkg/api/core.skv2.solo.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Query Get ListenerOptions", func() {

	var (
		ctx      context.Context
		deps     []client.Object
		gw       *gwv1.Gateway
		listener *gwv1.Listener
		qry      query.ListenerOptionQueries
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
				attachedListenerOption(),
				diffNamespaceListenerOption(),
			}
		})
		It("should find the only attached option", func() {
			listenerOptions, err := qry.GetAttachedListenerOptions(ctx, listener, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(listenerOptions).NotTo(BeNil())
			Expect(listenerOptions).To(HaveLen(1))
			Expect(listenerOptions[0].GetName()).To(Equal("good-policy"))
			Expect(listenerOptions[0].GetNamespace()).To(Equal("default"))
		})
	})

	When("no options in same namespace as gateway", func() {
		BeforeEach(func() {
			deps = []client.Object{
				gw,
				diffNamespaceListenerOption(),
			}
		})
		It("should not find an attached option", func() {
			listenerOptions, err := qry.GetAttachedListenerOptions(ctx, listener, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(listenerOptions).To(BeNil())
		})
	})

	When("targetRef has omitted namespace", func() {
		BeforeEach(func() {
			deps = []client.Object{
				gw,
				attachedListenerOptionOmitNamespace(),
				diffNamespaceListenerOption(),
			}
		})
		It("should find the attached option", func() {
			listenerOptions, err := qry.GetAttachedListenerOptions(ctx, listener, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(listenerOptions).NotTo(BeNil())
			Expect(listenerOptions).To(HaveLen(1))

			Expect(listenerOptions[0].GetName()).To(Equal("good-policy-no-ns"))
			Expect(listenerOptions[0].GetNamespace()).To(Equal("default"))
		})
	})

	When("no options in namespace as gateway with omitted namespace", func() {
		BeforeEach(func() {
			deps = []client.Object{
				gw,
				diffNamespaceListenerOptionOmitNamespace(),
			}
		})
		It("should not find an attached option", func() {
			listenerOptions, err := qry.GetAttachedListenerOptions(ctx, listener, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(listenerOptions).To(BeNil())
		})
	})

	When("multiple targetRefs fully present without sectionName", func() {
		When("first targetRef in list matches", func() {
			BeforeEach(func() {
				deps = []client.Object{
					gw,
					attachedListenerOptionMultipleTargetRefHit(),
				}
			})
			It("should find the only attached option", func() {
				listenerOptions, err := qry.GetAttachedListenerOptions(ctx, listener, gw)
				Expect(err).NotTo(HaveOccurred())
				Expect(listenerOptions).NotTo(BeNil())
				Expect(listenerOptions).To(HaveLen(1))
				Expect(listenerOptions[0].GetName()).To(Equal("good-policy"))
				Expect(listenerOptions[0].GetNamespace()).To(Equal("default"))
			})
		})
		When("first targetRef in list does not match", func() {
			BeforeEach(func() {
				deps = []client.Object{
					gw,
					attachedListenerOptionMultipleTargetRefMiss(),
				}
			})
			It("should not find an attached option", func() {
				listenerOptions, err := qry.GetAttachedListenerOptions(ctx, listener, gw)
				Expect(err).NotTo(HaveOccurred())
				Expect(listenerOptions).To(BeNil())
			})
		})
	})

	When("targetRef has section name matching listener", func() {
		When("no other options", func() {
			BeforeEach(func() {
				deps = []client.Object{
					gw,
					attachedListenerOptionWithSectionName(),
				}
			})
			It("should find the attached option specified by section name", func() {
				listenerOptions, err := qry.GetAttachedListenerOptions(ctx, listener, gw)
				Expect(err).NotTo(HaveOccurred())
				Expect(listenerOptions).NotTo(BeNil())
				Expect(listenerOptions).To(HaveLen(1))

				Expect(listenerOptions[0].GetName()).To(Equal("good-policy-with-section-name"))
				Expect(listenerOptions[0].GetNamespace()).To(Equal("default"))
			})
		})
		When("no other options with section name", func() {
			BeforeEach(func() {
				deps = []client.Object{
					gw,
					attachedListenerOptionWithSectionName(),
					attachedListenerOption(),
				}
			})
			It("should find the attached option with and without section name", func() {
				listenerOptions, err := qry.GetAttachedListenerOptions(ctx, listener, gw)
				Expect(err).NotTo(HaveOccurred())
				Expect(listenerOptions).NotTo(BeNil())

				Expect(listenerOptions).To(HaveLen(2))
				Expect(listenerOptions[0].GetName()).To(Equal("good-policy-with-section-name"))
				Expect(listenerOptions[0].GetNamespace()).To(Equal("default"))

				Expect(listenerOptions[1].GetName()).To(Equal("good-policy"))
				Expect(listenerOptions[1].GetNamespace()).To(Equal("default"))
			})
		})
		When("targetRef has non-matching section name", func() {
			When("no other options", func() {
				BeforeEach(func() {
					deps = []client.Object{
						gw,
						attachedListenerOptionWithDiffSectionName(),
					}
				})
				It("should not find any attached options", func() {
					listenerOptions, err := qry.GetAttachedListenerOptions(ctx, listener, gw)
					Expect(err).NotTo(HaveOccurred())
					Expect(listenerOptions).To(BeNil())
				})
			})
			When("gateway-targeted options exist", func() {
				BeforeEach(func() {
					deps = []client.Object{
						gw,
						attachedListenerOption(),
						attachedListenerOptionWithDiffSectionName(),
					}
				})
				It("should find the gateway-level attached options", func() {
					listenerOptions, err := qry.GetAttachedListenerOptions(ctx, listener, gw)
					Expect(err).NotTo(HaveOccurred())
					Expect(listenerOptions).NotTo(BeNil())
					Expect(listenerOptions).To(HaveLen(1))
					Expect(listenerOptions[0].GetName()).To(Equal("good-policy"))
					Expect(listenerOptions[0].GetNamespace()).To(Equal("default"))
				})
			})
		})
	})
})

func attachedListenerOption() *solokubev1.ListenerOption {
	now := metav1.Now()
	return &solokubev1.ListenerOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "good-policy",
			Namespace:         "default",
			CreationTimestamp: now,
		},
		Spec: sologatewayv1.ListenerOption{
			TargetRefs: []*corev1.PolicyTargetReferenceWithSectionName{
				{
					Group:     gwv1.GroupVersion.Group,
					Kind:      wellknown.GatewayKind,
					Name:      "test",
					Namespace: wrapperspb.String("default"),
				},
			},
			Options: &v1.ListenerOptions{},
		},
	}
}
func attachedListenerOptionWithSectionName() *solokubev1.ListenerOption {
	opt := attachedListenerOption()
	opt.ObjectMeta.Name = "good-policy-with-section-name"
	opt.Spec.TargetRefs[0].SectionName = &wrapperspb.StringValue{
		Value: "test-listener",
	}
	return opt
}

func attachedListenerOptionWithDiffSectionName() *solokubev1.ListenerOption {
	opt := attachedListenerOption()
	opt.ObjectMeta.Name = "bad-policy-with-section-name"
	opt.Spec.TargetRefs[0].SectionName = &wrapperspb.StringValue{
		Value: "not-our-listener",
	}
	return opt
}

func attachedListenerOptionOmitNamespace() *solokubev1.ListenerOption {
	opt := attachedListenerOption()
	opt.ObjectMeta.Name = "good-policy-no-ns"
	opt.Spec.TargetRefs[0].Namespace = nil
	return opt
}

func diffNamespaceListenerOption() *solokubev1.ListenerOption {
	opt := attachedListenerOption()
	opt.ObjectMeta.Name = "bad-policy"
	opt.ObjectMeta.Namespace = "non-default"
	return opt
}

func diffNamespaceListenerOptionOmitNamespace() *solokubev1.ListenerOption {
	opt := attachedListenerOption()
	opt.ObjectMeta.Name = "bad-policy"
	opt.ObjectMeta.Namespace = "non-default"
	opt.Spec.TargetRefs[0].Namespace = nil
	return opt
}

func attachedListenerOptionMultipleTargetRefHit() *solokubev1.ListenerOption {
	now := metav1.Now()
	return &solokubev1.ListenerOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "good-policy",
			Namespace:         "default",
			CreationTimestamp: now,
		},
		Spec: sologatewayv1.ListenerOption{
			TargetRefs: []*corev1.PolicyTargetReferenceWithSectionName{
				{
					Group:     gwv1.GroupVersion.Group,
					Kind:      wellknown.GatewayKind,
					Name:      "test",
					Namespace: wrapperspb.String("default"),
				},
				{
					Group:     gwv1.GroupVersion.Group,
					Kind:      wellknown.GatewayKind,
					Name:      "another-gateway",
					Namespace: wrapperspb.String("default"),
				},
			},
			Options: &v1.ListenerOptions{},
		},
	}
}
func attachedListenerOptionMultipleTargetRefMiss() *solokubev1.ListenerOption {
	now := metav1.Now()
	return &solokubev1.ListenerOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "good-policy",
			Namespace:         "default",
			CreationTimestamp: now,
		},
		Spec: sologatewayv1.ListenerOption{
			TargetRefs: []*corev1.PolicyTargetReferenceWithSectionName{
				{
					Group:     gwv1.GroupVersion.Group,
					Kind:      wellknown.GatewayKind,
					Name:      "another-gateway",
					Namespace: wrapperspb.String("default"),
				},
				{
					Group:     gwv1.GroupVersion.Group,
					Kind:      wellknown.GatewayKind,
					Name:      "test",
					Namespace: wrapperspb.String("default"),
				},
			},
			Options: &v1.ListenerOptions{},
		},
	}
}
