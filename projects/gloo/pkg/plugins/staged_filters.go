package plugins

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	envoy_config_listener_v3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/golang/protobuf/proto"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/filters"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
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

type FilterStage struct {
	RelativeTo WellKnownFilterStage
	Weight     int
}

// FilterStageComparison helps implement the sort.Interface Less function for use in other implementations of sort.Interface
// returns -1 if less than, 0 if equal, 1 if greater than
// It is not sufficient to return a Less bool because calling functions need to know if equal or greater when Less is false
func FilterStageComparison(a, b FilterStage) int {
	if a.RelativeTo < b.RelativeTo {
		return -1
	} else if a.RelativeTo > b.RelativeTo {
		return 1
	}
	if a.Weight < b.Weight {
		return -1
	} else if a.Weight > b.Weight {
		return 1
	}
	return 0
}

func BeforeStage(wellKnown WellKnownFilterStage) FilterStage {
	return RelativeToStage(wellKnown, -1)
}
func DuringStage(wellKnown WellKnownFilterStage) FilterStage {
	return RelativeToStage(wellKnown, 0)
}
func AfterStage(wellKnown WellKnownFilterStage) FilterStage {
	return RelativeToStage(wellKnown, 1)
}
func RelativeToStage(wellKnown WellKnownFilterStage, weight int) FilterStage {
	return FilterStage{
		RelativeTo: wellKnown,
		Weight:     weight,
	}
}

type StagedHttpFilter struct {
	HttpFilter *envoyhttp.HttpFilter
	Stage      FilterStage
}

type StagedHttpFilterList []StagedHttpFilter

func (s StagedHttpFilterList) Len() int {
	return len(s)
}

// filters by Relative Stage, Weighting, Name, Config Type-Url, Config Value, and (to ensure stability) index.
// The assumption is that if two filters are in the same stage, their order doesn't matter, and we
// just need to make sure it is stable.
func (s StagedHttpFilterList) Less(i, j int) bool {
	if compare := FilterStageComparison(s[i].Stage, s[j].Stage); compare != 0 {
		return compare < 0
	}

	if compare := strings.Compare(s[i].HttpFilter.GetName(), s[j].HttpFilter.GetName()); compare != 0 {
		return compare < 0
	}

	if compare := strings.Compare(s[i].HttpFilter.GetTypedConfig().GetTypeUrl(), s[j].HttpFilter.GetTypedConfig().GetTypeUrl()); compare != 0 {
		return compare < 0
	}

	if compare := bytes.Compare(s[i].HttpFilter.GetTypedConfig().GetValue(), s[j].HttpFilter.GetTypedConfig().GetValue()); compare != 0 {
		return compare < 0
	}

	// ensure stability
	return i < j
}

func (s StagedHttpFilterList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

type StagedNetworkFilter struct {
	NetworkFilter *envoy_config_listener_v3.Filter
	Stage         FilterStage
}

type StagedNetworkFilterList []StagedNetworkFilter

func (s StagedNetworkFilterList) Len() int {
	return len(s)
}

// filters by Relative Stage, Weighting, Name, and (to ensure stability) index
func (s StagedNetworkFilterList) Less(i, j int) bool {
	switch FilterStageComparison(s[i].Stage, s[j].Stage) {
	case -1:
		return true
	case 1:
		return false
	}
	if s[i].NetworkFilter.GetName() < s[j].NetworkFilter.GetName() {
		return true
	}
	if s[i].NetworkFilter.GetName() > s[j].NetworkFilter.GetName() {
		return false
	}
	if s[i].NetworkFilter.String() < s[j].NetworkFilter.String() {
		return true
	}
	if s[i].NetworkFilter.String() > s[j].NetworkFilter.String() {
		return false
	}
	// ensure stability
	return i < j
}

func (s StagedNetworkFilterList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// NewStagedFilterWithConfig creates an instance of the named filter with the desired stage.
// Deprecated: config is now always needed and so NewStagedFilter should always be used.
func NewStagedFilterWithConfig(name string, config proto.Message, stage FilterStage) (StagedHttpFilter, error) {
	return NewStagedFilter(name, config, stage)
}

// MustNewStagedFilter creates an instance of the named filter with the desired stage.
// Returns a filter even if an error occurred.
// Should rarely be used as disregarding an error is bad practice but does make
// appending easier.
// If not directly appending consider using NewStagedFilter instead of this function.
func MustNewStagedFilter(name string, config proto.Message, stage FilterStage) StagedHttpFilter {
	s, _ := NewStagedFilter(name, config, stage)
	return s
}

// NewStagedFilter creates an instance of the named filter with the desired stage.
// Errors if the config is nil or we cannot determine the type of the config.
// Config type determination may fail if the config is both  unknown and has no fields.
func NewStagedFilter(name string, config proto.Message, stage FilterStage) (StagedHttpFilter, error) {

	s := StagedHttpFilter{
		HttpFilter: &envoyhttp.HttpFilter{
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

	s.HttpFilter.ConfigType = &envoyhttp.HttpFilter_TypedConfig{
		TypedConfig: marshalledConf,
	}

	return s, nil
}

// StagedFilterListContainsName checks for a given named filter.
// This is not a check of the type url but rather the now mostly unused name
func StagedFilterListContainsName(filters StagedHttpFilterList, filterName string) bool {
	for _, filter := range filters {
		if filter.HttpFilter.GetName() == filterName {
			return true
		}
	}

	return false
}

// ConvertFilterStage converts user-specified FilterStage options to the FilterStage representation used for translation.
func ConvertFilterStage(in *filters.FilterStage) *FilterStage {
	if in == nil {
		return nil
	}

	var outStage WellKnownFilterStage
	switch in.GetStage() {
	case filters.FilterStage_CorsStage:
		outStage = CorsStage
	case filters.FilterStage_WafStage:
		outStage = WafStage
	case filters.FilterStage_AuthNStage:
		outStage = AuthNStage
	case filters.FilterStage_AuthZStage:
		outStage = AuthZStage
	case filters.FilterStage_RateLimitStage:
		outStage = RateLimitStage
	case filters.FilterStage_AcceptedStage:
		outStage = AcceptedStage
	case filters.FilterStage_OutAuthStage:
		outStage = OutAuthStage
	case filters.FilterStage_RouteStage:
		outStage = RouteStage
	case filters.FilterStage_FaultStage:
		fallthrough
	default:
		// default to Fault stage
		outStage = FaultStage
	}

	var out FilterStage
	switch in.GetPredicate() {
	case filters.FilterStage_Before:
		out = BeforeStage(outStage)
	case filters.FilterStage_After:
		out = AfterStage(outStage)
	case filters.FilterStage_During:
		fallthrough
	default:
		// default to During
		out = DuringStage(outStage)
	}
	return &out
}
