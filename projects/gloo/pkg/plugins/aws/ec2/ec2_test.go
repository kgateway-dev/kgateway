package ec2

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws"
	awsapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/plugins/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	accessKeyValue = "some acccess value"
	secretKeyValue = "some secret value"
)

var _ = Describe("Plugin", func() {
	var (
		params      plugins.Params
		vhostParams plugins.VirtualHostParams
		plugin      plugins.Plugin
		upstream    *v1.Upstream
		route       *v1.Route
		out         *envoyapi.Cluster
		outroute    *envoyroute.Route
	)
	BeforeEach(func() {
		var b bool
		plugin = NewPlugin(&b)
		plugin.Init(plugins.InitParams{})
		upstreamName := "up"
		clusterName := upstreamName
		funcname := "foo"
		upstream = &v1.Upstream{
			Metadata: core.Metadata{
				Name: upstreamName,
				// TODO(yuval-k): namespace
				Namespace: "",
			},
			UpstreamSpec: &v1.UpstreamSpec{
				UpstreamType: &v1.UpstreamSpec_Aws{
					Aws: &aws.UpstreamSpec{
						LambdaFunctions: []*aws.LambdaFunctionSpec{{
							LogicalName:        funcname,
							LambdaFunctionName: "foo",
							Qualifier:          "v1",
						}},
						Region: "us-east1",
						SecretRef: core.ResourceRef{
							Namespace: "",
							Name:      "secretref",
						},
					},
				},
			},
		}
		route = &v1.Route{
			Action: &v1.Route_RouteAction{
				RouteAction: &v1.RouteAction{
					Destination: &v1.RouteAction_Single{
						Single: &v1.Destination{
							DestinationType: &v1.Destination_Upstream{
								Upstream: &core.ResourceRef{
									Namespace: "",
									Name:      upstreamName,
								},
							},
							DestinationSpec: &v1.DestinationSpec{
								DestinationType: &v1.DestinationSpec_Aws{
									Aws: &awsapi.DestinationSpec{
										LogicalName: funcname,
									},
								},
							},
						},
					},
				},
			},
		}

		out = &envoyapi.Cluster{}
		outroute = &envoyroute.Route{
			Action: &envoyroute.Route_Route{
				Route: &envoyroute.RouteAction{
					ClusterSpecifier: &envoyroute.RouteAction_Cluster{
						Cluster: clusterName,
					},
				},
			},
		}

		params.Snapshot = &v1.ApiSnapshot{
			Secrets: v1.SecretList{{
				Metadata: core.Metadata{
					Name: "secretref",
					// TODO(yuval-k): namespace
					Namespace: "",
				},
				Kind: &v1.Secret_Aws{
					Aws: &v1.AwsSecret{
						AccessKey: accessKeyValue,
						SecretKey: secretKeyValue,
					},
				},
			}},
		}
		vhostParams = plugins.VirtualHostParams{Params: params}

	})

	Context("filters", func() {
		It("should produce filters when upstream is present", func() {
			// process upstream
			err := plugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			err = plugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).NotTo(HaveOccurred())

			// check that we have filters
			filters, err := plugin.(plugins.HttpFilterPlugin).HttpFilters(params, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(filters).NotTo(BeEmpty())
		})

		It("should not produce filters when no upstreams are present", func() {
			filters, err := plugin.(plugins.HttpFilterPlugin).HttpFilters(params, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(filters).To(BeEmpty())
		})
	})
})
