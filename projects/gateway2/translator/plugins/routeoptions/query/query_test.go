package query_test

import (
	"context"
	"time"

	"github.com/solo-io/gloo/pkg/schemes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	sologatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	solokubev1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1/kube/apis/gateway.solo.io/v1"
	"github.com/solo-io/gloo/projects/gateway2/translator/plugins/routeoptions/query"
	"github.com/solo-io/gloo/projects/gateway2/translator/testutils"
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
		builder = fake.NewClientBuilder().WithScheme(schemes.DefaultScheme())
		query.IterateIndices(func(o client.Object, f string, fun client.IndexerFunc) error {
			builder.WithIndex(o, f, fun)
			return nil
		})
	})

	Describe("Get RouteOptions", func() {
		It("should find the only attached option with a full targetRef", func() {
			ctx := context.Background()

			hr := httpRoute()
			attachedOpt := attachedRouteOption()
			hrNsName := types.NamespacedName{Namespace: hr.GetNamespace(), Name: hr.GetName()}
			deps := []client.Object{
				hr,
				attachedOpt,
				diffNamespaceRouteOption(),
			}
			fakeClient := builder.WithObjects(deps...).Build()

			query := query.NewQuery(fakeClient)
			gwQuery := testutils.BuildGatewayQueriesWithClient(fakeClient)

			rtOpt, sources, err := query.GetRouteOptionForRouteRule(ctx, hrNsName, nil, gwQuery)

			Expect(err).NotTo(HaveOccurred())
			Expect(rtOpt).ToNot(BeNil())

			Expect(rtOpt.Spec.GetOptions().GetFaults().GetAbort().GetPercentage()).To(BeNumerically("==", attachedOpt.Spec.GetOptions().GetFaults().GetAbort().GetPercentage()))
			Expect(sources).To(HaveLen(1))
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
			gwQuery := testutils.BuildGatewayQueriesWithClient(fakeClient)

			rtOpt, sources, err := query.GetRouteOptionForRouteRule(ctx, hrNsName, nil, gwQuery)

			Expect(err).NotTo(HaveOccurred())
			Expect(rtOpt).To(BeNil())
			Expect(sources).To(BeEmpty())
		})

		It("should find the only attached option with a targetRef with omitted namespace", func() {
			ctx := context.Background()

			hr := httpRoute()
			hrNsName := types.NamespacedName{Namespace: hr.GetNamespace(), Name: hr.GetName()}

			attachedOpt := attachedRouteOptionOmitNamespace()
			deps := []client.Object{
				hr,
				attachedOpt,
				diffNamespaceRouteOption(),
			}
			fakeClient := builder.WithObjects(deps...).Build()

			query := query.NewQuery(fakeClient)
			gwQuery := testutils.BuildGatewayQueriesWithClient(fakeClient)

			rtOpt, sources, err := query.GetRouteOptionForRouteRule(ctx, hrNsName, nil, gwQuery)

			Expect(err).NotTo(HaveOccurred())
			Expect(rtOpt).ToNot(BeNil())

			Expect(rtOpt.Spec.GetOptions().GetFaults().GetAbort().GetPercentage()).To(BeNumerically("==", attachedOpt.Spec.GetOptions().GetFaults().GetAbort().GetPercentage()))
			Expect(sources).To(HaveLen(1))
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
			gwQuery := testutils.BuildGatewayQueriesWithClient(fakeClient)

			rtOpt, sources, err := query.GetRouteOptionForRouteRule(ctx, hrNsName, nil, gwQuery)

			Expect(err).NotTo(HaveOccurred())
			Expect(rtOpt).To(BeNil())
			Expect(sources).To(BeEmpty())
		})

		It("should merge extensionRef and targetRef RouteOptions", func() {
			ctx := context.Background()

			opt1 := attachedRouteOption1()
			opt2 := attachedRouteOption2()
			opt3 := attachedRouteOption3()

			hr := httpRouteWithFilters()
			hrNsName := types.NamespacedName{Namespace: hr.GetNamespace(), Name: hr.GetName()}
			deps := []client.Object{
				hr,
				opt1,
				opt2,
				opt3,
			}
			fakeClient := builder.WithObjects(deps...).Build()

			query := query.NewQuery(fakeClient)
			gwQuery := testutils.BuildGatewayQueriesWithClient(fakeClient)

			rtOpt, sources, err := query.GetRouteOptionForRouteRule(ctx, hrNsName, &hr.Spec.Rules[0], gwQuery)

			Expect(err).NotTo(HaveOccurred())
			Expect(rtOpt).ToNot(BeNil())
			Expect(sources).To(HaveLen(3))

			// First ExtensionRef gets first priority
			Expect(rtOpt.Spec.GetOptions().GetFaults().GetAbort().GetPercentage()).To(Equal(opt1.Spec.GetOptions().GetFaults().GetAbort().GetPercentage()))
			Expect(rtOpt.Spec.GetOptions().GetFaults().GetAbort().GetHttpStatus()).To(Equal(opt1.Spec.GetOptions().GetFaults().GetAbort().GetHttpStatus()))

			// Second ExtensionRef gets second priority
			Expect(rtOpt.Spec.GetOptions().GetPrefixRewrite().GetValue()).To(Equal(opt2.Spec.GetOptions().GetPrefixRewrite().GetValue()))

			// TargetRef gets last priority
			Expect(rtOpt.Spec.GetOptions().GetTimeout().GetNanos()).To(Equal(opt3.Spec.GetOptions().GetTimeout().GetNanos()))
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

func httpRouteWithFilters() *gwv1.HTTPRoute {
	return &gwv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test",
		},
		Spec: gwv1.HTTPRouteSpec{
			Rules: []gwv1.HTTPRouteRule{
				{
					Filters: []gwv1.HTTPRouteFilter{
						{
							Type: gwv1.HTTPRouteFilterExtensionRef,
							ExtensionRef: &gwv1.LocalObjectReference{
								Group: gwv1.Group(sologatewayv1.RouteOptionGVK.Group),
								Kind:  gwv1.Kind(sologatewayv1.RouteOptionGVK.Kind),
								Name:  gwv1.ObjectName(attachedRouteOption1().GetName()),
							},
						},
						{
							Type: gwv1.HTTPRouteFilterExtensionRef,
							ExtensionRef: &gwv1.LocalObjectReference{
								Group: gwv1.Group(sologatewayv1.RouteOptionGVK.Group),
								Kind:  gwv1.Kind(sologatewayv1.RouteOptionGVK.Kind),
								Name:  gwv1.ObjectName(attachedRouteOption2().GetName()),
							},
						},
					},
					BackendRefs: []gwv1.HTTPBackendRef{
						{
							BackendRef: gwv1.BackendRef{
								BackendObjectReference: gwv1.BackendObjectReference{
									Name: "foo",
								},
							},
						},
					},
				},
			},
		},
	}
}

func attachedRouteOption() *solokubev1.RouteOption {
	return &solokubev1.RouteOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "good-policy",
			Namespace:         "default",
			CreationTimestamp: metav1.Now(),
		},
		Spec: sologatewayv1.RouteOption{
			TargetRefs: []*corev1.PolicyTargetReference{
				{
					Group:     gwv1.GroupVersion.Group,
					Kind:      wellknown.HTTPRouteKind,
					Name:      "test",
					Namespace: wrapperspb.String("default"),
				},
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

func attachedRouteOption1() *solokubev1.RouteOption {
	return &solokubev1.RouteOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "good-policy",
			Namespace:         "default",
			CreationTimestamp: metav1.Now(),
		},
		Spec: sologatewayv1.RouteOption{
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

func attachedRouteOption2() *solokubev1.RouteOption {
	return &solokubev1.RouteOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "good-policy2",
			Namespace:         "default",
			CreationTimestamp: metav1.Now(),
		},
		Spec: sologatewayv1.RouteOption{
			Options: &v1.RouteOptions{
				Faults: &faultinjection.RouteFaults{
					Abort: &faultinjection.RouteAbort{
						Percentage: 2.00,
						HttpStatus: 400,
					},
				},
				PrefixRewrite: wrapperspb.String("/foo"),
			},
		},
	}
}

func attachedRouteOption3() *solokubev1.RouteOption {
	return &solokubev1.RouteOption{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "good-policy3",
			Namespace:         "default",
			CreationTimestamp: metav1.Now(),
		},
		Spec: sologatewayv1.RouteOption{
			TargetRefs: []*corev1.PolicyTargetReference{
				{
					Group:     gwv1.GroupVersion.Group,
					Kind:      wellknown.HTTPRouteKind,
					Name:      "test",
					Namespace: wrapperspb.String("default"),
				},
			},
			Options: &v1.RouteOptions{
				Faults: &faultinjection.RouteFaults{
					Abort: &faultinjection.RouteAbort{
						Percentage: 3.00,
						HttpStatus: 404,
					},
				},
				PrefixRewrite: wrapperspb.String("/baz"),
				Timeout:       durationpb.New(5 * time.Second),
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
			TargetRefs: []*corev1.PolicyTargetReference{
				{
					Group: gwv1.GroupVersion.Group,
					Kind:  wellknown.HTTPRouteKind,
					Name:  "test",
				},
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
			TargetRefs: []*corev1.PolicyTargetReference{
				{
					Group:     gwv1.GroupVersion.Group,
					Kind:      wellknown.HTTPRouteKind,
					Name:      "test",
					Namespace: wrapperspb.String("default"),
				},
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
			TargetRefs: []*corev1.PolicyTargetReference{
				{
					Group: gwv1.GroupVersion.Group,
					Kind:  wellknown.HTTPRouteKind,
					Name:  "test",
				},
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
