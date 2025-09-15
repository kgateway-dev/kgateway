package gateway_test

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
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
				gatewayStatus := reportsMap.BuildGWStatus(context.Background(), *gateway, nil)
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

	t.Run("httproute with backend port error reports correctly", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "backends/backend-ref-port-error.yaml",
			outputFile: "backends/backend-ref-port-error.yaml",
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
				a.Equal((&krtcollections.BackendPortNotAllowedError{BackendName: "example-backend"}).Error(), resolvedRefs.Message)
				a.Equal(int64(0), resolvedRefs.ObservedGeneration)
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

	t.Run("TrafficPolicy ExtAuth Simple", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/extauth-simple.yaml",
			outputFile: "traffic-policy/extauth-simple.yaml",
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

	t.Run("TrafficPolicy ExtProc Simple", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/extproc-simple.yaml",
			outputFile: "traffic-policy/extproc-simple.yaml",
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
			func(s *settings.Settings) {
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
			func(s *settings.Settings) {
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
			func(s *settings.Settings) {
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
			assertReports: translatortest.AssertRouteInvalid(t, "example-route", "default", reporter.RouteRuleReplacedReason, "no action specified"),
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
			assertReports: translatortest.AssertRouteInvalid(t, "example-route", "default", reporter.RouteRuleReplacedReason, "cannot be applied to route with existing action"),
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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				expectedPolicies := []reports.PolicyKey{
					{Group: "gateway.kgateway.dev", Kind: "TrafficPolicy", Namespace: "infra", Name: "test-policy"},
				}
				translatortest.AssertPolicyStatusWithGeneration(t, reportsMap, expectedPolicies, 42)
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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				expectedLS := gwxv1a1.XListenerSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-listenerset",
						Namespace: "default",
					},
				}
				expectedAccepted := metav1.Condition{
					Type:    string(gwxv1a1.ListenerSetConditionAccepted),
					Status:  metav1.ConditionFalse,
					Reason:  string(gwxv1a1.ListenerSetReasonNotAllowed),
					Message: "Unable to attach to parent, gateway has not enabled allowedListeners",
				}
				translatortest.AssertListenerSetCondition(t, reportsMap, expectedLS, expectedAccepted)
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
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				// The ListenerSet should be accepted since the Gateway allows listener sets
				expectedLS := gwxv1a1.XListenerSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-listenerset",
						Namespace: "default",
					},
				}
				expectedAccepted := metav1.Condition{
					Type:    string(gwxv1a1.ListenerSetConditionAccepted),
					Status:  metav1.ConditionTrue,
					Reason:  string(gwxv1a1.ListenerSetReasonAccepted),
					Message: "ListenerSet is accepted",
				}
				translatortest.AssertListenerSetCondition(t, reportsMap, expectedLS, expectedAccepted)
				// note: we can't assert on individual listener set status due to how
				// BuildListenerSetStatus() works where it skips rejected listeners
				// and only returns the status for accepted listeners.
			},
		})
	})

	t.Run("TrafficPolicy RateLimit Simple", func(t *testing.T) {
		test(t, translatorTestCase{
			inputFile:  "traffic-policy/rate-limit.yaml",
			outputFile: "traffic-policy/rate-limit.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		})
	})
}

func TestRouteReplacement(t *testing.T) {
	type routeReplacementTest struct {
		name           string
		category       string
		inputFile      string
		minMode        settings.RouteReplacementMode
		assertStandard func(t *testing.T) translatortest.AssertReports
		assertStrict   func(t *testing.T) translatortest.AssertReports
	}

	tt := []routeReplacementTest{
		{
			name:      "Path Prefix Invalid",
			category:  "matcher",
			inputFile: "matcher-path-prefix-invalid.yaml",
			minMode:   settings.RouteReplacementStandard,
			assertStandard: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"invalid-traffic-policy-route",
					"gwtest",
					reporter.RouteRuleReplacedReason,
					"the rewrite /new//../path is invalid",
				)
			},
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"invalid-traffic-policy-route",
					"gwtest",
					reporter.RouteRuleReplacedReason,
					"the rewrite /new//../path is invalid",
				)
			},
		},
		{
			name:      "Regex RE2 Unsupported",
			category:  "matcher",
			inputFile: "matcher-regex-re2-unsupported.yaml",
			minMode:   settings.RouteReplacementStandard,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"invalid-rds-route",
					"gwtest",
					reporter.RouteRuleDroppedReason,
					"invalid named capture group",
				)
			},
		},
		{
			name:      "Path Regex Invalid",
			category:  "matcher",
			inputFile: "matcher-path-regex-invalid.yaml",
			minMode:   settings.RouteReplacementStandard,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"invalid-regex-path-route",
					"gwtest",
					reporter.RouteRuleDroppedReason,
					"missing ]",
				)
			},
		},
		{
			name:      "Header Regex Invalid",
			category:  "matcher",
			inputFile: "matcher-header-regex-invalid.yaml",
			minMode:   settings.RouteReplacementStandard,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"invalid-regex-route",
					"gwtest",
					reporter.RouteRuleDroppedReason,
					"error initializing configuration '': missing ]: [invalid-regex",
				)
			},
		},
		{
			name:      "Extension Ref Invalid",
			category:  "policy",
			inputFile: "policy-extension-ref-invalid.yaml",
			minMode:   settings.RouteReplacementStandard,
			assertStandard: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"test-route",
					"gwtest",
					reporter.RouteRuleReplacedReason,
					"gateway.kgateway.dev/TrafficPolicy/gwtest/my-tp-that-doesnt-exist: policy not found",
				)
			},
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"test-route",
					"gwtest",
					reporter.RouteRuleReplacedReason,
					"gateway.kgateway.dev/TrafficPolicy/gwtest/my-tp-that-doesnt-exist: policy not found",
				)
			},
		},
		{
			name:      "Gateway",
			category:  "attachment",
			inputFile: "gateway-invalid.yaml",
			minMode:   settings.RouteReplacementStandard,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertPolicyNotAccepted(t, "gateway-level-invalid-policy", "test-route")
			},
		},
		{
			name:      "Gateway/Listener",
			category:  "attachment",
			inputFile: "gateway-listener-invalid.yaml",
			minMode:   settings.RouteReplacementStandard,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertPolicyNotAccepted(t, "listener-level-invalid-policy", "test-route")
			},
		},
		{
			name:      "XListenerSet",
			category:  "attachment",
			inputFile: "xlistenerset-invalid.yaml",
			minMode:   settings.RouteReplacementStandard,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertPolicyNotAccepted(t, "invalid-traffic-policy", "test-route")
			},
		},
		{
			name:      "XListenerSet/Listener",
			category:  "attachment",
			inputFile: "xlistenerset-listener-invalid.yaml",
			minMode:   settings.RouteReplacementStandard,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertPolicyNotAccepted(t, "invalid-traffic-policy", "test-route")
			},
		},
		{
			name:      "HTTPRoute",
			category:  "attachment",
			inputFile: "httproute-invalid.yaml",
			minMode:   settings.RouteReplacementStandard,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
					translatortest.AssertPolicyNotAccepted(t, "invalid-traffic-policy", "test-route")
				}
			},
		},
		{
			name:      "Multi-Target",
			category:  "attachment",
			inputFile: "multi-target-invalid.yaml",
			minMode:   settings.RouteReplacementStandard,
			assertStandard: func(t *testing.T) translatortest.AssertReports {
				return func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
					translatortest.AssertPolicyNotAccepted(t, "xlistenerset-wide-invalid-policy", "test-route")
				}
			},
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
					// just assert on the gateway-wide policy to appease translator test.go logic
					// that verifies all status' are in Accepted=true state when assertReports is nil.
					// we already have coverage for output status written to golden file.
					r := require.New(t)
					policy := reports.PolicyKey{
						Group:     "gateway.kgateway.dev",
						Kind:      "TrafficPolicy",
						Namespace: "gwtest",
						Name:      "gateway-wide-invalid-policy",
					}
					policyStatus := reportsMap.BuildPolicyStatus(context.Background(), policy, wellknown.DefaultGatewayControllerName, gwv1alpha2.PolicyStatus{})
					r.NotNil(policyStatus, "Policy status should not be nil")
					r.Len(policyStatus.Ancestors, 2, "Policy should have two ancestors")

					acceptedCondition := meta.FindStatusCondition(policyStatus.Ancestors[0].Conditions, string(v1alpha1.PolicyConditionAccepted))
					r.NotNil(acceptedCondition, "Accepted condition should not be nil")
					r.Equal(metav1.ConditionFalse, acceptedCondition.Status, "Policy should have Accepted=false")
					r.Equal(string(v1alpha1.PolicyReasonInvalid), acceptedCondition.Reason, "Policy should have Invalid reason")
					r.Contains(acceptedCondition.Message, "invalid xds configuration", "Policy message should contain validation error")
				}
			},
		},
		{
			name:      "URLRewrite Invalid",
			category:  "builtin",
			inputFile: "urlrewrite-invalid.yaml",
			minMode:   settings.RouteReplacementStandard,
			assertStandard: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"invalid-builtin-filter-route",
					"gwtest",
					reporter.RouteRuleReplacedReason,
					"must only contain valid characters matching pattern",
				)
			},
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"invalid-builtin-filter-route",
					"gwtest",
					reporter.RouteRuleReplacedReason,
					"must only contain valid characters matching pattern",
				)
			},
		},
		{
			name:      "Query Regex Invalid",
			category:  "matcher",
			inputFile: "matcher-query-regex-invalid.yaml",
			minMode:   settings.RouteReplacementStrict,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"invalid-route-matcher-query-params",
					"gwtest",
					reporter.RouteRuleDroppedReason,
					"invalid matcher configuration",
				)
			},
		},
		{
			name:      "CSRF Regex Invalid",
			category:  "policy",
			inputFile: "policy-csrf-regex-invalid.yaml",
			minMode:   settings.RouteReplacementStrict,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"test-route",
					"gwtest",
					reporter.RouteRuleReplacedReason,
					"invalid xds configuration",
				)
			},
		},
		{
			name:      "AI Invalid Default Values",
			category:  "policy",
			inputFile: "policy-ai-default-value-invalid.yaml",
			minMode:   settings.RouteReplacementStandard,
			assertStandard: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"example-route",
					"gwtest",
					reporter.RouteRuleReplacedReason,
					`field invalid_object contains invalid JSON string: "model":"gpt-4"`,
				)
			},
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"example-route",
					"gwtest",
					reporter.RouteRuleReplacedReason,
					`field invalid_object contains invalid JSON string: "model":"gpt-4"`,
				)
			},
		},
		// TODO(tim): Uncomment this test once #11995 is fixed.
		// {
		// 	name:      "Multiple Invalid Policies Conflict",
		// 	category:  "policy",
		// 	inputFile: "policy-multiple-invalid-conflict.yaml",
		// 	minMode:   settings.RouteReplacementStandard,
		// 	assertStandard: func(t *testing.T) translatortest.AssertReports {
		// 		return translatortest.AssertRouteInvalid(
		// 			t,
		// 			"conflict-route",
		// 			"gwtest",
		// 			reporter.RouteRuleReplacedReason,
		// 			"field config contains invalid JSON string",
		// 			"invalid template",
		// 		)
		// 	},
		// 	assertStrict: func(t *testing.T) translatortest.AssertReports {
		// 		return translatortest.AssertRouteInvalid(
		// 			t,
		// 			"conflict-route",
		// 			"gwtest",
		// 			reporter.RouteRuleReplacedReason,
		// 			"field config contains invalid JSON string",
		// 			"invalid template",
		// 		)
		// 	},
		// },
		{
			name:      "ExtAuth Extension Ref Invalid",
			category:  "policy",
			inputFile: "policy-extauth-extension-ref-invalid.yaml",
			minMode:   settings.RouteReplacementStrict,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"invalid-traffic-policy-route",
					"gwtest",
					reporter.RouteRuleReplacedReason,
					"extauthz: gateway extension gwtest/non-existent-auth-extension not found",
				)
			},
		},
		{
			name:      "Transformation Body Template Invalid",
			category:  "policy",
			inputFile: "policy-transformation-body-template-invalid.yaml",
			minMode:   settings.RouteReplacementStrict,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"invalid-traffic-policy-route",
					"gwtest",
					reporter.RouteRuleReplacedReason,
					"invalid xds configuration",
				)
			},
		},
		{
			name:      "Transformation Header Template Invalid",
			category:  "policy",
			inputFile: "policy-transformation-header-template-invalid.yaml",
			minMode:   settings.RouteReplacementStrict,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"invalid-traffic-policy-route",
					"gwtest",
					reporter.RouteRuleReplacedReason,
					"invalid xds configuration",
				)
			},
		},
		{
			name:      "Transformation Malformed Template Invalid",
			category:  "policy",
			inputFile: "policy-transformation-malformed-template-invalid.yaml",
			minMode:   settings.RouteReplacementStrict,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"invalid-traffic-policy-route",
					"gwtest",
					reporter.RouteRuleReplacedReason,
					"invalid xds configuration",
				)
			},
		},
		{
			name:      "Template Structure Invalid",
			category:  "policy",
			inputFile: "policy-template-structure-invalid.yaml",
			minMode:   settings.RouteReplacementStrict,
		},
		{
			name:      "Header Template Invalid",
			category:  "policy",
			inputFile: "policy-header-template-invalid.yaml",
			minMode:   settings.RouteReplacementStrict,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"invalid-header-template-route",
					"gwtest",
					reporter.RouteRuleReplacedReason,
					"invalid xds configuration",
				)
			},
		},
		{
			name:      "Request Header Modifier Invalid",
			category:  "builtin",
			inputFile: "request-header-modifier-invalid.yaml",
			minMode:   settings.RouteReplacementStrict,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"invalid-request-header-modifier-route",
					"gwtest",
					reporter.RouteRuleReplacedReason,
					"invalid route configuration",
				)
			},
		},
		{
			name:      "Response Header Modifier Invalid",
			category:  "builtin",
			inputFile: "response-header-modifier-invalid.yaml",
			minMode:   settings.RouteReplacementStrict,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertRouteInvalid(
					t,
					"invalid-response-header-modifier-route",
					"gwtest",
					reporter.RouteRuleReplacedReason,
					"Incorrect configuration: %RESPONSE(Invalid-Variable",
				)
			},
		},
		{
			name:      "Gateway/Listener/Merge",
			category:  "attachment",
			inputFile: "gateway-listener-merge-invalid.yaml",
			minMode:   settings.RouteReplacementStandard,
			assertStrict: func(t *testing.T) translatortest.AssertReports {
				return translatortest.AssertPolicyNotAccepted(t, "listener-merge-invalid-policy", "")
			},
		},
	}

	runTest := func(t *testing.T, test routeReplacementTest, mode settings.RouteReplacementMode) {
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

		var assertReports translatortest.AssertReports
		switch mode {
		case settings.RouteReplacementStandard:
			if test.assertStandard != nil {
				assertReports = test.assertStandard(t)
			}
		case settings.RouteReplacementStrict:
			if test.assertStrict != nil {
				assertReports = test.assertStrict(t)
			}
		}
		settingOpts := func(s *settings.Settings) {
			s.RouteReplacementMode = mode
		}
		translatortest.TestTranslation(t, ctx, []string{inputFile}, outputFile, gwNN, assertReports, settingOpts)
	}

	for _, mode := range []settings.RouteReplacementMode{settings.RouteReplacementStandard, settings.RouteReplacementStrict} {
		t.Run(strings.ToLower(string(mode)), func(t *testing.T) {
			for _, test := range tt {
				// Skip tests that require a higher mode
				if test.minMode == settings.RouteReplacementStrict && mode == settings.RouteReplacementStandard {
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
