package translator_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
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
			rt5 = buildRouteTableWithDelegateAction("rt-5", "ns-4", "/foo", nil,
				&v1.RouteTableSelector{
					Labels:     map[string]string{"team": "dev"},
					Namespaces: []string{"ns-1", "ns-5"},
				})
			rt6 = buildRouteTableWithSimpleAction("rt-6", "ns-5", "/foo/6", map[string]string{"team": "dev"})

			allRouteTables = v1.RouteTableList{rt1, rt2, rt3, rt4, rt5, rt6}

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

			Entry("when no labels nor namespaces are provided",
				toAction(&v1.RouteTableSelector{}),
				v1.RouteTableList{rt1, rt2},
			),

			Entry("when a label is specified in the selector (but no namespace)",
				toAction(&v1.RouteTableSelector{
					Labels: map[string]string{"foo": "bar"},
				}),
				v1.RouteTableList{rt2},
			),

			Entry("when namespaces are specified in the selector (but no labels)",
				toAction(&v1.RouteTableSelector{
					Namespaces: []string{"ns-1", "ns-2"},
				}),
				v1.RouteTableList{rt1, rt2, rt3},
			),

			Entry("when both namespaces and labels are specified in the selector",
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
				v1.RouteTableList{rt1, rt2, rt3, rt4, rt5, rt6},
			),
		)
	})
})
