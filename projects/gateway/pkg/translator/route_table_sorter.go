package translator

import (
	"sort"
	"strings"

	errors "github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
)

var RouteTablesWithSameWeightErr = func(tables v1.RouteTableList, weight int32) error {
	return errors.Errorf("the following route tables have the same weight (%d): [%s]. This can result in "+
		"unintended ordering of the resulting routes on the Proxy resource", weight, collectNames(tables))
}

type RouteTableSorter interface {
	// Indexes the given route tables by weight and returns them as a map.
	// The map key set is also returned as a sorted array so the client can range over the map in the desired order.
	// The error slice contain warning about route tables with duplicated weights.
	IndexByWeight(routeTables v1.RouteTableList) (map[int32]v1.RouteTableList, []int32, []error)
}

func NewRouteTableSorter() RouteTableSorter {
	return &sorter{}
}

type sorter struct{}

func (s *sorter) IndexByWeight(routeTables v1.RouteTableList) (map[int32]v1.RouteTableList, []int32, []error) {

	// Index by weight
	byWeight := map[int32]v1.RouteTableList{}
	for _, rt := range routeTables {
		if rt.Weight == nil {
			// Just to be safe, handle nil weights
			byWeight[defaultTableWeight] = append(byWeight[defaultTableWeight], rt)
		} else {
			byWeight[rt.Weight.Value] = append(byWeight[rt.Weight.Value], rt)
		}
	}

	// Warn if multiple tables have the same weight
	var warnings []error
	for weight, tablesForWeight := range byWeight {
		if len(tablesForWeight) > 1 {
			warnings = append(warnings, RouteTablesWithSameWeightErr(tablesForWeight, weight))
		}
	}

	// Collect and sort weights
	var sortedWeights []int32
	for weight := range byWeight {
		sortedWeights = append(sortedWeights, weight)
	}
	sort.SliceStable(sortedWeights, func(i, j int) bool { return sortedWeights[i] < sortedWeights[j] })

	return byWeight, sortedWeights, warnings
}

func collectNames(routeTables v1.RouteTableList) string {
	var names []string
	for _, t := range routeTables {
		names = append(names, t.Metadata.Ref().Key())
	}
	return strings.Join(names, ", ")
}
