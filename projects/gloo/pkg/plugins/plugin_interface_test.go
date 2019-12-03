package plugins

import (
	"sort"

	structpb "github.com/golang/protobuf/ptypes/struct"

	envoylistener "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Plugin", func() {

	It("should order http filter stages correctly", func() {
		By("base case")
		filters := StagedHttpFilterList{
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "mockFilter"}, BeforeStage(CorsStage)},
		}
		sort.Sort(filters)
		ExpectNameOrder(filters, []string{"mockFilter"})

		By("before/after stage")
		filters = StagedHttpFilterList{
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "A"}, BeforeStage(CorsStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "B"}, AfterStage(CorsStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "C"}, DuringStage(CorsStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "D"}, BeforeStage(CorsStage)},
		}
		sort.Sort(filters)
		ExpectNameOrder(filters, []string{"A", "D", "C", "B"})

		By("all relative to the same well known stage, should order by weight and name")
		filters = StagedHttpFilterList{
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "A"}, RelativeToStage(CorsStage, 5)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "B"}, RelativeToStage(CorsStage, 9)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "C"}, RelativeToStage(CorsStage, 0)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "D"}, RelativeToStage(CorsStage, -1)},
		}
		sort.Sort(filters)
		ExpectNameOrder(filters, []string{"D", "C", "A", "B"})

		By("expected well known ordering")
		filters = StagedHttpFilterList{
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "H"}, DuringStage(RouteStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "G"}, DuringStage(OutAuthStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "F"}, DuringStage(AcceptedStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "E"}, DuringStage(RateLimitStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "D"}, DuringStage(AuthZStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "C"}, DuringStage(AuthNStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "Waf"}, DuringStage(WafStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "B"}, DuringStage(CorsStage)},
			StagedHttpFilter{&envoyhttp.HttpFilter{Name: "A"}, DuringStage(FaultStage)},
		}
		sort.Sort(filters)
		ExpectNameOrder(filters, []string{"A", "B", "Waf", "C", "D", "E", "F", "G", "H"})

		By("verify stable sort")
		firstFilter := &envoyhttp.HttpFilter{Name: "A", ConfigType: &envoyhttp.HttpFilter_Config{Config: &structpb.Struct{Fields: map[string]*structpb.Value{"a": nil}}}}
		secondFilter := &envoyhttp.HttpFilter{Name: "A", ConfigType: &envoyhttp.HttpFilter_Config{Config: &structpb.Struct{Fields: map[string]*structpb.Value{"b": nil}}}}
		thirdFilter := &envoyhttp.HttpFilter{Name: "A", ConfigType: &envoyhttp.HttpFilter_Config{Config: &structpb.Struct{Fields: map[string]*structpb.Value{"c": nil}}}}
		filters = StagedHttpFilterList{
			StagedHttpFilter{firstFilter, DuringStage(RouteStage)},
			StagedHttpFilter{secondFilter, DuringStage(RouteStage)},
			StagedHttpFilter{thirdFilter, DuringStage(RouteStage)},
		}
		sort.Sort(filters)
		ExpectFilterConfigOrders(filters, []string{"a", "b", "c"})
	})

	It("should order listener filter stages correctly", func() {
		By("base case")
		filters := StagedListenerFilterList{
			StagedListenerFilter{&envoylistener.Filter{Name: "H"}, DuringStage(RouteStage)},
			StagedListenerFilter{&envoylistener.Filter{Name: "G"}, DuringStage(OutAuthStage)},
			StagedListenerFilter{&envoylistener.Filter{Name: "F"}, DuringStage(AcceptedStage)},
			StagedListenerFilter{&envoylistener.Filter{Name: "E"}, DuringStage(RateLimitStage)},
			StagedListenerFilter{&envoylistener.Filter{Name: "D"}, DuringStage(AuthZStage)},
			StagedListenerFilter{&envoylistener.Filter{Name: "C"}, DuringStage(AuthNStage)},
			StagedListenerFilter{&envoylistener.Filter{Name: "Waf"}, DuringStage(WafStage)},
			StagedListenerFilter{&envoylistener.Filter{Name: "B"}, DuringStage(CorsStage)},
			StagedListenerFilter{&envoylistener.Filter{Name: "A"}, DuringStage(FaultStage)},
		}
		sort.Sort(filters)
		ExpectListenerFilterNameOrder(filters, []string{"A", "B", "Waf", "C", "D", "E", "F", "G", "H"})
	})
})

func ExpectListenerFilterNameOrder(filters StagedListenerFilterList, names []string) {
	Expect(len(filters)).To(Equal(len(names)))
	for i, filter := range filters {
		Expect(filter.ListenerFilter.Name).To(Equal(names[i]))
	}
}

func ExpectNameOrder(filters StagedHttpFilterList, names []string) {
	Expect(len(filters)).To(Equal(len(names)))
	for i, filter := range filters {
		Expect(filter.HttpFilter.Name).To(Equal(names[i]))
	}
}

func ExpectFilterConfigOrders(filters StagedHttpFilterList, names []string) {
	Expect(len(filters)).To(Equal(len(names)))
	for i, filter := range filters {
		v, ok := filter.HttpFilter.ConfigType.(*envoyhttp.HttpFilter_Config).Config.Fields[names[i]]
		Expect(ok).To(BeTrue())
		Expect(v).To(BeNil())
	}
}
