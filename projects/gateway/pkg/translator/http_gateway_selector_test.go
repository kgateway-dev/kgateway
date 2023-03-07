package translator_test

import (
	"github.com/golang/protobuf/ptypes/duration"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/core/selectors"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("HttpGatewaySelector", func() {

	var (
		httpGatewaySelector translator.HttpGatewaySelector
		onSelectionError    func(err error)

		availableHttpGateways v1.MatchableHttpGatewayList
	)

	JustBeforeEach(func() {
		// Instantiate the HttpGatewaySelector in the JustBeforeEach to enable each sub-context
		// to modify the availableHttpGateways
		httpGatewaySelector = translator.NewHttpGatewaySelector(availableHttpGateways)

		// We do not expect a selection error to occur. To be extra certain, panic if one does
		onSelectionError = func(err error) {
			panic(err)
		}
	})

	Context("ref", func() {

		BeforeEach(func() {
			availableHttpGateways = v1.MatchableHttpGatewayList{
				createMatchableHttpGateway("name", "ns", nil),
			}
		})

		It("selects gw with matching ref", func() {
			selector := &v1.DelegatedHttpGateway{
				SelectionType: &v1.DelegatedHttpGateway_Ref{
					Ref: &core.ResourceRef{
						Name:      "name",
						Namespace: "ns",
					},
				},
			}
			selected := httpGatewaySelector.SelectMatchableHttpGateways(selector, onSelectionError)
			Expect(selected).To(HaveLen(1))
		})

		It("does not select gw with mismatched ref", func() {
			selector := &v1.DelegatedHttpGateway{
				SelectionType: &v1.DelegatedHttpGateway_Ref{
					Ref: &core.ResourceRef{
						Name:      "invalid-name",
						Namespace: "invalid-namespace",
					},
				},
			}
			selected := httpGatewaySelector.SelectMatchableHttpGateways(selector, onSelectionError)
			Expect(selected).To(HaveLen(0))
		})

	})

	Context("selector", func() {

		var (
			gw1 = createMatchableHttpGateway("gw-1", "ns-1", nil)
			gw2 = createMatchableHttpGateway("gw-2", "ns-1", map[string]string{"team": "edge"})
			gw3 = createMatchableHttpGateway("gw-3", "ns-1", map[string]string{"team": "mesh"})
			gw4 = createMatchableHttpGateway("gw-4", "ns-2", nil)
			gw5 = createMatchableHttpGateway("gw-5", "ns-2", map[string]string{"team": "edge"})
			gw6 = createMatchableHttpGateway("gw-6", "ns-2", map[string]string{"team": "mesh"})
			gw7 = createMatchableHttpGateway("gw-7", "ns-1", map[string]string{"team": "edge", "priority": "10"})
			gw8 = createMatchableHttpGateway("gw-8", "ns-2", map[string]string{"team": "edge", "priority": "15"})

			fullGatewayList = v1.MatchableHttpGatewayList{gw1, gw2, gw3, gw4, gw5, gw6, gw7, gw8}
		)

		BeforeEach(func() {
			availableHttpGateways = fullGatewayList
		})

		DescribeTable("selector works as expected",
			func(selector *selectors.Selector, expectedGatewayList v1.MatchableHttpGatewayList) {
				gatewaySelector := &v1.DelegatedHttpGateway{
					SelectionType: &v1.DelegatedHttpGateway_Selector{
						Selector: selector,
					},
				}

				actualGatewayList := httpGatewaySelector.SelectMatchableHttpGateways(gatewaySelector, onSelectionError)

				Expect(actualGatewayList).To(HaveLen(len(expectedGatewayList)))
				Expect(actualGatewayList).To(BeEquivalentTo(expectedGatewayList))
			},

			Entry("when no labels nor namespaces nor expressions are provided",
				&selectors.Selector{},
				fullGatewayList,
			),

			Entry("when a label is specified in the selector (but no namespace nor expressions)",
				&selectors.Selector{
					Labels: map[string]string{"team": "edge"},
				},
				v1.MatchableHttpGatewayList{gw2, gw5, gw7, gw8},
			),

			Entry("when namespaces are specified in the selector (but no labels nor expressions)",
				&selectors.Selector{
					Namespaces: []string{"ns-1", "ns-2"},
				},
				fullGatewayList,
			),

			Entry("when both namespaces and labels are specified in the selector (but no expressions)",
				&selectors.Selector{
					Labels:     map[string]string{"team": "edge"},
					Namespaces: []string{"ns-2"},
				},
				v1.MatchableHttpGatewayList{gw5, gw8},
			),

			Entry("when selector contains 'all namespaces' wildcard selector (*)",
				&selectors.Selector{
					Namespaces: []string{"ns-1", "*"},
				},
				fullGatewayList,
			),

			Entry("when an expression is specified in the selector (but no namespace or labels)",
				&selectors.Selector{
					Expressions: []*selectors.Selector_Expression{
						{
							Key:      "team",
							Operator: selectors.Selector_Expression_In,
							Values:   []string{"edge"},
						},
					},
				},
				v1.MatchableHttpGatewayList{gw2, gw5, gw7, gw8},
			),

			Entry("when an expression (in operator) and namespace are specified in the selector (but no labels)",
				&selectors.Selector{
					Expressions: []*selectors.Selector_Expression{
						{
							Key:      "team",
							Operator: selectors.Selector_Expression_In,
							Values:   []string{"edge", "mesh"},
						},
					},
					Namespaces: []string{"ns-1"},
				},
				v1.MatchableHttpGatewayList{gw2, gw3, gw7},
			),

			Entry("when an expression (notin operator) and namespace are specified in the selector (but no labels)",
				&selectors.Selector{
					Expressions: []*selectors.Selector_Expression{
						{
							Key:      "team",
							Operator: selectors.Selector_Expression_NotIn,
							Values:   []string{"edge", "mesh"},
						},
					},
					Namespaces: []string{"ns-1"},
				},
				v1.MatchableHttpGatewayList{gw1},
			),

			Entry("when an expression (exists operator) and a namespace are specified in the selector (but no labels)",
				&selectors.Selector{
					Expressions: []*selectors.Selector_Expression{
						{
							Key:      "team",
							Operator: selectors.Selector_Expression_Exists,
						},
					},
					Namespaces: []string{"ns-1"},
				},
				v1.MatchableHttpGatewayList{gw2, gw3, gw7},
			),

			Entry("when an expression (DoesNotExists operator) and a namespace are specified in the selector (but no labels)",
				&selectors.Selector{
					Expressions: []*selectors.Selector_Expression{
						{
							Key:      "team",
							Operator: selectors.Selector_Expression_DoesNotExist,
						},
					},
					Namespaces: []string{"ns-1"},
				},
				v1.MatchableHttpGatewayList{gw1},
			),

			Entry("when an expression (greaterThan operator) and a namespace are specified in the selector (but no labels)",
				&selectors.Selector{
					Expressions: []*selectors.Selector_Expression{
						{
							Key:      "priority",
							Operator: selectors.Selector_Expression_GreaterThan,
							Values:   []string{"5"},
						},
					},
					Namespaces: []string{"ns-1"},
				},
				v1.MatchableHttpGatewayList{gw7},
			),

			Entry("when an expression (LessThan operator) and a namespace are specified in the selector (but no labels)",
				&selectors.Selector{
					Expressions: []*selectors.Selector_Expression{
						{
							Key:      "priority",
							Operator: selectors.Selector_Expression_LessThan,
							Values:   []string{"20"},
						},
					},
					Namespaces: []string{"ns-2"},
				},
				v1.MatchableHttpGatewayList{gw8},
			),

			Entry("when multiple expressions and a namespace are specified in the selector (but no labels)",
				&selectors.Selector{
					Namespaces: []string{"*"},
					Expressions: []*selectors.Selector_Expression{
						{
							Key:      "priority",
							Operator: selectors.Selector_Expression_LessThan,
							Values:   []string{"11"},
						},
						{
							Key:      "team",
							Operator: selectors.Selector_Expression_In,
							Values:   []string{"edge"},
						},
					},
				},
				v1.MatchableHttpGatewayList{gw7},
			),
		)

	})

	Context("ssl", func() {

		var sslConfig = &ssl.SslConfig{
			TransportSocketConnectTimeout: &duration.Duration{
				Seconds: 30,
			},
		}

		BeforeEach(func() {
			availableHttpGateways = v1.MatchableHttpGatewayList{
				createSecureMatchableHttpGateway("name", "ns", sslConfig),
			}
		})

		It("selects gw with matching ns if ssl", func() {
			selector := &v1.DelegatedHttpGateway{
				SslConfig: sslConfig,
				SelectionType: &v1.DelegatedHttpGateway_Ref{
					Ref: &core.ResourceRef{
						Name:      "name",
						Namespace: "ns",
					},
				},
			}
			selected := httpGatewaySelector.SelectMatchableHttpGateways(selector, onSelectionError)
			Expect(selected).To(HaveLen(1))
		})

		It("does not select gw with matching ns if ssl not defined", func() {
			selector := &v1.DelegatedHttpGateway{
				SslConfig: nil,
				SelectionType: &v1.DelegatedHttpGateway_Ref{
					Ref: &core.ResourceRef{
						Name:      "name",
						Namespace: "ns",
					},
				},
			}
			selected := httpGatewaySelector.SelectMatchableHttpGateways(selector, onSelectionError)
			Expect(selected).To(HaveLen(0))
		})
	})

})

func createMatchableHttpGateway(name, namespace string, labels map[string]string) *v1.MatchableHttpGateway {
	return &v1.MatchableHttpGateway{
		Metadata: &core.Metadata{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Matcher:     &v1.MatchableHttpGateway_Matcher{},
		HttpGateway: &v1.HttpGateway{},
	}
}

func createSecureMatchableHttpGateway(name, namespace string, sslConfig *ssl.SslConfig) *v1.MatchableHttpGateway {
	return &v1.MatchableHttpGateway{
		Metadata: &core.Metadata{
			Name:      name,
			Namespace: namespace,
		},
		Matcher: &v1.MatchableHttpGateway_Matcher{
			SslConfig: sslConfig,
		},
		HttpGateway: &v1.HttpGateway{},
	}
}
