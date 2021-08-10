package translator

import (
	"sort"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
)

type RouteTableIndexer interface {
	// Indexes the given route tables by weight and returns them as a map.
	// The map key set is also returned as a sorted array so the client can range over the map in the desired order.
	// The error slice contain warning about route tables with duplicated weights.
	IndexByWeight(routeTables v1.RouteTableList) (map[int32]v1.RouteTableList, []int32)
}

func NewRouteTableIndexer() RouteTableIndexer {
	return &indexer{}
}

type indexer struct{}

func (i *indexer) IndexByWeight(routeTables v1.RouteTableList) (map[int32]v1.RouteTableList, []int32) {

	// Index by weight
	byWeight := map[int32]v1.RouteTableList{}
	for _, rt := range routeTables {
		if rt.GetWeight() == nil {
			// Just to be safe, handle nil weights
			byWeight[defaultTableWeight] = append(byWeight[defaultTableWeight], rt)
		} else {
			byWeight[rt.GetWeight().GetValue()] = append(byWeight[rt.GetWeight().GetValue()], rt)
		}
	}

	// Collect and sort weights
	var sortedWeights []int32
	for weight := range byWeight {
		sortedWeights = append(sortedWeights, weight)
	}
	sort.SliceStable(sortedWeights, func(i, j int) bool { return sortedWeights[i] < sortedWeights[j] })

	return byWeight, sortedWeights
}
