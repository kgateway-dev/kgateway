package gateway_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	translatortest "github.com/kgateway-dev/kgateway/v2/test/translator"
)

type translatorTestCase struct {
	inputFile  string
	outputFile string
	gwNN       types.NamespacedName
}

func TestBasic(t *testing.T) {
	test := func(t *testing.T, in translatorTestCase, settingOpts ...translatortest.SettingsOpts) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dir := fsutils.MustGetThisDir()

		inputFiles := []string{filepath.Join(dir, "testutils/inputs/", in.inputFile)}
		expectedProxyFile := filepath.Join(dir, "testutils/outputs/", in.outputFile)
		translatortest.TestTranslation(t, ctx, inputFiles, expectedProxyFile, in.gwNN, settingOpts...)
	}

	t.Run("gateway with no routes should not add empty filter chain", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "gateway-only/gateway.yaml",
			outputFile: "gateway-only/proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("gateway with no valid listeners should report correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "gateway-only/gateway-invalid-listener.yaml",
			outputFile: "gateway-only/gateway-invalid-listener-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("http gateway with per connection buffer limit", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "gateway-per-conn-buf-lim/gateway.yaml",
			outputFile: "gateway-per-conn-buf-lim/proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("http gateway with basic routing", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "http-routing",
			outputFile: "http-routing-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("http gateway with custom class", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "custom-gateway-class",
			outputFile: "custom-gateway-class.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("https gateway with basic routing", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "https-routing/gateway.yaml",
			outputFile: "https-routing-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("https gateway with invalid certificate ref", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "https-routing/invalid-cert.yaml",
			outputFile: "https-invalid-cert-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("http gateway with multiple listeners on the same port", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "multiple-listeners-http-routing",
			outputFile: "multiple-listeners-http-routing-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "http",
			},
		})
	})

	t.Run("https gateway with multiple listeners on the same port", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "multiple-listeners-https-routing",
			outputFile: "multiple-listeners-https-routing-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "http",
			},
		})
	})

	t.Run("http gateway with multiple routing rules and HeaderModifier filter", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "http-with-header-modifier",
			outputFile: "http-with-header-modifier-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "gw",
			},
		})
	})

	t.Run("Gateway API route sorting", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-sort.yaml",
			outputFile: "route-sort.yaml",
			gwNN: types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("weight based route sorting", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-sort-weighted.yaml",
			outputFile: "route-sort-weighted.yaml",
			gwNN: types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
		}, func(s *apisettings.Settings) {
			s.WeightedRoutePrecedence = true
		})
	})

	t.Run("httproute with missing backend reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "http-routing-missing-backend",
			outputFile: "http-routing-missing-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("httproute with invalid backend reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "http-routing-invalid-backend",
			outputFile: "http-routing-invalid-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("httproute with backend port error reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backends/backend-ref-port-error.yaml",
			outputFile: "backends/backend-ref-port-error.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy merging", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/merge.yaml",
			outputFile: "traffic-policy/merge.yaml",
			gwNN: types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy with targetSelectors", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/label_based.yaml",
			outputFile: "traffic-policy/label_based.yaml",
			gwNN: types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy with targetSelectors and global policy attachment", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/label_based.yaml",
			outputFile: "traffic-policy/label_based_global_policy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
		},
			func(s *apisettings.Settings) {
				s.GlobalPolicyNamespace = "kgateway-system"
			})
	})

	t.Run("TrafficPolicy ExtAuth different attachment points", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/extauth.yaml",
			outputFile: "traffic-policy/extauth.yaml",
			gwNN: types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
		})
	})

	// test the default and fully configured values for ExtAuth
	t.Run("TrafficPolicy ExtAuth Full Config", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/extauth-full-config.yaml",
			outputFile: "traffic-policy/extauth-full-config.yaml",
			gwNN: types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy ExtProc different attachment points", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/extproc.yaml",
			outputFile: "traffic-policy/extproc.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "test",
			},
		})
	})

	t.Run("TrafficPolicy ExtProc Full Config", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/extproc-full-config.yaml",
			outputFile: "traffic-policy/extproc-full-config.yaml",
			gwNN: types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy ExtAuth deep merge", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/extauth-deep-merge.yaml",
			outputFile: "traffic-policy/extauth-deep-merge.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "test",
			},
		},
			func(s *apisettings.Settings) {
				s.PolicyMerge = `{"trafficPolicy":{"extAuth":"DeepMerge"}}`
			})
	})

	t.Run("TrafficPolicy ExtProc deep merge", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/extproc-deep-merge.yaml",
			outputFile: "traffic-policy/extproc-deep-merge.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "test",
			},
		},
			func(s *apisettings.Settings) {
				s.PolicyMerge = `{"trafficPolicy":{"extProc":"DeepMerge"}}`
			})
	})

	t.Run("TrafficPolicy Transformation deep merge", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/transformation-deep-merge.yaml",
			outputFile: "traffic-policy/transformation-deep-merge.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "test",
			},
		},
			func(s *apisettings.Settings) {
				s.PolicyMerge = `{"trafficPolicy":{"transformation":"DeepMerge"}}`
			})
	})

	t.Run("Load balancer with hash policies", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "loadbalancer/hash-policies.yaml",
			outputFile: "loadbalancer/hash-policies.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy with buffer attached to gateway", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/buffer-gateway.yaml",
			outputFile: "traffic-policy/buffer-gateway.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy with buffer attached to route", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/buffer-route.yaml",
			outputFile: "traffic-policy/buffer-route.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy with header modifiers attached to gateway", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/header-modifiers-gateway.yaml",
			outputFile: "traffic-policy/header-modifiers-gateway.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy with header modifiers attached to routes", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/header-modifiers-route.yaml",
			outputFile: "traffic-policy/header-modifiers-route.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy with header modifiers attached to routes listenerset and gateway", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/header-modifiers-all.yaml",
			outputFile: "traffic-policy/header-modifiers-all.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("tcp gateway with basic routing", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "tcp-routing/basic.yaml",
			outputFile: "tcp-routing/basic-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("tcproute with missing backend reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "tcp-routing/missing-backend.yaml",
			outputFile: "tcp-routing/missing-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("tcproute with invalid backend reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "tcp-routing/invalid-backend.yaml",
			outputFile: "tcp-routing/invalid-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("tcp gateway with multiple backend services", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "tcp-routing/multi-backend.yaml",
			outputFile: "tcp-routing/multi-backend-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-tcp-gateway",
			},
		})
	})

	t.Run("tls gateway with basic routing", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "tls-routing/basic.yaml",
			outputFile: "tls-routing/basic-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("tlsroute with missing backend reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "tls-routing/missing-backend.yaml",
			outputFile: "tls-routing/missing-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("tlsroute with invalid backend reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "tls-routing/invalid-backend.yaml",
			outputFile: "tls-routing/invalid-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("tls gateway with multiple backend services", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "tls-routing/multi-backend.yaml",
			outputFile: "tls-routing/multi-backend-proxy.yaml",
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

	t.Run("Basic service backend", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backends/basic.yaml",
			outputFile: "backends/basic.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("AWS Lambda backend", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backends/aws_lambda.yaml",
			outputFile: "backends/aws_lambda.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("DFP Backend with TLS", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "dfp/tls.yaml",
			outputFile: "dfp/tls.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("DFP Backend with simple", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "dfp/simple.yaml",
			outputFile: "dfp/simple.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Backend TLS Policy", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backendtlspolicy/tls.yaml",
			outputFile: "backendtlspolicy/tls.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Backend TLS Policy with SAN", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backendtlspolicy/tls-san.yaml",
			outputFile: "backendtlspolicy/tls-san.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Proxy with no routes", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "edge-cases/no_route.yaml",
			outputFile: "no_route.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Direct response", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "directresponse/manifest.yaml",
			outputFile: "directresponse.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("DirectResponse with missing reference reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "directresponse/missing-ref.yaml",
			outputFile: "directresponse/missing-ref.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("DirectResponse with overlapping filters reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "directresponse/overlapping-filters.yaml",
			outputFile: "directresponse/overlapping-filters.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("DirectResponse with invalid backendRef filter reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "directresponse/invalid-backendref-filter.yaml",
			outputFile: "directresponse/invalid-backendref-filter.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("HTTPRoutes with builtin timeout and retry", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "httproute-timeout-retry/builtin.yaml",
			outputFile: "httproute-timeout-retry-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy timeout and retry", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/timeout-retry.yaml",
			outputFile: "traffic-policy/timeout-retry.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("http gateway with session persistence (cookie)", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "session-persistence/cookie.yaml",
			outputFile: "session-persistence/cookie.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("http gateway with session persistence (header)", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "session-persistence/header.yaml",
			outputFile: "session-persistence/header.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("HTTPListenerPolicy with upgrades", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "https-listener-pol/upgrades.yaml",
			outputFile: "https-listener-pol/upgrades.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("HTTPListenerPolicy with healthCheck", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "httplistenerpolicy/route-and-pol.yaml",
			outputFile: "httplistenerpolicy/route-and-pol.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("HTTPListenerPolicy with idleTimeout", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "httplistenerpolicy/idle-timeout.yaml",
			outputFile: "httplistenerpolicy/idle-timeout.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("HTTPListenerPolicy with preserveHttp1HeaderCase", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "httplistenerpolicy/preserve-http1-header-case.yaml",
			outputFile: "httplistenerpolicy/preserve-http1-header-case.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("HTTPListenerPolicy with useRemoteAddress absent", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "httplistenerpolicy/use-remote-addr-absent.yaml",
			outputFile: "httplistenerpolicy/use-remote-addr-absent.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("HTTPListenerPolicy with useRemoteAddress true", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "httplistenerpolicy/use-remote-addr-true.yaml",
			outputFile: "httplistenerpolicy/use-remote-addr-true.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("HTTPListenerPolicy with useRemoteAddress false", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "httplistenerpolicy/use-remote-addr-false.yaml",
			outputFile: "httplistenerpolicy/use-remote-addr-false.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("HTTPListenerPolicy with acceptHttp10", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "httplistenerpolicy/accept-http10.yaml",
			outputFile: "httplistenerpolicy/accept-http10.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("HTTPListenerPolicy with defaultHostForHttp10", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "httplistenerpolicy/default-host-for-http10.yaml",
			outputFile: "httplistenerpolicy/default-host-for-http10.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("HTTPListenerPolicy with defaultHostForHttp10 and no acceptHttp10", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "httplistenerpolicy/default-host-for-http10-without-accept-http10.yaml",
			outputFile: "httplistenerpolicy/default-host-for-http10-without-accept-http10.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("HTTPListenerPolicy merging", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "httplistenerpolicy/merge.yaml",
			outputFile: "httplistenerpolicy/merge.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Service with appProtocol=kubernetes.io/h2c", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend-protocol/svc-h2c.yaml",
			outputFile: "backend-protocol/svc-h2c.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Service with appProtocol=kubernetes.io/ws", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend-protocol/svc-ws.yaml",
			outputFile: "backend-protocol/svc-ws.yaml",
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

	t.Run("Static Backend with appProtocol=kubernetes.io/h2c", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend-protocol/backend-h2c.yaml",
			outputFile: "backend-protocol/backend-h2c.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Static Backend with appProtocol=kubernetes.io/ws", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backend-protocol/backend-ws.yaml",
			outputFile: "backend-protocol/backend-ws.yaml",
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

	t.Run("Backend Config Policy with LB Config", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backendconfigpolicy/lb-config.yaml",
			outputFile: "backendconfigpolicy/lb-config.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Backend Config Policy with LB UseHostnameForHashing", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backendconfigpolicy/lb-usehostnameforhashing.yaml",
			outputFile: "backendconfigpolicy/lb-usehostnameforhashing.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Backend Config Policy with Health Check", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backendconfigpolicy/healthcheck.yaml",
			outputFile: "backendconfigpolicy/healthcheck.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Backend Config Policy with OutlierDetection", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backendconfigpolicy/outlierdetection.yaml",
			outputFile: "backendconfigpolicy/outlierdetection.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Backend Config Policy with Common HTTP Protocol - HTTP backend", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backendconfigpolicy/commonhttpprotocol-httpbackend.yaml",
			outputFile: "backendconfigpolicy/commonhttpprotocol-httpbackend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Backend Config Policy with Common HTTP Protocol - HTTP2 backend", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backendconfigpolicy/commonhttpprotocol-http2backend.yaml",
			outputFile: "backendconfigpolicy/commonhttpprotocol-http2backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Backend Config Policy with HTTP2 Protocol Options", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backendconfigpolicy/http2.yaml",
			outputFile: "backendconfigpolicy/http2.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Backend Config Policy with TLS and SAN verification", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backendconfigpolicy/tls-san.yaml",
			outputFile: "backendconfigpolicy/tls-san.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Backend Config Policy with TLS and insecure skip verify", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backendconfigpolicy/tls-insecureskipverify.yaml",
			outputFile: "backendconfigpolicy/tls-insecureskipverify.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Backend Config Policy with simple TLS", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backendconfigpolicy/simple-tls.yaml",
			outputFile: "backendconfigpolicy/simple-tls.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Backend Config Policy with system ca TLS", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backendconfigpolicy/tls-system-ca.yaml",
			outputFile: "backendconfigpolicy/tls-system-ca.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy with explicit generation", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/generation.yaml",
			outputFile: "traffic-policy/generation.yaml",
			gwNN: types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("RBAC Policy at route level", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "rbac/route-cel-rbac.yaml",
			outputFile: "rbac/route-cel-rbac.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("RBAC Policy at httproute level", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "rbac/httproute-cel-rbac.yaml",
			outputFile: "rbac/httproute-cel-rbac.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("RBAC Policy at gateway level", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "rbac/gateway-cel-rbac.yaml",
			outputFile: "rbac/gateway-cel-rbac.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("basic listener set", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "listener-sets/basic.yaml",
			outputFile: "listener-sets/basic.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("listener set and gateway with no allowed listeners", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "listener-sets/no-allowed-lis.yaml",
			outputFile: "listener-sets/no-allowed-lis.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("listener set accepted with rejected individual listener", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "listener-sets/accepted-ls-rejected-listener.yaml",
			outputFile: "listener-sets/accepted-ls-rejected-listener.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TrafficPolicy RateLimit Full Config", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/rate-limit-full-config.yaml",
			outputFile: "traffic-policy/rate-limit-full-config.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TLS listener with no routes", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "invalid-filter-chains/tls-listener-no-routes.yaml",
			outputFile: "invalid-filter-chains/tls-listener-no-routes.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TCP listener with no routes", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "invalid-filter-chains/tcp-listener-no-routes.yaml",
			outputFile: "invalid-filter-chains/tcp-listener-no-routes.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("HTTPS listener with invalid secret ref", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "invalid-filter-chains/https-listener-invalid-secret-ref.yaml",
			outputFile: "invalid-filter-chains/https-listener-invalid-secret-ref.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("HTTPS listener with invalid secret (missing private key)", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "invalid-filter-chains/https-listener-invalid-secret.yaml",
			outputFile: "invalid-filter-chains/https-listener-invalid-secret.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TLS mixed listeners - no routes and with routes", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "invalid-filter-chains/tls-mixed-listeners.yaml",
			outputFile: "invalid-filter-chains/tls-mixed-listeners.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TCP mixed listeners - no routes and with routes", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "invalid-filter-chains/tcp-mixed-listeners.yaml",
			outputFile: "invalid-filter-chains/tcp-mixed-listeners.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TLS same port listeners - both with no routes", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "invalid-filter-chains/tls-same-port-both-no-routes.yaml",
			outputFile: "invalid-filter-chains/tls-same-port-both-no-routes.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TLS same port listeners - mixed routes", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "invalid-filter-chains/tls-same-port-mixed-routes.yaml",
			outputFile: "invalid-filter-chains/tls-same-port-mixed-routes.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("TLS route with invalid backend", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "invalid-filter-chains/tls-route-invalid-backend.yaml",
			outputFile: "invalid-filter-chains/tls-route-invalid-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("HTTPS mixed listeners - invalid and valid secret refs", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "invalid-filter-chains/https-mixed-listeners.yaml",
			outputFile: "invalid-filter-chains/https-mixed-listeners.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Gateway empty with ListenerSet TCP listener no routes", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "invalid-filter-chains/gateway-empty-listenerset-tcp-no-routes.yaml",
			outputFile: "invalid-filter-chains/gateway-empty-listenerset-tcp-no-routes.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Gateway empty with ListenerSet TLS listener no routes", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "invalid-filter-chains/gateway-empty-listenerset-tls-no-routes.yaml",
			outputFile: "invalid-filter-chains/gateway-empty-listenerset-tls-no-routes.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Gateway empty with ListenerSet TLS mixed listeners", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "invalid-filter-chains/gateway-empty-listenerset-tls-mixed.yaml",
			outputFile: "invalid-filter-chains/gateway-empty-listenerset-tls-mixed.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Gateway HTTP listener with ListenerSet TCP listener no routes", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "invalid-filter-chains/gateway-http-listenerset-tcp-no-routes.yaml",
			outputFile: "invalid-filter-chains/gateway-http-listenerset-tcp-no-routes.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Gateway TCP listener no routes with ListenerSet HTTP listener", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "invalid-filter-chains/gateway-tcp-no-routes-listenerset-http.yaml",
			outputFile: "invalid-filter-chains/gateway-tcp-no-routes-listenerset-http.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})

	t.Run("Gateway with reserved port should be rejected", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "validation/gateway-reserved-port.yaml",
			outputFile: "validation/gateway-reserved-port.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "test",
			},
		})
	})

	t.Run("XListenerSet with reserved port should be rejected", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "validation/xlistenerset-reserved-port.yaml",
			outputFile: "validation/xlistenerset-reserved-port.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "test",
			},
		})
	})
}

func TestValidation(t *testing.T) {
	type validationTest struct {
		name      string
		category  string
		inputFile string
		minMode   apisettings.ValidationMode
	}

	tt := []validationTest{
		{
			name:      "Path Prefix Invalid",
			category:  "matcher",
			inputFile: "matcher-path-prefix-invalid.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		{
			name:      "Regex RE2 Unsupported",
			category:  "matcher",
			inputFile: "matcher-regex-re2-unsupported.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		{
			name:      "Path Regex Invalid",
			category:  "matcher",
			inputFile: "matcher-path-regex-invalid.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		{
			name:      "Header Regex Invalid",
			category:  "matcher",
			inputFile: "matcher-header-regex-invalid.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		{
			name:      "Extension Ref Invalid",
			category:  "policy",
			inputFile: "policy-extension-ref-invalid.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		{
			name:      "Gateway",
			category:  "attachment",
			inputFile: "gateway-invalid.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		{
			name:      "Gateway/Listener",
			category:  "attachment",
			inputFile: "gateway-listener-invalid.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		{
			name:      "XListenerSet",
			category:  "attachment",
			inputFile: "xlistenerset-invalid.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		{
			name:      "XListenerSet/Listener",
			category:  "attachment",
			inputFile: "xlistenerset-listener-invalid.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		{
			name:      "HTTPRoute",
			category:  "attachment",
			inputFile: "httproute-invalid.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		{
			name:      "Multi-Target",
			category:  "attachment",
			inputFile: "multi-target-invalid.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		{
			name:      "URLRewrite Invalid",
			category:  "builtin",
			inputFile: "urlrewrite-invalid.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		{
			name:      "Query Regex Invalid",
			category:  "matcher",
			inputFile: "matcher-query-regex-invalid.yaml",
			minMode:   apisettings.ValidationStrict,
		},
		{
			name:      "CSRF Regex Invalid",
			category:  "policy",
			inputFile: "policy-csrf-regex-invalid.yaml",
			minMode:   apisettings.ValidationStrict,
		},
		{
			name:      "AI Invalid Default Values",
			category:  "policy",
			inputFile: "policy-ai-default-value-invalid.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		// TODO(tim): Uncomment this test once #11995 is fixed.
		// {
		// 	name:      "Multiple Invalid Policies Conflict",
		// 	category:  "policy",
		// 	inputFile: "policy-multiple-invalid-conflict.yaml",
		// 	minMode:   apisettings.ValidationStandard,
		// },
		{
			name:      "ExtAuth Extension Ref Invalid",
			category:  "policy",
			inputFile: "policy-extauth-extension-ref-invalid.yaml",
			minMode:   apisettings.ValidationStrict,
		},
		{
			name:      "Transformation Body Template Invalid",
			category:  "policy",
			inputFile: "policy-transformation-body-template-invalid.yaml",
			minMode:   apisettings.ValidationStrict,
		},
		{
			name:      "Transformation Header Template Invalid",
			category:  "policy",
			inputFile: "policy-transformation-header-template-invalid.yaml",
			minMode:   apisettings.ValidationStrict,
		},
		{
			name:      "Transformation Malformed Template Invalid",
			category:  "policy",
			inputFile: "policy-transformation-malformed-template-invalid.yaml",
			minMode:   apisettings.ValidationStrict,
		},
		{
			name:      "Template Structure Invalid",
			category:  "policy",
			inputFile: "policy-template-structure-invalid.yaml",
			minMode:   apisettings.ValidationStrict,
		},
		{
			name:      "Header Template Invalid",
			category:  "policy",
			inputFile: "policy-header-template-invalid.yaml",
			minMode:   apisettings.ValidationStrict,
		},
		{
			name:      "Request Header Modifier Invalid",
			category:  "builtin",
			inputFile: "request-header-modifier-invalid.yaml",
			minMode:   apisettings.ValidationStrict,
		},
		{
			name:      "Response Header Modifier Invalid",
			category:  "builtin",
			inputFile: "response-header-modifier-invalid.yaml",
			minMode:   apisettings.ValidationStrict,
		},
		{
			name:      "Gateway/Listener/Merge",
			category:  "attachment",
			inputFile: "gateway-listener-merge-invalid.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		{
			name:      "BackendConfigPolicy Missing Secret",
			category:  "backendconfigpolicy",
			inputFile: "invalid-missing-secret.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		{
			name:      "BackendConfigPolicy Invalid Cipher Suites",
			category:  "backendconfigpolicy",
			inputFile: "invalid-cipher-suites.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		{
			name:      "BackendConfigPolicy Invalid TLS Files Non-existent",
			category:  "backendconfigpolicy",
			inputFile: "invalid-tlsfiles-nonexistent.yaml",
			minMode:   apisettings.ValidationStandard,
		},
		{
			name:      "BackendConfigPolicy Invalid Outlier Detection Zero Interval",
			category:  "backendconfigpolicy",
			inputFile: "invalid-outlier-detection-zero-interval.yaml",
			minMode:   apisettings.ValidationStandard,
		},
	}

	runTest := func(t *testing.T, test validationTest, mode apisettings.ValidationMode) {
		t.Helper()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dir := fsutils.MustGetThisDir()

		inputFile := filepath.Join(dir, "testutils/inputs/route-replacement", test.category, test.inputFile)
		baseOutputName := strings.Replace(test.inputFile, ".yaml", "-out.yaml", 1)
		modeDir := strings.ToLower(string(mode))
		outputFile := filepath.Join(dir, "testutils/outputs/route-replacement", modeDir, test.category, baseOutputName)

		gwNN := types.NamespacedName{
			Namespace: "gwtest",
			Name:      "example-gateway",
		}

		settingOpts := func(s *apisettings.Settings) {
			s.ValidationMode = mode
		}
		translatortest.TestTranslation(t, ctx, []string{inputFile}, outputFile, gwNN, settingOpts)
	}

	for _, mode := range []apisettings.ValidationMode{apisettings.ValidationStandard, apisettings.ValidationStrict} {
		t.Run(strings.ToLower(string(mode)), func(t *testing.T) {
			for _, test := range tt {
				// Skip tests that require a higher mode
				if test.minMode == apisettings.ValidationStrict && mode == apisettings.ValidationStandard {
					continue
				}
				t.Run(fmt.Sprintf("%s/%s", test.category, test.name), func(t *testing.T) {
					runTest(t, test, mode)
				})
			}
		})
	}
}

func TestRouteDelegation(t *testing.T) {
	test := func(t *testing.T, inputFile string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dir := fsutils.MustGetThisDir()

		inputFiles := []string{
			filepath.Join(dir, "testutils/inputs/delegation/common.yaml"),
			filepath.Join(dir, "testutils/inputs/delegation", inputFile),
		}
		outputFile := filepath.Join(dir, "testutils/outputs/delegation", inputFile)
		gwNN := types.NamespacedName{
			Namespace: "infra",
			Name:      "example-gateway",
		}
		translatortest.TestTranslation(t, ctx, inputFiles, outputFile, gwNN)
	}
	t.Run("Basic config", func(t *testing.T) {
		test(t, "basic.yaml")
	})

	t.Run("Child matches parent via parentRefs", func(t *testing.T) {
		test(t, "basic_parentref_match.yaml")
	})

	t.Run("Child doesn't match parent via parentRefs", func(t *testing.T) {
		test(t, "basic_parentref_mismatch.yaml")
	})

	t.Run("Children using parentRefs and inherit-parent-matcher", func(t *testing.T) {
		test(t, "inherit_parentref.yaml")
	})

	t.Run("Parent delegates to multiple chidren", func(t *testing.T) {
		test(t, "multiple_children.yaml")
	})

	t.Run("Child is invalid as it is delegatee and specifies hostnames", func(t *testing.T) {
		test(t, "basic_invalid_hostname.yaml")
	})

	t.Run("Multi-level recursive delegation", func(t *testing.T) {
		test(t, "recursive.yaml")
	})

	t.Run("Cyclic child route", func(t *testing.T) {
		test(t, "cyclic1.yaml")
	})

	t.Run("Multi-level cyclic child route", func(t *testing.T) {
		test(t, "cyclic2.yaml")
	})

	t.Run("Child rule matcher", func(t *testing.T) {
		test(t, "child_rule_matcher.yaml")
	})

	t.Run("Child with multiple parents", func(t *testing.T) {
		test(t, "multiple_parents.yaml")
	})

	t.Run("Child can be an invalid delegatee but valid standalone", func(t *testing.T) {
		test(t, "invalid_child_valid_standalone.yaml")
	})

	t.Run("Relative paths", func(t *testing.T) {
		test(t, "relative_paths.yaml")
	})

	t.Run("Nested absolute and relative path inheritance", func(t *testing.T) {
		test(t, "nested_absolute_relative.yaml")
	})

	t.Run("Child route matcher does not match parent", func(t *testing.T) {
		test(t, "discard_invalid_child_matches.yaml")
	})

	t.Run("Multi-level multiple parents delegation", func(t *testing.T) {
		test(t, "multi_level_multiple_parents.yaml")
	})

	t.Run("TrafficPolicy only on child", func(t *testing.T) {
		test(t, "traffic_policy.yaml")
	})

	t.Run("TrafficPolicy with policy applied to output route", func(t *testing.T) {
		test(t, "traffic_policy_route_policy.yaml")
	})

	t.Run("TrafficPolicy inheritance from parent", func(t *testing.T) {
		test(t, "traffic_policy_inheritance.yaml")
	})

	t.Run("TrafficPolicy ignore child override on conflict", func(t *testing.T) {
		test(t, "traffic_policy_inheritance_child_override_ignore.yaml")
	})

	t.Run("TrafficPolicy merge child override on no conflict", func(t *testing.T) {
		test(t, "traffic_policy_inheritance_child_override_ok.yaml")
	})

	t.Run("TrafficPolicy multi level inheritance with child override disabled", func(t *testing.T) {
		test(t, "traffic_policy_multi_level_inheritance_override_disabled.yaml")
	})

	t.Run("TrafficPolicy multi level inheritance with child override enabled", func(t *testing.T) {
		test(t, "traffic_policy_multi_level_inheritance_override_enabled.yaml")
	})

	t.Run("TrafficPolicy filter override merge", func(t *testing.T) {
		test(t, "traffic_policy_filter_override_merge.yaml")
	})

	t.Run("Built-in rule inheritance", func(t *testing.T) {
		test(t, "builtin_rule_inheritance.yaml")
	})

	t.Run("Label based delegation", func(t *testing.T) {
		test(t, "label_based.yaml")
	})

	t.Run("Unresolved child reference", func(t *testing.T) {
		test(t, "unresolved_ref.yaml")
	})

	t.Run("Policy deep merge", func(t *testing.T) {
		test(t, "policy_deep_merge.yaml")
	})
}

func TestDiscoveryNamespaceSelector(t *testing.T) {
	test := func(t *testing.T, cfgJSON string, inputFile string, outputFile string, errdesc string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dir := fsutils.MustGetThisDir()

		inputFiles := []string{
			filepath.Join(dir, "testutils/inputs/discovery-namespace-selector", inputFile),
		}
		expectedOutputFile := filepath.Join(dir, "testutils/outputs/discovery-namespace-selector", outputFile)
		gwNN := types.NamespacedName{
			Namespace: "infra",
			Name:      "example-gateway",
		}
		settingOpts := []translatortest.SettingsOpts{
			func(s *apisettings.Settings) {
				s.DiscoveryNamespaceSelectors = cfgJSON
			},
		}

		translatortest.TestTranslation(t, ctx, inputFiles, expectedOutputFile, gwNN, settingOpts...)
	}
	t.Run("Select all resources", func(t *testing.T) {
		test(t, `[
  {
    "matchExpressions": [
      {
        "key": "kubernetes.io/metadata.name",
        "operator": "In",
        "values": [
          "infra"
        ]
      }
    ]
  },
	{
		"matchLabels": {
			"app": "a"
		}
	}
]`, "base.yaml", "base_select_all.yaml", "")
	})

	t.Run("Select all resources; AND matchExpressions and matchLabels", func(t *testing.T) {
		test(t, `[
  {
    "matchExpressions": [
      {
        "key": "kubernetes.io/metadata.name",
        "operator": "In",
        "values": [
          "infra"
        ]
      }
    ]
  },
	{
    "matchExpressions": [
      {
        "key": "kubernetes.io/metadata.name",
        "operator": "In",
        "values": [
          "a"
        ]
      }
    ],
		"matchLabels": {
			"app": "a"
		}
	}
]`, "base.yaml", "base_select_all.yaml", "")
	})

	t.Run("Select only namespace infra", func(t *testing.T) {
		test(t, `[
  {
    "matchExpressions": [
      {
        "key": "kubernetes.io/metadata.name",
        "operator": "In",
        "values": [
          "infra"
        ]
      }
    ]
  }
]`, "base.yaml", "base_select_infra.yaml", "condition error for httproute: infra/example-route")
	})
}
