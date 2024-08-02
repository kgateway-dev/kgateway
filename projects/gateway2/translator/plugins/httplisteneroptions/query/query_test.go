package query_test

import (
	"context"

	"github.com/solo-io/gloo/pkg/schemes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/wrapperspb"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/httplisteneroptions/query"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	corev1 "github.com/solo-io/skv2/pkg/api/core.skv2.solo.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var _ = Describe("Query Get HttpListenerOptions", func() {

	var (
		ctx      context.Context
		deps     []client.Object
		gw       *gwv1.Gateway
		listener *gwv1.Listener
		qry      query.HttpListenerOptionQueries
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
				attachedHttpListenerOption(),
				diffNamespaceHttpListenerOption(),
			}
		})
		It("should find the only attached option", func() {
			httpListenerOptions, err := qry.GetAttachedHttpListenerOptions(ctx, listener, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(httpListenerOptions).NotTo(BeNil())
			Expect(httpListenerOptions).To(HaveLen(1))
			Expect(httpListenerOptions[0].GetName()).To(Equal("good-policy"))
			Expect(httpListenerOptions[0].GetNamespace()).To(Equal("default"))
		})
	})

	When("no options in same namespace as gateway", func() {
		BeforeEach(func() {
			deps = []client.Object{
				gw,
				diffNamespaceHttpListenerOption(),
			}
		})
		It("should not find an attached option", func() {
			httpListenerOptions, err := qry.GetAttachedHttpListenerOptions(ctx, listener, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(httpListenerOptions).To(BeNil())
		})
	})

	When("targetRef has omitted namespace", func() {
		BeforeEach(func() {
			deps = []client.Object{
				gw,
				attachedHttpListenerOptionOmitNamespace(),
				diffNamespaceHttpListenerOption(),
			}
		})
		It("should find the attached option", func() {
			httpListenerOptions, err := qry.GetAttachedHttpListenerOptions(ctx, listener, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(httpListenerOptions).NotTo(BeNil())
			Expect(httpListenerOptions).To(HaveLen(1))

			Expect(httpListenerOptions[0].GetName()).To(Equal("good-policy-no-ns"))
			Expect(httpListenerOptions[0].GetNamespace()).To(Equal("default"))
		})
	})

	When("no options in namespace as gateway with omitted namespace", func() {
		BeforeEach(func() {
			deps = []client.Object{
				gw,
				diffNamespaceHttpListenerOptionOmitNamespace(),
			}
		})
		It("should not find an attached option", func() {
			httpListenerOptions, err := qry.GetAttachedHttpListenerOptions(ctx, listener, gw)
			Expect(err).NotTo(HaveOccurred())
			Expect(httpListenerOptions).To(BeNil())
		})
	})

	When("multiple targetRefs fully present without sectionName", func() {
		When("first targetRef in list matches", func() {
			BeforeEach(func() {
				deps = []client.Object{
					gw,
					attachedHttpListenerOptionMultipleTargetRefHit(),
				}
			})
			It("should find the only attached option", func() {
				httpListenerOptions, err := qry.GetAttachedHttpListenerOptions(ctx, listener, gw)
				Expect(err).NotTo(HaveOccurred())
				Expect(httpListenerOptions).NotTo(BeNil())
				Expect(httpListenerOptions).To(HaveLen(1))
				Expect(httpListenerOptions[0].GetName()).To(Equal("good-policy"))
				Expect(httpListenerOptions[0].GetNamespace()).To(Equal("default"))
			})
		})
		When("first targetRef in list does not match", func() {
			BeforeEach(func() {
				deps = []client.Object{
					gw,
					attachedHttpListenerOptionMultipleTargetRefMiss(),
				}
			})
			It("should not find an attached option", func() {
				httpListenerOptions, err := qry.GetAttachedHttpListenerOptions(ctx, listener, gw)
				Expect(err).NotTo(HaveOccurred())
				Expect(httpListenerOptions).To(BeNil())
			})
		})
	})

	When("targetRef has section name matching listener", func() {
		When("no other options", func() {
			BeforeEach(func() {
				deps = []client.Object{
					gw,
					attachedHttpListenerOptionWithSectionName(),
				}
			})
			It("should find the attached option specified by section name", func() {
				httpListenerOptions, err := qry.GetAttachedHttpListenerOptions(ctx, listener, gw)
				Expect(err).NotTo(HaveOccurred())
				Expect(httpListenerOptions).NotTo(BeNil())
				Expect(httpListenerOptions).To(HaveLen(1))

				Expect(httpListenerOptions[0].GetName()).To(Equal("good-policy-with-section-name"))
				Expect(httpListenerOptions[0].GetNamespace()).To(Equal("default"))
			})
		})
		When("no other options with section name", func() {
			BeforeEach(func() {
				deps = []client.Object{
					gw,
					attachedHttpListenerOptionWithSectionName(),
					attachedHttpListenerOption(),
				}
			})
			It("should find the attached option with and without section name", func() {
				httpListenerOptions, err := qry.GetAttachedHttpListenerOptions(ctx, listener, gw)
				Expect(err).NotTo(HaveOccurred())
				Expect(httpListenerOptions).NotTo(BeNil())

				Expect(httpListenerOptions).To(HaveLen(2))
				Expect(httpListenerOptions[0].GetName()).To(Equal("good-policy-with-section-name"))
				Expect(httpListenerOptions[0].GetNamespace()).To(Equal("default"))

				Expect(httpListenerOptions[1].GetName()).To(Equal("good-policy"))
				Expect(httpListenerOptions[1].GetNamespace()).To(Equal("default"))
			})
		})
		When("targetRef has non-matching section name", func() {
			When("no other options", func() {
				BeforeEach(func() {
					deps = []client.Object{
						gw,
						attachedHttpListenerOptionWithDiffSectionName(),
					}
				})
				It("should not find any attached options", func() {
					httpListenerOptions, err := qry.GetAttachedHttpListenerOptions(ctx, listener, gw)
					Expect(err).NotTo(HaveOccurred())
					Expect(httpListenerOptions).To(BeNil())
				})
			})
			When("gateway-targeted options exist", func() {
				BeforeEach(func() {
					deps = []client.Object{
						gw,
						attachedHttpListenerOption(),
						attachedHttpListenerOptionWithDiffSectionName(),
					}
				})
				It("should find the gateway-level attached options", func() {
					httpListenerOptions, err := qry.GetAttachedHttpListenerOptions(ctx, listener, gw)
					Expect(err).NotTo(HaveOccurred())
					Expect(httpListenerOptions).NotTo(BeNil())
					Expect(httpListenerOptions).To(HaveLen(1))
					Expect(httpListenerOptions[0].GetName()).To(Equal("good-policy"))
					Expect(httpListenerOptions[0].GetNamespace()).To(Equal("default"))
				})
			})
		})
	})
})

func attachedHttpListenerOption() *solokubev1.HttpListenerOption {
	now := metav1.Now()
	return &solokubev1.HttpListenerOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "good-policy",
			Namespace:         "default",
			CreationTimestamp: now,
		},
		Spec: sologatewayv1.HttpListenerOption{
			TargetRefs: []*corev1.PolicyTargetReferenceWithSectionName{
				{
					Group:     gwv1.GroupVersion.Group,
					Kind:      wellknown.GatewayKind,
					Name:      "test",
					Namespace: wrapperspb.String("default"),
				},
			},
			Options: &v1.HttpListenerOptions{},
		},
	}
}
func attachedHttpListenerOptionWithSectionName() *solokubev1.HttpListenerOption {
	opt := attachedHttpListenerOption()
	opt.ObjectMeta.Name = "good-policy-with-section-name"
	opt.Spec.TargetRefs[0].SectionName = &wrapperspb.StringValue{
		Value: "test-listener",
	}
	return opt
}

func attachedHttpListenerOptionWithDiffSectionName() *solokubev1.HttpListenerOption {
	opt := attachedHttpListenerOption()
	opt.ObjectMeta.Name = "bad-policy-with-section-name"
	opt.Spec.TargetRefs[0].SectionName = &wrapperspb.StringValue{
		Value: "not-our-listener",
	}
	return opt
}

func attachedHttpListenerOptionOmitNamespace() *solokubev1.HttpListenerOption {
	opt := attachedHttpListenerOption()
	opt.ObjectMeta.Name = "good-policy-no-ns"
	opt.Spec.TargetRefs[0].Namespace = nil
	return opt
}

func diffNamespaceHttpListenerOption() *solokubev1.HttpListenerOption {
	opt := attachedHttpListenerOption()
	opt.ObjectMeta.Name = "bad-policy"
	opt.ObjectMeta.Namespace = "non-default"
	return opt
}

func diffNamespaceHttpListenerOptionOmitNamespace() *solokubev1.HttpListenerOption {
	opt := attachedHttpListenerOption()
	opt.ObjectMeta.Name = "bad-policy"
	opt.ObjectMeta.Namespace = "non-default"
	opt.Spec.TargetRefs[0].Namespace = nil
	return opt
}

func attachedHttpListenerOptionMultipleTargetRefHit() *solokubev1.HttpListenerOption {
	now := metav1.Now()
	return &solokubev1.HttpListenerOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "good-policy",
			Namespace:         "default",
			CreationTimestamp: now,
		},
		Spec: sologatewayv1.HttpListenerOption{
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
			Options: &v1.HttpListenerOptions{},
		},
	}
}
func attachedHttpListenerOptionMultipleTargetRefMiss() *solokubev1.HttpListenerOption {
	now := metav1.Now()
	return &solokubev1.HttpListenerOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "good-policy",
			Namespace:         "default",
			CreationTimestamp: now,
		},
		Spec: sologatewayv1.HttpListenerOption{
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
			Options: &v1.HttpListenerOptions{},
		},
	}
}
