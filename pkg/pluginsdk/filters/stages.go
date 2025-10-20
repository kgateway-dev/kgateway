package filters

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	envoylistenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
)

var (
	_ sort.Interface = new(StagedHttpFilterList)
	_ sort.Interface = new(StagedNetworkFilterList)
)

// WellKnownFilterStages are represented by an integer that reflects their relative ordering
type WellKnownFilterStage int

// The set of WellKnownFilterStages, whose order corresponds to the order used to sort filters
// If new well known filter stages are added, they should be inserted in a position corresponding to their order
const (
	FaultStage     WellKnownFilterStage = iota // Fault injection // First Filter Stage
	CorsStage                                  // Cors stage
	WafStage                                   // Web application firewall stage
	AuthNStage                                 // Authentication stage
	AuthZStage                                 // Authorization stage
	RateLimitStage                             // Rate limiting stage
	AcceptedStage                              // Request passed all the checks and will be forwarded upstream
	OutAuthStage                               // Add auth for the upstream (i.e. aws λ)
	RouteStage                                 // Request is going to upstream // Last Filter Stage
)

type WellKnownUpstreamHTTPFilterStage int

// The set of WellKnownUpstreamHTTPFilterStages, whose order corresponds to the order used to sort filters
// If new well known filter stages are added, they should be inserted in a position corresponding to their order
const (
	TransformationStage WellKnownUpstreamHTTPFilterStage = iota // Transformation stage
)

// FilterStageComparison helps implement the sort.Interface Less function for use in other implementations of sort.Interface
// returns -1 if less than, 0 if equal, 1 if greater than
// It is not sufficient to return a Less bool because calling functions need to know if equal or greater when Less is false
func FilterStageComparison[WellKnown ~int](a, b FilterStage[WellKnown]) int {
	if a.RelativeTo < b.RelativeTo {
		return -1
	} else if a.RelativeTo > b.RelativeTo {
		return 1
	}
	if a.RelativeWeight < b.RelativeWeight {
		return -1
	} else if a.RelativeWeight > b.RelativeWeight {
		return 1
	}
	return 0
}

func BeforeStage[WellKnown ~int](wellKnown WellKnown) FilterStage[WellKnown] {
	return RelativeToStage(wellKnown, -1)
}

func DuringStage[WellKnown ~int](wellKnown WellKnown) FilterStage[WellKnown] {
	return RelativeToStage(wellKnown, 0)
}

func AfterStage[WellKnown ~int](wellKnown WellKnown) FilterStage[WellKnown] {
	return RelativeToStage(wellKnown, 1)
}

// RelativeToStage creates a FilterStage that is relative to a well-known stage by a given weight
func RelativeToStage[WellKnown ~int](wellKnown WellKnown, relativeWeight int) FilterStage[WellKnown] {
	return FilterStage[WellKnown]{
		RelativeTo:     wellKnown,
		RelativeWeight: relativeWeight,
	}
}

type FilterStage[WellKnown ~int] struct {
	RelativeTo     WellKnown
	RelativeWeight int
}

type (
	HTTPOrNetworkFilterStage = FilterStage[WellKnownFilterStage]
	HTTPFilterStage          = FilterStage[WellKnownFilterStage]
	NetworkFilterStage       = FilterStage[WellKnownFilterStage]
	UpstreamHTTPFilterStage  = FilterStage[WellKnownUpstreamHTTPFilterStage]
)

type Filter interface {
	proto.Message
	GetName() string
	GetTypedConfig() *anypb.Any
}

type StagedFilter[WellKnown ~int, FilterType Filter] struct {
	Filter FilterType
	Stage  FilterStage[WellKnown]
	Weight int32
}

type StagedFilterList[WellKnown ~int, FilterType Filter] []StagedFilter[WellKnown, FilterType]

func (s StagedFilterList[WellKnown, FilterType]) Len() int {
	return len(s)
}

// filters by Relative Stage, Weighting, Name, Config Type-Url, Config Value, and (to ensure stability) index.
// The assumption is that if two filters are in the same stage, their order doesn't matter, and we
// just need to make sure it is stable.
func (s StagedFilterList[WellKnown, FilterType]) Less(i, j int) bool {
	if compare := FilterStageComparison(s[i].Stage, s[j].Stage); compare != 0 {
		return compare < 0
	}

	// If the filters are of the same type, compare their weights. Higher weights are ordered
	// before lower weights.
	if s[i].Filter.GetTypedConfig().GetTypeUrl() == s[j].Filter.GetTypedConfig().GetTypeUrl() {
		if s[i].Weight > s[j].Weight {
			return true
		} else if s[i].Weight < s[j].Weight {
			return false
		}
	}

	if compare := strings.Compare(s[i].Filter.GetName(), s[j].Filter.GetName()); compare != 0 {
		return compare < 0
	}

	if compare := strings.Compare(s[i].Filter.GetTypedConfig().GetTypeUrl(), s[j].Filter.GetTypedConfig().GetTypeUrl()); compare != 0 {
		return compare < 0
	}

	if compare := bytes.Compare(s[i].Filter.GetTypedConfig().GetValue(), s[j].Filter.GetTypedConfig().GetValue()); compare != 0 {
		return compare < 0
	}

	// ensure stability
	return i < j
}

func (s StagedFilterList[WellKnown, FilterType]) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

type (
	StagedHttpFilter         = StagedFilter[WellKnownFilterStage, *envoyhttp.HttpFilter]
	StagedNetworkFilter      = StagedFilter[WellKnownFilterStage, *envoylistenerv3.Filter]
	StagedUpstreamHttpFilter = StagedFilter[WellKnownUpstreamHTTPFilterStage, *envoyhttp.HttpFilter]
)

type (
	StagedHttpFilterList         = StagedFilterList[WellKnownFilterStage, *envoyhttp.HttpFilter]
	StagedNetworkFilterList      = StagedFilterList[WellKnownFilterStage, *envoylistenerv3.Filter]
	StagedUpstreamHttpFilterList = StagedFilterList[WellKnownUpstreamHTTPFilterStage, *envoyhttp.HttpFilter]
)

// MustNewStagedFilter creates an instance of the named filter with the desired stage.
// Returns a filter even if an error occurred.
// Should rarely be used as disregarding an error is bad practice but does make
// appending easier.
// If not directly appending consider using NewStagedFilter instead of this function.
func MustNewStagedFilter(name string, config proto.Message, stage FilterStage[WellKnownFilterStage]) StagedHttpFilter {
	s, _ := NewStagedFilter(name, config, stage)
	return s
}

// MustNewStagedFilterWithWeight creates an instance of the named filter with the desired stage and weight.
// The weight is used to order filters of the same type within the same stage based on their weights
func MustNewStagedFilterWithWeight(name string, config proto.Message, stage FilterStage[WellKnownFilterStage], weight int32) StagedHttpFilter {
	s, _ := NewStagedFilter(name, config, stage)
	s.Weight = weight
	return s
}

// NewStagedFilter creates an instance of the named filter with the desired stage.
// Errors if the config is nil or we cannot determine the type of the config.
// Config type determination may fail if the config is both  unknown and has no fields.
func NewStagedFilter(name string, config proto.Message, stage FilterStage[WellKnownFilterStage]) (StagedHttpFilter, error) {
	s := StagedHttpFilter{
		Filter: &envoyhttp.HttpFilter{
			Name: name,
		},
		Stage: stage,
	}

	if config == nil {
		return s, fmt.Errorf("filters must have a config specified to derive its type filtername:%s", name)
	}

	marshalledConf, err := utils.MessageToAny(config)
	if err != nil {
		// all config types should already be known
		// therefore this should never happen
		return StagedHttpFilter{}, err
	}

	s.Filter.ConfigType = &envoyhttp.HttpFilter_TypedConfig{
		TypedConfig: marshalledConf,
	}

	return s, nil
}

// StagedFilterListContainsName checks for a given named filter.
// This is not a check of the type url but rather the now mostly unused name
func StagedFilterListContainsName(filters StagedHttpFilterList, filterName string) bool {
	for _, filter := range filters {
		if filter.Filter.GetName() == filterName {
			return true
		}
	}

	return false
}

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

// FilterStageSpec allows configuration of where in a filter chain a given HTTP filter is inserted,
// relative to one of the pre-defined stages.
type FilterStageSpec struct {
	// Stage of the filter chain in which the selected filter should be added.
	Stage FilterStage_Stage
	// How this filter should be placed relative to the stage.
	Predicate FilterStage_Predicate
}

func (x *FilterStageSpec) GetStage() FilterStage_Stage {
	if x != nil {
		return x.Stage
	}
	return FilterStage_FaultStage
}

func (x *FilterStageSpec) GetPredicate() FilterStage_Predicate {
	if x != nil {
		return x.Predicate
	}
	return FilterStage_During
}

// ConvertFilterStage converts user-specified FilterStageSpec options to the FilterStage representation used for translation.
func ConvertFilterStage(in *FilterStageSpec) *FilterStage[WellKnownFilterStage] {
	if in == nil {
		return nil
	}

	var outStage WellKnownFilterStage
	switch in.GetStage() {
	case FilterStage_CorsStage:
		outStage = CorsStage
	case FilterStage_WafStage:
		outStage = WafStage
	case FilterStage_AuthNStage:
		outStage = AuthNStage
	case FilterStage_AuthZStage:
		outStage = AuthZStage
	case FilterStage_RateLimitStage:
		outStage = RateLimitStage
	case FilterStage_AcceptedStage:
		outStage = AcceptedStage
	case FilterStage_OutAuthStage:
		outStage = OutAuthStage
	case FilterStage_RouteStage:
		outStage = RouteStage
	case FilterStage_FaultStage:
		fallthrough
	default:
		// default to Fault stage
		outStage = FaultStage
	}

	var out FilterStage[WellKnownFilterStage]
	switch in.GetPredicate() {
	case FilterStage_Before:
		out = BeforeStage(outStage)
	case FilterStage_After:
		out = AfterStage(outStage)
	case FilterStage_During:
		fallthrough
	default:
		// default to During
		out = DuringStage(outStage)
	}
	return &out
}
