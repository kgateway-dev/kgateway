package krtcollections_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

func TestTransformGRPCRoute(t *testing.T) {
	svcGk := schema.GroupKind{
		Group: "",
		Kind:  "Service",
	}

	// Create test cases with different GRPCRoute configurations
	testCases := []struct {
		name           string
		grpcRoute      *gwv1.GRPCRoute
		services       []*corev1.Service
		referenceGrant *gwv1beta1.ReferenceGrant
		expectedResult func(httpRouteIR *ir.HttpRouteIR) bool
	}{
		{
			name: "basic_grpc_route",
			grpcRoute: &gwv1.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-grpc-route",
					Namespace: "default",
				},
				Spec: gwv1.GRPCRouteSpec{
					Hostnames: []gwv1.Hostname{"test.example.com"},
					Rules: []gwv1.GRPCRouteRule{
						{
							Matches: []gwv1.GRPCRouteMatch{
								{
									Method: &gwv1.GRPCMethodMatch{
										Service: ptr.To("TestService"),
										Method:  ptr.To("TestMethod"),
										Type:    ptr.To(gwv1.GRPCMethodMatchExact),
									},
								},
							},
							BackendRefs: []gwv1.GRPCBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
											Port: ptr.To(gwv1.PortNumber(8080)),
										},
									},
								},
							},
						},
					},
				},
			},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "grpc",
								Port: 8080,
							},
						},
					},
				},
			},
			expectedResult: func(httpRouteIR *ir.HttpRouteIR) bool {
				if httpRouteIR == nil {
					return false
				}

				// Verify the basic properties were copied
				if httpRouteIR.Name != "test-grpc-route" ||
					httpRouteIR.Namespace != "default" ||
					len(httpRouteIR.Hostnames) != 1 ||
					httpRouteIR.Hostnames[0] != "test.example.com" {
					return false
				}

				// Verify rules were transformed correctly
				if len(httpRouteIR.Rules) != 1 {
					return false
				}

				// Verify matches
				rule := httpRouteIR.Rules[0]
				if len(rule.Matches) != 1 {
					return false
				}

				match := rule.Matches[0]
				if match.Path == nil ||
					*match.Path.Value != "/TestService/TestMethod" ||
					*match.Path.Type != gwv1.PathMatchExact {
					return false
				}

				// Verify backends
				if len(rule.Backends) != 1 {
					return false
				}

				backend := rule.Backends[0]
				if backend.Backend == nil ||
					backend.Backend.ClusterName != "service_default_test-service_8080" {
					return false
				}

				return true
			},
		},
		{
			name: "grpc_route_with_regex_method",
			grpcRoute: &gwv1.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-grpc-route-regex",
					Namespace: "default",
				},
				Spec: gwv1.GRPCRouteSpec{
					Rules: []gwv1.GRPCRouteRule{
						{
							Matches: []gwv1.GRPCRouteMatch{
								{
									Method: &gwv1.GRPCMethodMatch{
										Service: ptr.To("TestService"),
										Type:    ptr.To(gwv1.GRPCMethodMatchRegularExpression),
									},
								},
							},
							BackendRefs: []gwv1.GRPCBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
											Port: ptr.To(gwv1.PortNumber(8080)),
										},
									},
								},
							},
						},
					},
				},
			},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "grpc",
								Port: 8080,
							},
						},
					},
				},
			},
			expectedResult: func(httpRouteIR *ir.HttpRouteIR) bool {
				if httpRouteIR == nil {
					return false
				}

				// Verify the rules were transformed correctly
				if len(httpRouteIR.Rules) != 1 {
					return false
				}

				// Verify matches - should use regex path type
				rule := httpRouteIR.Rules[0]
				if len(rule.Matches) != 1 {
					return false
				}

				match := rule.Matches[0]
				if match.Path == nil ||
					*match.Path.Type != gwv1.PathMatchRegularExpression ||
					*match.Path.Value != "/TestService/.+" {
					return false
				}

				return true
			},
		},
		{
			name: "grpc_route_with_headers",
			grpcRoute: &gwv1.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-grpc-route-headers",
					Namespace: "default",
				},
				Spec: gwv1.GRPCRouteSpec{
					Rules: []gwv1.GRPCRouteRule{
						{
							Matches: []gwv1.GRPCRouteMatch{
								{
									Method: &gwv1.GRPCMethodMatch{
										Service: ptr.To("TestService"),
										Method:  ptr.To("TestMethod"),
									},
									Headers: []gwv1.GRPCHeaderMatch{
										{
											Name:  "x-test-header",
											Value: "test-value",
											Type:  ptr.To(gwv1.GRPCHeaderMatchExact),
										},
									},
								},
							},
							BackendRefs: []gwv1.GRPCBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
											Port: ptr.To(gwv1.PortNumber(8080)),
										},
									},
								},
							},
						},
					},
				},
			},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "grpc",
								Port: 8080,
							},
						},
					},
				},
			},
			expectedResult: func(httpRouteIR *ir.HttpRouteIR) bool {
				if httpRouteIR == nil {
					return false
				}

				// Verify headers were transformed correctly
				if len(httpRouteIR.Rules) != 1 {
					return false
				}

				rule := httpRouteIR.Rules[0]
				if len(rule.Matches) != 1 {
					return false
				}

				match := rule.Matches[0]
				if len(match.Headers) != 1 {
					return false
				}

				header := match.Headers[0]
				if string(header.Name) != "x-test-header" ||
					header.Value != "test-value" ||
					*header.Type != gwv1.HeaderMatchExact {
					return false
				}

				return true
			},
		},
		{
			name: "cross_namespace_backend",
			grpcRoute: &gwv1.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-grpc-route-cross-ns",
					Namespace: "default",
				},
				Spec: gwv1.GRPCRouteSpec{
					Rules: []gwv1.GRPCRouteRule{
						{
							Matches: []gwv1.GRPCRouteMatch{
								{
									Method: &gwv1.GRPCMethodMatch{
										Service: ptr.To("TestService"),
									},
								},
							},
							BackendRefs: []gwv1.GRPCBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name:      "test-service",
											Namespace: ptr.To(gwv1.Namespace("other")),
											Port:      ptr.To(gwv1.PortNumber(8080)),
										},
									},
								},
							},
						},
					},
				},
			},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service",
						Namespace: "other",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "grpc",
								Port: 8080,
							},
						},
					},
				},
			},
			referenceGrant: &gwv1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "allow-grpc-to-service",
					Namespace: "other",
				},
				Spec: gwv1beta1.ReferenceGrantSpec{
					From: []gwv1beta1.ReferenceGrantFrom{
						{
							Group:     gwv1.Group("gateway.networking.k8s.io"),
							Kind:      gwv1.Kind("GRPCRoute"),
							Namespace: gwv1.Namespace("default"),
						},
					},
					To: []gwv1beta1.ReferenceGrantTo{
						{
							Group: gwv1.Group(""),
							Kind:  gwv1.Kind("Service"),
						},
					},
				},
			},
			expectedResult: func(httpRouteIR *ir.HttpRouteIR) bool {
				if httpRouteIR == nil {
					return false
				}

				// Verify cross-namespace backend is configured correctly
				if len(httpRouteIR.Rules) != 1 {
					return false
				}

				rule := httpRouteIR.Rules[0]
				if len(rule.Backends) != 1 {
					return false
				}

				backend := rule.Backends[0]
				if backend.Backend == nil ||
					backend.Backend.ClusterName != "service_other_test-service_8080" ||
					backend.Backend.Err != nil {
					return false
				}

				return true
			},
		},
		{
			name: "no_method_match",
			grpcRoute: &gwv1.GRPCRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-grpc-route-no-method",
					Namespace: "default",
				},
				Spec: gwv1.GRPCRouteSpec{
					Rules: []gwv1.GRPCRouteRule{
						{
							Matches: []gwv1.GRPCRouteMatch{
								{
									// No method match
								},
							},
							BackendRefs: []gwv1.GRPCBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
											Port: ptr.To(gwv1.PortNumber(8080)),
										},
									},
								},
							},
						},
					},
				},
			},
			services: []*corev1.Service{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-service",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Name: "grpc",
								Port: 8080,
							},
						},
					},
				},
			},
			expectedResult: func(httpRouteIR *ir.HttpRouteIR) bool {
				if httpRouteIR == nil {
					return false
				}

				// Verify default path is set for no method match
				if len(httpRouteIR.Rules) != 1 {
					return false
				}

				rule := httpRouteIR.Rules[0]
				if len(rule.Matches) != 1 {
					return false
				}

				match := rule.Matches[0]
				if match.Path == nil ||
					*match.Path.Value != "/" ||
					*match.Path.Type != gwv1.PathMatchPathPrefix {
					return false
				}

				return true
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup mock collections
			var allInputs []any
			allInputs = append(allInputs, tc.grpcRoute)
			for _, svc := range tc.services {
				allInputs = append(allInputs, svc)
			}
			if tc.referenceGrant != nil {
				allInputs = append(allInputs, tc.referenceGrant)
			}

			mock := krttest.NewMock(t, allInputs)

			// Setup collections
			grpcRoutes := krttest.GetMockCollection[*gwv1.GRPCRoute](mock)
			services := krttest.GetMockCollection[*corev1.Service](mock)
			refgrants := krtcollections.NewRefGrantIndex(krttest.GetMockCollection[*gwv1beta1.ReferenceGrant](mock))
			policies := krtcollections.NewPolicyIndex(krtutil.KrtOptions{}, sdk.ContributesPolicies{}, apisettings.Settings{})

			// Set up backend index
			backends := krtcollections.NewBackendIndex(krtutil.KrtOptions{}, policies, refgrants)
			backends.AddBackends(svcGk, k8sSvcUpstreams(services))

			// Create RouteIndex with minimal collections needed for GRPC route transformation
			routesIndex := krtcollections.NewRoutesIndex(
				krtutil.KrtOptions{},
				krttest.GetMockCollection[*gwv1.HTTPRoute](mock),
				grpcRoutes,
				krttest.GetMockCollection[*gwv1a2.TCPRoute](mock),
				krttest.GetMockCollection[*gwv1a2.TLSRoute](mock),
				policies,
				backends,
				refgrants,
				apisettings.Settings{},
			)

			// Wait until collections are synced
			// grpcRoutes.WaitUntilSynced(context.Background().Done())
			services.WaitUntilSynced(context.Background().Done())
			if tc.referenceGrant != nil {
				refgrants.HasSynced()
			}

			for !routesIndex.HasSynced() {
				time.Sleep(time.Second / 10)
			}

			// Get the GRPCRoute
			routeWrapper := routesIndex.Fetch(krt.TestingDummyContext{},
				schema.GroupKind{Group: "gateway.networking.k8s.io", Kind: "GRPCRoute"},
				tc.grpcRoute.Namespace,
				tc.grpcRoute.Name)

			// Ensure we got a result
			assert.NotNil(t, routeWrapper, "Expected to get a RouteWrapper")

			// Verify the transformed route
			httpRouteIR, ok := routeWrapper.Route.(*ir.HttpRouteIR)
			assert.True(t, ok, "Expected route to be HttpRouteIR")

			// Run the test-specific validation
			assert.True(t, tc.expectedResult(httpRouteIR), "Route did not match expected transformation")
		})
	}
}

// Helper function to create service backend collections
func k8sSvcUpstreams(services krt.Collection[*corev1.Service]) krt.Collection[ir.BackendObjectIR] {
	return krt.NewManyCollection(services, func(kctx krt.HandlerContext, svc *corev1.Service) []ir.BackendObjectIR {
		uss := []ir.BackendObjectIR{}

		for _, port := range svc.Spec.Ports {
			backend := ir.NewBackendObjectIR(ir.ObjectSource{
				Kind:      "Service",
				Group:     "",
				Namespace: svc.Namespace,
				Name:      svc.Name,
			}, port.Port, "")
			backend.Obj = svc

			uss = append(uss, backend)
		}
		return uss
	})
}
