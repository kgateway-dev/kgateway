package translator_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/skv2/codegen/util"
	"k8s.io/apimachinery/pkg/types"
)

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
		Expect(results).To(HaveKey(types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		}))
		Expect(results[types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		}]).To(BeTrue())
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
		Expect(results).To(HaveKey(types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		}))
		Expect(results[types.NamespacedName{
			Namespace: "default",
			Name:      "example-gateway",
		}]).To(BeTrue())
	})

	It("should translate an http gateway with multiple routing rules and use the HeaderModifier filter", func() {
		results, err := TestCase{
			Name:       "http-routing-with-header-modifier-filter",
			InputFiles: []string{dir + "/testutils/inputs/http-with-header-modifier"},
			ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
				{
					Namespace: "default",
					Name:      "gw",
				}: {
					Proxy: dir + "/testutils/outputs/http-with-header-modifier-proxy.yaml",
					// Reports:     nil,
				},
			},
		}.Run(ctx)

		Expect(err).NotTo(HaveOccurred())
		Expect(results).To(HaveLen(1))
		Expect(results[types.NamespacedName{
			Namespace: "default",
			Name:      "gw",
		}]).To(BeTrue())
	})

	Context("route delegation", func() {
		It("should translate a basic configuration", func() {
			results, err := TestCase{
				Name:       "delegation-basic",
				InputFiles: []string{dir + "/testutils/inputs/delegation/basic.yaml"},
				ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
					{
						Namespace: "infra",
						Name:      "example-gateway",
					}: {
						Proxy: dir + "/testutils/outputs/delegation/basic.yaml",
						// Reports:     nil,
					},
				},
			}.Run(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results).To(HaveKey(types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}))
			Expect(results[types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}]).To(BeTrue())
		})

		It("should translate a basic configuration with child specifying matching parentRefs", func() {
			results, err := TestCase{
				Name:       "delegation-parentRefs-match",
				InputFiles: []string{dir + "/testutils/inputs/delegation/basic_parentref_match.yaml"},
				ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
					{
						Namespace: "infra",
						Name:      "example-gateway",
					}: {
						Proxy: dir + "/testutils/outputs/delegation/basic.yaml",
						// Reports:     nil,
					},
				},
			}.Run(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results).To(HaveKey(types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}))
			Expect(results[types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}]).To(BeTrue())
		})

		It("should translate a basic configuration with child specifying mismatched parentRefs", func() {
			results, err := TestCase{
				Name:       "delegation-parentRefs-mismatch",
				InputFiles: []string{dir + "/testutils/inputs/delegation/basic_parentref_mismatch.yaml"},
				ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
					{
						Namespace: "infra",
						Name:      "example-gateway",
					}: {
						Proxy: dir + "/testutils/outputs/delegation/basic_parentref_mismatch.yaml",
						// Reports:     nil,
					},
				},
			}.Run(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results).To(HaveKey(types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}))
			Expect(results[types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}]).To(BeTrue())
		})

		It("should translate a configuration with multiple child routes", func() {
			results, err := TestCase{
				Name:       "delegation-multiple-children",
				InputFiles: []string{dir + "/testutils/inputs/delegation/multiple_children.yaml"},
				ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
					{
						Namespace: "infra",
						Name:      "example-gateway",
					}: {
						Proxy: dir + "/testutils/outputs/delegation/multiple_children.yaml",
						// Reports:     nil,
					},
				},
			}.Run(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results).To(HaveKey(types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}))
			Expect(results[types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}]).To(BeTrue())
		})

		It("should detect invalid hostname on a delegatee route", func() {
			results, err := TestCase{
				Name:       "delegation-invalid-child-hostname",
				InputFiles: []string{dir + "/testutils/inputs/delegation/basic_invalid_hostname.yaml"},
				ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
					{
						Namespace: "infra",
						Name:      "example-gateway",
					}: {
						Proxy: dir + "/testutils/outputs/delegation/basic_invalid_hostname.yaml",
						// Reports:     nil,
					},
				},
			}.Run(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results).To(HaveKey(types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}))
			Expect(results[types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}]).To(BeTrue())
		})

		It("should translate a recursive configuration", func() {
			results, err := TestCase{
				Name:       "delegation-recursive",
				InputFiles: []string{dir + "/testutils/inputs/delegation/recursive.yaml"},
				ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
					{
						Namespace: "infra",
						Name:      "example-gateway",
					}: {
						Proxy: dir + "/testutils/outputs/delegation/recursive.yaml",
						// Reports:     nil,
					},
				},
			}.Run(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results).To(HaveKey(types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}))
			Expect(results[types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}]).To(BeTrue())
		})

		It("should not translate a cyclic child route", func() {
			results, err := TestCase{
				Name:       "delegation-cyclic",
				InputFiles: []string{dir + "/testutils/inputs/delegation/cyclic1.yaml"},
				ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
					{
						Namespace: "infra",
						Name:      "example-gateway",
					}: {
						Proxy: dir + "/testutils/outputs/delegation/cyclic1.yaml",
						// Reports:     nil,
					},
				},
			}.Run(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results).To(HaveKey(types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}))
			Expect(results[types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}]).To(BeTrue())
		})

		It("should not translate a recursive cyclic child route", func() {
			results, err := TestCase{
				Name:       "delegation-recursive-cyclic",
				InputFiles: []string{dir + "/testutils/inputs/delegation/cyclic2.yaml"},
				ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
					{
						Namespace: "infra",
						Name:      "example-gateway",
					}: {
						Proxy: dir + "/testutils/outputs/delegation/cyclic2.yaml",
						// Reports:     nil,
					},
				},
			}.Run(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results).To(HaveKey(types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}))
			Expect(results[types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}]).To(BeTrue())
		})

		It("should translate only matching child routes", func() {
			results, err := TestCase{
				Name:       "delegation-rule-matcher",
				InputFiles: []string{dir + "/testutils/inputs/delegation/child_rule_matcher.yaml"},
				ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
					{
						Namespace: "infra",
						Name:      "example-gateway",
					}: {
						Proxy: dir + "/testutils/outputs/delegation/child_rule_matcher.yaml",
						// Reports:     nil,
					},
				},
			}.Run(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results).To(HaveKey(types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}))
			Expect(results[types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}]).To(BeTrue())
		})

		It("should translate delegatee routes with multiple parents", func() {
			results, err := TestCase{
				Name:       "delegation-multiple-parents",
				InputFiles: []string{dir + "/testutils/inputs/delegation/multiple_parents.yaml"},
				ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
					{
						Namespace: "infra",
						Name:      "example-gateway",
					}: {
						Proxy: dir + "/testutils/outputs/delegation/multiple_parents.yaml",
						// Reports:     nil,
					},
				},
			}.Run(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results).To(HaveKey(types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}))
			Expect(results[types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}]).To(BeTrue())
		})

		It("should translate a route that is valid standalone but invalid as a delegatee", func() {
			results, err := TestCase{
				Name:       "delegation-invalid-child-valid-standalone",
				InputFiles: []string{dir + "/testutils/inputs/delegation/invalid_child_valid_standalone.yaml"},
				ResultsByGateway: map[types.NamespacedName]ExpectedTestResult{
					{
						Namespace: "infra",
						Name:      "example-gateway",
					}: {
						Proxy: dir + "/testutils/outputs/delegation/invalid_child_valid_standalone.yaml",
						// Reports:     nil,
					},
				},
			}.Run(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))
			Expect(results).To(HaveKey(types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}))
			Expect(results[types.NamespacedName{
				Namespace: "infra",
				Name:      "example-gateway",
			}]).To(BeTrue())
		})
	})
})
