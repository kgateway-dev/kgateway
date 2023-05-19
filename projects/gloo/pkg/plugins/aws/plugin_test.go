package aws_test

import (
	"context"
	"net/url"

	"github.com/onsi/gomega/types"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	gogoproto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/duration"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/aws"
	envoytransform "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	v1snap "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/gloosnapshot"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	v1transformation "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/test/matchers"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	accessKeyValue    = "some access value"
	secretKeyValue    = "some secret value"
	sessionTokenValue = "some session token value"
)

var _ = Describe("Plugin", func() {

	var (
		ctx         context.Context
		cancel      context.CancelFunc
		initParams  plugins.InitParams
		params      plugins.Params
		vhostParams plugins.VirtualHostParams
		awsPlugin   plugins.Plugin
		upstream    *v1.Upstream
		route       *v1.Route
		out         *envoy_config_cluster_v3.Cluster
		outroute    *envoy_config_route_v3.Route
		lpe         *AWSLambdaProtocolExtension
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		params.Ctx = ctx
		awsPlugin = NewPlugin(GenerateAWSLambdaRouteConfig)

		upstreamName := "up"
		clusterName := upstreamName
		funcName := "foo"

		upstream = &v1.Upstream{
			Metadata: &core.Metadata{
				Name:      upstreamName,
				Namespace: "ns",
			},
			UpstreamType: &v1.Upstream_Aws{
				Aws: &aws.UpstreamSpec{
					LambdaFunctions: []*aws.LambdaFunctionSpec{{
						LogicalName:        funcName,
						LambdaFunctionName: "foo",
						Qualifier:          "v1",
					}},
					Region: "us-east1",
					SecretRef: &core.ResourceRef{
						Namespace: "ns",
						Name:      "secretref",
					},
					DisableRoleChaining: true,
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
									Namespace: "ns",
									Name:      upstreamName,
								},
							},
							DestinationSpec: &v1.DestinationSpec{
								DestinationType: &v1.DestinationSpec_Aws{
									Aws: &aws.DestinationSpec{
										LogicalName: funcName,
									},
								},
							},
						},
					},
				},
			},
		}

		out = &envoy_config_cluster_v3.Cluster{}
		outroute = &envoy_config_route_v3.Route{
			Action: &envoy_config_route_v3.Route_Route{
				Route: &envoy_config_route_v3.RouteAction{
					ClusterSpecifier: &envoy_config_route_v3.RouteAction_Cluster{
						Cluster: clusterName,
					},
				},
			},
		}

		initParams = plugins.InitParams{}
		params.Snapshot = &v1snap.ApiSnapshot{
			Secrets: v1.SecretList{{
				Metadata: &core.Metadata{
					Name:      "secretref",
					Namespace: "ns",
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
		lpe = &AWSLambdaProtocolExtension{}
	})

	JustBeforeEach(func() {
		awsPlugin.Init(initParams)
	})

	AfterEach(func() {
		cancel()
	})

	processProtocolOptions := func() {
		anyExt := out.TypedExtensionProtocolOptions[FilterName]
		err := gogoproto.Unmarshal(anyExt.Value, lpe)
		Expect(err).NotTo(HaveOccurred())
	}

	Context("upstreams", func() {

		It("should process upstream with secrets", func() {
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedExtensionProtocolOptions).NotTo(BeEmpty())
			Expect(out.TypedExtensionProtocolOptions).To(HaveKey(FilterName))
			processProtocolOptions()

			Expect(lpe.AccessKey).To(Equal(accessKeyValue))
			Expect(lpe.SecretKey).To(Equal(secretKeyValue))
			Expect(lpe.Region).To(Equal("us-east1"))
			Expect(lpe.Host).To(Equal("lambda.us-east1.amazonaws.com"))
		})

		It("should error upstream with no secrets", func() {
			params.Snapshot.Secrets = nil
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).To(HaveOccurred())
		})

		It("should error non aws secret", func() {
			params.Snapshot.Secrets[0].Kind = &v1.Secret_Tls{}
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err.Error()).To(Equal(`secret (secretref.ns) is not an AWS secret`))
		})

		It("should error upstream with no secret ref", func() {
			upstream.GetAws().SecretRef = nil
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).To(MatchError("no aws secret provided. consider setting enableCredentialsDiscovey to true or enabling service account credentials if running in EKS"))
		})

		It("should error upstream with no access_key", func() {
			params.Snapshot.Secrets[0].Kind.(*v1.Secret_Aws).Aws.AccessKey = ""
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).To(HaveOccurred())
		})
		It("should error upstream with no secret_key", func() {
			params.Snapshot.Secrets[0].Kind.(*v1.Secret_Aws).Aws.SecretKey = ""
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).To(HaveOccurred())
		})
		It("should error upstream with invalid access_key", func() {
			params.Snapshot.Secrets[0].Kind.(*v1.Secret_Aws).Aws.AccessKey = "\xffbinary!"
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).To(HaveOccurred())
		})
		It("should error upstream with invalid secret_key", func() {
			params.Snapshot.Secrets[0].Kind.(*v1.Secret_Aws).Aws.SecretKey = "\xffbinary!"
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).To(HaveOccurred())
		})

		It("should not process and not error with non aws upstream", func() {
			upstream.UpstreamType = nil
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			Expect(out.TypedExtensionProtocolOptions).To(BeEmpty())
			Expect(outroute.TypedPerFilterConfig).NotTo(HaveKey(FilterName))

		})

		Context("with ssl", func() {

			var err error

			JustBeforeEach(func() {
				err = awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			})

			Context("should allow configuring ssl without settings.UpstreamOptions", func() {

				BeforeEach(func() {
					initParams.Settings = &v1.Settings{}
				})

				It("should not error", func() {
					Expect(err).NotTo(HaveOccurred())
				})

				It("should configure CommonTlsContext without TlsParams", func() {
					commonTlsContext := getClusterTlsContext(out).GetCommonTlsContext()
					Expect(commonTlsContext).NotTo(BeNil())

					tlsParams := commonTlsContext.GetTlsParams()
					Expect(tlsParams).To(BeNil())
				})

			})

			Context("should allow configuring ssl with settings.UpstreamOptions", func() {

				BeforeEach(func() {
					initParams.Settings = &v1.Settings{
						UpstreamOptions: &v1.UpstreamOptions{
							SslParameters: &ssl.SslParameters{
								MinimumProtocolVersion: ssl.SslParameters_TLSv1_1,
								MaximumProtocolVersion: ssl.SslParameters_TLSv1_2,
								CipherSuites:           []string{"cipher-test"},
								EcdhCurves:             []string{"ec-dh-test"},
							},
						},
					}
				})

				It("should not error", func() {
					Expect(err).NotTo(HaveOccurred())
				})

				It("should configure CommonTlsContext", func() {
					commonTlsContext := getClusterTlsContext(out).GetCommonTlsContext()
					Expect(commonTlsContext).NotTo(BeNil())

					tlsParams := commonTlsContext.GetTlsParams()
					Expect(tlsParams).NotTo(BeNil())

					Expect(tlsParams.GetCipherSuites()).To(Equal([]string{"cipher-test"}))
					Expect(tlsParams.GetEcdhCurves()).To(Equal([]string{"ec-dh-test"}))
					Expect(tlsParams.GetTlsMinimumProtocolVersion()).To(Equal(envoyauth.TlsParameters_TLSv1_1))
					Expect(tlsParams.GetTlsMaximumProtocolVersion()).To(Equal(envoyauth.TlsParameters_TLSv1_2))
				})

			})

			Context("should error while configuring ssl with invalid tls versions in settings.UpstreamOptions", func() {

				var invalidProtocolVersion ssl.SslParameters_ProtocolVersion = 5 // INVALID

				BeforeEach(func() {
					initParams.Settings = &v1.Settings{
						UpstreamOptions: &v1.UpstreamOptions{
							SslParameters: &ssl.SslParameters{
								MinimumProtocolVersion: invalidProtocolVersion,
								MaximumProtocolVersion: ssl.SslParameters_TLSv1_2,
								CipherSuites:           []string{"cipher-test"},
								EcdhCurves:             []string{"ec-dh-test"},
							},
						},
					}
				})

				It("should error", func() {
					Expect(err).To(HaveOccurred())
				})

			})

		})
	})

	Context("no spec", func() {
		var destination *v1.Destination
		var curParams plugins.Params
		defaultSettings := initParams.Settings
		JustBeforeEach(func() {
			destination = route.Action.(*v1.Route_RouteAction).RouteAction.Destination.(*v1.RouteAction_Single).Single
			destination.DestinationSpec = nil
			curParams = params.CopyWithoutContext()
		})
		// Force a cleanup to make it less likely to have pollution via programming error
		JustAfterEach(func() {
			initParams.Settings = defaultSettings
		})

		DescribeTable("processes as expected with various fallback settings", func(pluginSettings *v1.Settings, assertKeyExists types.GomegaMatcher) {
			initParams.Settings = pluginSettings
			awsPlugin.(*Plugin).Init(initParams)
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(curParams, upstream, out)
			Expect(err).NotTo(HaveOccurred())

			err = awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).NotTo(HaveOccurred())

			Expect(outroute.TypedPerFilterConfig).To(assertKeyExists)
		},
			Entry("does not process without fallback set", defaultSettings, Not(HaveKey(FilterName))),
			Entry("does not process with fallback set to false", &v1.Settings{
				Gloo: &v1.GlooOptions{
					AwsOptions: &v1.GlooOptions_AWSOptions{
						FallbackToFirstFunction: &wrapperspb.BoolValue{Value: false},
					},
				},
			}, Not(HaveKey(FilterName))),
			Entry("does process with fallback set to true", &v1.Settings{
				Gloo: &v1.GlooOptions{
					AwsOptions: &v1.GlooOptions_AWSOptions{
						FallbackToFirstFunction: &wrapperspb.BoolValue{Value: true},
					},
				},
			}, HaveKey(FilterName)),
		)

		DescribeTable("response transform override", func(fallback bool, outrouteAssertions ...types.GomegaMatcher) {
			initParams.Settings = &v1.Settings{
				Gloo: &v1.GlooOptions{
					AwsOptions: &v1.GlooOptions_AWSOptions{
						FallbackToFirstFunction: &wrapperspb.BoolValue{Value: fallback},
					},
				},
			}
			awsPlugin.(*Plugin).Init(initParams)
			destination = route.Action.(*v1.Route_RouteAction).RouteAction.Destination.(*v1.RouteAction_Single).Single
			destination.DestinationSpec = nil

			upstream.GetAws().DestinationOverrides = &aws.DestinationSpec{ResponseTransformation: true}
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())

			err = awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).NotTo(HaveOccurred())
			// go through the list of outroute assertions passed
			for _, assert := range outrouteAssertions {
				Expect(outroute.TypedPerFilterConfig).To(assert)
			}
		},
			Entry("gets applied with fallback enabled", true, HaveKey(FilterName), HaveKey(transformation.FilterName)),
			Entry("does not get applies with fallback disabled", false, Not(HaveKey(FilterName)), Not(HaveKey(transformation.FilterName))),
		)
	})

	Context("routes with params", func() {
		// setup similar to the routes context but should exercise special params.
		var curParams plugins.Params
		defaultSettings := initParams.Settings
		JustBeforeEach(func() {
			curParams = params.CopyWithoutContext()
		})
		// Force a cleanup to make it less likely to have pollution via programming error
		JustAfterEach(func() {
			initParams.Settings = defaultSettings
		})
		It("should process if fallback exists", func() {
			initParams.Settings = &v1.Settings{
				Gloo: &v1.GlooOptions{
					AwsOptions: &v1.GlooOptions_AWSOptions{
						FallbackToFirstFunction: &wrapperspb.BoolValue{Value: true},
					},
				},
			}
			awsPlugin.(*Plugin).Init(initParams)
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(curParams, upstream, out)
			Expect(err).NotTo(HaveOccurred())

			err = awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).NotTo(HaveOccurred())
			Expect(outroute.TypedPerFilterConfig).To(HaveKey(FilterName))
		})
	})

	Context("route with defaults", func() {

		It("should apply response transform override", func() {
			upstream.GetAws().DestinationOverrides = &aws.DestinationSpec{ResponseTransformation: true}
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			// destination = route.Action.(*v1.Route_RouteAction).RouteAction.Destination.(*v1.RouteAction_Single).Single

			route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().ResponseTransformation = false

			err = awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).NotTo(HaveOccurred())
			Expect(outroute.TypedPerFilterConfig).To(HaveKey(FilterName))
			Expect(outroute.TypedPerFilterConfig).To(HaveKey(transformation.FilterName))
		})
		It("empty response override should not override route level", func() {
			upstream.GetAws().DestinationOverrides = &aws.DestinationSpec{ResponseTransformation: false}
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())

			route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().ResponseTransformation = true

			err = awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).NotTo(HaveOccurred())
			Expect(outroute.TypedPerFilterConfig).To(HaveKey(FilterName))
			Expect(outroute.TypedPerFilterConfig).To(HaveKey(transformation.FilterName))
		})

	})

	Context("routes", func() {

		var destination *v1.Destination

		JustBeforeEach(func() {
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			destination = route.Action.(*v1.Route_RouteAction).RouteAction.Destination.(*v1.RouteAction_Single).Single
		})

		It("should process route", func() {
			err := awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).NotTo(HaveOccurred())
			Expect(outroute.TypedPerFilterConfig).To(HaveKey(FilterName))
		})

		It("should not process with no spec", func() {
			destination.DestinationSpec = nil

			err := awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).NotTo(HaveOccurred())
			Expect(outroute.TypedPerFilterConfig).NotTo(HaveKey(FilterName))
		})

		It("should not process with a function mismatch", func() {
			destination.DestinationSpec.DestinationType.(*v1.DestinationSpec_Aws).Aws.LogicalName = "somethingelse"

			err := awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).To(HaveOccurred())
			Expect(outroute.TypedPerFilterConfig).NotTo(HaveKey(FilterName))
		})

		It("should process route with response transform", func() {
			route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().ResponseTransformation = true
			err := awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).NotTo(HaveOccurred())
			Expect(outroute.TypedPerFilterConfig).To(HaveKey(FilterName))
			Expect(outroute.TypedPerFilterConfig).To(HaveKey(transformation.FilterName))
		})

		getPerRouteConfig := func(outroute *envoy_config_route_v3.Route) *AWSLambdaPerRoute {
			Expect(outroute.TypedPerFilterConfig).To(HaveKey(FilterName))
			pfc := outroute.GetTypedPerFilterConfig()[FilterName]
			var perRouteCfg AWSLambdaPerRoute
			err := pfc.UnmarshalTo(&perRouteCfg)
			Expect(err).NotTo(HaveOccurred())
			return &perRouteCfg
		}

		It("should set route transformer when unwrapAsApiGateway=True && unwrapAsAlb=False", func() {
			route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().UnwrapAsApiGateway = true
			route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().UnwrapAsAlb = false
			err := awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).NotTo(HaveOccurred())

			cfg := getPerRouteConfig(outroute)
			Expect(cfg.GetUnwrapAsAlb()).To(BeFalse())
			Expect(cfg.GetTransformerConfig()).ToNot(BeNil())
			Expect(cfg.GetTransformerConfig().GetTypedConfig().GetTypeUrl()).To(Equal(ResponseTransformationTypeUrl))
		})

		It("should set route transformer when responseTransformation is true", func() {
			route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().UnwrapAsApiGateway = false
			route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().ResponseTransformation = true
			err := awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).NotTo(HaveOccurred())

			cfg := getPerRouteConfig(outroute)
			Expect(cfg.GetTransformerConfig()).NotTo(BeNil())
			Expect(cfg.GetTransformerConfig().GetTypedConfig().GetTypeUrl()).To(Equal(ResponseTransformationTypeUrl))
		})

		It("should error when unwrapAsApiGateway=True && unwrapAsAlb=True", func() {
			route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().UnwrapAsApiGateway = true
			route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().UnwrapAsAlb = true
			err := awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).To(MatchError("only one of unwrapAsAlb and unwrapAsApiGateway/responseTransformation may be set"))
		})

		It("should not set route transformer when unwrapAsApiGateway=False && responseTransformation=False", func() {
			route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().UnwrapAsApiGateway = false
			route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().ResponseTransformation = false
			err := awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).NotTo(HaveOccurred())

			cfg := getPerRouteConfig(outroute)
			Expect(cfg.GetTransformerConfig()).To(BeNil())
		})

		Context("should interact well with transform plugin", func() {

			var (
				transformationPlugin *transformation.Plugin
			)

			BeforeEach(func() {
				route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().RequestTransformation = true
				route.Options = &v1.RouteOptions{
					StagedTransformations: &v1transformation.TransformationStages{
						Regular: &v1transformation.RequestResponseTransformations{
							RequestTransforms: []*v1transformation.RequestMatch{
								{
									ClearRouteCache: true,
								},
							},
						},
					},
				}
				// The transformation plugin is responsible for validating transformations
				// It does this by executing Envoy in validate mode
				// This functionality is not necessary in our unit tests, so we disable it
				transformationPlugin = transformation.NewPlugin()
				initParams.Settings = &v1.Settings{
					Gateway: &v1.GatewayOptions{
						Validation: &v1.GatewayOptions_ValidationOptions{
							DisableTransformationValidation: &wrapperspb.BoolValue{
								Value: true,
							},
						},
					},
				}
				transformationPlugin.Init(initParams)
			})
			verify := func() {
				Expect(outroute.TypedPerFilterConfig).To(HaveKey(FilterName))
				Expect(outroute.TypedPerFilterConfig).To(HaveKey(transformation.FilterName))

				pfc := outroute.GetTypedPerFilterConfig()[transformation.FilterName]
				var transforms envoytransform.RouteTransformations
				pfc.UnmarshalTo(&transforms)

				Expect(transforms.Transformations).To(HaveLen(2))
			}
			It("should work with aws first", func() {
				err := awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
				Expect(err).NotTo(HaveOccurred())
				err = transformationPlugin.ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
				Expect(err).NotTo(HaveOccurred())
				verify()
			})
			It("should work with transformation first", func() {
				// the same but in reverse order
				err := transformationPlugin.ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
				Expect(err).NotTo(HaveOccurred())
				err = awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
				Expect(err).NotTo(HaveOccurred())
				verify()
			})
		})

		It("should process route with addon options", func() {
			route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().UnwrapAsAlb = true
			route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().InvocationStyle = 1
			err := awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).NotTo(HaveOccurred())

			msg, err := utils.AnyToMessage(outroute.GetTypedPerFilterConfig()[FilterName])
			Expect(err).Should(BeNil())
			cfg := msg.(*AWSLambdaPerRoute)
			Expect(cfg.UnwrapAsAlb).Should(Equal(true))
			Expect(cfg.Async).Should(Equal(true))
		})

		When("unwrapping response", func() {
			BeforeEach(func() {
				route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().UnwrapAsAlb = true
			})
			It("should not apply transformations", func() {
				err := awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
				Expect(err).NotTo(HaveOccurred())

				msg, err := utils.AnyToMessage(outroute.GetTypedPerFilterConfig()[FilterName])
				Expect(err).Should(BeNil())
				cfg := msg.(*AWSLambdaPerRoute)
				Expect(cfg.UnwrapAsAlb).Should(Equal(true))
				Expect(cfg.GetTransformerConfig()).Should(BeNil())
			})
		})
	})

	Context("filters", func() {
		It("should produce filters when upstream is present", func() {
			// process upstream
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			err = awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).NotTo(HaveOccurred())

			// check that we have filters
			filters, err := awsPlugin.(plugins.HttpFilterPlugin).HttpFilters(params, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(filters).To(HaveLen(1))
		})

		It("should not produce filters when no upstreams are present", func() {
			filters, err := awsPlugin.(plugins.HttpFilterPlugin).HttpFilters(params, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(filters).To(BeEmpty())
		})

		When("transformations are present", func() {
			It("should produce 2 filters when not unwrapping", func() {
				err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
				Expect(err).NotTo(HaveOccurred())
				route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().RequestTransformation = true
				err = awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
				Expect(err).NotTo(HaveOccurred())

				filters, err := awsPlugin.(plugins.HttpFilterPlugin).HttpFilters(params, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(filters).To(HaveLen(2))
			})
			It("should error when unwrapping", func() {
				err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
				Expect(err).NotTo(HaveOccurred())
				route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().ResponseTransformation = true
				route.GetRouteAction().GetSingle().GetDestinationSpec().GetAws().UnwrapAsAlb = true
				err = awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
				Expect(err).To(MatchError("only one of unwrapAsAlb and unwrapAsApiGateway/responseTransformation may be set"))
			})
		})
	})

	Context("enabled default creds", func() {

		var (
			cfg *AWSLambdaConfig
		)

		BeforeEach(func() {
			cfg = &AWSLambdaConfig{}

			initParams.Settings = &v1.Settings{
				Gloo: &v1.GlooOptions{
					AwsOptions: &v1.GlooOptions_AWSOptions{
						CredentialsFetcher: &v1.GlooOptions_AWSOptions_EnableCredentialsDiscovey{
							EnableCredentialsDiscovey: true,
						},
					},
				},
			}
			// remove secrets from upstream
			upstream.GetAws().SecretRef = nil
		})

		process := func() {
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			processProtocolOptions()

			filters, err := awsPlugin.(plugins.HttpFilterPlugin).HttpFilters(params, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(filters).To(HaveLen(1))
			goTypedConfig := filters[0].HttpFilter.GetTypedConfig()
			err = ptypes.UnmarshalAny(goTypedConfig, cfg)
			Expect(err).NotTo(HaveOccurred())

		}

		It("should enable default credentials in the filter", func() {
			process()
			Expect(cfg.GetUseDefaultCredentials().GetValue()).To(BeTrue())
		})

		It("should enable default but still use secret ref if it is there", func() {
			upstream.GetAws().SecretRef = &core.ResourceRef{
				Namespace: "ns",
				Name:      "secretref",
			}

			process()

			Expect(cfg.GetUseDefaultCredentials().GetValue()).To(BeTrue())
			Expect(lpe.AccessKey).To(Equal(accessKeyValue))
			Expect(lpe.SecretKey).To(Equal(secretKeyValue))
		})

		It("will add the token if it is present on the secret", func() {
			upstream.GetAws().SecretRef = &core.ResourceRef{
				Namespace: "ns",
				Name:      "secretref",
			}
			awsSecret := params.Snapshot.Secrets[0].GetAws()
			awsSecret.SessionToken = sessionTokenValue

			process()

			Expect(cfg.GetUseDefaultCredentials().GetValue()).To(BeTrue())
			Expect(lpe.AccessKey).To(Equal(accessKeyValue))
			Expect(lpe.SecretKey).To(Equal(secretKeyValue))
			Expect(lpe.SessionToken).To(Equal(sessionTokenValue))
		})

	})

	Context("service account creds", func() {

		var (
			cfg *AWSLambdaConfig

			saCredentials = &AWSLambdaConfig_ServiceAccountCredentials{
				Cluster: "aws_sts",
				Uri:     "sts.amazonaws.com",
				Timeout: &duration.Duration{
					Seconds: 5,
					Nanos:   5,
				},
			}

			roleArn = "role_arn"
		)

		BeforeEach(func() {
			cfg = &AWSLambdaConfig{}

			initParams.Settings = &v1.Settings{
				Gloo: &v1.GlooOptions{
					AwsOptions: &v1.GlooOptions_AWSOptions{
						CredentialsFetcher: &v1.GlooOptions_AWSOptions_ServiceAccountCredentials{
							ServiceAccountCredentials: saCredentials,
						},
					},
				},
			}
			// remove secrets from upstream
			upstream.GetAws().SecretRef = nil
			upstream.GetAws().RoleArn = roleArn
		})

		process := func() {
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
			processProtocolOptions()

			filters, err := awsPlugin.(plugins.HttpFilterPlugin).HttpFilters(params, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(filters).To(HaveLen(1))
			goTypedConfig := filters[0].HttpFilter.GetTypedConfig()
			err = ptypes.UnmarshalAny(goTypedConfig, cfg)
			Expect(err).NotTo(HaveOccurred())
		}

		It("should enable service account credentials in the filter", func() {
			process()
			saCredentialsExpected := cfg.GetServiceAccountCredentials()
			Expect(saCredentialsExpected).NotTo(BeNil())
			Expect(saCredentialsExpected).To(matchers.MatchProto(saCredentials))
		})
		It("can enable all config options", func() {

			initParams.Settings.Gloo.AwsOptions.PropagateOriginalRouting = wrapperspb.Bool(true)
			initParams.Settings.Gloo.AwsOptions.CredentialRefreshDelay = &duration.Duration{Seconds: 1}

			process()
			Expect(cfg.PropagateOriginalRouting).To(Equal(true))
			Expect(*cfg.CredentialRefreshDelay).To(Equal(duration.Duration{Seconds: 1}))
		})

		It("will add the token if it is present on the secret", func() {
			upstream.GetAws().SecretRef = &core.ResourceRef{
				Namespace: "ns",
				Name:      "secretref",
			}
			awsSecret := params.Snapshot.Secrets[0].GetAws()
			awsSecret.SessionToken = sessionTokenValue

			process()

			saCredentialsExpected := cfg.GetServiceAccountCredentials()
			Expect(saCredentialsExpected).NotTo(BeNil())
			Expect(saCredentialsExpected).To(matchers.MatchProto(saCredentials))
			Expect(lpe.AccessKey).To(Equal(accessKeyValue))
			Expect(lpe.SecretKey).To(Equal(secretKeyValue))
			Expect(lpe.SessionToken).To(Equal(sessionTokenValue))
			Expect(lpe.RoleArn).To(Equal(roleArn))
			Expect(lpe.DisableRoleChaining).To(Equal(true))
		})

	})

	Context("ExtraAccountCredentials", func() {
		JustBeforeEach(func() {
			upstream.UpstreamType.(*v1.Upstream_Aws).Aws.AwsAccountId = "222222222222"
			err := awsPlugin.(plugins.UpstreamPlugin).ProcessUpstream(params, upstream, out)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should process route", func() {
			err := awsPlugin.(plugins.RoutePlugin).ProcessRoute(plugins.RouteParams{VirtualHostParams: vhostParams}, route, outroute)
			Expect(err).NotTo(HaveOccurred())
			msg, err := utils.AnyToMessage(outroute.GetTypedPerFilterConfig()[FilterName])
			Expect(err).Should(BeNil())
			cfg := msg.(*AWSLambdaPerRoute)

			Expect(cfg.Name).Should(Equal(url.QueryEscape("arn:aws:lambda:us-east1:222222222222:function:foo")))
		})
	})
})

func getClusterTlsContext(cluster *envoy_config_cluster_v3.Cluster) *envoyauth.UpstreamTlsContext {
	return utils.MustAnyToMessage(cluster.TransportSocket.GetTypedConfig()).(*envoyauth.UpstreamTlsContext)
}
