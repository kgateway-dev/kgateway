package translator_test

import (
	"context"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/projects/gateway2/reports"
	"github.com/solo-io/skv2/codegen/util"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type translatorTestCase struct {
	inputFile     string
	outputFile    string
	gwNN          types.NamespacedName
	assertReports func(gwNN types.NamespacedName, reportsMap reports.ReportMap)
}

var _ = DescribeTable("Basic GatewayTranslator Tests",
	func(in translatorTestCase) {
		ctx := context.TODO()
		dir := util.MustGetThisDir()

		results, err := TestCase{
			InputFiles: []string{filepath.Join(dir, "testutils/inputs/", in.inputFile)},
		}.Run(ctx)

		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(HaveLen(1))
		Expect(results).To(HaveKey(in.gwNN))
		result := results[in.gwNN]
		expectedProxyFile := filepath.Join(dir, "testutils/outputs/", in.outputFile)
		Expect(CompareProxy(expectedProxyFile, result.Proxy)).To(BeEmpty())

		if in.assertReports != nil {
			in.assertReports(in.gwNN, result.ReportsMap)
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
			}}),
	Entry(
		"https gateway with basic routing",
		translatorTestCase{
			inputFile:  "https-routing",
			outputFile: "https-routing-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			}}),
	Entry(
		"http gateway with multiple listeners on the same port",
		translatorTestCase{
			inputFile:  "multiple-listeners-http-routing",
			outputFile: "multiple-listeners-http-routing-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "http",
			}}),
	Entry(
		"https gateway with multiple listeners on the same port",
		translatorTestCase{
			inputFile:  "multiple-listeners-https-routing",
			outputFile: "multiple-listeners-https-routing-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "http",
			}}),
	Entry(
		"http gateway with multiple routing rules and HeaderModifier filter",
		translatorTestCase{
			inputFile:  "http-with-header-modifier",
			outputFile: "http-with-header-modifier-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "gw",
			}}),
	Entry(
		"http gateway with lambda destination",
		translatorTestCase{
			inputFile:  "http-with-lambda-destination",
			outputFile: "http-with-lambda-destination-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "gw",
			}}),
	Entry(
		"http gateway with azure destination",
		translatorTestCase{
			inputFile:  "http-with-azure-destination",
			outputFile: "http-with-azure-destination-proxy.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "gw",
			}}),
	Entry(
		"gateway with correctly sorted routes",
		translatorTestCase{
			inputFile:  "route-sort.yaml",
			outputFile: "route-sort.yaml",
			gwNN: types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}}),
	Entry(
		"route with missing backend reports correctly",
		translatorTestCase{
			inputFile:  "http-routing-missing-backend",
			outputFile: "http-routing-missing-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := gwv1.HTTPRoute{
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
				Expect(resolvedRefs.Message).To(Equal("services \"example-svc\" not found"))
			},
		}),
	Entry(
		"route with invalid backend reports correctly",
		translatorTestCase{
			inputFile:  "http-routing-invalid-backend",
			outputFile: "http-routing-invalid-backend.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			assertReports: func(gwNN types.NamespacedName, reportsMap reports.ReportMap) {
				route := gwv1.HTTPRoute{
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
	Entry(
		"RouteOptions merging",
		translatorTestCase{
			inputFile:  "route_options/merge.yaml",
			outputFile: "route_options/merge.yaml",
			gwNN: types.NamespacedName{
				Namespace: "default",
				Name:      "gw",
			},
		}),
)

var _ = DescribeTable("Route Delegation translator",
	func(inputFile string) {
		ctx := context.TODO()
		dir := util.MustGetThisDir()

		results, err := TestCase{
			InputFiles: []string{filepath.Join(dir, "testutils/inputs/delegation", inputFile)},
		}.Run(ctx)

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
	},
	Entry("Basic config", "basic.yaml"),
	Entry("Child matches parent via parentRefs", "basic_parentref_match.yaml"),
	Entry("Child doesn't match parent via parentRefs", "basic_parentref_mismatch.yaml"),
	Entry("Parent delegates to multiple chidren", "multiple_children.yaml"),
	Entry("Child is invalid as it is delegatee and specifies hostnames", "basic_invalid_hostname.yaml"),
	Entry("Multi-level recursive delegation", "recursive.yaml"),
	Entry("Cyclic child route", "cyclic1.yaml"),
	Entry("Multi-level cyclic child route", "cyclic2.yaml"),
	Entry("Child rule matcher", "child_rule_matcher.yaml"),
	Entry("Child with multiple parents", "multiple_parents.yaml"),
	Entry("Child can be an invalid delegatee but valid standalone", "invalid_child_valid_standalone.yaml"),
	Entry("Relative paths", "relative_paths.yaml"),
	Entry("Nested absolute and relative path inheritance", "nested_absolute_relative.yaml"),
	Entry("RouteOptions only on child", "route_options.yaml"),
	Entry("RouteOptions inheritance from parent", "route_options_inheritance.yaml"),
	Entry("RouteOptions ignore child override on conflict", "route_options_inheritance_child_override_ignore.yaml"),
	Entry("RouteOptions merge child override on no conflict", "route_options_inheritance_child_override_ok.yaml"),
	Entry("RouteOptions multi level inheritance with child override", "route_options_multi_level_inheritance_override_ok.yaml"),
	Entry("RouteOptions filter override merge", "route_options_filter_override_merge.yaml"),
	Entry("Child route matcher does not match parent", "bug-6621.yaml"),
)
