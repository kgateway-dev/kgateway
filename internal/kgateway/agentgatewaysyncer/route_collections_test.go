package agentgatewaysyncer

import (
	"context"
	"testing"

	"github.com/agentgateway/agentgateway/go/api"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingclient "istio.io/client-go/pkg/apis/networking/v1"
	"istio.io/istio/pkg/kube/krt/krttest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	inf "sigs.k8s.io/gateway-api-inference-extension/api/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	krtinternal "github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils/krtutil"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

var (
	groupName = gwv1.Group(gwv1.GroupName)
)

func TestADPRouteCollection(t *testing.T) {
	testCases := []struct {
		name           string
		httpRoutes     []*gwv1.HTTPRoute
		services       []*corev1.Service
		namespaces     []*corev1.Namespace
		gateways       []GatewayListener
		refGrants      []ReferenceGrant
		expectedCount  int
		expectedRoutes []*api.Route
	}{
		{
			name: "Simple HTTP route with single rule",
			httpRoutes: []*gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route",
						Namespace: "default",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Name: "test-gateway",
								},
							},
						},
						Hostnames: []gwv1.Hostname{"example.com"},
						Rules: []gwv1.HTTPRouteRule{
							{
								Matches: []gwv1.HTTPRouteMatch{
									{
										Path: &gwv1.HTTPPathMatch{
											Type:  ptr.To(gwv1.PathMatchPathPrefix),
											Value: ptr.To("/api"),
										},
									},
								},
								BackendRefs: []gwv1.HTTPBackendRef{
									{
										BackendRef: gwv1.BackendRef{
											BackendObjectReference: gwv1.BackendObjectReference{
												Name: "test-service",
												Port: ptr.To(gwv1.PortNumber(80)),
											},
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
								Port: 80,
							},
						},
					},
				},
			},
			namespaces: []*corev1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
			},
			gateways: []GatewayListener{
				{
					Config: &Config{
						Meta: Meta{
							Name:      "test-gateway",
							Namespace: "default",
						},
					},
					parent: parentKey{
						Kind:      wellknown.GatewayGVK,
						Name:      "test-gateway",
						Namespace: "default",
					},
					parentInfo: parentInfo{
						InternalName: "default/test-gateway",
						Protocol:     gwv1.HTTPProtocolType,
						Port:         80,
						SectionName:  "http",
						AllowedKinds: []gwv1.RouteGroupKind{
							{
								Group: &groupName,
								Kind:  gwv1.Kind(wellknown.HTTPRouteKind),
							},
						},
					},
					Valid: true,
				},
			},
			refGrants:     []ReferenceGrant{},
			expectedCount: 1,
			expectedRoutes: []*api.Route{
				{
					Key:       "default/test-route.0.0.http",
					RouteName: "default/test-route",
					Hostnames: []string{"example.com"},
					Matches: []*api.RouteMatch{
						{
							Path: &api.PathMatch{
								Kind: &api.PathMatch_PathPrefix{
									PathPrefix: "/api",
								},
							},
						},
					},
					Backends: []*api.RouteBackend{
						{
							Backend: &api.BackendReference{
								Kind: &api.BackendReference_Service{
									Service: "default/test-service.default.svc.cluster.local",
								},
								Port: 80,
							},
						},
					},
				},
			},
		},
		{
			name: "Two HTTP routes on same gateway",
			httpRoutes: []*gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route-1",
						Namespace: "default",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Name: "test-gateway",
								},
							},
						},
						Hostnames: []gwv1.Hostname{"example.com"},
						Rules: []gwv1.HTTPRouteRule{
							{
								Matches: []gwv1.HTTPRouteMatch{
									{
										Path: &gwv1.HTTPPathMatch{
											Type:  ptr.To(gwv1.PathMatchPathPrefix),
											Value: ptr.To("/api"),
										},
									},
								},
								BackendRefs: []gwv1.HTTPBackendRef{
									{
										BackendRef: gwv1.BackendRef{
											BackendObjectReference: gwv1.BackendObjectReference{
												Name: "test-service",
												Port: ptr.To(gwv1.PortNumber(80)),
											},
										},
									},
								},
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route-2",
						Namespace: "default",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Name: "test-gateway",
								},
							},
						},
						Hostnames: []gwv1.Hostname{"example2.com"},
						Rules: []gwv1.HTTPRouteRule{
							{
								Matches: []gwv1.HTTPRouteMatch{
									{
										Path: &gwv1.HTTPPathMatch{
											Type:  ptr.To(gwv1.PathMatchPathPrefix),
											Value: ptr.To("/api2"),
										},
									},
								},
								BackendRefs: []gwv1.HTTPBackendRef{
									{
										BackendRef: gwv1.BackendRef{
											BackendObjectReference: gwv1.BackendObjectReference{
												Name: "test-service",
												Port: ptr.To(gwv1.PortNumber(80)),
											},
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
								Port: 80,
							},
						},
					},
				},
			},
			namespaces: []*corev1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
			},
			gateways: []GatewayListener{
				{
					Config: &Config{
						Meta: Meta{
							Name:      "test-gateway",
							Namespace: "default",
						},
					},
					parent: parentKey{
						Kind:      wellknown.GatewayGVK,
						Name:      "test-gateway",
						Namespace: "default",
					},
					parentInfo: parentInfo{
						InternalName: "default/test-gateway",
						Protocol:     gwv1.HTTPProtocolType,
						Port:         80,
						SectionName:  "http",
						AllowedKinds: []gwv1.RouteGroupKind{
							{
								Group: &groupName,
								Kind:  gwv1.Kind(wellknown.HTTPRouteKind),
							},
						},
					},
					Valid: true,
				},
			},
			refGrants:     []ReferenceGrant{},
			expectedCount: 2,
			expectedRoutes: []*api.Route{
				{
					Key:       "default/test-route-1.0.0.http",
					RouteName: "default/test-route-1",
					Hostnames: []string{"example.com"},
					Matches: []*api.RouteMatch{
						{
							Path: &api.PathMatch{
								Kind: &api.PathMatch_PathPrefix{
									PathPrefix: "/api",
								},
							},
						},
					},
					Backends: []*api.RouteBackend{
						{
							Backend: &api.BackendReference{
								Kind: &api.BackendReference_Service{
									Service: "default/test-service.default.svc.cluster.local",
								},
								Port: 80,
							},
						},
					},
				},
				{
					Key:       "default/test-route-2.0.0.http",
					RouteName: "default/test-route-2",
					Hostnames: []string{"example2.com"},
					Matches: []*api.RouteMatch{
						{
							Path: &api.PathMatch{
								Kind: &api.PathMatch_PathPrefix{
									PathPrefix: "/api2",
								},
							},
						},
					},
					Backends: []*api.RouteBackend{
						{
							Backend: &api.BackendReference{
								Kind: &api.BackendReference_Service{
									Service: "default/test-service.default.svc.cluster.local",
								},
								Port: 80,
							},
						},
					},
				},
			},
		},
		{
			name: "HTTP route with multiple rules",
			httpRoutes: []*gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multi-rule-route",
						Namespace: "default",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Name: "test-gateway",
								},
							},
						},
						Hostnames: []gwv1.Hostname{"example.com"},
						Rules: []gwv1.HTTPRouteRule{
							{
								Matches: []gwv1.HTTPRouteMatch{
									{
										Path: &gwv1.HTTPPathMatch{
											Type:  ptr.To(gwv1.PathMatchPathPrefix),
											Value: ptr.To("/api"),
										},
									},
								},
								BackendRefs: []gwv1.HTTPBackendRef{
									{
										BackendRef: gwv1.BackendRef{
											BackendObjectReference: gwv1.BackendObjectReference{
												Name: "test-service",
												Port: ptr.To(gwv1.PortNumber(80)),
											},
										},
									},
								},
							},
							{
								Matches: []gwv1.HTTPRouteMatch{
									{
										Path: &gwv1.HTTPPathMatch{
											Type:  ptr.To(gwv1.PathMatchPathPrefix),
											Value: ptr.To("/admin"),
										},
									},
								},
								BackendRefs: []gwv1.HTTPBackendRef{
									{
										BackendRef: gwv1.BackendRef{
											BackendObjectReference: gwv1.BackendObjectReference{
												Name: "admin-service",
												Port: ptr.To(gwv1.PortNumber(8080)),
											},
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
								Port: 80,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "admin-service",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Port: 8080,
							},
						},
					},
				},
			},
			namespaces: []*corev1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
			},
			gateways: []GatewayListener{
				{
					Config: &Config{
						Meta: Meta{
							Name:      "test-gateway",
							Namespace: "default",
						},
					},
					parent: parentKey{
						Kind:      wellknown.GatewayGVK,
						Name:      "test-gateway",
						Namespace: "default",
					},
					parentInfo: parentInfo{
						InternalName: "default/test-gateway",
						Protocol:     gwv1.HTTPProtocolType,
						Port:         80,
						SectionName:  "http",
						AllowedKinds: []gwv1.RouteGroupKind{
							{
								Group: &groupName,
								Kind:  gwv1.Kind(wellknown.HTTPRouteKind),
							},
						},
					},
					Valid: true,
				},
			},
			refGrants:     []ReferenceGrant{},
			expectedCount: 2,
			expectedRoutes: []*api.Route{ // TODO: consistent ordering of routes?
				{
					Key:       "default/multi-rule-route.0.0.http",
					RouteName: "default/multi-rule-route",
					Hostnames: []string{"example.com"},
					Matches: []*api.RouteMatch{
						{
							Path: &api.PathMatch{
								Kind: &api.PathMatch_PathPrefix{
									PathPrefix: "/api",
								},
							},
						},
					},
					Backends: []*api.RouteBackend{
						{
							Backend: &api.BackendReference{
								Kind: &api.BackendReference_Service{
									Service: "default/test-service.default.svc.cluster.local",
								},
								Port: 80,
							},
						},
					},
				},
				{
					Key:       "default/multi-rule-route.1.0.http",
					RouteName: "default/multi-rule-route",
					Hostnames: []string{"example.com"},
					Matches: []*api.RouteMatch{
						{
							Path: &api.PathMatch{
								Kind: &api.PathMatch_PathPrefix{
									PathPrefix: "/admin",
								},
							},
						},
					},
					Backends: []*api.RouteBackend{
						{
							Backend: &api.BackendReference{
								Kind: &api.BackendReference_Service{
									Service: "default/admin-service.default.svc.cluster.local",
								},
								Port: 8080,
							},
						},
					},
				},
			},
		},
		{
			name: "HTTP route with exact path match",
			httpRoutes: []*gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "exact-match-route",
						Namespace: "default",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Name: "test-gateway",
								},
							},
						},
						Hostnames: []gwv1.Hostname{"api.example.com"},
						Rules: []gwv1.HTTPRouteRule{
							{
								Matches: []gwv1.HTTPRouteMatch{
									{
										Path: &gwv1.HTTPPathMatch{
											Type:  ptr.To(gwv1.PathMatchExact),
											Value: ptr.To("/exact"),
										},
									},
								},
								BackendRefs: []gwv1.HTTPBackendRef{
									{
										BackendRef: gwv1.BackendRef{
											BackendObjectReference: gwv1.BackendObjectReference{
												Name: "test-service",
												Port: ptr.To(gwv1.PortNumber(80)),
											},
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
								Port: 80,
							},
						},
					},
				},
			},
			namespaces: []*corev1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
			},
			gateways: []GatewayListener{
				{
					Config: &Config{
						Meta: Meta{
							Name:      "test-gateway",
							Namespace: "default",
						},
					},
					parent: parentKey{
						Kind:      wellknown.GatewayGVK,
						Name:      "test-gateway",
						Namespace: "default",
					},
					parentInfo: parentInfo{
						InternalName: "default/test-gateway",
						Protocol:     gwv1.HTTPProtocolType,
						Port:         80,
						SectionName:  "http",
						AllowedKinds: []gwv1.RouteGroupKind{
							{
								Group: &groupName,
								Kind:  gwv1.Kind(wellknown.HTTPRouteKind),
							},
						},
					},
					Valid: true,
				},
			},
			refGrants:     []ReferenceGrant{},
			expectedCount: 1,
			expectedRoutes: []*api.Route{
				{
					Key:       "default/exact-match-route.0.0.http",
					RouteName: "default/exact-match-route",
					Hostnames: []string{"api.example.com"},
					Matches: []*api.RouteMatch{
						{
							Path: &api.PathMatch{
								Kind: &api.PathMatch_Exact{
									Exact: "/exact",
								},
							},
						},
					},
					Backends: []*api.RouteBackend{
						{
							Backend: &api.BackendReference{
								Kind: &api.BackendReference_Service{
									Service: "default/test-service.default.svc.cluster.local",
								},
								Port: 80,
							},
						},
					},
				},
			},
		},
		{
			name: "HTTP route with header match",
			httpRoutes: []*gwv1.HTTPRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "header-match-route",
						Namespace: "default",
					},
					Spec: gwv1.HTTPRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Name: "test-gateway",
								},
							},
						},
						Hostnames: []gwv1.Hostname{"example.com"},
						Rules: []gwv1.HTTPRouteRule{
							{
								Matches: []gwv1.HTTPRouteMatch{
									{
										Path: &gwv1.HTTPPathMatch{
											Type:  ptr.To(gwv1.PathMatchPathPrefix),
											Value: ptr.To("/api"),
										},
										Headers: []gwv1.HTTPHeaderMatch{
											{
												Type:  ptr.To(gwv1.HeaderMatchExact),
												Name:  "X-API-Version",
												Value: "v1",
											},
										},
									},
								},
								BackendRefs: []gwv1.HTTPBackendRef{
									{
										BackendRef: gwv1.BackendRef{
											BackendObjectReference: gwv1.BackendObjectReference{
												Name: "test-service",
												Port: ptr.To(gwv1.PortNumber(80)),
											},
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
								Port: 80,
							},
						},
					},
				},
			},
			namespaces: []*corev1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
			},
			gateways: []GatewayListener{
				{
					Config: &Config{
						Meta: Meta{
							Name:      "test-gateway",
							Namespace: "default",
						},
					},
					parent: parentKey{
						Kind:      wellknown.GatewayGVK,
						Name:      "test-gateway",
						Namespace: "default",
					},
					parentInfo: parentInfo{
						InternalName: "default/test-gateway",
						Protocol:     gwv1.HTTPProtocolType,
						Port:         80,
						SectionName:  "http",
						AllowedKinds: []gwv1.RouteGroupKind{
							{
								Group: &groupName,
								Kind:  gwv1.Kind(wellknown.HTTPRouteKind),
							},
						},
					},
					Valid: true,
				},
			},
			refGrants:     []ReferenceGrant{},
			expectedCount: 1,
			expectedRoutes: []*api.Route{
				{
					Key:       "default/header-match-route.0.0.http",
					RouteName: "default/header-match-route",
					Hostnames: []string{"example.com"},
					Matches: []*api.RouteMatch{
						{
							Path: &api.PathMatch{
								Kind: &api.PathMatch_PathPrefix{
									PathPrefix: "/api",
								},
							},
							Headers: []*api.HeaderMatch{
								{
									Name: "X-API-Version",
									Value: &api.HeaderMatch_Exact{
										Exact: "v1",
									},
								},
							},
						},
					},
					Backends: []*api.RouteBackend{
						{
							Backend: &api.BackendReference{
								Kind: &api.BackendReference_Service{
									Service: "default/test-service.default.svc.cluster.local",
								},
								Port: 80,
							},
						},
					},
				},
			},
		},
		{
			name:           "No HTTP routes",
			httpRoutes:     []*gwv1.HTTPRoute{},
			services:       []*corev1.Service{},
			namespaces:     []*corev1.Namespace{},
			gateways:       []GatewayListener{},
			refGrants:      []ReferenceGrant{},
			expectedCount:  0,
			expectedRoutes: []*api.Route{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Prepare inputs
			var inputs []any
			for _, route := range tc.httpRoutes {
				inputs = append(inputs, route)
			}
			for _, svc := range tc.services {
				inputs = append(inputs, svc)
			}
			for _, ns := range tc.namespaces {
				inputs = append(inputs, ns)
			}
			for _, gw := range tc.gateways {
				inputs = append(inputs, gw)
			}
			for _, gw := range tc.refGrants {
				inputs = append(inputs, gw)
			}

			// Create mock collections
			mock := krttest.NewMock(t, inputs)
			gateways := krttest.GetMockCollection[GatewayListener](mock)
			gatewayObjs := krttest.GetMockCollection[*gwv1.Gateway](mock)
			httpRoutes := krttest.GetMockCollection[*gwv1.HTTPRoute](mock)
			grpcRoutes := krttest.GetMockCollection[*gwv1.GRPCRoute](mock)
			tcpRoutes := krttest.GetMockCollection[*gwv1alpha2.TCPRoute](mock)
			tlsRoutes := krttest.GetMockCollection[*gwv1alpha2.TLSRoute](mock)
			refGrantsCollection := krttest.GetMockCollection[ReferenceGrant](mock)
			services := krttest.GetMockCollection[*corev1.Service](mock)
			namespaces := krttest.GetMockCollection[*corev1.Namespace](mock)
			serviceEntries := krttest.GetMockCollection[*networkingclient.ServiceEntry](mock)
			inferencePools := krttest.GetMockCollection[*inf.InferencePool](mock)

			// Wait for collections to sync
			gatewayObjs.WaitUntilSynced(context.Background().Done())
			httpRoutes.WaitUntilSynced(context.Background().Done())
			grpcRoutes.WaitUntilSynced(context.Background().Done())
			tcpRoutes.WaitUntilSynced(context.Background().Done())
			tlsRoutes.WaitUntilSynced(context.Background().Done())
			refGrantsCollection.WaitUntilSynced(context.Background().Done())
			services.WaitUntilSynced(context.Background().Done())
			namespaces.WaitUntilSynced(context.Background().Done())

			routeParents := BuildRouteParents(gateways)
			refGrants := BuildReferenceGrants(refGrantsCollection)
			// Create route context inputs
			routeInputs := RouteContextInputs{
				Grants:         refGrants,
				RouteParents:   routeParents,
				Services:       services,
				Namespaces:     namespaces,
				ServiceEntries: serviceEntries,
				InferencePools: inferencePools,
			}

			// Create KRT options
			krtopts := krtinternal.KrtOptions{}

			// Call ADPRouteCollection
			adpRoutes := ADPRouteCollection(httpRoutes, grpcRoutes, tcpRoutes, tlsRoutes, routeInputs, krtopts, pluginsdk.Plugin{})

			// Wait for the collection to process
			adpRoutes.WaitUntilSynced(context.Background().Done())

			// Get results
			results := adpRoutes.List()

			// Create a map of actual routes by key for easy lookup
			actualRoutes := make(map[string]*api.Route)
			for _, result := range results {
				require.NotNil(t, result.Resources, "Resource should not be nil")
				for _, resource := range result.Resources {
					routeResource := resource.GetRoute()
					require.NotNil(t, routeResource, "Route resource should not be nil")
					actualRoutes[routeResource.GetKey()] = routeResource
				}
			}
			// Verify expected count
			assert.Equal(t, tc.expectedCount, len(actualRoutes), "Expected %d routes but got %d", tc.expectedCount, len(actualRoutes))

			// Verify each expected route exists in the actual results
			for _, expectedRoute := range tc.expectedRoutes {
				expected := expectedRoute
				routeResource, found := actualRoutes[expected.GetKey()]
				require.True(t, found, "Expected route with key %s not found", expected.GetKey())

				// Verify route properties using the expected api.Route
				assert.Equal(t, expected.GetKey(), routeResource.GetKey(), "Route key mismatch")
				assert.Equal(t, expected.GetRouteName(), routeResource.GetRouteName(), "Route name mismatch")
				assert.Equal(t, expected.GetHostnames(), routeResource.GetHostnames(), "Hostnames mismatch")

				// Verify matches
				require.Equal(t, len(expected.GetMatches()), len(routeResource.GetMatches()), "Matches count mismatch")
				for j, expectedMatch := range expected.GetMatches() {
					actualMatch := routeResource.GetMatches()[j]

					// Verify path match
					if expectedMatch.GetPath() != nil {
						require.NotNil(t, actualMatch.GetPath(), "Path match should not be nil")
						switch expectedPath := expectedMatch.GetPath().GetKind().(type) {
						case *api.PathMatch_PathPrefix:
							actualPath, ok := actualMatch.GetPath().GetKind().(*api.PathMatch_PathPrefix)
							require.True(t, ok, "Expected PathPrefix match")
							assert.Equal(t, expectedPath.PathPrefix, actualPath.PathPrefix, "PathPrefix mismatch")
						case *api.PathMatch_Exact:
							actualPath, ok := actualMatch.GetPath().GetKind().(*api.PathMatch_Exact)
							require.True(t, ok, "Expected Exact match")
							assert.Equal(t, expectedPath.Exact, actualPath.Exact, "Exact path mismatch")
						}
					}

					// Verify header matches
					require.Equal(t, len(expectedMatch.GetHeaders()), len(actualMatch.GetHeaders()), "Header matches count mismatch")
					for k, expectedHeader := range expectedMatch.GetHeaders() {
						actualHeader := actualMatch.GetHeaders()[k]
						assert.Equal(t, expectedHeader.GetName(), actualHeader.GetName(), "Header name mismatch")
						switch expectedValue := expectedHeader.GetValue().(type) {
						case *api.HeaderMatch_Exact:
							actualValue, ok := actualHeader.GetValue().(*api.HeaderMatch_Exact)
							require.True(t, ok, "Expected exact header match")
							assert.Equal(t, expectedValue.Exact, actualValue.Exact, "Header exact value mismatch")
						}
					}
				}

				// Verify backends
				require.Equal(t, len(expected.GetBackends()), len(routeResource.GetBackends()), "Backends count mismatch")
				for j, expectedBackend := range expected.GetBackends() {
					actualBackend := routeResource.GetBackends()[j]
					assert.Equal(t, expectedBackend.GetBackend().GetPort(), actualBackend.GetBackend().GetPort(), "Backend port mismatch")

					// Verify service backend
					expectedKind := expectedBackend.GetBackend()
					actualKind := actualBackend.GetBackend()
					require.NotNil(t, expectedKind, "Expected backend kind should not be nil")
					require.NotNil(t, actualKind, "Actual backend kind should not be nil")

					switch expectedService := expectedKind.GetKind().(type) {
					case *api.BackendReference_Service:
						actualService, ok := actualKind.GetKind().(*api.BackendReference_Service)
						require.True(t, ok, "Expected service backend")
						assert.Equal(t, expectedService.Service, actualService.Service, "Service mismatch")
					}
				}
			}
		})
	}
}

func TestADPRouteCollectionGRPC(t *testing.T) {
	testCases := []struct {
		name           string
		grpcRoutes     []*gwv1.GRPCRoute
		services       []*corev1.Service
		namespaces     []*corev1.Namespace
		gateways       []GatewayListener
		refGrants      []ReferenceGrant
		expectedCount  int
		expectedRoutes []*api.Route
	}{
		{
			name: "Simple gRPC route with single rule",
			grpcRoutes: []*gwv1.GRPCRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-grpc-route",
						Namespace: "default",
					},
					Spec: gwv1.GRPCRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Name: "test-gateway",
								},
							},
						},
						Hostnames: []gwv1.Hostname{"grpc.example.com"},
						Rules: []gwv1.GRPCRouteRule{
							{
								Matches: []gwv1.GRPCRouteMatch{
									{
										Method: &gwv1.GRPCMethodMatch{
											Service: ptr.To("example.Service"),
											Method:  ptr.To("GetUser"),
										},
									},
								},
								BackendRefs: []gwv1.GRPCBackendRef{
									{
										BackendRef: gwv1.BackendRef{
											BackendObjectReference: gwv1.BackendObjectReference{
												Name: "grpc-service",
												Port: ptr.To(gwv1.PortNumber(9090)),
											},
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
						Name:      "grpc-service",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Port: 9090,
							},
						},
					},
				},
			},
			namespaces: []*corev1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
			},
			gateways: []GatewayListener{
				{
					Config: &Config{
						Meta: Meta{
							Name:      "test-gateway",
							Namespace: "default",
						},
					},
					parent: parentKey{
						Kind:      wellknown.GatewayGVK,
						Name:      "test-gateway",
						Namespace: "default",
					},
					parentInfo: parentInfo{
						InternalName: "default/test-gateway",
						Protocol:     gwv1.HTTPProtocolType,
						Port:         9090,
						SectionName:  "grpc",
						AllowedKinds: []gwv1.RouteGroupKind{
							{
								Group: &groupName,
								Kind:  gwv1.Kind(wellknown.GRPCRouteKind),
							},
						},
					},
					Valid: true,
				},
			},
			refGrants:     []ReferenceGrant{},
			expectedCount: 1,
			expectedRoutes: []*api.Route{
				{
					Key:       "default/test-grpc-route.0.grpc",
					RouteName: "default/test-grpc-route",
					Hostnames: []string{"grpc.example.com"},
					Matches: []*api.RouteMatch{
						{
							Path: &api.PathMatch{
								Kind: &api.PathMatch_Exact{
									Exact: "/example.Service/GetUser",
								},
							},
						},
					},
					Backends: []*api.RouteBackend{
						{
							Backend: &api.BackendReference{
								Kind: &api.BackendReference_Service{
									Service: "default/grpc-service.default.svc.cluster.local",
								},
								Port: 9090,
							},
						},
					},
				},
			},
		},
		{
			name: "gRPC route with multiple rules",
			grpcRoutes: []*gwv1.GRPCRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multi-rule-grpc-route",
						Namespace: "default",
					},
					Spec: gwv1.GRPCRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Name: "test-gateway",
								},
							},
						},
						Hostnames: []gwv1.Hostname{"grpc.example.com"},
						Rules: []gwv1.GRPCRouteRule{
							{
								Matches: []gwv1.GRPCRouteMatch{
									{
										Method: &gwv1.GRPCMethodMatch{
											Service: ptr.To("user.Service"),
											Method:  ptr.To("GetUser"),
										},
									},
								},
								BackendRefs: []gwv1.GRPCBackendRef{
									{
										BackendRef: gwv1.BackendRef{
											BackendObjectReference: gwv1.BackendObjectReference{
												Name: "user-service",
												Port: ptr.To(gwv1.PortNumber(9090)),
											},
										},
									},
								},
							},
							{
								Matches: []gwv1.GRPCRouteMatch{
									{
										Method: &gwv1.GRPCMethodMatch{
											Service: ptr.To("order.Service"),
											Method:  ptr.To("CreateOrder"),
										},
									},
								},
								BackendRefs: []gwv1.GRPCBackendRef{
									{
										BackendRef: gwv1.BackendRef{
											BackendObjectReference: gwv1.BackendObjectReference{
												Name: "order-service",
												Port: ptr.To(gwv1.PortNumber(9091)),
											},
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
						Name:      "user-service",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Port: 9090,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "order-service",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Port: 9091,
							},
						},
					},
				},
			},
			namespaces: []*corev1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
			},
			gateways: []GatewayListener{
				{
					Config: &Config{
						Meta: Meta{
							Name:      "test-gateway",
							Namespace: "default",
						},
					},
					parent: parentKey{
						Kind:      wellknown.GatewayGVK,
						Name:      "test-gateway",
						Namespace: "default",
					},
					parentInfo: parentInfo{
						InternalName: "default/test-gateway",
						Protocol:     gwv1.HTTPProtocolType,
						Port:         9090,
						SectionName:  "grpc",
						AllowedKinds: []gwv1.RouteGroupKind{
							{
								Group: &groupName,
								Kind:  gwv1.Kind(wellknown.GRPCRouteKind),
							},
						},
					},
					Valid: true,
				},
			},
			refGrants:     []ReferenceGrant{},
			expectedCount: 2,
			expectedRoutes: []*api.Route{
				{
					Key:       "default/multi-rule-grpc-route.0.grpc",
					RouteName: "default/multi-rule-grpc-route",
					Hostnames: []string{"grpc.example.com"},
					Matches: []*api.RouteMatch{
						{
							Path: &api.PathMatch{
								Kind: &api.PathMatch_Exact{
									Exact: "/user.Service/GetUser",
								},
							},
						},
					},
					Backends: []*api.RouteBackend{
						{
							Backend: &api.BackendReference{
								Kind: &api.BackendReference_Service{
									Service: "default/user-service.default.svc.cluster.local",
								},
								Port: 9090,
							},
						},
					},
				},
				{
					Key:       "default/multi-rule-grpc-route.1.grpc",
					RouteName: "default/multi-rule-grpc-route",
					Hostnames: []string{"grpc.example.com"},
					Matches: []*api.RouteMatch{
						{
							Path: &api.PathMatch{
								Kind: &api.PathMatch_Exact{
									Exact: "/order.Service/CreateOrder",
								},
							},
						},
					},
					Backends: []*api.RouteBackend{
						{
							Backend: &api.BackendReference{
								Kind: &api.BackendReference_Service{
									Service: "default/order-service.default.svc.cluster.local",
								},
								Port: 9091,
							},
						},
					},
				},
			},
		},
		{
			name: "gRPC route with header match",
			grpcRoutes: []*gwv1.GRPCRoute{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "grpc-header-route",
						Namespace: "default",
					},
					Spec: gwv1.GRPCRouteSpec{
						CommonRouteSpec: gwv1.CommonRouteSpec{
							ParentRefs: []gwv1.ParentReference{
								{
									Name: "test-gateway",
								},
							},
						},
						Hostnames: []gwv1.Hostname{"grpc.example.com"},
						Rules: []gwv1.GRPCRouteRule{
							{
								Matches: []gwv1.GRPCRouteMatch{
									{
										Method: &gwv1.GRPCMethodMatch{
											Service: ptr.To("example.Service"),
											Method:  ptr.To("GetUser"),
										},
										Headers: []gwv1.GRPCHeaderMatch{
											{
												Type:  ptr.To(gwv1.GRPCHeaderMatchExact),
												Name:  "authorization",
												Value: "Bearer token",
											},
										},
									},
								},
								BackendRefs: []gwv1.GRPCBackendRef{
									{
										BackendRef: gwv1.BackendRef{
											BackendObjectReference: gwv1.BackendObjectReference{
												Name: "grpc-service",
												Port: ptr.To(gwv1.PortNumber(9090)),
											},
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
						Name:      "grpc-service",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Port: 9090,
							},
						},
					},
				},
			},
			namespaces: []*corev1.Namespace{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
				},
			},
			gateways: []GatewayListener{
				{
					Config: &Config{
						Meta: Meta{
							Name:      "test-gateway",
							Namespace: "default",
						},
					},
					parent: parentKey{
						Kind:      wellknown.GatewayGVK,
						Name:      "test-gateway",
						Namespace: "default",
					},
					parentInfo: parentInfo{
						InternalName: "default/test-gateway",
						Protocol:     gwv1.HTTPProtocolType,
						Port:         9090,
						SectionName:  "grpc",
						AllowedKinds: []gwv1.RouteGroupKind{
							{
								Group: &groupName,
								Kind:  gwv1.Kind(wellknown.GRPCRouteKind),
							},
						},
					},
					Valid: true,
				},
			},
			refGrants:     []ReferenceGrant{},
			expectedCount: 1,
			expectedRoutes: []*api.Route{
				{
					Key:       "default/grpc-header-route.0.grpc",
					RouteName: "default/grpc-header-route",
					Hostnames: []string{"grpc.example.com"},
					Matches: []*api.RouteMatch{
						{
							Path: &api.PathMatch{
								Kind: &api.PathMatch_Exact{
									Exact: "/example.Service/GetUser",
								},
							},
							Headers: []*api.HeaderMatch{
								{
									Name: "authorization",
									Value: &api.HeaderMatch_Exact{
										Exact: "Bearer token",
									},
								},
							},
						},
					},
					Backends: []*api.RouteBackend{
						{
							Backend: &api.BackendReference{
								Kind: &api.BackendReference_Service{
									Service: "default/grpc-service.default.svc.cluster.local",
								},
								Port: 9090,
							},
						},
					},
				},
			},
		},
		{
			name:           "No gRPC routes",
			grpcRoutes:     []*gwv1.GRPCRoute{},
			services:       []*corev1.Service{},
			namespaces:     []*corev1.Namespace{},
			gateways:       []GatewayListener{},
			refGrants:      []ReferenceGrant{},
			expectedCount:  0,
			expectedRoutes: []*api.Route{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Prepare inputs
			var inputs []any
			for _, route := range tc.grpcRoutes {
				inputs = append(inputs, route)
			}
			for _, svc := range tc.services {
				inputs = append(inputs, svc)
			}
			for _, ns := range tc.namespaces {
				inputs = append(inputs, ns)
			}
			for _, gw := range tc.gateways {
				inputs = append(inputs, gw)
			}
			for _, gw := range tc.refGrants {
				inputs = append(inputs, gw)
			}

			// Create mock collections
			mock := krttest.NewMock(t, inputs)
			gateways := krttest.GetMockCollection[GatewayListener](mock)
			gatewayObjs := krttest.GetMockCollection[*gwv1.Gateway](mock)
			httpRoutes := krttest.GetMockCollection[*gwv1.HTTPRoute](mock)
			grpcRoutes := krttest.GetMockCollection[*gwv1.GRPCRoute](mock)
			tcpRoutes := krttest.GetMockCollection[*gwv1alpha2.TCPRoute](mock)
			tlsRoutes := krttest.GetMockCollection[*gwv1alpha2.TLSRoute](mock)
			refGrantsCollection := krttest.GetMockCollection[ReferenceGrant](mock)
			services := krttest.GetMockCollection[*corev1.Service](mock)
			namespaces := krttest.GetMockCollection[*corev1.Namespace](mock)
			serviceEntries := krttest.GetMockCollection[*networkingclient.ServiceEntry](mock)
			inferencePools := krttest.GetMockCollection[*inf.InferencePool](mock)

			// Wait for collections to sync
			gatewayObjs.WaitUntilSynced(context.Background().Done())
			httpRoutes.WaitUntilSynced(context.Background().Done())
			grpcRoutes.WaitUntilSynced(context.Background().Done())
			tcpRoutes.WaitUntilSynced(context.Background().Done())
			tlsRoutes.WaitUntilSynced(context.Background().Done())
			refGrantsCollection.WaitUntilSynced(context.Background().Done())
			services.WaitUntilSynced(context.Background().Done())
			namespaces.WaitUntilSynced(context.Background().Done())

			routeParents := BuildRouteParents(gateways)
			refGrants := BuildReferenceGrants(refGrantsCollection)
			// Create route context inputs
			routeInputs := RouteContextInputs{
				Grants:         refGrants,
				RouteParents:   routeParents,
				Services:       services,
				Namespaces:     namespaces,
				ServiceEntries: serviceEntries,
				InferencePools: inferencePools,
			}

			// Create KRT options
			krtopts := krtinternal.KrtOptions{}

			// Call ADPRouteCollection
			adpRoutes := ADPRouteCollection(httpRoutes, grpcRoutes, tcpRoutes, tlsRoutes, routeInputs, krtopts, pluginsdk.Plugin{})

			// Wait for the collection to process
			adpRoutes.WaitUntilSynced(context.Background().Done())

			// Get results
			results := adpRoutes.List()

			// Create a map of actual routes by key for easy lookup
			actualRoutes := make(map[string]*api.Route)
			for _, result := range results {
				require.NotNil(t, result.Resources, "Resource should not be nil")
				for _, resource := range result.Resources {
					routeResource := resource.GetRoute()
					require.NotNil(t, routeResource, "Route resource should not be nil")
					actualRoutes[routeResource.GetKey()] = routeResource
				}
			}
			// Verify expected count
			assert.Equal(t, tc.expectedCount, len(actualRoutes), "Expected %d routes but got %d", tc.expectedCount, len(actualRoutes))

			// Verify each expected route exists in the actual results
			for _, expectedRoute := range tc.expectedRoutes {
				expected := expectedRoute
				routeResource, found := actualRoutes[expected.GetKey()]
				require.True(t, found, "Expected route with key %s not found", expected.GetKey())

				// Verify route properties using the expected api.Route
				assert.Equal(t, expected.GetKey(), routeResource.GetKey(), "Route key mismatch")
				assert.Equal(t, expected.GetRouteName(), routeResource.GetRouteName(), "Route name mismatch")
				assert.Equal(t, expected.GetHostnames(), routeResource.GetHostnames(), "Hostnames mismatch")

				// Verify matches
				require.Equal(t, len(expected.GetMatches()), len(routeResource.GetMatches()), "Matches count mismatch")
				for j, expectedMatch := range expected.GetMatches() {
					actualMatch := routeResource.GetMatches()[j]

					// Verify path match (gRPC service/method is converted to path)
					if expectedMatch.GetPath() != nil {
						require.NotNil(t, actualMatch.GetPath(), "Path match should not be nil")
						switch expectedPath := expectedMatch.GetPath().GetKind().(type) {
						case *api.PathMatch_Exact:
							actualPath, ok := actualMatch.GetPath().GetKind().(*api.PathMatch_Exact)
							require.True(t, ok, "Expected Exact path match")
							assert.Equal(t, expectedPath.Exact, actualPath.Exact, "Exact path mismatch")
						case *api.PathMatch_Regex:
							actualPath, ok := actualMatch.GetPath().GetKind().(*api.PathMatch_Regex)
							require.True(t, ok, "Expected Regex path match")
							assert.Equal(t, expectedPath.Regex, actualPath.Regex, "Regex path mismatch")
						}
					}

					// Verify header matches
					require.Equal(t, len(expectedMatch.GetHeaders()), len(actualMatch.GetHeaders()), "Header matches count mismatch")
					for k, expectedHeader := range expectedMatch.GetHeaders() {
						actualHeader := actualMatch.GetHeaders()[k]
						assert.Equal(t, expectedHeader.GetName(), actualHeader.GetName(), "Header name mismatch")
						switch expectedValue := expectedHeader.GetValue().(type) {
						case *api.HeaderMatch_Exact:
							actualValue, ok := actualHeader.GetValue().(*api.HeaderMatch_Exact)
							require.True(t, ok, "Expected exact header match")
							assert.Equal(t, expectedValue.Exact, actualValue.Exact, "Header exact value mismatch")
						}
					}
				}

				// Verify backends
				require.Equal(t, len(expected.GetBackends()), len(routeResource.GetBackends()), "Backends count mismatch")
				for j, expectedBackend := range expected.GetBackends() {
					actualBackend := routeResource.GetBackends()[j]
					assert.Equal(t, expectedBackend.GetBackend().GetPort(), actualBackend.GetBackend().GetPort(), "Backend port mismatch")

					// Verify service backend
					expectedKind := expectedBackend.GetBackend()
					actualKind := actualBackend.GetBackend()
					require.NotNil(t, expectedKind, "Expected backend kind should not be nil")
					require.NotNil(t, actualKind, "Actual backend kind should not be nil")

					switch expectedService := expectedKind.GetKind().(type) {
					case *api.BackendReference_Service:
						actualService, ok := actualKind.GetKind().(*api.BackendReference_Service)
						require.True(t, ok, "Expected service backend")
						assert.Equal(t, expectedService.Service, actualService.Service, "Service mismatch")
					}
				}
			}
		})
	}
}

func TestADPRouteCollectionWithFilters(t *testing.T) {
	testCases := []struct {
		name           string
		httpRoute      *gwv1.HTTPRoute
		expectedFilter *api.RouteFilter
	}{
		{
			name: "Route with request header modifier",
			httpRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "header-route",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							Filters: []gwv1.HTTPRouteFilter{
								{
									Type: gwv1.HTTPRouteFilterRequestHeaderModifier,
									RequestHeaderModifier: &gwv1.HTTPHeaderFilter{
										Set: []gwv1.HTTPHeader{
											{
												Name:  "X-Test-Header",
												Value: "test-value",
											},
										},
									},
								},
							},
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
											Port: ptr.To(gwv1.PortNumber(80)),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedFilter: &api.RouteFilter{
				Kind: &api.RouteFilter_RequestHeaderModifier{
					RequestHeaderModifier: &api.HeaderModifier{
						Set: []*api.Header{
							{
								Name:  "X-Test-Header",
								Value: "test-value",
							},
						},
					},
				},
			},
		},
		{
			name: "Route with redirect filter",
			httpRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "redirect-route",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							Filters: []gwv1.HTTPRouteFilter{
								{
									Type: gwv1.HTTPRouteFilterRequestRedirect,
									RequestRedirect: &gwv1.HTTPRequestRedirectFilter{
										Scheme:     ptr.To("https"),
										StatusCode: ptr.To(301),
									},
								},
							},
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
											Port: ptr.To(gwv1.PortNumber(80)),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedFilter: &api.RouteFilter{
				Kind: &api.RouteFilter_RequestRedirect{
					RequestRedirect: &api.RequestRedirect{
						Scheme: "https",
						Status: 301,
					},
				},
			},
		},
		{
			name: "Route with CORS filter",
			httpRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cors-route",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
					Rules: []gwv1.HTTPRouteRule{
						{
							Filters: []gwv1.HTTPRouteFilter{
								{
									Type: gwv1.HTTPRouteFilterCORS,
									CORS: &gwv1.HTTPCORSFilter{
										AllowCredentials: true,
										AllowHeaders: []gwv1.HTTPHeaderName{
											"Content-Type",
											"Authorization",
										},
										AllowMethods: []gwv1.HTTPMethodWithWildcard{
											"GET",
											"POST",
											"PUT",
										},
										AllowOrigins: []gwv1.AbsoluteURI{
											"https://example.com",
											"https://*.example.org",
										},
										ExposeHeaders: []gwv1.HTTPHeaderName{
											"X-Custom-Header",
										},
										MaxAge: 300,
									},
								},
							},
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
											Port: ptr.To(gwv1.PortNumber(80)),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedFilter: &api.RouteFilter{
				Kind: &api.RouteFilter_Cors{
					Cors: &api.CORS{
						AllowCredentials: true,
						AllowHeaders:     []string{"Content-Type", "Authorization"},
						AllowMethods:     []string{"GET", "POST", "PUT"},
						AllowOrigins:     []string{"https://example.com", "https://*.example.org"},
						ExposeHeaders:    []string{"X-Custom-Header"},
						MaxAge: &duration.Duration{
							Seconds: 300,
						},
					},
				},
			},
		},
		{
			name: "Route with DirectResponse ExtensionRef filter",
			httpRoute: &gwv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "direct-response-route",
					Namespace: "default",
				},
				Spec: gwv1.HTTPRouteSpec{
					CommonRouteSpec: gwv1.CommonRouteSpec{
						ParentRefs: []gwv1.ParentReference{
							{
								Name: "test-gateway",
							},
						},
					},
					Hostnames: []gwv1.Hostname{"example.com"},
					Rules: []gwv1.HTTPRouteRule{
						{
							Matches: []gwv1.HTTPRouteMatch{
								{
									Path: &gwv1.HTTPPathMatch{
										Type:  ptr.To(gwv1.PathMatchPathPrefix),
										Value: ptr.To("/robots.txt"),
									},
								},
							},
							Filters: []gwv1.HTTPRouteFilter{
								{
									Type: gwv1.HTTPRouteFilterExtensionRef,
									ExtensionRef: &gwv1.LocalObjectReference{
										Group: "gateway.kgateway.dev",
										Kind:  "DirectResponse",
										Name:  "robots-response",
									},
								},
							},
							BackendRefs: []gwv1.HTTPBackendRef{
								{
									BackendRef: gwv1.BackendRef{
										BackendObjectReference: gwv1.BackendObjectReference{
											Name: "test-service",
											Port: ptr.To(gwv1.PortNumber(80)),
										},
									},
								},
							},
						},
					},
				},
			},
			expectedFilter: &api.RouteFilter{
				Kind: &api.RouteFilter_DirectResponse{
					DirectResponse: &api.DirectResponse{
						Status: 200,
						Body:   []byte("User-agent: *\nDisallow: /admin\nAllow: /"),
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Prepare inputs
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port: 80,
						},
					},
				},
			}

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}

			gateway := GatewayListener{
				Config: &Config{
					Meta: Meta{
						Name:      "test-gateway",
						Namespace: "default",
					},
				},
				parent: parentKey{
					Kind:      wellknown.GatewayGVK,
					Name:      "test-gateway",
					Namespace: "default",
				},
				parentInfo: parentInfo{
					InternalName: "default/test-gateway",
					Protocol:     gwv1.HTTPProtocolType,
					Port:         80,
					SectionName:  "http",
					AllowedKinds: []gwv1.RouteGroupKind{
						{
							Group: &groupName,
							Kind:  gwv1.Kind(wellknown.HTTPRouteKind),
						},
					},
				},
				Valid: true,
			}

			refGrant := ReferenceGrant{}

			var inputs []any
			inputs = []any{tc.httpRoute, service, namespace, gateway, refGrant}

			// Add DirectResponse resources for the DirectResponse filter tests
			if tc.name == "Route with DirectResponse ExtensionRef filter" {
				directResponse := &v1alpha1.DirectResponse{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "robots-response",
						Namespace: "default",
					},
					Spec: v1alpha1.DirectResponseSpec{
						StatusCode: 200,
						Body:       ptr.To("User-agent: *\nDisallow: /admin\nAllow: /"),
					},
				}
				inputs = append(inputs, directResponse)
			}

			// Create mock collections
			mock := krttest.NewMock(t, inputs)
			gateways := krttest.GetMockCollection[GatewayListener](mock)
			gatewayObjs := krttest.GetMockCollection[*gwv1.Gateway](mock)
			httpRoutes := krttest.GetMockCollection[*gwv1.HTTPRoute](mock)
			grpcRoutes := krttest.GetMockCollection[*gwv1.GRPCRoute](mock)
			tcpRoutes := krttest.GetMockCollection[*gwv1alpha2.TCPRoute](mock)
			tlsRoutes := krttest.GetMockCollection[*gwv1alpha2.TLSRoute](mock)
			refGrantsCollection := krttest.GetMockCollection[ReferenceGrant](mock)
			services := krttest.GetMockCollection[*corev1.Service](mock)
			namespaces := krttest.GetMockCollection[*corev1.Namespace](mock)
			serviceEntries := krttest.GetMockCollection[*networkingclient.ServiceEntry](mock)
			inferencePools := krttest.GetMockCollection[*inf.InferencePool](mock)
			directResponses := krttest.GetMockCollection[*v1alpha1.DirectResponse](mock)

			// Wait for collections to sync
			gatewayObjs.WaitUntilSynced(context.Background().Done())
			httpRoutes.WaitUntilSynced(context.Background().Done())
			grpcRoutes.WaitUntilSynced(context.Background().Done())
			tcpRoutes.WaitUntilSynced(context.Background().Done())
			tlsRoutes.WaitUntilSynced(context.Background().Done())
			refGrantsCollection.WaitUntilSynced(context.Background().Done())
			services.WaitUntilSynced(context.Background().Done())
			namespaces.WaitUntilSynced(context.Background().Done())

			routeParents := BuildRouteParents(gateways)
			refGrants := BuildReferenceGrants(refGrantsCollection)
			// Create route context inputs
			routeInputs := RouteContextInputs{
				Grants:          refGrants,
				RouteParents:    routeParents,
				Services:        services,
				Namespaces:      namespaces,
				ServiceEntries:  serviceEntries,
				InferencePools:  inferencePools,
				DirectResponses: directResponses,
			}

			// Create KRT options
			krtopts := krtinternal.KrtOptions{}

			// Call ADPRouteCollection
			adpRoutes := ADPRouteCollection(httpRoutes, grpcRoutes, tcpRoutes, tlsRoutes, routeInputs, krtopts, pluginsdk.Plugin{})

			// Wait for the collection to process
			adpRoutes.WaitUntilSynced(context.Background().Done())

			// Get results
			results := adpRoutes.List()

			// Verify we got a result
			require.Len(t, results, 1, "Expected exactly one route")

			result := results[0]
			require.NotNil(t, result.Resources, "Resource should not be nil")

			routeResource := result.Resources[0].GetRoute()
			require.NotNil(t, routeResource, "Route resource should not be nil")

			// Verify filters
			require.Len(t, routeResource.GetFilters(), 1, "Expected exactly one filter")
			actualFilter := routeResource.GetFilters()[0]

			// Verify filter type and content
			switch expectedKind := tc.expectedFilter.GetKind().(type) {
			case *api.RouteFilter_RequestHeaderModifier:
				actualKind, ok := actualFilter.GetKind().(*api.RouteFilter_RequestHeaderModifier)
				require.True(t, ok, "Expected RequestHeaderModifier filter")

				expectedMod := expectedKind.RequestHeaderModifier
				actualMod := actualKind.RequestHeaderModifier

				require.Equal(t, len(expectedMod.GetSet()), len(actualMod.GetSet()), "Set headers count mismatch")
				for i, expectedHeader := range expectedMod.GetSet() {
					actualHeader := actualMod.GetSet()[i]
					assert.Equal(t, expectedHeader.GetName(), actualHeader.GetName(), "Header name mismatch")
					assert.Equal(t, expectedHeader.GetValue(), actualHeader.GetValue(), "Header value mismatch")
				}

			case *api.RouteFilter_RequestRedirect:
				actualKind, ok := actualFilter.GetKind().(*api.RouteFilter_RequestRedirect)
				require.True(t, ok, "Expected RequestRedirect filter")

				expectedRedirect := expectedKind.RequestRedirect
				actualRedirect := actualKind.RequestRedirect

				assert.Equal(t, expectedRedirect.GetScheme(), actualRedirect.GetScheme(), "Redirect scheme mismatch")
				assert.Equal(t, expectedRedirect.GetStatus(), actualRedirect.GetStatus(), "Redirect status mismatch")
			case *api.RouteFilter_Cors:
				actualKind, ok := actualFilter.GetKind().(*api.RouteFilter_Cors)
				require.True(t, ok, "Expected CORS filter")

				expectedCors := expectedKind.Cors
				actualCors := actualKind.Cors

				assert.Equal(t, expectedCors.GetAllowCredentials(), actualCors.GetAllowCredentials(), "CORS AllowCredentials mismatch")
				assert.Equal(t, expectedCors.GetAllowHeaders(), actualCors.GetAllowHeaders(), "CORS AllowHeaders mismatch")
				assert.Equal(t, expectedCors.GetAllowMethods(), actualCors.GetAllowMethods(), "CORS AllowMethods mismatch")
				assert.Equal(t, expectedCors.GetAllowOrigins(), actualCors.GetAllowOrigins(), "CORS AllowOrigins mismatch")
				assert.Equal(t, expectedCors.GetExposeHeaders(), actualCors.GetExposeHeaders(), "CORS ExposeHeaders mismatch")
				assert.Equal(t, expectedCors.GetMaxAge().GetSeconds(), actualCors.GetMaxAge().GetSeconds(), "CORS MaxAge mismatch")
			case *api.RouteFilter_DirectResponse:
				actualKind, ok := actualFilter.GetKind().(*api.RouteFilter_DirectResponse)
				require.True(t, ok, "Expected DirectResponse filter")

				expectedDirect := expectedKind.DirectResponse
				actualDirect := actualKind.DirectResponse

				assert.Equal(t, expectedDirect.GetStatus(), actualDirect.GetStatus(), "DirectResponse status mismatch")

				// Verify body if it exists
				if expectedDirect.GetBody() != nil {
					require.NotNil(t, actualDirect.GetBody(), "Expected DirectResponse to have body")
					assert.Equal(t, string(expectedDirect.GetBody()), string(actualDirect.GetBody()), "DirectResponse body mismatch")
				} else {
					assert.Nil(t, actualDirect.GetBody(), "Expected DirectResponse to have no body")
				}
			}
		})
	}
}

func TestADPRouteCollectionEquals(t *testing.T) {
	// Test that ADPResourcesForGateway implements Equals correctly
	route1 := &api.Route{
		Key:       "test-key",
		RouteName: "test-route",
	}

	route2 := &api.Route{
		Key:       "test-key",
		RouteName: "test-route",
	}

	route3 := &api.Route{
		Key:       "different-key",
		RouteName: "test-route",
	}

	gateway := types.NamespacedName{
		Name:      "test-gateway",
		Namespace: "default",
	}

	adpResource1 := ADPResourcesForGateway{
		Resources: []*api.Resource{
			{
				Kind: &api.Resource_Route{
					Route: route1,
				},
			},
		},
		Gateway: gateway,
	}

	adpResource2 := ADPResourcesForGateway{
		Resources: []*api.Resource{
			{
				Kind: &api.Resource_Route{
					Route: route2,
				},
			},
		},
		Gateway: gateway,
	}

	adpResource3 := ADPResourcesForGateway{
		Resources: []*api.Resource{
			{
				Kind: &api.Resource_Route{
					Route: route3,
				},
			},
		},
		Gateway: gateway,
	}

	assert.True(t, adpResource1.Equals(adpResource2), "Equal resources should return true")
	assert.False(t, adpResource1.Equals(adpResource3), "Different resources should return false")
}
