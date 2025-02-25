package gateway_test

import (
	"context"
	"fmt"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/reports"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
)

type translatorTestCase struct {
	inputFile     string
	outputFile    string
	gwNN          types.NamespacedName
	assertReports func(gwNN types.NamespacedName, reportsMap reports.ReportMap)
}

var _ = DescribeTable("Basic GatewayTranslator Tests",
	func(in translatorTestCase) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dir := fsutils.MustGetThisDir()

		results, err := TestCase{
			InputFiles: []string{filepath.Join(dir, "testutils/inputs/", in.inputFile)},
		}.Run(GinkgoT(), ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(HaveLen(1))
		Expect(results).To(HaveKey(in.gwNN))
		result := results[in.gwNN]
		expectedProxyFile := filepath.Join(dir, "testutils/outputs/", in.outputFile)
		fmt.Fprintf(GinkgoWriter, "Comparing expected proxy from %s to actual proxy generated from %s\n", in.outputFile, in.inputFile)

		//// do a json round trip to normalize the output (i.e. things like omit empty)
		//b, _ := json.Marshal(result.Proxy)
		//var proxy ir.GatewayIR
		//Expect(json.Unmarshal(b, &proxy)).NotTo(HaveOccurred())

		Expect(CompareProxy(expectedProxyFile, result.Proxy)).To(BeEmpty())

		if in.assertReports != nil {
			in.assertReports(in.gwNN, result.ReportsMap)
		} else {
			Expect(AreReportsSuccess(in.gwNN, result.ReportsMap)).NotTo(HaveOccurred())
		}
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
	Entry(
		"http gateway with lambda destination",
		translatorTestCase{
			inputFile:  "http-with-lambda-destination",
			outputFile: "http-with-lambda-destination-proxy.yaml",
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
				routeStatus := reportsMap.BuildRouteStatus(context.TODO(), route, "")
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
				routeStatus := reportsMap.BuildRouteStatus(context.TODO(), route, "")
				Expect(routeStatus).NotTo(BeNil())
				Expect(routeStatus.Parents).To(HaveLen(1))
				resolvedRefs := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gwv1.RouteConditionResolvedRefs))
				Expect(resolvedRefs).NotTo(BeNil())
				Expect(resolvedRefs.Message).To(Equal("unknown backend kind"))
			},
		}),
	XEntry(
		"RouteOptions merging",
		translatorTestCase{
			inputFile:  "route_options/merge.yaml",
			outputFile: "route_options/merge.yaml",
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
				routeStatus := reportsMap.BuildRouteStatus(context.TODO(), route, "")
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
				routeStatus := reportsMap.BuildRouteStatus(context.TODO(), route, "")
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
				routeStatus := reportsMap.BuildRouteStatus(context.TODO(), route, "")
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
		ctx := context.TODO()
		dir := fsutils.MustGetThisDir()

		results, err := TestCase{
			InputFiles: []string{filepath.Join(dir, "testutils/inputs/delegation", inputFile)},
		}.Run(GinkgoT(), ctx)

		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(HaveLen(1))
		gwNN := types.NamespacedName{
			Namespace: "infra",
			Name:      "example-gateway",
		}
		result, ok := results[gwNN]
		Expect(ok).To(BeTrue())
		outputProxyFile := filepath.Join(dir, "testutils/outputs/delegation", inputFile)
		Expect(CompareProxy(outputProxyFile, result.Proxy)).To(BeEmpty())
		if errdesc == "" {
			Expect(AreReportsSuccess(gwNN, result.ReportsMap)).NotTo(HaveOccurred())
		} else {
			Expect(AreReportsSuccess(gwNN, result.ReportsMap)).To(MatchError(ContainSubstring(errdesc)))
		}
	},
	Entry("Basic config", "basic.yaml", ""),
	Entry("Child matches parent via parentRefs", "basic_parentref_match.yaml", ""),
	Entry("Child doesn't match parent via parentRefs", "basic_parentref_mismatch.yaml", ""),
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
	XEntry("RouteOptions only on child", "route_options.yaml", ""),
	XEntry("RouteOptions inheritance from parent", "route_options_inheritance.yaml", ""),
	XEntry("RouteOptions ignore child override on conflict", "route_options_inheritance_child_override_ignore.yaml", ""),
	XEntry("RouteOptions merge child override on no conflict", "route_options_inheritance_child_override_ok.yaml", ""),
	XEntry("RouteOptions multi level inheritance with child override", "route_options_multi_level_inheritance_override_ok.yaml", ""),
	XEntry("RouteOptions filter override merge", "route_options_filter_override_merge.yaml", ""),
	Entry("Child route matcher does not match parent", "bug-6621.yaml", ""),
	Entry("Multi-level multiple parents delegation", "bug-10379.yaml", ""),
)
