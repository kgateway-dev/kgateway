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
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/gateway/testutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

type translatorTestCase struct {
	inputFile     string
	outputFile    string
	gwNN          types.NamespacedName
	assertReports testutils.AssertReports
}

var _ = DescribeTable("Basic GatewayTranslator Tests",
	func(in translatorTestCase) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dir := fsutils.MustGetThisDir()

		inputFiles := []string{filepath.Join(dir, "testutils/inputs/", in.inputFile)}
		expectedProxyFile := filepath.Join(dir, "testutils/outputs/", in.outputFile)
		testutils.TestTranslation(GinkgoT(), ctx, inputFiles, expectedProxyFile, in.gwNN, in.assertReports)
	},
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
		"gateway with correctly sorted routes",
		translatorTestCase{
			inputFile:  "route-sort.yaml",
			outputFile: "route-sort.yaml",
			gwNN: types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			},
		}),
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
				routeStatus := reportsMap.BuildRouteStatus(context.TODO(), route, wellknown.GatewayControllerName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Message).To(Equal("Service \"example-svc\" not found"))
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
				routeStatus := reportsMap.BuildRouteStatus(context.TODO(), route, wellknown.GatewayControllerName)
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Message).To(Equal("unknown backend kind"))
			},
		}),
	XEntry(
		"TrafficPolicy merging",
		translatorTestCase{
			inputFile:  "route_policy/merge.yaml",
			outputFile: "route_policy/merge.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "gw",
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
				route := &gwv1a2.TCPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tcp-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.TODO(), route, wellknown.GatewayControllerName)
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
				route := &gwv1a2.TCPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tcp-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.TODO(), route, wellknown.GatewayControllerName)
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
				route := &gwv1a2.TCPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tcp-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.TODO(), route, wellknown.GatewayControllerName)
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
				route := &gwv1a2.TLSRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tls-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.TODO(), route, wellknown.GatewayControllerName)
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
				route := &gwv1a2.TLSRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tls-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.TODO(), route, wellknown.GatewayControllerName)
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
				route := &gwv1a2.TLSRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "example-tls-route",
						Namespace: "default",
					},
				}
				routeStatus := reportsMap.BuildRouteStatus(context.TODO(), route, wellknown.GatewayControllerName)
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
	Entry("Plugin Backend", translatorTestCase{
		inputFile:  "backend-plugin/gateway.yaml",
		outputFile: "backend-plugin-proxy.yaml",
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
)

var _ = DescribeTable("Route Delegation translator",
	func(inputFile string, errdesc string) {
		dir := fsutils.MustGetThisDir()
		testutils.TestTranslation(
			GinkgoT(),
			context.TODO(),
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
				if errdesc == "" {
					Expect(testutils.AreReportsSuccess(gwNN, reportsMap)).NotTo(HaveOccurred())
				} else {
					Expect(testutils.AreReportsSuccess(gwNN, reportsMap)).To(MatchError(ContainSubstring(errdesc)))
				}
			},
		)
	},
	Entry("Basic config", "basic.yaml", ""),
	Entry("Child matches parent via parentRefs", "basic_parentref_match.yaml", ""),
	Entry("Child doesn't match parent via parentRefs", "basic_parentref_mismatch.yaml", ""),
	Entry("Children using parentRefs and inherit-parent-matcher", "inherit_parentref.yaml", ""),
	Entry("Parent delegates to multiple chidren", "multiple_children.yaml", ""),
	Entry("Child is invalid as it is delegatee and specifies hostnames", "basic_invalid_hostname.yaml", "spec.hostnames must be unset on a delegatee route as they are inherited from the parent route"),
	Entry("Multi-level recursive delegation", "recursive.yaml", ""),
	Entry("Cyclic child route", "cyclic1.yaml", "cyclic reference detected while evaluating delegated routes"),
	Entry("Multi-level cyclic child route", "cyclic2.yaml", "cyclic reference detected while evaluating delegated routes"),
	Entry("Child rule matcher", "child_rule_matcher.yaml", ""),
	Entry("Child with multiple parents", "multiple_parents.yaml", ""),
	Entry("Child can be an invalid delegatee but valid standalone", "invalid_child_valid_standalone.yaml", "spec.hostnames must be unset on a delegatee route as they are inherited from the parent route"),
	Entry("Relative paths", "relative_paths.yaml", ""),
	Entry("Nested absolute and relative path inheritance", "nested_absolute_relative.yaml", ""),
	Entry("Child route matcher does not match parent", "discard_invalid_child_matches.yaml", ""),
	XEntry("Multi-level multiple parents delegation", "multi_level_multiple_parents.yaml", ""),
	Entry("TrafficPolicy only on child", "traffic_policy.yaml", ""),
	Entry("TrafficPolicy inheritance from parent", "traffic_policy_inheritance.yaml", ""),
	Entry("TrafficPolicy ignore child override on conflict", "traffic_policy_inheritance_child_override_ignore.yaml", ""),
	Entry("TrafficPolicy merge child override on no conflict", "traffic_policy_inheritance_child_override_ok.yaml", ""),
	XEntry("TrafficPolicy multi level inheritance with child override", "traffic_policy_multi_level_inheritance_override_ok.yaml", ""),
	Entry("TrafficPolicy filter override merge", "traffic_policy_filter_override_merge.yaml", ""),
)
