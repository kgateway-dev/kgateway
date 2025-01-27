package metadata_test

import (
	envoy_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/metadata"
	. "github.com/solo-io/solo-kit/test/matchers"
)

var _ = Describe("Metadata Plugin", func() {

	var (
		plugin     plugins.RoutePlugin
		glooRoute  *v1.Route
		envoyRoute *envoy_route_v3.Route
	)

	BeforeEach(func() {
		plugin = NewPlugin()
		glooRoute = &v1.Route{}
		envoyRoute = &envoy_route_v3.Route{}
	})

	When("no metadata is set on the input route", func() {
		It("does not set any metadata on the output route", func() {
			err := plugin.ProcessRoute(plugins.RouteParams{}, glooRoute, envoyRoute)
			Expect(err).NotTo(HaveOccurred())
			Expect(envoyRoute.GetMetadata()).To(BeNil())
		})
	})

	When("an empty metadata object is set on the input route", func() {
		It("does not set any metadata on the output route", func() {
			glooRoute = &v1.Route{
				Options: &v1.RouteOptions{
					EnvoyMetadata: map[string]*structpb.Struct{},
				},
			}

			err := plugin.ProcessRoute(plugins.RouteParams{}, glooRoute, envoyRoute)
			Expect(err).NotTo(HaveOccurred())
			Expect(envoyRoute.GetMetadata()).To(BeNil())
		})
	})

	When("metadata has been set on the input route", func() {
		It("copies it over to the output route", func() {
			inputMetadata1 := &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"foo": {
						Kind: &structpb.Value_StringValue{
							StringValue: "bar",
						},
					},
				},
			}
			inputMetadata2 := &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"baz": {
						Kind: &structpb.Value_ListValue{
							ListValue: &structpb.ListValue{
								Values: []*structpb.Value{
									{
										Kind: &structpb.Value_NumberValue{
											NumberValue: 1.0,
										},
									},
									{
										Kind: &structpb.Value_NumberValue{
											NumberValue: 2.0,
										},
									},
								},
							},
						},
					},
					"qux": {
						Kind: &structpb.Value_NullValue{
							NullValue: structpb.NullValue_NULL_VALUE,
						},
					},
				},
			}

			// Clone to prevent potential mutations of the input objects from invalidating the expectations
			expectedOutputMetadata1 := proto.Clone(inputMetadata1).(*structpb.Struct)
			expectedOutputMetadata2 := proto.Clone(inputMetadata2).(*structpb.Struct)

			glooRoute = &v1.Route{
				Options: &v1.RouteOptions{
					EnvoyMetadata: map[string]*structpb.Struct{
						"io.solo.test.one": inputMetadata1,
						"io.solo.test.two": inputMetadata2,
					},
				},
			}

			err := plugin.ProcessRoute(plugins.RouteParams{}, glooRoute, envoyRoute)
			Expect(err).NotTo(HaveOccurred())

			actualFilterMetadata := envoyRoute.GetMetadata().GetFilterMetadata()
			Expect(actualFilterMetadata).To(HaveLen(2))
			Expect(actualFilterMetadata["io.solo.test.one"]).To(MatchProto(expectedOutputMetadata1))
			Expect(actualFilterMetadata["io.solo.test.two"]).To(MatchProto(expectedOutputMetadata2))
		})
	})
})
