package filters

// List of filter stages which can be selected for a HTTP filter.
type FilterStage_Stage int32

const (
	FilterStage_FaultStage     FilterStage_Stage = 0
	FilterStage_CorsStage      FilterStage_Stage = 1
	FilterStage_WafStage       FilterStage_Stage = 2
	FilterStage_AuthNStage     FilterStage_Stage = 3
	FilterStage_AuthZStage     FilterStage_Stage = 4
	FilterStage_RateLimitStage FilterStage_Stage = 5
	FilterStage_AcceptedStage  FilterStage_Stage = 6
	FilterStage_OutAuthStage   FilterStage_Stage = 7
	FilterStage_RouteStage     FilterStage_Stage = 8
)

// Enum value maps for FilterStage_Stage.
var (
	FilterStage_Stage_name = map[int32]string{
		0: "FaultStage",
		1: "CorsStage",
		2: "WafStage",
		3: "AuthNStage",
		4: "AuthZStage",
		5: "RateLimitStage",
		6: "AcceptedStage",
		7: "OutAuthStage",
		8: "RouteStage",
	}
	FilterStage_Stage_value = map[string]int32{
		"FaultStage":     0,
		"CorsStage":      1,
		"WafStage":       2,
		"AuthNStage":     3,
		"AuthZStage":     4,
		"RateLimitStage": 5,
		"AcceptedStage":  6,
		"OutAuthStage":   7,
		"RouteStage":     8,
	}
)

// Desired placement of the HTTP filter relative to the stage. The default is `During`.
type FilterStage_Predicate int32

const (
	FilterStage_During FilterStage_Predicate = 0
	FilterStage_Before FilterStage_Predicate = 1
	FilterStage_After  FilterStage_Predicate = 2
)

// Enum value maps for FilterStage_Predicate.
var (
	FilterStage_Predicate_name = map[int32]string{
		0: "During",
		1: "Before",
		2: "After",
	}
	FilterStage_Predicate_value = map[string]int32{
		"During": 0,
		"Before": 1,
		"After":  2,
	}
)

// FilterStage allows configuration of where in a filter chain a given HTTP filter is inserted,
// relative to one of the pre-defined stages.
type FilterStage struct {
	// Stage of the filter chain in which the selected filter should be added.
	Stage FilterStage_Stage `protobuf:"varint,1,opt,name=stage,proto3,enum=filters.gloo.solo.io.FilterStage_Stage" json:"stage,omitempty"`
	// How this filter should be placed relative to the stage.
	Predicate FilterStage_Predicate `protobuf:"varint,2,opt,name=predicate,proto3,enum=filters.gloo.solo.io.FilterStage_Predicate" json:"predicate,omitempty"`
}

func (x *FilterStage) GetStage() FilterStage_Stage {
	if x != nil {
		return x.Stage
	}
	return FilterStage_FaultStage
}

func (x *FilterStage) GetPredicate() FilterStage_Predicate {
	if x != nil {
		return x.Predicate
	}
	return FilterStage_During
}
