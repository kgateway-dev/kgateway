package headers

import (
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/headers"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"

	envoyroute "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
)

var _ = Describe("Plugin", func() {
	p := NewPlugin()
	It("errors if the header is nil", func() {
		out := &envoyroute.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				HeaderManipulation: testBrokenConfig,
			},
		}, out)
		Expect(err).To(HaveOccurred())
		Expect(err).To(Equal(MissingHeaderValueError))
	})
	It("converts the header manipulation config for weighted destinations", func() {
		out := &envoyroute.WeightedCluster_ClusterWeight{}
		err := p.ProcessWeightedDestination(plugins.RouteParams{}, &v1.WeightedDestination{
			Options: &v1.WeightedDestinationOptions{
				HeaderManipulation: testHeaderManip,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.RequestHeadersToAdd).To(Equal(expectedHeaders.RequestHeadersToAdd))
		Expect(out.RequestHeadersToRemove).To(Equal(expectedHeaders.RequestHeadersToRemove))
		Expect(out.ResponseHeadersToAdd).To(Equal(expectedHeaders.ResponseHeadersToAdd))
		Expect(out.ResponseHeadersToRemove).To(Equal(expectedHeaders.ResponseHeadersToRemove))
	})
	It("converts the header manipulation config for virtual hosts", func() {
		out := &envoyroute.VirtualHost{}
		err := p.ProcessVirtualHost(plugins.VirtualHostParams{}, &v1.VirtualHost{
			Options: &v1.VirtualHostOptions{
				HeaderManipulation: testHeaderManip,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.RequestHeadersToAdd).To(Equal(expectedHeaders.RequestHeadersToAdd))
		Expect(out.RequestHeadersToRemove).To(Equal(expectedHeaders.RequestHeadersToRemove))
		Expect(out.ResponseHeadersToAdd).To(Equal(expectedHeaders.ResponseHeadersToAdd))
		Expect(out.ResponseHeadersToRemove).To(Equal(expectedHeaders.ResponseHeadersToRemove))
	})
	It("converts the header manipulation config for routes", func() {
		out := &envoyroute.Route{}
		err := p.ProcessRoute(plugins.RouteParams{}, &v1.Route{
			Options: &v1.RouteOptions{
				HeaderManipulation: testHeaderManip,
			},
		}, out)
		Expect(err).NotTo(HaveOccurred())
		Expect(out.RequestHeadersToAdd).To(Equal(expectedHeaders.RequestHeadersToAdd))
		Expect(out.RequestHeadersToRemove).To(Equal(expectedHeaders.RequestHeadersToRemove))
		Expect(out.ResponseHeadersToAdd).To(Equal(expectedHeaders.ResponseHeadersToAdd))
		Expect(out.ResponseHeadersToRemove).To(Equal(expectedHeaders.ResponseHeadersToRemove))
	})
})

var testBrokenConfig = &headers.HeaderManipulation{
	RequestHeadersToAdd:     []*headers.HeaderValueOption{{Header: nil, Append: &types.BoolValue{Value: true}}},
	RequestHeadersToRemove:  []string{"a"},
	ResponseHeadersToAdd:    []*headers.HeaderValueOption{{Header: &headers.HeaderValue{Key: "foo", Value: "bar"}, Append: &types.BoolValue{Value: true}}},
	ResponseHeadersToRemove: []string{"b"},
}

var testHeaderManip = &headers.HeaderManipulation{
	RequestHeadersToAdd:     []*headers.HeaderValueOption{{Header: &headers.HeaderValue{Key: "foo", Value: "bar"}, Append: &types.BoolValue{Value: true}}},
	RequestHeadersToRemove:  []string{"a"},
	ResponseHeadersToAdd:    []*headers.HeaderValueOption{{Header: &headers.HeaderValue{Key: "foo", Value: "bar"}, Append: &types.BoolValue{Value: true}}},
	ResponseHeadersToRemove: []string{"b"},
}

var expectedHeaders = envoyHeaderManipulation{
	RequestHeadersToAdd:     []*core.HeaderValueOption{{Header: &core.HeaderValue{Key: "foo", Value: "bar"}, Append: &wrappers.BoolValue{Value: true}}},
	RequestHeadersToRemove:  []string{"a"},
	ResponseHeadersToAdd:    []*core.HeaderValueOption{{Header: &core.HeaderValue{Key: "foo", Value: "bar"}, Append: &wrappers.BoolValue{Value: true}}},
	ResponseHeadersToRemove: []string{"b"},
}
