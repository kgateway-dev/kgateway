package query_test

import (
	"container/list"
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/wrapperspb"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	gwscheme "github.com/solo-io/gloo/projects/gateway2/controller/scheme"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/routeoptions/query"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/faultinjection"
	corev1 "github.com/solo-io/skv2/pkg/api/core.skv2.solo.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

	Describe("Get RouteOptions", func() {
		It("should find the only attached option with a full targetRef", func() {
			ctx := context.Background()

			hr := httpRoute()
			hrNsName := types.NamespacedName{Namespace: hr.GetNamespace(), Name: hr.GetName()}
			deps := []client.Object{
				hr,
				attachedRouteOption(),
				diffNamespaceRouteOption(),
			}
			fakeClient := builder.WithObjects(deps...).Build()

			query := query.NewQuery(fakeClient)
			rtOpt, err := query.GetRouteOptionForRoute(ctx, hrNsName, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(rtOpt).ToNot(BeNil())
			Expect(rtOpt.GetName()).To(Equal("good-policy"))
			Expect(rtOpt.GetNamespace()).To(Equal("default"))
		})

		It("should not find an attached option when none are in the same namespace as route", func() {
			ctx := context.Background()

			hr := httpRoute()
			hrNsName := types.NamespacedName{Namespace: hr.GetNamespace(), Name: hr.GetName()}

			deps := []client.Object{
				hr,
				diffNamespaceRouteOption(),
			}
			fakeClient := builder.WithObjects(deps...).Build()

			query := query.NewQuery(fakeClient)
			rtOpt, err := query.GetRouteOptionForRoute(ctx, hrNsName, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(rtOpt).To(BeNil())
		})

		It("should find the only attached option with a targetRef with omitted namespace", func() {
			ctx := context.Background()

			hr := httpRoute()
			hrNsName := types.NamespacedName{Namespace: hr.GetNamespace(), Name: hr.GetName()}

			deps := []client.Object{
				hr,
				attachedRouteOptionOmitNamespace(),
				diffNamespaceRouteOption(),
			}
			fakeClient := builder.WithObjects(deps...).Build()

			query := query.NewQuery(fakeClient)
			rtOpt, err := query.GetRouteOptionForRoute(ctx, hrNsName, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(rtOpt).ToNot(BeNil())
			Expect(rtOpt.GetName()).To(Equal("good-policy-no-ns"))
			Expect(rtOpt.GetNamespace()).To(Equal("default"))
		})

		It("should not find an attached option when none are in the same namespace as route with omitted namespace", func() {
			ctx := context.Background()

			hr := httpRoute()
			hrNsName := types.NamespacedName{Namespace: hr.GetNamespace(), Name: hr.GetName()}

			deps := []client.Object{
				hr,
				diffNamespaceRouteOptionOmitNamespace(),
			}
			fakeClient := builder.WithObjects(deps...).Build()

			query := query.NewQuery(fakeClient)
			rtOpt, err := query.GetRouteOptionForRoute(ctx, hrNsName, nil)

			Expect(err).NotTo(HaveOccurred())
			Expect(rtOpt).To(BeNil())
		})

		Context("with delegation", func() {
			It("should merge the parent and child RouteOption", func() {
				ctx := context.Background()

				parent := httpRoute()
				parentNsName := types.NamespacedName{Namespace: parent.GetNamespace(), Name: parent.GetName()}
				child := childHTTPRoute()
				childNsName := types.NamespacedName{Namespace: child.GetNamespace(), Name: child.GetName()}
				parentRouteOpt := attachedRouteOption()
				childRouteOpt := childRouteOption()

				delegationChain := list.New()
				delegationChain.PushFront(parentNsName)

				deps := []client.Object{
					parent,
					child,
					parentRouteOpt,
					childRouteOpt,
				}
				fakeClient := builder.WithObjects(deps...).Build()

				query := query.NewQuery(fakeClient)
				rtOpt, err := query.GetRouteOptionForRoute(ctx, childNsName, delegationChain.Front())

				Expect(err).NotTo(HaveOccurred())
				Expect(rtOpt).ToNot(BeNil())
				Expect(rtOpt.GetName()).To(Equal("good-policy"))
				Expect(rtOpt.GetNamespace()).To(Equal("default"))
				// Assert that parent options are prioritized in the merge
				Expect(rtOpt.Spec.GetOptions().GetFaults().GetAbort().GetPercentage()).To(Equal(parentRouteOpt.Spec.GetOptions().GetFaults().GetAbort().GetPercentage()))
				Expect(rtOpt.Spec.GetOptions().GetFaults().GetAbort().GetHttpStatus()).To(Equal(parentRouteOpt.Spec.GetOptions().GetFaults().GetAbort().GetHttpStatus()))
				// Assert that child options are augmented with the parent options in the merge
				Expect(rtOpt.Spec.GetOptions().GetPrefixRewrite().GetValue()).To(Equal(childRouteOpt.Spec.GetOptions().GetPrefixRewrite().GetValue()))
			})
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

func attachedRouteOption() *solokubev1.RouteOption {
	now := metav1.Now()
	return &solokubev1.RouteOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "good-policy",
			Namespace:         "default",
			CreationTimestamp: now,
		},
		Spec: sologatewayv1.RouteOption{
			TargetRef: &corev1.PolicyTargetReference{
				Group:     gwv1.GroupVersion.Group,
				Kind:      wellknown.HTTPRouteKind,
				Name:      "test",
				Namespace: wrapperspb.String("default"),
			},
			Options: &v1.RouteOptions{
				Faults: &faultinjection.RouteFaults{
					Abort: &faultinjection.RouteAbort{
						Percentage: 1.00,
						HttpStatus: 500,
					},
				},
			},
		},
	}
}

func attachedRouteOptionOmitNamespace() *solokubev1.RouteOption {
	now := metav1.Now()
	return &solokubev1.RouteOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "good-policy-no-ns",
			Namespace:         "default",
			CreationTimestamp: now,
		},
		Spec: sologatewayv1.RouteOption{
			TargetRef: &corev1.PolicyTargetReference{
				Group: gwv1.GroupVersion.Group,
				Kind:  wellknown.HTTPRouteKind,
				Name:  "test",
			},
			Options: &v1.RouteOptions{
				Faults: &faultinjection.RouteFaults{
					Abort: &faultinjection.RouteAbort{
						Percentage: 1.00,
						HttpStatus: 500,
					},
				},
			},
		},
	}
}

func diffNamespaceRouteOption() *solokubev1.RouteOption {
	now := metav1.Now()
	return &solokubev1.RouteOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "bad-policy",
			Namespace:         "non-default",
			CreationTimestamp: now,
		},
		Spec: sologatewayv1.RouteOption{
			TargetRef: &corev1.PolicyTargetReference{
				Group:     gwv1.GroupVersion.Group,
				Kind:      wellknown.HTTPRouteKind,
				Name:      "test",
				Namespace: wrapperspb.String("default"),
			},
			Options: &v1.RouteOptions{
				Faults: &faultinjection.RouteFaults{
					Abort: &faultinjection.RouteAbort{
						Percentage: 1.00,
						HttpStatus: 500,
					},
				},
			},
		},
	}
}

func diffNamespaceRouteOptionOmitNamespace() *solokubev1.RouteOption {
	now := metav1.Now()
	return &solokubev1.RouteOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "bad-policy",
			Namespace:         "non-default",
			CreationTimestamp: now,
		},
		Spec: sologatewayv1.RouteOption{
			TargetRef: &corev1.PolicyTargetReference{
				Group: gwv1.GroupVersion.Group,
				Kind:  wellknown.HTTPRouteKind,
				Name:  "test",
			},
			Options: &v1.RouteOptions{
				Faults: &faultinjection.RouteFaults{
					Abort: &faultinjection.RouteAbort{
						Percentage: 1.00,
						HttpStatus: 500,
					},
				},
			},
		},
	}
}

func childHTTPRoute() *gwv1.HTTPRoute {
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "child-route",
			Namespace: "child",
		},
	}
}

func childRouteOption() *solokubev1.RouteOption {
	now := metav1.Now()
	return &solokubev1.RouteOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "child-policy",
			Namespace:         "child",
			CreationTimestamp: now,
		},
		Spec: sologatewayv1.RouteOption{
			TargetRef: &corev1.PolicyTargetReference{
				Group: gwv1.GroupVersion.Group,
				Kind:  wellknown.HTTPRouteKind,
				Name:  "child-route",
			},
			Options: &v1.RouteOptions{
				// This should be ignored by the RouteOption merge
				// because the parent RouteOption has the same field set
				Faults: &faultinjection.RouteFaults{
					Abort: &faultinjection.RouteAbort{
						Percentage: 0.80,
						HttpStatus: 418,
					},
				},
				PrefixRewrite: wrapperspb.String("/foo"),
			},
		},
	}
}
