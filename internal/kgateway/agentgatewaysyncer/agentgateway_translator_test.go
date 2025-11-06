package agentgatewaysyncer

import (
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

type translatorTestCase struct {
	inputFile  string
	outputFile string
	gwNN       types.NamespacedName
}

func TestBasic(t *testing.T) {
	test := func(t *testing.T, in translatorTestCase, settingOpts ...SettingsOpts) {
		dir := fsutils.MustGetThisDir()

		inputFiles := []string{filepath.Join(dir, "testdata/inputs/", in.inputFile)}
		expectedProxyFile := filepath.Join(dir, "testdata/outputs/", in.outputFile)
		expectedstatusFile := filepath.Join(dir, "testdata/outputs/", strings.Replace(in.outputFile, ".yaml", ".status.yaml", -1))
		TestTranslation(t, t.Context(), inputFiles, expectedProxyFile, expectedstatusFile, in.gwNN, settingOpts...)
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

	t.Run("tlsroute", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "tls-routing/gateway.yaml",
			outputFile: "tls-routing/gateway.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
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
			inputFile:  "backend/svc-default.yaml",
			outputFile: "backend/svc-default.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Static Backend with no appProtocol", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend/backend-default.yaml",
			outputFile: "backend/backend-default.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("MCP Backend with selector target", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend/mcp-backend-selector.yaml",
			outputFile: "backend/mcp-backend-selector.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("MCP Backend with static target", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend/mcp-backend-static.yaml",
			outputFile: "backend/mcp-backend-static.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("AI Backend with openai provider", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend/openai-backend.yaml",
			outputFile: "backend/openai-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Backend with a2a provider", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend/a2a-backend.yaml",
			outputFile: "backend/a2a-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("AI Backend with bedrock provider", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend/bedrock-backend.yaml",
			outputFile: "backend/bedrock-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Backend TLS", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend/backend-tls.yaml",
			outputFile: "backend/backend-tls.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("PriorityGroups Backend with inline auth", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend/multipool-inline-auth.yaml",
			outputFile: "backend/multipool-inline-auth.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("PriorityGroups Backend with secret auth", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend/multipool-secret-auth.yaml",
			outputFile: "backend/multipool-secret-auth.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("PriorityGroups Backend with multiple priority levels", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend/multipool-priority-levels.yaml",
			outputFile: "backend/multipool-priority-levels.yaml",
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
			outputFile: "direct-response/missing-ref-output.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("DirectResponse with overlapping filters reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "direct-response/overlapping-filters.yaml",
			outputFile: "direct-response/overlapping-filters-output.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
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
	t.Run("TrafficPolicy with extauth on listener", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "trafficpolicy/extauth-listener.yaml",
			outputFile: "trafficpolicy/extauth-listener.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})
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

	t.Run("AI TrafficPolicy on route level with Bearer secret and OpenAI Moderation", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "trafficpolicy/ai/route-level-bearer.yaml",
			outputFile: "trafficpolicy/ai/route-level-bearer.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy with rbac on http route with Static backend", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "trafficpolicy/rbac/http-rbac.yaml",
			outputFile: "trafficpolicy/rbac/http-rbac.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy with rbac on http route with MCP backend", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "trafficpolicy/rbac/mcp-rbac.yaml",
			outputFile: "trafficpolicy/rbac/mcp-rbac.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy with transformation", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "trafficpolicy/transformation.yaml",
			outputFile: "trafficpolicy/transformation.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy with CSRF", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "trafficpolicy/csrf.yaml",
			outputFile: "trafficpolicy/csrf.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})
}
