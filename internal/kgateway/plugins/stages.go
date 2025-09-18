package plugins

import (
	sdkfilters "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/filters"
)

// The set of WellKnownFilterStages, whose order corresponds to the order used to sort filters
// If new well known filter stages are added, they should be inserted in a position corresponding to their order
const (
	FaultStage     = sdkfilters.FaultStage
	CorsStage      = sdkfilters.CorsStage
	WafStage       = sdkfilters.WafStage
	AuthNStage     = sdkfilters.AuthNStage
	AuthZStage     = sdkfilters.AuthZStage
	RateLimitStage = sdkfilters.RateLimitStage
	AcceptedStage  = sdkfilters.AcceptedStage
	OutAuthStage   = sdkfilters.OutAuthStage
	RouteStage     = sdkfilters.RouteStage
)

type (
	WellKnownFilterStage             = sdkfilters.WellKnownFilterStage
	WellKnownUpstreamHTTPFilterStage = sdkfilters.WellKnownUpstreamHTTPFilterStage
	StagedHttpFilter                 = sdkfilters.StagedHttpFilter
	StagedNetworkFilter              = sdkfilters.StagedNetworkFilter
	HTTPOrNetworkFilterStage         = sdkfilters.HTTPOrNetworkFilterStage
	StagedNetworkFilterList          = sdkfilters.StagedNetworkFilterList
	StagedHttpFilterList             = sdkfilters.StagedHttpFilterList
	StagedUpstreamHttpFilter         = sdkfilters.StagedUpstreamHttpFilter
	HTTPFilterStage                  = sdkfilters.HTTPFilterStage
)

var (
	// function alias
	NewStagedFilter     = sdkfilters.NewStagedFilter
	MustNewStagedFilter = sdkfilters.MustNewStagedFilter
)

// The set of WellKnownUpstreamHTTPFilterStages, whose order corresponds to the order used to sort filters
// If new well known filter stages are added, they should be inserted in a position corresponding to their order
const (
	TransformationStage WellKnownUpstreamHTTPFilterStage = sdkfilters.TransformationStage
)

// FilterStageComparison helps implement the sort.Interface Less function for use in other implementations of sort.Interface
// returns -1 if less than, 0 if equal, 1 if greater than
// It is not sufficient to return a Less bool because calling functions need to know if equal or greater when Less is false
func FilterStageComparison[WellKnown ~int](a, b sdkfilters.FilterStage[WellKnown]) int {
	return sdkfilters.FilterStageComparison(a, b)
}

func BeforeStage[WellKnown ~int](wellKnown WellKnown) sdkfilters.FilterStage[WellKnown] {
	return RelativeToStage(wellKnown, -1)
}

func DuringStage[WellKnown ~int](wellKnown WellKnown) sdkfilters.FilterStage[WellKnown] {
	return RelativeToStage(wellKnown, 0)
}

func AfterStage[WellKnown ~int](wellKnown WellKnown) sdkfilters.FilterStage[WellKnown] {
	return RelativeToStage(wellKnown, 1)
}

func RelativeToStage[WellKnown ~int](wellKnown WellKnown, relativeWeight int) sdkfilters.FilterStage[WellKnown] {
	return sdkfilters.FilterStage[WellKnown]{
		RelativeTo:     wellKnown,
		RelativeWeight: relativeWeight,
	}
}
