package istio_integration_test

import (
	"context"

	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	kubeplugin "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/kubernetes"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/istio_integration"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var _ = Describe("Plugin", func() {
	const (
		serviceNamespace = "serviceNs"
		serviceName      = "serviceName"
		rewrittenHost    = "serviceName.serviceNs"
		upstreamName     = "test-us"
		glooNamespace    = "ns"
	)

	var (
		ctx context.Context

		upstreams v1.UpstreamList
	)

	BeforeEach(func() {
		ctx = context.Background()

		upstreams = v1.UpstreamList{makeKubeUpstream(upstreamName, glooNamespace, serviceName, serviceNamespace)}
	})

	Describe("GetHostFromDestination", func() {
		It("Gets the host from a kube destination", func() {
			destination := &v1.RouteAction_Single{
				Single: &v1.Destination{
					DestinationType: &v1.Destination_Kube{
						Kube: &v1.KubernetesServiceDestination{
							Ref: &core.ResourceRef{
								Namespace: serviceNamespace,
								Name:      serviceName,
							},
						},
					},
				},
			}
			host, err := istio_integration.GetHostFromDestination(destination, upstreams)
			Expect(err).NotTo(HaveOccurred())
			Expect(host).To(Equal(rewrittenHost))
		})

		It("Gets the host from a gloo upstream", func() {
			destination := &v1.RouteAction_Single{
				Single: &v1.Destination{
					DestinationType: &v1.Destination_Upstream{
						Upstream: &core.ResourceRef{
							Namespace: glooNamespace,
							Name:      upstreamName,
						},
					},
				},
			}
			host, err := istio_integration.GetHostFromDestination(destination, upstreams)
			Expect(err).NotTo(HaveOccurred())
			Expect(host).To(Equal(rewrittenHost))
		})
	})

	Describe("ProcessRoute", func() {
		var (
			istioPlugin plugins.Plugin
			initParams  plugins.InitParams
		)

		BeforeEach(func() {
			istioPlugin = istio_integration.NewPlugin(ctx)
			initParams = plugins.InitParams{
				Ctx: ctx,
			}
		})

		DescribeTable("sets appendXForwardedHost according to value from settings",
			func(settingValue *wrappers.BoolValue, matcher types.GomegaMatcher) {
				initParams.Settings = &v1.Settings{
					Gloo: &v1.GlooOptions{
						IstioOptions: &v1.GlooOptions_IstioOptions{
							AppendXForwardedHost: settingValue,
						},
					},
				}
				istioPlugin.(plugins.Plugin).Init(initParams)

				params := plugins.RouteParams{
					VirtualHostParams: plugins.VirtualHostParams{
						Params: plugins.Params{
							Snapshot: &gloosnapshot.ApiSnapshot{
								Upstreams: upstreams,
							},
						},
					},
				}
				inRoute := &v1.Route{
					Action: &v1.Route_RouteAction{
						RouteAction: &v1.RouteAction{
							Destination: &v1.RouteAction_Single{
								Single: &v1.Destination{
									DestinationType: &v1.Destination_Kube{
										Kube: &v1.KubernetesServiceDestination{
											Ref: &core.ResourceRef{
												Namespace: "ns",
												Name:      "foo",
											},
										},
									},
								},
							},
						},
					},
				}
				outRoute := &envoy_config_route_v3.Route{
					Action: &envoy_config_route_v3.Route_Route{
						Route: &envoy_config_route_v3.RouteAction{},
					},
				}

				err := istioPlugin.(plugins.RoutePlugin).ProcessRoute(params, inRoute, outRoute)
				Expect(err).NotTo(HaveOccurred())
				routeAction, ok := outRoute.GetAction().(*envoy_config_route_v3.Route_Route)
				Expect(ok).To(BeTrue())
				Expect(routeAction.Route.AppendXForwardedHost).To(matcher)
			},
			Entry("setting is true", &wrappers.BoolValue{Value: true}, BeTrue()),
			Entry("setting is false", &wrappers.BoolValue{Value: false}, BeFalse()),
			Entry("setting is nil (default false)", nil, BeTrue()),
		)
	})
})

func makeKubeUpstream(name, namespace, serviceName, serviceNamespace string) *v1.Upstream {
	us := v1.NewUpstream(namespace, name)
	us.UpstreamType = &v1.Upstream_Kube{
		Kube: &kubeplugin.UpstreamSpec{
			ServiceNamespace: serviceNamespace,
			ServiceName:      serviceName,
		},
	}
	return us
}
