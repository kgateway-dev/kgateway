package translator_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/go-utils/testutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("RouteTableSelector", func() {

	When("missing ref on a delegate route", func() {
		It("returns nil and adds a warning", func() {
			ref := core.ResourceRef{
				Name: "any",
			}

			rtSelector := translator.NewRouteTableSelector(nil)
			list, err := rtSelector.SelectRouteTables(&v1.DelegateAction{
				DelegationType: &v1.DelegateAction_Ref{
					Ref: &ref,
				},
			}, "")

			Expect(list).To(HaveLen(0))
			Expect(err).To(HaveOccurred())
			Expect(err).To(testutils.HaveInErrorChain(translator.RouteTableMissingWarning(ref)))
		})
	})

	When("missing ref on a delegate route", func() {
		It("returns nil and adds a warning", func() {
			action := &v1.DelegateAction{
				DelegationType: &v1.DelegateAction_Ref{},
			}

			rtSelector := translator.NewRouteTableSelector(nil)
			list, err := rtSelector.SelectRouteTables(action, "")

			Expect(list).To(HaveLen(0))
			Expect(err).To(HaveOccurred())
			Expect(err).To(testutils.HaveInErrorChain(translator.MissingRefAndSelectorWarning))
		})
	})

	When("expressions and labels are both specified in the selector", func() {
		It("returns nil and adds a warning", func() {
			selector := &v1.RouteTableSelector{
				Expressions: []*v1.RouteTableSelector_Expression{
					{
						Key:      "foo",
						Operator: v1.RouteTableSelector_Expression_In,
						Values: []string{
							"bar",
							"baz",
						},
					},
				},
				Labels: map[string]string{"team": "dev", "foo": "bar"},
			}

			rtSelector := translator.NewRouteTableSelector(nil)
			list, err := rtSelector.SelectRouteTables(&v1.DelegateAction{
				DelegationType: &v1.DelegateAction_Selector{
					Selector: selector,
				},
			}, "")

			Expect(list).To(HaveLen(0))
			Expect(err).To(HaveOccurred())
			Expect(err).To(testutils.HaveInErrorChain(translator.RouteTableSelectorExpressionsAndLabelsWarning))
		})
	})

	When("selector has no matches", func() {
		It("returns the appropriate warning", func() {
			action := &v1.DelegateAction{
				DelegationType: &v1.DelegateAction_Selector{
					Selector: &v1.RouteTableSelector{
						Labels:     map[string]string{"team": "dev", "foo": "bar"},
						Namespaces: []string{"*"},
					},
				},
			}

			rtSelector := translator.NewRouteTableSelector(v1.RouteTableList{
				buildRouteTableWithSimpleAction("rt-1", "ns-1", "/foo/1", nil),
				buildRouteTableWithSimpleAction("rt-2", "ns-1", "/foo/2", map[string]string{"foo": "bar"}),
			})

			list, err := rtSelector.SelectRouteTables(action, "")

			Expect(err).To(HaveOccurred())
			Expect(list).To(HaveLen(0))
			Expect(err).To(testutils.HaveInErrorChain(translator.NoMatchingRouteTablesWarning))
		})
	})

	When("selector has matches", func() {

		var (
			rt1 = buildRouteTableWithSimpleAction("rt-1", "ns-1", "/foo/1", nil)
			rt2 = buildRouteTableWithSimpleAction("rt-2", "ns-1", "/foo/2", map[string]string{"foo": "bar", "team": "dev"})
			rt3 = buildRouteTableWithSimpleAction("rt-3", "ns-2", "/foo/3", map[string]string{"foo": "bar"})
			rt4 = buildRouteTableWithSimpleAction("rt-4", "ns-3", "/foo/4", map[string]string{"foo": "baz"})
			rt5 = buildRouteTableWithSelector("rt-5", "ns-4", "/foo", nil,
				&v1.RouteTableSelector{
					Labels:     map[string]string{"team": "dev"},
					Namespaces: []string{"ns-1", "ns-5"},
				})
			rt6 = buildRouteTableWithSimpleAction("rt-6", "ns-5", "/foo/6", map[string]string{"team": "dev"})
			rt7 = buildRouteTableWithSimpleAction("rt-7", "ns-1", "/foo/7", map[string]string{"complex.key-label/threshold": "10"})
			rt8 = buildRouteTableWithSimpleAction("rt-8", "ns-2", "/foo/8", map[string]string{"complex.key-label/threshold": "20", "team": "ops"})

			allRouteTables = v1.RouteTableList{rt1, rt2, rt3, rt4, rt5, rt6, rt7, rt8}

			toAction = func(selector *v1.RouteTableSelector) *v1.DelegateAction {
				return &v1.DelegateAction{
					DelegationType: &v1.DelegateAction_Selector{
						Selector: selector,
					},
				}
			}
		)

		DescribeTable("selector works as expected",
			func(action *v1.DelegateAction, expectedRouteTables v1.RouteTableList) {

				parentNamespace := "ns-1"

				rtSelector := translator.NewRouteTableSelector(allRouteTables)
				list, err := rtSelector.SelectRouteTables(action, parentNamespace)

				Expect(err).NotTo(HaveOccurred())
				Expect(list).To(HaveLen(len(expectedRouteTables)))
				Expect(list).To(BeEquivalentTo(expectedRouteTables))
			},

			Entry("when no labels nor namespaces nor expressions are provided",
				toAction(&v1.RouteTableSelector{}),
				v1.RouteTableList{rt1, rt2, rt7},
			),

			Entry("when a label is specified in the selector (but no namespace nor expressions)",
				toAction(&v1.RouteTableSelector{
					Labels: map[string]string{"foo": "bar"},
				}),
				v1.RouteTableList{rt2},
			),

			Entry("when namespaces are specified in the selector (but no labels nor expressions)",
				toAction(&v1.RouteTableSelector{
					Namespaces: []string{"ns-1", "ns-2"},
				}),
				v1.RouteTableList{rt1, rt2, rt3, rt7, rt8},
			),

			Entry("when both namespaces and labels are specified in the selector (but no expressions)",
				toAction(&v1.RouteTableSelector{
					Labels:     map[string]string{"foo": "bar"},
					Namespaces: []string{"ns-2"},
				}),
				v1.RouteTableList{rt3},
			),

			Entry("when selector contains 'all namespaces' wildcard selector (*)",
				toAction(&v1.RouteTableSelector{
					Namespaces: []string{"ns-1", "*"},
				}),
				allRouteTables,
			),

			Entry("when an expression is specified in the selector (but no namespace or labels)",
				toAction(&v1.RouteTableSelector{
					Expressions: []*v1.RouteTableSelector_Expression{
						{
							Key:      "foo",
							Operator: v1.RouteTableSelector_Expression_In,
							Values:   []string{"bar"},
						},
					},
				}),
				v1.RouteTableList{rt2},
			),

			Entry("when an expression (in operator) and namespace are specified in the selector (but no labels)",
				toAction(&v1.RouteTableSelector{
					Namespaces: []string{"*"},
					Expressions: []*v1.RouteTableSelector_Expression{
						{
							Key:      "foo",
							Operator: v1.RouteTableSelector_Expression_In,
							Values:   []string{"bar"},
						},
					},
				}),
				v1.RouteTableList{rt2, rt3},
			),

			Entry("when an expression (notin operator) and namespace are specified in the selector (but no labels)",
				toAction(&v1.RouteTableSelector{
					Namespaces: []string{"*"},
					Expressions: []*v1.RouteTableSelector_Expression{
						{
							Key:      "foo",
							Operator: v1.RouteTableSelector_Expression_NotIn,
							Values:   []string{"bar"},
						},
					},
				}),
				v1.RouteTableList{rt1, rt4, rt5, rt6, rt7, rt8},
			),

			Entry("when an expression (exists operator) and a namespace are specified in the selector (but no labels)",
				toAction(&v1.RouteTableSelector{
					Namespaces: []string{"*"},
					Expressions: []*v1.RouteTableSelector_Expression{
						{
							Key:      "foo",
							Operator: v1.RouteTableSelector_Expression_Exists,
						},
					},
				}),
				v1.RouteTableList{rt2, rt3, rt4},
			),

			Entry("when an expression (greaterThan operator) and a namespace are specified in the selector (but no labels)",
				toAction(&v1.RouteTableSelector{
					Namespaces: []string{"*"},
					Expressions: []*v1.RouteTableSelector_Expression{
						{
							Key:      "complex.key-label/threshold",
							Operator: v1.RouteTableSelector_Expression_GreaterThan,
							Values:   []string{"15"},
						},
					},
				}),
				v1.RouteTableList{rt8},
			),

			Entry("when an expression (LessThan operator) and a namespace are specified in the selector (but no labels)",
				toAction(&v1.RouteTableSelector{
					Namespaces: []string{"*"},
					Expressions: []*v1.RouteTableSelector_Expression{
						{
							Key:      "complex.key-label/threshold",
							Operator: v1.RouteTableSelector_Expression_LessThan,
							Values:   []string{"15"},
						},
					},
				}),
				v1.RouteTableList{rt7},
			),

			Entry("when an expression (DoesNotExists operator) and a namespace are specified in the selector (but no labels)",
				toAction(&v1.RouteTableSelector{
					Namespaces: []string{"*"},
					Expressions: []*v1.RouteTableSelector_Expression{
						{
							Key:      "complex.key-label/threshold",
							Operator: v1.RouteTableSelector_Expression_DoesNotExist,
						},
					},
				}),
				v1.RouteTableList{rt1, rt2, rt3, rt4, rt5, rt6},
			),

			Entry("when multiple expressions and a namespace are specified in the selector (but no labels)",
				toAction(&v1.RouteTableSelector{
					Namespaces: []string{"*"},
					Expressions: []*v1.RouteTableSelector_Expression{
						{
							Key:      "complex.key-label/threshold",
							Operator: v1.RouteTableSelector_Expression_GreaterThan,
							Values:   []string{"5"},
						},
						{
							Key:      "team",
							Operator: v1.RouteTableSelector_Expression_In,
							Values:   []string{"dev", "ops"},
						},
						{
							Key:      "bar",
							Operator: v1.RouteTableSelector_Expression_NotEquals,
							Values:   []string{"baz"},
						},
					},
				}),
				v1.RouteTableList{rt8},
			),
		)
	})
})
