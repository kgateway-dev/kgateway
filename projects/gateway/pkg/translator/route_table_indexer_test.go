package translator_test

import (
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/defaults"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("RouteTableIndexer", func() {

	var (
		indexer translator.RouteTableIndexer

		noWeight1,
		noWeight2,
		noWeight3,
		weightMinus10,
		weightZero,
		weightTen,
		weightTwenty *v1.RouteTable

		routeTableWithWeight = func(weight *int32, name string) *v1.RouteTable {
			var w *wrappers.Int32Value
			if weight != nil {
				w = &wrappers.Int32Value{Value: *weight}
			}

			table := &v1.RouteTable{
				Metadata: &core.Metadata{
					Name:      name,
					Namespace: defaults.GlooSystem,
				},
				Weight: w,
				Routes: []*v1.Route{},
			}

			return table
		}
	)

	BeforeEach(func() {
		indexer = translator.NewRouteTableIndexer()

		minus10 := int32(-10)
		zero := int32(0)
		ten := int32(10)
		twenty := int32(20)

		noWeight1 = routeTableWithWeight(nil, "no-weight-1")
		noWeight2 = routeTableWithWeight(nil, "no-weight-2")
		noWeight3 = routeTableWithWeight(nil, "no-weight-3")
		weightMinus10 = routeTableWithWeight(&minus10, "minus-ten")
		weightZero = routeTableWithWeight(&zero, "zero")
		weightTen = routeTableWithWeight(&ten, "ten")
		weightTwenty = routeTableWithWeight(&twenty, "twenty")
	})

	When("an empty list is passed", func() {
		It("returns nothing", func() {
			byWeight, weights := indexer.IndexByWeight(nil)
			Expect(byWeight).To(BeEmpty())
			Expect(weights).To(BeEmpty())
		})
	})

	When("a single route table is passed", func() {
		It("returns the table", func() {
			byWeight, weights := indexer.IndexByWeight(v1.RouteTableList{noWeight1})
			Expect(weights).To(ConsistOf(BeEquivalentTo(0)))
			Expect(byWeight).To(ConsistOf(BeEquivalentTo(v1.RouteTableList{noWeight1})))
		})
	})

	Context("multiple route tables are passed", func() {

		When("no route tables have weights", func() {
			It("correctly indexes them", func() {
				tables := v1.RouteTableList{noWeight1, noWeight2, noWeight3}
				byWeight, weights := indexer.IndexByWeight(tables)

				Expect(weights).To(HaveLen(1))
				Expect(weights).To(ConsistOf(BeEquivalentTo(0)))

				Expect(byWeight).To(HaveLen(1))
				Expect(byWeight[0]).To(ConsistOf(noWeight1, noWeight2, noWeight3))
			})
		})

		When("all route tables have distinct weights", func() {
			It("correctly indexes them", func() {
				tables := v1.RouteTableList{weightTen, weightMinus10, weightTwenty, weightZero}
				byWeight, weights := indexer.IndexByWeight(tables)

				Expect(weights).To(HaveLen(4))
				Expect(weights).To(Equal([]int32{-10, 0, 10, 20}))

				Expect(byWeight).To(HaveLen(4))
				Expect(byWeight[-10]).To(ConsistOf(weightMinus10))
				Expect(byWeight[0]).To(ConsistOf(weightZero))
				Expect(byWeight[10]).To(ConsistOf(weightTen))
				Expect(byWeight[20]).To(ConsistOf(weightTwenty))
			})
		})

		When("some route tables have weights and others don't", func() {
			It("correctly indexes them", func() {
				tables := v1.RouteTableList{weightTen, noWeight1, weightMinus10, weightTwenty}
				byWeight, weights := indexer.IndexByWeight(tables)

				Expect(weights).To(HaveLen(4))
				Expect(weights).To(Equal([]int32{-10, 0, 10, 20}))

				Expect(byWeight).To(HaveLen(4))
				Expect(byWeight[-10]).To(ConsistOf(weightMinus10))
				Expect(byWeight[0]).To(ConsistOf(noWeight1))
				Expect(byWeight[10]).To(ConsistOf(weightTen))
				Expect(byWeight[20]).To(ConsistOf(weightTwenty))
			})
		})

		When("some route tables have the same weight", func() {
			It("correctly indexes them", func() {
				weightTenClone := proto.Clone(weightTen).(*v1.RouteTable)
				weightTenClone.Metadata.Name = "ten-dup"
				tables := v1.RouteTableList{weightZero, weightTen, weightTwenty, weightTenClone}

				byWeight, weights := indexer.IndexByWeight(tables)

				Expect(weights).To(HaveLen(3))
				Expect(weights).To(Equal([]int32{0, 10, 20}))

				Expect(byWeight).To(HaveLen(3))
				Expect(byWeight[0]).To(ConsistOf(weightZero))
				Expect(byWeight[10]).To(HaveLen(2))
				Expect(byWeight[10]).To(ConsistOf(weightTen, weightTenClone))
				Expect(byWeight[20]).To(ConsistOf(weightTwenty))
			})
		})

	})
})
