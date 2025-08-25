package agentgatewaysyncer

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

type translatorTestCase struct {
	inputFile     string
	outputFile    string
	assertReports AssertReports
	gwNN          types.NamespacedName
}

func TestBasic(t *testing.T) {
	test := func(t *testing.T, in translatorTestCase, settingOpts ...SettingsOpts) {
		dir := fsutils.MustGetThisDir()

		inputFiles := []string{filepath.Join(dir, "testdata/inputs/", in.inputFile)}
		expectedProxyFile := filepath.Join(dir, "testdata/outputs/", in.outputFile)
		TestTranslation(t, t.Context(), inputFiles, expectedProxyFile, in.gwNN, in.assertReports, settingOpts...)
	}

	t.Run("http gateway with basic http routing", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "http-routing",
			outputFile: "http-routing-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("grpc gateway with basic routing", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "grpc-routing/basic.yaml",
			outputFile: "grpc-routing/basic-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1.GRPCRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-grpc-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				assert.NotNil(t, routeStatus)
				assert.Len(t, routeStatus.Parents, 1)
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				assert.NotNil(t, resolvedRefs)
				assert.Equal(t, metav1.ConditionTrue, resolvedRefs.Status)
				assert.Equal(t, string(gwv1.RouteReasonResolvedRefs), resolvedRefs.Reason)
			},
		})
	})

	t.Run("grpcroute with missing backend reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "grpc-routing/missing-backend.yaml",
			outputFile: "grpc-routing/missing-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1.GRPCRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-grpc-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				assert.NotNil(t, routeStatus)
				assert.Len(t, routeStatus.Parents, 1)
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				assert.NotNil(t, resolvedRefs)
				assert.Equal(t, metav1.ConditionFalse, resolvedRefs.Status)
				assert.Equal(t, `backend(example-grpc-svc.default.svc.cluster.local) not found`, resolvedRefs.Message)
			},
		})
	})

	t.Run("grpcroute with invalid backend reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "grpc-routing/invalid-backend.yaml",
			outputFile: "grpc-routing/invalid-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1.GRPCRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-grpc-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				assert.NotNil(t, routeStatus)
				assert.Len(t, routeStatus.Parents, 1)
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				assert.NotNil(t, resolvedRefs)
				assert.Equal(t, metav1.ConditionFalse, resolvedRefs.Status)
				assert.Equal(t, "referencing unsupported backendRef: group \"\" kind \"ConfigMap\"", resolvedRefs.Message)
			},
		})
	})

	t.Run("grpc gateway with multiple backend services", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "grpc-routing/multi-backend.yaml",
			outputFile: "grpc-routing/multi-backend-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-grpc-gateway",
			},
		})
	})

	t.Run("Proxy with no routes", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "edge-cases/no-route.yaml",
			outputFile: "no-route.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("HTTPRoutes with timeout and retry", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "httproute-timeout-retry/manifest.yaml",
			outputFile: "httproute-timeout-retry-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Service with appProtocol=anything", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend-protocol/svc-default.yaml",
			outputFile: "backend-protocol/svc-default.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Static Backend with no appProtocol", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend-protocol/backend-default.yaml",
			outputFile: "backend-protocol/backend-default.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("MCP Backend with selector target", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend-protocol/mcp-backend-selector.yaml",
			outputFile: "backend-protocol/mcp-backend-selector.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("MCP Backend with static target", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend-protocol/mcp-backend-static.yaml",
			outputFile: "backend-protocol/mcp-backend-static.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("AI Backend with openai provider", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend-protocol/openai-backend.yaml",
			outputFile: "backend-protocol/openai-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Backend with a2a provider", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend-protocol/a2a-backend.yaml",
			outputFile: "backend-protocol/a2a-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("AI Backend with bedrock provider", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend-protocol/bedrock-backend.yaml",
			outputFile: "backend-protocol/bedrock-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Direct response", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "direct-response/manifest.yaml",
			outputFile: "direct-response.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("DirectResponse with missing reference reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "direct-response/missing-ref.yaml",
			outputFile: "direct-response/missing-ref.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				assert.NotNil(t, routeStatus)
				assert.Len(t, routeStatus.Parents, 1)

				// Assert ResolvedRefs=True since the route structure is valid
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				assert.NotNil(t, resolvedRefs)
				assert.Equal(t, metav1.ConditionTrue, resolvedRefs.Status)
				assert.Equal(t, string(gwv1.RouteReasonResolvedRefs), resolvedRefs.Reason)

				// Assert Accepted=False reports the missing DirectResponse
				acceptedCond := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				assert.NotNil(t, acceptedCond)
				assert.Equal(t, metav1.ConditionFalse, acceptedCond.Status)
				assert.Equal(t, string(gwv1.RouteReasonBackendNotFound), acceptedCond.Reason)
				assert.Equal(t, "DirectResponse default/non-existent-ref not found", acceptedCond.Message)
			},
		})
	})

	t.Run("DirectResponse with overlapping filters reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "direct-response/overlapping-filters.yaml",
			outputFile: "direct-response/overlapping-filters.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				assert.NotNil(t, routeStatus)
				assert.Len(t, routeStatus.Parents, 1)

				// Check for Accepted=False condition due to overlapping terminal filters
				acceptedCond := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				assert.NotNil(t, acceptedCond)
				assert.Equal(t, metav1.ConditionFalse, acceptedCond.Status)
				assert.Equal(t, string(gwv1.RouteReasonIncompatibleFilters), acceptedCond.Reason)
				assert.Contains(t, acceptedCond.Message, "terminal filter")
			},
		})
	})

	t.Run("DirectResponse with invalid backendRef filter reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "direct-response/invalid-backendref-filter.yaml",
			outputFile: "direct-response/invalid-backendref-filter.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				assert.NotNil(t, routeStatus)
				assert.Len(t, routeStatus.Parents, 1)

				// DirectResponse attached to backendRef should be ignored, route should resolve normally
				acceptedCond := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				assert.NotNil(t, acceptedCond)
				assert.Equal(t, metav1.ConditionTrue, acceptedCond.Status)
				assert.Equal(t, string(gwv1.RouteReasonAccepted), acceptedCond.Reason)

				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				assert.NotNil(t, resolvedRefs)
				assert.Equal(t, metav1.ConditionTrue, resolvedRefs.Status)
			},
		})
	})

	t.Run("TrafficPolicy with extauth on route", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "trafficpolicy/extauth-route.yaml",
			outputFile: "trafficpolicy/extauth-route.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy with extauth on gateway", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "trafficpolicy/extauth-gateway.yaml",
			outputFile: "trafficpolicy/extauth-gateway.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	// TODO(npolshak): re-enable once listener policies are supported once https://github.com/agentgateway/agentgateway/pull/323 goes in
	// t.Run("TrafficPolicy with extauth on listener", func(t *testing.T) {
	//	test(t, translatorTestCase{
	//		inputFile:  "trafficpolicy/extauth-listener.yaml",
	//		outputFile: "trafficpolicy/extauth-listener.yaml",
	//		gwNN: types.NamespacedName{
	//			Namespace: "default",
	//			Name:      "example-gateway",
	//		},
	//	})
	t.Run("AI TrafficPolicy on route level", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "trafficpolicy/ai/route-level.yaml",
			outputFile: "trafficpolicy/ai/route-level.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})
}
