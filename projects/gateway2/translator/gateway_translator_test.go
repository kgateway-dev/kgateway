package translator_test

import (
	"context"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/skv2/codegen/util"
	"k8s.io/apimachinery/pkg/types"
)

func ExpectKeyWithNoDiff(results map[types.NamespacedName]string, key types.NamespacedName) {
	Expect(results).To(HaveKey(key))
	Expect(results[key]).To(BeEmpty())
}

var _ = Describe("GatewayTranslator", func() {
	ctx := context.TODO()
	dir := util.MustGetThisDir()

	It("should translate a gateway with basic routing", func() {
		results, err := TestCase{
			Name:       "basic-http-routing",
			InputFiles: []string{dir + "/testutils/inputs/http-routing"},
			ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
				{
					Namespace: "default",
					Name:      "example-gateway",
				}: {
					Proxy: dir + "/testutils/outputs/http-routing-proxy.yaml",
					// Reports:     nil,
				},
			},
		}.Run(ctx)

		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(HaveLen(1))
		ExpectKeyWithNoDiff(results, types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		})
	})

	It("should translate a gateway with https routing", func() {
		results, err := TestCase{
			Name:       "basic-http-routing",
			InputFiles: []string{dir + "/testutils/inputs/https-routing"},
			ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
				{
					Namespace: "default",
					Name:      "example-gateway",
				}: {
					Proxy: dir + "/testutils/outputs/https-routing-proxy.yaml",
					// Reports:     nil,
				},
			},
		}.Run(ctx)

		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(HaveLen(1))
		ExpectKeyWithNoDiff(results, types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		})
	})

	It("should translate a gateway with http routing with multiple listeners on the same port", func() {
		results, err := TestCase{
			Name:       "multiple-listeners-http-routing",
			InputFiles: []string{dir + "/testutils/inputs/multiple-listeners-http-routing"},
			ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
				{
					Namespace: "default",
					Name:      "http",
				}: {
					Proxy: dir + "/testutils/outputs/multiple-listeners-http-routing-proxy.yaml",
					// Reports:     nil,
				},
			},
		}.Run(ctx)

		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(HaveLen(1))
		ExpectKeyWithNoDiff(results, types.NamespacedName{
			Namespace: "default",
			Name:      "http",
		})
	})

	It("should translate a gateway with https routing with multiple listeners on the same port", func() {
		results, err := TestCase{
			Name:       "multiple-listeners-https-routing",
			InputFiles: []string{dir + "/testutils/inputs/multiple-listeners-https-routing"},
			ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
				{
					Namespace: "default",
					Name:      "http",
				}: {
					Proxy: dir + "/testutils/outputs/multiple-listeners-https-routing-proxy.yaml",
					// Reports:     nil,
				},
			},
		}.Run(ctx)

		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(HaveLen(1))
		ExpectKeyWithNoDiff(results, types.NamespacedName{
			Namespace: "default",
			Name:      "http",
		})
	})

	It("should translate an http gateway with multiple routing rules and use the HeaderModifier filter", func() {
		results, err := TestCase{
			Name:       "http-routing-with-header-modifier-filter",
			InputFiles: []string{dir + "/testutils/inputs/http-with-header-modifier"},
			ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
				{
					Name:      "gw",
					Namespace: "default",
				}: {
					Proxy: dir + "/testutils/outputs/http-with-header-modifier-proxy.yaml",
					// Reports:     nil,
				},
			},
		}.Run(ctx)

		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(HaveLen(1))
		ExpectKeyWithNoDiff(results, types.NamespacedName{
			Namespace: "default",
			Name:      "gw",
		})
	})

	It("should translate an http gateway with a lambda destination", func() {
		results, err := TestCase{
			Name:       "http-routing-with-lambda-destination",
			InputFiles: []string{dir + "/testutils/inputs/http-with-lambda-destination"},
			ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
				{
					Name:      "gw",
					Namespace: "default",
				}: {
					Proxy: dir + "/testutils/outputs/http-with-lambda-destination-proxy.yaml",
					// Reports:     nil,
				},
			},
		}.Run(ctx)

		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(HaveLen(1))
		ExpectKeyWithNoDiff(results, types.NamespacedName{
			Namespace: "default",
			Name:      "gw",
		})
	})

	It("should translate an http gateway with a azure destination", func() {
		results, err := TestCase{
			Name:       "http-routing-with-azure-destination",
			InputFiles: []string{dir + "/testutils/inputs/http-with-azure-destination"},
			ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
				{
					Name:      "gw",
					Namespace: "default",
				}: {
					Proxy: dir + "/testutils/outputs/http-with-azure-destination-proxy.yaml",
					// Reports:     nil,
				},
			},
		}.Run(ctx)

		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(HaveLen(1))
		ExpectKeyWithNoDiff(results, types.NamespacedName{
			Namespace: "default",
			Name:      "gw",
		})
	})

	It("should correctly sort routes", func() {
		results, err := TestCase{
			Name:       "delegation-basic",
			InputFiles: []string{filepath.Join(dir, "testutils/inputs/route-sort.yaml")},
			ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
				{
					Namespace: "infra",
					Name:      "example-gateway",
				}: {
					Proxy: filepath.Join(dir, "testutils/outputs/route-sort.yaml"),
					// Reports:     nil,
				},
			},
		}.Run(ctx)

		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(HaveLen(1))
		ExpectKeyWithNoDiff(results, types.NamespacedName{
			Namespace: "infra",
			Name:      "example-gateway",
		})
	})
})

var _ = DescribeTable("Route Delegation translator",
	func(inputFile string) {
		ctx := context.TODO()
		dir := util.MustGetThisDir()

		results, err := TestCase{
			Name:       inputFile,
			InputFiles: []string{filepath.Join(dir, "testutils/inputs/delegation", inputFile)},
			ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
				{
					Namespace: "infra",
					Name:      "example-gateway",
				}: {
					Proxy: filepath.Join(dir, "testutils/outputs/delegation", inputFile),
					// Reports:     nil,
				},
			},
		}.Run(ctx)

		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(HaveLen(1))
		ExpectKeyWithNoDiff(results, types.NamespacedName{
			Namespace: "infra",
			Name:      "example-gateway",
		})
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
)
