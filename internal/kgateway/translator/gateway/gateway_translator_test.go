package gateway_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	translatortest "github.com/kgateway-dev/kgateway/v2/test/translator"
)

type translatorTestCase struct {
	inputFile     string
	outputFile    string
	gwNN          types.NamespacedName
	assertReports translatortest.AssertReports
}

func TestBasic(t *testing.T) {
	test := func(t *testing.T, in translatorTestCase, settingOpts ...translatortest.SettingsOpts) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dir := fsutils.MustGetThisDir()

		inputFiles := []string{filepath.Join(dir, "testutils/inputs/", in.inputFile)}
		expectedProxyFile := filepath.Join(dir, "testutils/outputs/", in.outputFile)
		translatortest.TestTranslation(t, ctx, inputFiles, expectedProxyFile, in.gwNN, in.assertReports, settingOpts...)
	}

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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				a := assert.New(t)
				gateway := &gwv1.Gateway{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-gateway",
						Namespace: "default",
					},
					Spec: gwv1.GatewaySpec{
						Listeners: []gwv1.Listener{
							{
								Name: "https",
							},
							{
								Name: "https2",
							},
						},
					},
				}
				gatewayStatus := reportsMap.BuildGWStatus(context.Background(), *gateway)
				a.NotNil(gatewayStatus)
				a.Len(gatewayStatus.Listeners, 2)
				httpsListener := gatewayStatus.Listeners[0]
				resolvedRefs := meta.FindStatusCondition(httpsListener.Conditions, string(gwv1.ListenerConditionResolvedRefs))
				a.NotNil(resolvedRefs)
				a.Equal(metav1.ConditionFalse, resolvedRefs.Status)
				a.Equal(string(gwv1.ListenerReasonInvalidCertificateRef), resolvedRefs.Reason)
				a.Equal("Secret default/missing-cert not found.", resolvedRefs.Message)

				programmed := meta.FindStatusCondition(httpsListener.Conditions, string(gwv1.ListenerConditionProgrammed))
				a.NotNil(programmed)
				a.Equal(metav1.ConditionFalse, programmed.Status)
				a.Equal(string(gwv1.ListenerReasonInvalid), programmed.Reason)
				a.Equal("Secret default/missing-cert not found.", programmed.Message)

				https2Listener := gatewayStatus.Listeners[1]
				resolvedRefs = meta.FindStatusCondition(https2Listener.Conditions, string(gwv1.ListenerConditionResolvedRefs))
				a.NotNil(resolvedRefs)
				a.Equal(metav1.ConditionFalse, resolvedRefs.Status)
				a.Equal(string(gwv1.ListenerReasonInvalidCertificateRef), resolvedRefs.Reason)
				a.Equal("invalid TLS secret default/invalid-cert: tls: failed to find any PEM data in key input", resolvedRefs.Message)
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

	t.Run("http gateway with azure destination", func(t *testing.T) {
		t.Skip("TODO: enable this test when ready")
		test(t, translatorTestCase{
			inputFile:  "http-with-azure-destination",
			outputFile: "http-with-azure-destination-proxy.yaml",
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
		}, func(s *settings.Settings) {
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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				a := assert.New(t)
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				a.NotNil(routeStatus)
				a.Len(routeStatus.Parents, 1)
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				a.NotNil(resolvedRefs)
				a.Equal(string(gwv1.RouteReasonBackendNotFound), resolvedRefs.Reason)
				a.Equal(metav1.ConditionFalse, resolvedRefs.Status)
				a.Equal(`Service "example-svc" not found`, resolvedRefs.Message)
				a.Equal(int64(0), resolvedRefs.ObservedGeneration)
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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				a := assert.New(t)
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				a.NotNil(routeStatus)
				a.Len(routeStatus.Parents, 1)
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				a.NotNil(resolvedRefs)
				a.Equal(string(gwv1.RouteReasonInvalidKind), resolvedRefs.Reason)
				a.Equal(metav1.ConditionFalse, resolvedRefs.Status)
				a.Equal(`unknown backend kind`, resolvedRefs.Message)
				a.Equal(int64(0), resolvedRefs.ObservedGeneration)
			},
		})
	})

	t.Run("TrafficPolicy with ai invalided default values", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/ai-invalid-default-value.yaml",
			outputFile: "traffic-policy/ai-invalid-default-value.yaml",
			gwNN: types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				a := assert.New(t)
				// we expect the httproute to reflect an invalid status
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-route",
						Namespace: "infra",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				a.NotNil(routeStatus)
				a.Len(routeStatus.Parents, 1)

				acceptedCond := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				a.NotNil(acceptedCond)
				a.Equal(metav1.ConditionFalse, acceptedCond.Status)
				a.Equal(reporter.RouteRuleDroppedReason, acceptedCond.Reason)
				a.Equal(2, strings.Count(acceptedCond.Message, `field invalid_object contains invalid JSON string: "model":"gpt-4"}`),
					"Expected 'invalid_object' message to appear exactly twice")
				a.Equal(2, strings.Count(acceptedCond.Message, `field invalid_slices contains invalid JSON string: [1,2,3`),
					"Expected 'invalid_slices' message to appear exactly twice")
				a.Equal(int64(0), acceptedCond.ObservedGeneration)
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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				expectedPolicies := []reports.PolicyKey{
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "infra", Name: "policy-with-section-name"},
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "infra", Name: "policy-without-section-name"},
				}
				translatortest.AssertAcceptedPolicyStatus(t, reportsMap, expectedPolicies)
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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				expectedPolicies := []reports.PolicyKey{
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "infra", Name: "transform"},
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "infra", Name: "rate-limit"},
				}
				translatortest.AssertAcceptedPolicyStatus(t, reportsMap, expectedPolicies)
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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				expectedPolicies := []reports.PolicyKey{
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "infra", Name: "transform"},
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "infra", Name: "rate-limit"},
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "kgateway-system", Name: "global-policy"},
				}
				translatortest.AssertAcceptedPolicyStatus(t, reportsMap, expectedPolicies)
			},
		}, func(s *settings.Settings) {
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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				expectedPolicies := []reports.PolicyKey{
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "infra", Name: "extauth-for-gateway-section-name"},
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "infra", Name: "extauth-for-gateway"},
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "infra", Name: "extauth-for-http-route"},
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "infra", Name: "extauth-for-extension-ref"},
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "infra", Name: "extauth-for-route-section-name"},
				}
				translatortest.AssertAcceptedPolicyStatus(t, reportsMap, expectedPolicies)
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

	t.Run("Load balancer with hash policies, route level", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "loadbalancer/route.yaml",
			outputFile: "loadbalancer/route.yaml",
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

	t.Run("tcp gateway with basic routing", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "tcp-routing/basic.yaml",
			outputFile: "tcp-routing/basic-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				a := assert.New(t)
				route := &gwv1alpha2.TCPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tcp-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				a.NotNil(routeStatus)
				a.Len(routeStatus.Parents, 1)
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				a.NotNil(resolvedRefs)
				a.Equal(metav1.ConditionTrue, resolvedRefs.Status)
				a.Equal(string(gwv1.RouteReasonResolvedRefs), resolvedRefs.Reason)
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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				a := assert.New(t)
				route := &gwv1alpha2.TCPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tcp-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				a.NotNil(routeStatus)
				a.Len(routeStatus.Parents, 1)
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				a.NotNil(resolvedRefs)
				a.Equal(metav1.ConditionFalse, resolvedRefs.Status)
				a.Equal(`Service "example-tcp-svc" not found`, resolvedRefs.Message)
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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				a := assert.New(t)
				route := &gwv1alpha2.TCPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tcp-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				a.NotNil(routeStatus)
				a.Len(routeStatus.Parents, 1)
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				a.NotNil(resolvedRefs)
				a.Equal(metav1.ConditionFalse, resolvedRefs.Status)
				a.Equal("unknown backend kind", resolvedRefs.Message)
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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				a := assert.New(t)
				route := &gwv1alpha2.TLSRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tls-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				a.NotNil(routeStatus)
				a.Len(routeStatus.Parents, 1)
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				a.NotNil(resolvedRefs)
				a.Equal(metav1.ConditionTrue, resolvedRefs.Status)
				a.Equal(string(gwv1.RouteReasonResolvedRefs), resolvedRefs.Reason)
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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				a := assert.New(t)
				route := &gwv1alpha2.TLSRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tls-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				a.NotNil(routeStatus)
				a.Len(routeStatus.Parents, 1)
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				a.NotNil(resolvedRefs)
				a.Equal(metav1.ConditionFalse, resolvedRefs.Status)
				a.Equal("Service \"example-tls-svc\" not found", resolvedRefs.Message)
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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				a := assert.New(t)
				route := &gwv1alpha2.TLSRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tls-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				a.NotNil(routeStatus)
				a.Len(routeStatus.Parents, 1)
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				a.NotNil(resolvedRefs)
				a.Equal(metav1.ConditionFalse, resolvedRefs.Status)
				a.Equal("unknown backend kind", resolvedRefs.Message)
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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				a := assert.New(t)
				route := &gwv1.GRPCRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-grpc-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				a.NotNil(routeStatus)
				a.Len(routeStatus.Parents, 1)
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				a.NotNil(resolvedRefs)
				a.Equal(metav1.ConditionTrue, resolvedRefs.Status)
				a.Equal(string(gwv1.RouteReasonResolvedRefs), resolvedRefs.Reason)
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
				a := assert.New(t)
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				a.NotNil(routeStatus)
				a.Len(routeStatus.Parents, 1)
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				a.NotNil(resolvedRefs)
				a.Equal(metav1.ConditionFalse, resolvedRefs.Status)
				a.Equal(`Service "example-grpc-svc" not found`, resolvedRefs.Message)
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
				a := assert.New(t)

				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				a.NotNil(routeStatus)
				a.Len(routeStatus.Parents, 1)
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				a.NotNil(resolvedRefs)
				a.Equal(metav1.ConditionFalse, resolvedRefs.Status)
				a.Equal("unknown backend kind", resolvedRefs.Message)
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
			assertReports: translatortest.AssertRouteInvalidDropped(t, "example-route", "default", "no action specified"),
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
			assertReports: translatortest.AssertRouteInvalidDropped(t, "example-route", "default", "cannot be applied to route with existing action"),
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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-route",
						Namespace: "default",
					},
				}
				a := assert.New(t)

				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				a.NotNil(routeStatus)
				a.Len(routeStatus.Parents, 1)

				// DirectResponse attached to backendRef should be ignored, route should resolve normally
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				a.NotNil(resolvedRefs)
				a.Equal(metav1.ConditionTrue, resolvedRefs.Status)
				a.Equal(string(gwv1.RouteReasonResolvedRefs), resolvedRefs.Reason)
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

	t.Run("TrafficPolicy with explicit generation", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/generation.yaml",
			outputFile: "traffic-policy/generation.yaml",
			gwNN: types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				expectedPolicies := []reports.PolicyKey{
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "infra", Name: "test-policy"},
				}
				translatortest.AssertPolicyStatusWithGeneration(t, reportsMap, expectedPolicies, 42)
			},
		})
	})

	t.Run("listener sets", func(t *testing.T) {
		t.Skip("TODO: Add this once istio adds support for listener sets")
		test(t, translatorTestCase{
			inputFile:  "listener-sets/manifest.yaml",
			outputFile: "listener-sets-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})
}

func TestRouteReplacement(t *testing.T) {
	test := func(t *testing.T, in translatorTestCase, settingOpts ...translatortest.SettingsOpts) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dir := fsutils.MustGetThisDir()

		inputFiles := []string{filepath.Join(dir, "testutils/inputs/", in.inputFile)}
		expectedProxyFile := filepath.Join(dir, "testutils/outputs/", in.outputFile)
		translatortest.TestTranslation(t, ctx, inputFiles, expectedProxyFile, in.gwNN, in.assertReports, settingOpts...)
	}

	t.Run("Standard/Matcher/Path Prefix Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/standard/matcher-path-prefix-invalid.yaml",
			outputFile: "route-replacement/standard/matcher-path-prefix-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertRouteInvalidDropped(
				t,
				"invalid-traffic-policy-route",
				"gwtest",
				"the rewrite /new//../path is invalid",
			),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStandard
		})
	})

	t.Run("Standard/Matcher/Regex RE2 Unsupported", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/standard/matcher-regex-re2-unsupported.yaml",
			outputFile: "route-replacement/standard/matcher-regex-re2-unsupported-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStandard
		})
	})

	t.Run("Standard/Matcher/Path Regex Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/standard/matcher-path-regex-invalid.yaml",
			outputFile: "route-replacement/standard/matcher-path-regex-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStandard
		})
	})

	t.Run("Standard/Matcher/Path Regex Test", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/standard/matcher-path-regex-test.yaml",
			outputFile: "route-replacement/standard/matcher-path-regex-test-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStandard
		})
	})

	t.Run("Standard/Matcher/Header Regex Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/standard/matcher-header-regex-invalid.yaml",
			outputFile: "route-replacement/standard/matcher-header-regex-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStandard
		})
	})

	t.Run("Standard/Policy/Extension Ref Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/standard/policy-extension-ref-invalid.yaml",
			outputFile: "route-replacement/standard/policy-extension-ref-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertRouteInvalidDropped(
				t,
				"test-route",
				"gwtest",
				"gateway.kgateway.dev/TrafficPolicy/gwtest/my-tp-that-doesnt-exist: policy not found",
			),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStandard
		})
	})

	t.Run("Standard/Policy/Gateway Wide Invalid Attachment", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/standard/policy-gateway-wide-invalid.yaml",
			outputFile: "route-replacement/standard/policy-gateway-wide-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				translatortest.AssertAcceptedPolicyStatus(t, reportsMap, []reports.PolicyKey{
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "gwtest", Name: "gateway-level-invalid-policy"},
				})
			},
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStandard
		})
	})

	t.Run("Standard/Policy/Listener Wide Invalid Attachment", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/standard/policy-listener-wide-invalid.yaml",
			outputFile: "route-replacement/standard/policy-listener-wide-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				translatortest.AssertAcceptedPolicyStatus(t, reportsMap, []reports.PolicyKey{
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "gwtest", Name: "listener-level-invalid-policy"},
				})
			},
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStandard
		})
	})

	t.Run("Standard/Policy/HTTPRoute Wide Invalid Attachment", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/standard/policy-httproute-wide-invalid.yaml",
			outputFile: "route-replacement/standard/policy-httproute-wide-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				translatortest.AssertAcceptedPolicyStatus(t, reportsMap, []reports.PolicyKey{
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "gwtest", Name: "invalid-traffic-policy"},
				})
			},
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStandard
		})
	})

	t.Run("Standard/Built-in/URLRewrite Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/standard/builtin-filter-urlrewrite-invalid.yaml",
			outputFile: "route-replacement/standard/builtin-filter-urlrewrite-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertRouteInvalidDropped(
				t,
				"invalid-builtin-filter-route",
				"gwtest",
				"must only contain valid characters matching pattern",
			),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStandard
		})
	})

	t.Run("Strict/Policy/CSRF Regex Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/strict/policy-csrf-regex-invalid.yaml",
			outputFile: "route-replacement/strict/policy-csrf-regex-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertRouteInvalidDropped(
				t,
				"test-route",
				"gwtest",
				"invalid xds configuration",
			),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		})
	})

	t.Run("Strict/Policy/ExtAuth Extension Ref Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/strict/policy-extauth-extension-ref-invalid.yaml",
			outputFile: "route-replacement/strict/policy-extauth-extension-ref-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertRouteInvalidDropped(
				t,
				"invalid-traffic-policy-route",
				"gwtest",
				"extauthz: gateway extension gwtest/non-existent-auth-extension not found",
			),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		})
	})

	t.Run("Strict/Policy/Transformation Body Template Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/strict/policy-transformation-body-template-invalid.yaml",
			outputFile: "route-replacement/strict/policy-transformation-body-template-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertRouteInvalidDropped(
				t,
				"invalid-traffic-policy-route",
				"gwtest",
				"invalid xds configuration",
			),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		})
	})

	t.Run("Strict/Policy/Transformation Header Template Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/strict/policy-transformation-header-template-invalid.yaml",
			outputFile: "route-replacement/strict/policy-transformation-header-template-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertRouteInvalidDropped(
				t,
				"invalid-traffic-policy-route",
				"gwtest",
				"invalid xds configuration",
			),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		})
	})

	t.Run("Strict/Policy/Transformation Malformed Template Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/strict/policy-transformation-malformed-template-invalid.yaml",
			outputFile: "route-replacement/strict/policy-transformation-malformed-template-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertRouteInvalidDropped(
				t,
				"invalid-traffic-policy-route",
				"gwtest",
				"invalid xds configuration",
			),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		})
	})

	t.Run("Strict/Policy/Template Structure Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/strict/policy-template-structure-invalid.yaml",
			outputFile: "route-replacement/strict/policy-template-structure-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		})
	})

	t.Run("Strict/Policy/Gateway Wide Invalid Attachment", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/strict/policy-gateway-wide-invalid.yaml",
			outputFile: "route-replacement/strict/policy-gateway-wide-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertPolicyNotAccepted(t, "gateway-level-invalid-policy", "test-route"),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		})
	})

	t.Run("Strict/Policy/Listener Wide Invalid Attachment", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/strict/policy-listener-wide-invalid.yaml",
			outputFile: "route-replacement/strict/policy-listener-wide-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertPolicyNotAccepted(t, "listener-level-invalid-policy", "test-route"),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		})
	})

	t.Run("Strict/Policy/Header Template Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/strict/policy-header-template-invalid.yaml",
			outputFile: "route-replacement/strict/policy-header-template-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertRouteInvalidDropped(
				t,
				"invalid-header-template-route",
				"gwtest",
				"invalid xds configuration",
			),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		})
	})

	// Matcher Tests
	t.Run("Strict/Matcher/Header Regex Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/strict/matcher-header-regex-invalid.yaml",
			outputFile: "route-replacement/strict/matcher-header-regex-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertRouteInvalidDropped(
				t,
				"invalid-regex-route",
				"gwtest",
				"error initializing configuration '': missing ]: [invalid-regex",
			),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		})
	})

	t.Run("Strict/Matcher/Query Regex Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/strict/matcher-query-regex-invalid.yaml",
			outputFile: "route-replacement/strict/matcher-query-regex-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertRouteInvalidDropped(
				t,
				"invalid-route-matcher-query-params",
				"gwtest",
				"invalid matcher configuration",
			),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		})
	})

	t.Run("Strict/Matcher/Path Regex Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/strict/matcher-path-regex-invalid.yaml",
			outputFile: "route-replacement/strict/matcher-path-regex-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertRouteInvalidDropped(
				t,
				"invalid-regex-path-comprehensive-route",
				"gwtest",
				"bad repetition operator",
			),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		})
	})

	t.Run("Strict/Matcher/Regex RE2 Unsupported", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/strict/matcher-regex-re2-unsupported.yaml",
			outputFile: "route-replacement/strict/matcher-regex-re2-unsupported-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertRouteInvalidDropped(
				t,
				"invalid-rds-route",
				"gwtest",
				"invalid named capture group",
			),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		})
	})

	// Built-in Filter Tests
	t.Run("Strict/Built-in/Request Header Modifier Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/strict/builtin-filter-request-header-modifier-invalid.yaml",
			outputFile: "route-replacement/strict/builtin-filter-request-header-modifier-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertRouteInvalidDropped(
				t,
				"invalid-request-header-modifier-route",
				"gwtest",
				"invalid route configuration",
			),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		})
	})

	t.Run("Strict/Built-in/Response Header Modifier Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/strict/builtin-filter-response-header-modifier-invalid.yaml",
			outputFile: "route-replacement/strict/builtin-filter-response-header-modifier-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertRouteInvalidDropped(
				t,
				"invalid-response-header-modifier-route",
				"gwtest",
				"Incorrect configuration: %RESPONSE(Invalid-Variable",
			),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		})
	})

	t.Run("Strict/Built-in/URLRewrite Invalid", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "route-replacement/strict/builtin-filter-urlrewrite-invalid.yaml",
			outputFile: "route-replacement/strict/builtin-filter-urlrewrite-invalid-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: translatortest.AssertRouteInvalidDropped(
				t,
				"invalid-builtin-filter-route",
				"gwtest",
				"must only contain valid characters matching pattern",
			),
		}, func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		})
	})
}

func TestRouteDelegation(t *testing.T) {
	test := func(t *testing.T, inputFile string, wantHTTPRouteErrors map[types.NamespacedName]string) {
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
		assertReports := func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
			a := assert.New(t)
			if wantHTTPRouteErrors == nil {
				// validate status on all routes
				a.NoError(translatortest.GetHTTPRouteStatusError(reportsMap, nil))
			}
			for route, err := range wantHTTPRouteErrors {
				a.ErrorContains(translatortest.GetHTTPRouteStatusError(reportsMap, &route), err)
			}
		}

		translatortest.TestTranslation(t, ctx, inputFiles, outputFile, gwNN, assertReports)
	}
	t.Run("Basic config", func(t *testing.T) {
		test(t, "basic.yaml", nil)
	})

	t.Run("Child matches parent via parentRefs", func(t *testing.T) {
		test(t, "basic_parentref_match.yaml", nil)
	})

	t.Run("Child doesn't match parent via parentRefs", func(t *testing.T) {
		test(t, "basic_parentref_mismatch.yaml", map[types.NamespacedName]string{
			{Name: "example-route", Namespace: "infra"}: "BackendNotFound gateway.networking.k8s.io/HTTPRoute/a/*: unresolved reference",
		})
	})

	t.Run("Children using parentRefs and inherit-parent-matcher", func(t *testing.T) {
		test(t, "inherit_parentref.yaml", nil)
	})

	t.Run("Parent delegates to multiple chidren", func(t *testing.T) {
		test(t, "multiple_children.yaml", nil)
	})

	t.Run("Child is invalid as it is delegatee and specifies hostnames", func(t *testing.T) {
		test(t, "basic_invalid_hostname.yaml", map[types.NamespacedName]string{
			{Name: "route-a", Namespace: "a"}:           "spec.hostnames must be unset on a delegatee route as they are inherited from the parent route",
			{Name: "example-route", Namespace: "infra"}: "BackendNotFound gateway.networking.k8s.io/HTTPRoute/a/*: unresolved reference",
		})
	})

	t.Run("Multi-level recursive delegation", func(t *testing.T) {
		test(t, "recursive.yaml", nil)
	})

	t.Run("Cyclic child route", func(t *testing.T) {
		test(t, "cyclic1.yaml", map[types.NamespacedName]string{
			{Name: "route-a", Namespace: "a"}: "cyclic reference detected while evaluating delegated routes",
		})
	})

	t.Run("Multi-level cyclic child route", func(t *testing.T) {
		test(t, "cyclic2.yaml", map[types.NamespacedName]string{
			{Name: "route-a-b", Namespace: "a-b"}: "cyclic reference detected while evaluating delegated routes",
		})
	})

	t.Run("Child rule matcher", func(t *testing.T) {
		test(t, "child_rule_matcher.yaml", map[types.NamespacedName]string{
			{Name: "example-route", Namespace: "infra"}: "BackendNotFound gateway.networking.k8s.io/HTTPRoute/b/*: unresolved reference",
		})
	})

	t.Run("Child with multiple parents", func(t *testing.T) {
		test(t, "multiple_parents.yaml", map[types.NamespacedName]string{
			{Name: "foo-route", Namespace: "infra"}: "BackendNotFound gateway.networking.k8s.io/HTTPRoute/b/*: unresolved reference",
		})
	})

	t.Run("Child can be an invalid delegatee but valid standalone", func(t *testing.T) {
		test(t, "invalid_child_valid_standalone.yaml", map[types.NamespacedName]string{
			{Name: "route-a", Namespace: "a"}: "spec.hostnames must be unset on a delegatee route as they are inherited from the parent route",
		})
	})

	t.Run("Relative paths", func(t *testing.T) {
		test(t, "relative_paths.yaml", nil)
	})

	t.Run("Nested absolute and relative path inheritance", func(t *testing.T) {
		test(t, "nested_absolute_relative.yaml", nil)
	})

	t.Run("Child route matcher does not match parent", func(t *testing.T) {
		test(t, "discard_invalid_child_matches.yaml", nil)
	})

	t.Run("Multi-level multiple parents delegation", func(t *testing.T) {
		test(t, "multi_level_multiple_parents.yaml", nil)
	})

	t.Run("TrafficPolicy only on child", func(t *testing.T) {
		test(t, "traffic_policy.yaml", nil)
	})

	t.Run("TrafficPolicy with policy applied to output route", func(t *testing.T) {
		test(t, "traffic_policy_route_policy.yaml", nil)
	})

	t.Run("TrafficPolicy inheritance from parent", func(t *testing.T) {
		test(t, "traffic_policy_inheritance.yaml", nil)
	})

	t.Run("TrafficPolicy ignore child override on conflict", func(t *testing.T) {
		test(t, "traffic_policy_inheritance_child_override_ignore.yaml", nil)
	})

	t.Run("TrafficPolicy merge child override on no conflict", func(t *testing.T) {
		test(t, "traffic_policy_inheritance_child_override_ok.yaml", nil)
	})

	t.Run("TrafficPolicy multi level inheritance with child override disabled", func(t *testing.T) {
		test(t, "traffic_policy_multi_level_inheritance_override_disabled.yaml", nil)
	})

	t.Run("TrafficPolicy multi level inheritance with child override enabled", func(t *testing.T) {
		test(t, "traffic_policy_multi_level_inheritance_override_enabled.yaml", nil)
	})

	t.Run("TrafficPolicy filter override merge", func(t *testing.T) {
		test(t, "traffic_policy_filter_override_merge.yaml", nil)
	})

	t.Run("Built-in rule inheritance", func(t *testing.T) {
		test(t, "builtin_rule_inheritance.yaml", nil)
	})

	t.Run("Label based delegation", func(t *testing.T) {
		test(t, "label_based.yaml", nil)
	})

	t.Run("Unresolved child reference", func(t *testing.T) {
		test(t, "unresolved_ref.yaml", map[types.NamespacedName]string{
			{Name: "example-route", Namespace: "infra"}: "BackendNotFound gateway.networking.k8s.io/HTTPRoute/b/*: unresolved reference",
			{Name: "route-a", Namespace: "a"}:           "BackendNotFound gateway.networking.k8s.io/HTTPRoute/a-c/: unresolved reference",
		})
	})

	t.Run("Policy deep merge", func(t *testing.T) {
		test(t, "policy_deep_merge.yaml", nil)
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
		assertReports := func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
			a := assert.New(t)
			if errdesc == "" {
				a.NoError(translatortest.AreReportsSuccess(gwNN, reportsMap))
			} else {
				a.ErrorContains(translatortest.AreReportsSuccess(gwNN, reportsMap), errdesc)
			}
		}
		settingOpts := []translatortest.SettingsOpts{
			func(s *settings.Settings) {
				s.DiscoveryNamespaceSelectors = cfgJSON
			},
		}

		translatortest.TestTranslation(t, ctx, inputFiles, expectedOutputFile, gwNN, assertReports, settingOpts...)
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
