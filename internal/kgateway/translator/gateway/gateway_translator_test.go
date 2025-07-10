package gateway_test

import (
	"context"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

var _ = DescribeTable("Basic GatewayTranslator Tests",
	func(in translatorTestCase, settingOpts ...translatortest.SettingsOpts) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dir := fsutils.MustGetThisDir()

		inputFiles := []string{filepath.Join(dir, "testutils/inputs/", in.inputFile)}
		expectedProxyFile := filepath.Join(dir, "testutils/outputs/", in.outputFile)
		translatortest.TestTranslation(GinkgoT(), ctx, inputFiles, expectedProxyFile, in.gwNN, in.assertReports, settingOpts...)
	},
	Entry(
		"http gateway with per connection buffer limit",
		translatorTestCase{
			inputFile:  "gateway-per-conn-buf-lim/gateway.yaml",
			outputFile: "gateway-per-conn-buf-lim/proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		}),
	Entry(
		"http gateway with basic routing",
		translatorTestCase{
			inputFile:  "http-routing",
			outputFile: "http-routing-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		}),
	Entry(
		"http gateway with custom class",
		translatorTestCase{
			inputFile:  "custom-gateway-class",
			outputFile: "custom-gateway-class.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		}),
	Entry(
		"https gateway with basic routing",
		translatorTestCase{
			inputFile:  "https-routing",
			outputFile: "https-routing-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		}),
	Entry(
		"http gateway with multiple listeners on the same port",
		translatorTestCase{
			inputFile:  "multiple-listeners-http-routing",
			outputFile: "multiple-listeners-http-routing-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "http",
			},
		}),
	Entry(
		"https gateway with multiple listeners on the same port",
		translatorTestCase{
			inputFile:  "multiple-listeners-https-routing",
			outputFile: "multiple-listeners-https-routing-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "http",
			},
		}),
	Entry(
		"http gateway with multiple routing rules and HeaderModifier filter",
		translatorTestCase{
			inputFile:  "http-with-header-modifier",
			outputFile: "http-with-header-modifier-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "gw",
			},
		}),
	XEntry(
		"http gateway with azure destination",
		translatorTestCase{
			inputFile:  "http-with-azure-destination",
			outputFile: "http-with-azure-destination-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "gw",
			},
		}),
	Entry(
		"Gateway API route sorting",
		translatorTestCase{
			inputFile:  "route-sort.yaml",
			outputFile: "route-sort.yaml",
			gwNN: types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
		}),
	Entry(
		"weight based route sorting",
		translatorTestCase{
			inputFile:  "route-sort-weighted.yaml",
			outputFile: "route-sort-weighted.yaml",
			gwNN: types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
		},
		func(s *settings.Settings) {
			s.WeightedRoutePrecedence = true
		},
	),
	Entry(
		"httproute with missing backend reports correctly",
		translatorTestCase{
			inputFile:  "http-routing-missing-backend",
			outputFile: "http-routing-missing-backend.yaml",
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
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Reason).To(Equal(string(gwv1.RouteReasonBackendNotFound)))
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
				Expect(resolvedRefs.Message).To(Equal(`Service "example-svc" not found`))
				Expect(resolvedRefs.ObservedGeneration).To(Equal(int64(0)))
			},
		}),
	Entry(
		"httproute with invalid backend reports correctly",
		translatorTestCase{
			inputFile:  "http-routing-invalid-backend",
			outputFile: "http-routing-invalid-backend.yaml",
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
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Reason).To(Equal(string(gwv1.RouteReasonInvalidKind)))
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
				Expect(resolvedRefs.Message).To(Equal(`unknown backend kind`))
				Expect(resolvedRefs.ObservedGeneration).To(Equal(int64(0)))
			},
		}),
	Entry(
		"TrafficPolicy merging",
		translatorTestCase{
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
				assertAcceptedPolicyStatus(reportsMap, expectedPolicies)
			},
		}),
	Entry(
		"TrafficPolicy with targetSelectors",
		translatorTestCase{
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
				assertAcceptedPolicyStatus(reportsMap, expectedPolicies)
			},
		}),
	Entry(
		"TrafficPolicy with targetSelectors and global policy attachment",
		translatorTestCase{
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
				assertAcceptedPolicyStatus(reportsMap, expectedPolicies)
			},
		},
		func(s *settings.Settings) {
			s.GlobalPolicyNamespace = "kgateway-system"
		},
	),
	Entry(
		"TrafficPolicy edge cases",
		translatorTestCase{
			inputFile:  "traffic-policy/extauth.yaml",
			outputFile: "traffic-policy/extauth.yaml",
			gwNN: types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
		}),
	Entry(
		"TrafficPolicy with buffer attached to gateway",
		translatorTestCase{
			inputFile:  "traffic-policy/buffer-gateway.yaml",
			outputFile: "traffic-policy/buffer-gateway.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		}),
	Entry(
		"TrafficPolicy with buffer attached to route",
		translatorTestCase{
			inputFile:  "traffic-policy/buffer-route.yaml",
			outputFile: "traffic-policy/buffer-route.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		}),
	Entry(
		"tcp gateway with basic routing",
		translatorTestCase{
			inputFile:  "tcp-routing/basic.yaml",
			outputFile: "tcp-routing/basic-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1alpha2.TCPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tcp-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionTrue))
				Expect(resolvedRefs.Reason).To(Equal(string(gwv1.RouteReasonResolvedRefs)))
			},
		}),
	Entry(
		"tcproute with missing backend reports correctly",
		translatorTestCase{
			inputFile:  "tcp-routing/missing-backend.yaml",
			outputFile: "tcp-routing/missing-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1alpha2.TCPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tcp-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
				Expect(resolvedRefs.Message).To(Equal("Service \"example-tcp-svc\" not found"))
			},
		}),
	Entry(
		"tcproute with invalid backend reports correctly",
		translatorTestCase{
			inputFile:  "tcp-routing/invalid-backend.yaml",
			outputFile: "tcp-routing/invalid-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1alpha2.TCPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tcp-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
				Expect(resolvedRefs.Message).To(Equal("unknown backend kind"))
			},
		}),
	Entry(
		"tcp gateway with multiple backend services",
		translatorTestCase{
			inputFile:  "tcp-routing/multi-backend.yaml",
			outputFile: "tcp-routing/multi-backend-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-tcp-gateway",
			},
		}),
	Entry(
		"tls gateway with basic routing",
		translatorTestCase{
			inputFile:  "tls-routing/basic.yaml",
			outputFile: "tls-routing/basic-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1alpha2.TLSRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tls-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionTrue))
				Expect(resolvedRefs.Reason).To(Equal(string(gwv1.RouteReasonResolvedRefs)))
			},
		}),
	Entry(
		"tlsroute with missing backend reports correctly",
		translatorTestCase{
			inputFile:  "tls-routing/missing-backend.yaml",
			outputFile: "tls-routing/missing-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1alpha2.TLSRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tls-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
				Expect(resolvedRefs.Message).To(Equal("Service \"example-tls-svc\" not found"))
			},
		}),
	Entry(
		"tlsroute with invalid backend reports correctly",
		translatorTestCase{
			inputFile:  "tls-routing/invalid-backend.yaml",
			outputFile: "tls-routing/invalid-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1alpha2.TLSRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tls-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
				Expect(resolvedRefs.Message).To(Equal("unknown backend kind"))
			},
		}),
	Entry(
		"tls gateway with multiple backend services",
		translatorTestCase{
			inputFile:  "tls-routing/multi-backend.yaml",
			outputFile: "tls-routing/multi-backend-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		}),
	Entry(
		"grpc gateway with basic routing",
		translatorTestCase{
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
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionTrue))
				Expect(resolvedRefs.Reason).To(Equal(string(gwv1.RouteReasonResolvedRefs)))
			},
		}),
	Entry(
		"grpcroute with missing backend reports correctly",
		translatorTestCase{
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
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
				Expect(resolvedRefs.Message).To(Equal(`Service "example-grpc-svc" not found`))
			},
		}),
	Entry(
		"grpcroute with invalid backend reports correctly",
		translatorTestCase{
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
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Status).To(Equal(metav1.ConditionFalse))
				Expect(resolvedRefs.Message).To(Equal("unknown backend kind"))
			},
		}),
	Entry(
		"grpc gateway with multiple backend services",
		translatorTestCase{
			inputFile:  "grpc-routing/multi-backend.yaml",
			outputFile: "grpc-routing/multi-backend-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-grpc-gateway",
			},
		}),
	Entry("Basic service backend", translatorTestCase{
		inputFile:  "backends/basic.yaml",
		outputFile: "backends/basic.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("AWS Lambda backend", translatorTestCase{
		inputFile:  "backends/aws_lambda.yaml",
		outputFile: "backends/aws_lambda.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("DFP Backend with TLS", translatorTestCase{
		inputFile:  "dfp/tls.yaml",
		outputFile: "dfp/tls.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("DFP Backend with simple", translatorTestCase{
		inputFile:  "dfp/simple.yaml",
		outputFile: "dfp/simple.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("Backend TLS Policy", translatorTestCase{
		inputFile:  "backendtlspolicy/tls.yaml",
		outputFile: "backendtlspolicy/tls.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("Backend TLS Policy with SAN", translatorTestCase{
		inputFile:  "backendtlspolicy/tls-san.yaml",
		outputFile: "backendtlspolicy/tls-san.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("Proxy with no routes", translatorTestCase{
		inputFile:  "edge-cases/no_route.yaml",
		outputFile: "no_route.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("Direct response", translatorTestCase{
		inputFile:  "directresponse/manifest.yaml",
		outputFile: "directresponse.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("DirectResponse with missing reference reports correctly", translatorTestCase{
		inputFile:  "directresponse/missing-ref.yaml",
		outputFile: "directresponse/missing-ref.yaml",
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
			Expect(routeStatus).NotTo(BeNil())
			Expect(routeStatus.Parents).To(HaveLen(1))

			// The route itself is considered resolved, but there should be a condition indicating the DirectResponse issue
			resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
			Expect(resolvedRefs).NotTo(BeNil())
			Expect(resolvedRefs.Status).To(Equal(metav1.ConditionTrue))
			Expect(resolvedRefs.Reason).To(Equal(string(gwv1.RouteReasonResolvedRefs)))

			// Check if there's a PartiallyInvalid condition that reports the missing DirectResponse
			partiallyInvalid := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionPartiallyInvalid))
			Expect(partiallyInvalid).NotTo(BeNil())
			Expect(partiallyInvalid.Status).To(Equal(metav1.ConditionTrue))
			Expect(partiallyInvalid.Message).To(ContainSubstring("Dropped Rule"))
			Expect(partiallyInvalid.Message).To(ContainSubstring("no action specified"))
		},
	}),
	Entry("DirectResponse with overlapping filters reports correctly", translatorTestCase{
		inputFile:  "directresponse/overlapping-filters.yaml",
		outputFile: "directresponse/overlapping-filters.yaml",
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
			Expect(routeStatus).NotTo(BeNil())
			Expect(routeStatus.Parents).To(HaveLen(1))

			// Check for PartiallyInvalid condition due to overlapping filters
			partiallyInvalid := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionPartiallyInvalid))
			Expect(partiallyInvalid).NotTo(BeNil())
			Expect(partiallyInvalid.Status).To(Equal(metav1.ConditionTrue))
			Expect(partiallyInvalid.Reason).To(Equal(string(gwv1.RouteReasonUnsupportedValue)))
			Expect(partiallyInvalid.Message).To(ContainSubstring("cannot be applied to route with existing action"))
		},
	}),
	Entry("DirectResponse with invalid backendRef filter reports correctly", translatorTestCase{
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
			routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
			Expect(routeStatus).NotTo(BeNil())
			Expect(routeStatus.Parents).To(HaveLen(1))

			// DirectResponse attached to backendRef should be ignored, route should resolve normally
			resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
			Expect(resolvedRefs).NotTo(BeNil())
			Expect(resolvedRefs.Status).To(Equal(metav1.ConditionTrue))
			Expect(resolvedRefs.Reason).To(Equal(string(gwv1.RouteReasonResolvedRefs)))
		},
	}),
	Entry("HTTPRoutes with timeout and retry", translatorTestCase{
		inputFile:  "httproute-timeout-retry/manifest.yaml",
		outputFile: "httproute-timeout-retry-proxy.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry(
		"http gateway with session persistence (cookie)",
		translatorTestCase{
			inputFile:  "session-persistence/cookie.yaml",
			outputFile: "session-persistence/cookie.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		}),
	Entry(
		"http gateway with session persistence (header)",
		translatorTestCase{
			inputFile:  "session-persistence/header.yaml",
			outputFile: "session-persistence/header.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
		}),
	Entry("HTTPListenerPolicy with upgrades", translatorTestCase{
		inputFile:  "https-listener-pol/upgrades.yaml",
		outputFile: "https-listener-pol/upgrades.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("Service with appProtocol=kubernetes.io/h2c", translatorTestCase{
		inputFile:  "backend-protocol/svc-h2c.yaml",
		outputFile: "backend-protocol/svc-h2c.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("Service with appProtocol=kubernetes.io/ws", translatorTestCase{
		inputFile:  "backend-protocol/svc-ws.yaml",
		outputFile: "backend-protocol/svc-ws.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("Service with appProtocol=anything", translatorTestCase{
		inputFile:  "backend-protocol/svc-default.yaml",
		outputFile: "backend-protocol/svc-default.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("Static Backend with appProtocol=kubernetes.io/h2c", translatorTestCase{
		inputFile:  "backend-protocol/backend-h2c.yaml",
		outputFile: "backend-protocol/backend-h2c.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("Static Backend with appProtocol=kubernetes.io/ws", translatorTestCase{
		inputFile:  "backend-protocol/backend-ws.yaml",
		outputFile: "backend-protocol/backend-ws.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("Static Backend with no appProtocol", translatorTestCase{
		inputFile:  "backend-protocol/backend-default.yaml",
		outputFile: "backend-protocol/backend-default.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("Backend Config Policy with LB Config", translatorTestCase{
		inputFile:  "backendconfigpolicy/lb-config.yaml",
		outputFile: "backendconfigpolicy/lb-config.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("Backend Config Policy with Health Check", translatorTestCase{
		inputFile:  "backendconfigpolicy/healthcheck.yaml",
		outputFile: "backendconfigpolicy/healthcheck.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("Backend Config Policy with Common HTTP Protocol - HTTP backend", translatorTestCase{
		inputFile:  "backendconfigpolicy/commonhttpprotocol-httpbackend.yaml",
		outputFile: "backendconfigpolicy/commonhttpprotocol-httpbackend.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("Backend Config Policy with Common HTTP Protocol - HTTP2 backend", translatorTestCase{
		inputFile:  "backendconfigpolicy/commonhttpprotocol-http2backend.yaml",
		outputFile: "backendconfigpolicy/commonhttpprotocol-http2backend.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("Backend Config Policy with HTTP2 Protocol Options", translatorTestCase{
		inputFile:  "backendconfigpolicy/http2.yaml",
		outputFile: "backendconfigpolicy/http2.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry("Backend Config Policy with TLS and SAN verification", translatorTestCase{
		inputFile:  "backendconfigpolicy/tls-san.yaml",
		outputFile: "backendconfigpolicy/tls-san.yaml",
		gwNN: types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		},
	}),
	Entry(
		"TrafficPolicy with explicit generation",
		translatorTestCase{
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
				assertPolicyStatusWithGeneration(reportsMap, expectedPolicies, 42)
			},
		}),
	// TODO: Add this once istio adds support for listener sets
	// Entry(
	//
	//	"listener sets",
	//	translatorTestCase{
	//		inputFile:  "listener-sets/manifest.yaml",
	//		outputFile: "listener-sets-proxy.yaml",
	//		gwNN: types.NamespacedName{
	//			Namespace: "default",
	//			Name:      "example-gateway",
	//		},
	//	}),
)

var _ = DescribeTable("Route Replacement Tests",
	func(in translatorTestCase, settingOpts ...translatortest.SettingsOpts) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dir := fsutils.MustGetThisDir()

		inputFiles := []string{filepath.Join(dir, "testutils/inputs/", in.inputFile)}
		expectedProxyFile := filepath.Join(dir, "testutils/outputs/", in.outputFile)
		translatortest.TestTranslation(GinkgoT(), ctx, inputFiles, expectedProxyFile, in.gwNN, in.assertReports, settingOpts...)
	},
	Entry("Standard Mode - Invalid HTTPRoute Prefix Match",
		translatorTestCase{
			inputFile:  "route-replacement/standard/invalid-httproute-prefix-match.yaml",
			outputFile: "route-replacement/standard/invalid-httproute-prefix-match-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-traffic-policy-route",
						Namespace: "gwtest",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))

				partiallyInvalid := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionPartiallyInvalid))
				Expect(partiallyInvalid).NotTo(BeNil())
				Expect(partiallyInvalid.Status).To(Equal(metav1.ConditionTrue))
				Expect(partiallyInvalid.Reason).To(Equal(string(gwv1.RouteReasonUnsupportedValue)))
				Expect(partiallyInvalid.Message).To(ContainSubstring("Dropped Rule (0)"))
				Expect(partiallyInvalid.Message).To(ContainSubstring("the rewrite /new//../path is invalid"))
				Expect(partiallyInvalid.ObservedGeneration).To(Equal(int64(1)))
			},
		},
		func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStandard
		}),

	Entry("Standard Mode - Invalid Rate Limit Global Fields",
		translatorTestCase{
			inputFile:  "route-replacement/standard/invalid-ratelimit-global-empty-fields.yaml",
			outputFile: "route-replacement/standard/invalid-ratelimit-global-empty-fields-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route",
						Namespace: "gwtest",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))

				partiallyInvalid := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionPartiallyInvalid))
				Expect(partiallyInvalid).NotTo(BeNil())
				Expect(partiallyInvalid.Status).To(Equal(metav1.ConditionTrue))
				Expect(partiallyInvalid.Reason).To(Equal(string(gwv1.RouteReasonUnsupportedValue)))
				Expect(partiallyInvalid.Message).To(ContainSubstring("Dropped Rule (0)"))
				Expect(partiallyInvalid.Message).To(ContainSubstring("failed to create rate limit actions"))
				Expect(partiallyInvalid.ObservedGeneration).To(Equal(int64(0)))
			},
		},
		func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStandard
		}),
	Entry("Standard Mode - Gateway Level Policy Invalid Rate Limit (Not Validated)",
		translatorTestCase{
			inputFile:  "route-replacement/standard/gateway-level-policy-validation.yaml",
			outputFile: "route-replacement/standard/gateway-level-policy-validation-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				// Verify that despite the invalid rate limit config, attaching the policy to the gateway
				// does not cause the route to be invalidated as the route translator does not currently
				// handle IR errors outside of the envoyRoutes method. This will be fixed in the future.
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route",
						Namespace: "gwtest",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))

				accepted := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				Expect(accepted).NotTo(BeNil())
				Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
				Expect(accepted.Reason).To(Equal(string(gwv1.RouteReasonAccepted)))
				Expect(accepted.Message).To(Equal(""))
				Expect(accepted.ObservedGeneration).To(Equal(int64(0)))

				// Expect no PartiallyInvalid condition since template validation is skipped in standard mode
				partiallyInvalid := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionPartiallyInvalid))
				Expect(partiallyInvalid).To(BeNil())
			},
		},
		func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStandard
		}),
	Entry("Standard Mode - Gateway Listener Policy Invalid Rate Limit (Not Validated)",
		translatorTestCase{
			inputFile:  "route-replacement/standard/gateway-listener-policy-validation.yaml",
			outputFile: "route-replacement/standard/gateway-listener-policy-validation-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				// Verify that despite the invalid rate limit config, attaching the policy to a specific listener
				// does not cause the route to be invalidated as the route translator does not currently
				// handle IR errors outside of the envoyRoutes method. This will be fixed in the future.
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route",
						Namespace: "gwtest",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))

				accepted := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				Expect(accepted).NotTo(BeNil())
				Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
				Expect(accepted.Reason).To(Equal(string(gwv1.RouteReasonAccepted)))
				Expect(accepted.Message).To(Equal(""))
				Expect(accepted.ObservedGeneration).To(Equal(int64(0)))

				// Expect no PartiallyInvalid condition since template validation is skipped in standard mode
				partiallyInvalid := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionPartiallyInvalid))
				Expect(partiallyInvalid).To(BeNil())
			},
		},
		func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStandard
		}),
	Entry("Standard Mode - Invalid Transformation Template (Not Validated)",
		translatorTestCase{
			inputFile:  "route-replacement/standard/transformation-template-not-validated.yaml",
			outputFile: "route-replacement/standard/transformation-template-not-validated-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				// Verify that in standard mode, invalid transformation templates are not validated
				// and thus no route replacement occurs
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-traffic-policy-route",
						Namespace: "gwtest",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))

				accepted := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				Expect(accepted).NotTo(BeNil())
				Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
				Expect(accepted.Reason).To(Equal(string(gwv1.RouteReasonAccepted)))
				Expect(accepted.Message).To(Equal(""))
				Expect(accepted.ObservedGeneration).To(Equal(int64(0)))

				// Expect no PartiallyInvalid condition since template validation is skipped in standard mode
				partiallyInvalid := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionPartiallyInvalid))
				Expect(partiallyInvalid).To(BeNil())
			},
		},
		func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStandard
		}),
	Entry("Strict Mode - Invalid CSRF Regex Configuration",
		translatorTestCase{
			inputFile:  "route-replacement/strict/invalid-csrf-regex-config.yaml",
			outputFile: "route-replacement/strict/invalid-csrf-regex-config-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-route",
						Namespace: "gwtest",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))

				partiallyInvalid := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionPartiallyInvalid))
				Expect(partiallyInvalid).NotTo(BeNil())
				Expect(partiallyInvalid.Status).To(Equal(metav1.ConditionTrue))
				Expect(partiallyInvalid.Reason).To(Equal(string(gwv1.RouteReasonUnsupportedValue)))
				Expect(partiallyInvalid.Message).To(ContainSubstring("Dropped Rule (0)"))
				Expect(partiallyInvalid.Message).To(ContainSubstring("invalid xds configuration"))
				Expect(partiallyInvalid.ObservedGeneration).To(Equal(int64(0)))
			},
		},
		func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		}),
	Entry("Strict Mode - Invalid ExtAuth Extension Reference (Referential Error)",
		translatorTestCase{
			inputFile:  "route-replacement/strict/invalid-extauth-extension-ref.yaml",
			outputFile: "route-replacement/strict/invalid-extauth-extension-ref-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-traffic-policy-route",
						Namespace: "gwtest",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))

				partiallyInvalid := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionPartiallyInvalid))
				Expect(partiallyInvalid).NotTo(BeNil())
				Expect(partiallyInvalid.Status).To(Equal(metav1.ConditionTrue))
				Expect(partiallyInvalid.Reason).To(Equal(string(gwv1.RouteReasonUnsupportedValue)))
				Expect(partiallyInvalid.Message).To(ContainSubstring("Dropped Rule (0)"))
				Expect(partiallyInvalid.Message).To(ContainSubstring("extauthz: extension not found"))
				Expect(partiallyInvalid.ObservedGeneration).To(Equal(int64(0)))
			},
		},
		func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		}),
	Entry("Strict Mode - Invalid Transformation Body Template",
		translatorTestCase{
			inputFile:  "route-replacement/strict/invalid-transformation-body-template.yaml",
			outputFile: "route-replacement/strict/invalid-transformation-body-template-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-traffic-policy-route",
						Namespace: "gwtest",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))

				partiallyInvalid := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionPartiallyInvalid))
				Expect(partiallyInvalid).NotTo(BeNil())
				Expect(partiallyInvalid.Status).To(Equal(metav1.ConditionTrue))
				Expect(partiallyInvalid.Reason).To(Equal(string(gwv1.RouteReasonUnsupportedValue)))
				Expect(partiallyInvalid.Message).To(ContainSubstring("Dropped Rule (0)"))
				Expect(partiallyInvalid.Message).To(ContainSubstring("invalid xds configuration"))
				Expect(partiallyInvalid.ObservedGeneration).To(Equal(int64(0)))
			},
		},
		func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		}),
	Entry("Strict Mode - Invalid Transformation Header Template",
		translatorTestCase{
			inputFile:  "route-replacement/strict/invalid-transformation-header-template.yaml",
			outputFile: "route-replacement/strict/invalid-transformation-header-template-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-traffic-policy-route",
						Namespace: "gwtest",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))

				partiallyInvalid := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionPartiallyInvalid))
				Expect(partiallyInvalid).NotTo(BeNil())
				Expect(partiallyInvalid.Status).To(Equal(metav1.ConditionTrue))
				Expect(partiallyInvalid.Reason).To(Equal(string(gwv1.RouteReasonUnsupportedValue)))
				Expect(partiallyInvalid.Message).To(ContainSubstring("Dropped Rule (0)"))
				Expect(partiallyInvalid.Message).To(ContainSubstring("invalid xds configuration"))
				Expect(partiallyInvalid.ObservedGeneration).To(Equal(int64(0)))
			},
		},
		func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		}),
	Entry("Strict Mode - Invalid Transformation Malformed Template",
		translatorTestCase{
			inputFile:  "route-replacement/strict/invalid-transformation-malformed-template.yaml",
			outputFile: "route-replacement/strict/invalid-transformation-malformed-template-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-traffic-policy-route",
						Namespace: "gwtest",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))

				partiallyInvalid := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionPartiallyInvalid))
				Expect(partiallyInvalid).NotTo(BeNil())
				Expect(partiallyInvalid.Status).To(Equal(metav1.ConditionTrue))
				Expect(partiallyInvalid.Reason).To(Equal(string(gwv1.RouteReasonUnsupportedValue)))
				Expect(partiallyInvalid.Message).To(ContainSubstring("Dropped Rule (0)"))
				Expect(partiallyInvalid.Message).To(ContainSubstring("invalid xds configuration"))
				Expect(partiallyInvalid.ObservedGeneration).To(Equal(int64(0)))
			},
		},
		func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		}),
	Entry("Strict Mode - Valid Structure Invalid Template (Runtime Error)",
		translatorTestCase{
			inputFile:  "route-replacement/strict/valid-structure-invalid-template-policy.yaml",
			outputFile: "route-replacement/strict/valid-structure-invalid-template-policy-out.yaml",
			gwNN: types.NamespacedName{
				Namespace: "gwtest",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := &gwv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-traffic-policy-route",
						Namespace: "gwtest",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.Background(), route, wellknown.DefaultGatewayClassName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))

				accepted := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionAccepted))
				Expect(accepted).NotTo(BeNil())
				Expect(accepted.Status).To(Equal(metav1.ConditionTrue))
				Expect(accepted.Reason).To(Equal(string(gwv1.RouteReasonAccepted)))
				Expect(accepted.Message).To(Equal(""))
				Expect(accepted.ObservedGeneration).To(Equal(int64(0)))

				// Template is structurally valid (passes xDS validation) but would fail at runtime
				// No PartiallyInvalid condition should be set since it passes validation
				partiallyInvalid := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionPartiallyInvalid))
				Expect(partiallyInvalid).To(BeNil())
			},
		},
		func(s *settings.Settings) {
			s.RouteReplacementMode = settings.RouteReplacementStrict
		}),
)

var _ = DescribeTable("Route Delegation translator",
	func(inputFile string, errors map[types.NamespacedName]string) {
		dir := fsutils.MustGetThisDir()
		translatortest.TestTranslation(
			GinkgoT(),
			context.Background(),
			[]string{
				filepath.Join(dir, "testutils/inputs/delegation/common.yaml"),
				filepath.Join(dir, "testutils/inputs/delegation", inputFile),
			},
			filepath.Join(dir, "testutils/outputs/delegation", inputFile),
			types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
			func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				if errors == nil {
					Expect(translatortest.AreReportsSuccess(gwNN, reportsMap)).NotTo(HaveOccurred())
				} else {
					for route, err := range errors {
						Expect(translatortest.GetHTTPRouteStatusError(reportsMap, &route)).To(MatchError(ContainSubstring(err)))
					}
				}
			},
		)
	},
	Entry("Basic config", "basic.yaml", nil),
	Entry("Child matches parent via parentRefs", "basic_parentref_match.yaml", nil),
	Entry("Child doesn't match parent via parentRefs", "basic_parentref_mismatch.yaml",
		map[types.NamespacedName]string{
			{Name: "example-route", Namespace: "infra"}: "BackendNotFound gateway.networking.k8s.io/HTTPRoute/a/*: unresolved reference",
		},
	),
	Entry("Children using parentRefs and inherit-parent-matcher", "inherit_parentref.yaml", nil),
	Entry("Parent delegates to multiple chidren", "multiple_children.yaml", nil),
	Entry("Child is invalid as it is delegatee and specifies hostnames", "basic_invalid_hostname.yaml",
		map[types.NamespacedName]string{
			{Name: "route-a", Namespace: "a"}:           "spec.hostnames must be unset on a delegatee route as they are inherited from the parent route",
			{Name: "example-route", Namespace: "infra"}: "BackendNotFound gateway.networking.k8s.io/HTTPRoute/a/*: unresolved reference",
		},
	),
	Entry("Multi-level recursive delegation", "recursive.yaml", nil),
	Entry("Cyclic child route", "cyclic1.yaml",
		map[types.NamespacedName]string{
			{Name: "route-a", Namespace: "a"}: "cyclic reference detected while evaluating delegated routes",
		},
	),
	Entry("Multi-level cyclic child route", "cyclic2.yaml",
		map[types.NamespacedName]string{
			{Name: "route-a-b", Namespace: "a-b"}: "cyclic reference detected while evaluating delegated routes",
		},
	),
	Entry("Child rule matcher", "child_rule_matcher.yaml",
		map[types.NamespacedName]string{
			{Name: "example-route", Namespace: "infra"}: "BackendNotFound gateway.networking.k8s.io/HTTPRoute/b/*: unresolved reference",
		},
	),
	Entry("Child with multiple parents", "multiple_parents.yaml",
		map[types.NamespacedName]string{
			{Name: "foo-route", Namespace: "infra"}: "BackendNotFound gateway.networking.k8s.io/HTTPRoute/b/*: unresolved reference",
		},
	),
	Entry("Child can be an invalid delegatee but valid standalone", "invalid_child_valid_standalone.yaml",
		map[types.NamespacedName]string{
			{Name: "route-a", Namespace: "a"}: "spec.hostnames must be unset on a delegatee route as they are inherited from the parent route",
		},
	),
	Entry("Relative paths", "relative_paths.yaml", nil),
	Entry("Nested absolute and relative path inheritance", "nested_absolute_relative.yaml", nil),
	Entry("Child route matcher does not match parent", "discard_invalid_child_matches.yaml", nil),
	Entry("Multi-level multiple parents delegation", "multi_level_multiple_parents.yaml", nil),
	Entry("TrafficPolicy only on child", "traffic_policy.yaml", nil),
	Entry("TrafficPolicy inheritance from parent", "traffic_policy_inheritance.yaml", nil),
	Entry("TrafficPolicy ignore child override on conflict", "traffic_policy_inheritance_child_override_ignore.yaml", nil),
	Entry("TrafficPolicy merge child override on no conflict", "traffic_policy_inheritance_child_override_ok.yaml", nil),
	Entry("TrafficPolicy multi level inheritance with child override disabled", "traffic_policy_multi_level_inheritance_override_disabled.yaml", nil),
	Entry("TrafficPolicy multi level inheritance with child override enabled", "traffic_policy_multi_level_inheritance_override_enabled.yaml", nil),
	Entry("TrafficPolicy filter override merge", "traffic_policy_filter_override_merge.yaml", nil),
	Entry("Built-in rule inheritance", "builtin_rule_inheritance.yaml", nil),
	Entry("Label based delegation", "label_based.yaml", nil),
	Entry("Unresolved child reference", "unresolved_ref.yaml",
		map[types.NamespacedName]string{
			{Name: "example-route", Namespace: "infra"}: "BackendNotFound gateway.networking.k8s.io/HTTPRoute/b/*: unresolved reference",
			{Name: "route-a", Namespace: "a"}:           "BackendNotFound gateway.networking.k8s.io/HTTPRoute/a-c/: unresolved reference",
		},
	),
)

var _ = DescribeTable("Discovery Namespace Selector",
	func(cfgJSON string, inputFile string, outputFile string, errdesc string) {
		dir := fsutils.MustGetThisDir()
		translatortest.TestTranslation(
			GinkgoT(),
			context.Background(),
			[]string{
				filepath.Join(dir, "testutils/inputs/discovery-namespace-selector", inputFile),
			},
			filepath.Join(dir, "testutils/outputs/discovery-namespace-selector", outputFile),
			types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
			func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				if errdesc == "" {
					Expect(translatortest.AreReportsSuccess(gwNN, reportsMap)).NotTo(HaveOccurred())
				} else {
					Expect(translatortest.AreReportsSuccess(gwNN, reportsMap)).To(MatchError(ContainSubstring(errdesc)))
				}
			},
			func(s *settings.Settings) {
				s.DiscoveryNamespaceSelectors = cfgJSON
			},
		)
	},
	Entry("Select all resources",
		`[
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
]`,
		"base.yaml", "base_select_all.yaml", ""),
	Entry("Select all resources; AND matchExpressions and matchLabels",
		`[
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
]`,
		"base.yaml", "base_select_all.yaml", ""),
	Entry("Select only namespace infra",
		`[
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
]`,
		"base.yaml", "base_select_infra.yaml", "condition error for httproute: infra/example-route"),
)

// assertPolicyStatusWithGeneration is a helper function to verify policy status conditions with a specific generation
func assertPolicyStatusWithGeneration(reportsMap reports.ReportMap, policies []reports.PolicyKey, expectedGeneration int64) {
	var currentStatus gwv1alpha2.PolicyStatus

	for _, policy := range policies {
		// Validate each policy's status
		status := reportsMap.BuildPolicyStatus(context.Background(), policy, wellknown.DefaultGatewayControllerName, currentStatus)
		Expect(status).NotTo(BeNil(), "status missing for policy %v", policy)
		Expect(status.Ancestors).To(HaveLen(1), "ancestor missing for policy %v", policy) // 1 Gateway(ancestor)

		acceptedCondition := meta.FindStatusCondition(status.Ancestors[0].Conditions, string(gwv1alpha2.PolicyConditionAccepted))
		Expect(acceptedCondition).NotTo(BeNil())
		Expect(acceptedCondition.Status).To(Equal(metav1.ConditionTrue))
		Expect(acceptedCondition.Reason).To(Equal(string(gwv1alpha2.PolicyReasonAccepted)))
		Expect(acceptedCondition.Message).To(Equal(reporter.PolicyAcceptedMsg))
		Expect(acceptedCondition.ObservedGeneration).To(Equal(expectedGeneration))
	}
}

// assertAcceptedPolicyStatus is a helper function to verify policy status conditions
func assertAcceptedPolicyStatus(reportsMap reports.ReportMap, policies []reports.PolicyKey) {
	assertPolicyStatusWithGeneration(reportsMap, policies, 0)
}
