package aws_test

import (
	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	gogoproto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/duration"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/aws"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	awsapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/test/matchers"
)

const (
	accessKeyValue    = "some acccess value"
	secretKeyValue    = "some secret value"
	sessionTokenValue = "some session token value"
)

var _ = Describe("Plugin", func() {

	var (
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
		var b bool
		awsPlugin = NewPlugin(&b)

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
									Aws: &awsapi.DestinationSpec{
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
		params.Snapshot = &v1.ApiSnapshot{
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
		err := awsPlugin.Init(initParams)
		Expect(err).NotTo(HaveOccurred())
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
							SslParameters: &v1.SslParameters{
								MinimumProtocolVersion: v1.SslParameters_TLSv1_1,
								MaximumProtocolVersion: v1.SslParameters_TLSv1_2,
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

				var invalidProtocolVersion v1.SslParameters_ProtocolVersion = 5 // INVALID

				BeforeEach(func() {
					initParams.Settings = &v1.Settings{
						UpstreamOptions: &v1.UpstreamOptions{
							SslParameters: &v1.SslParameters{
								MinimumProtocolVersion: invalidProtocolVersion,
								MaximumProtocolVersion: v1.SslParameters_TLSv1_2,
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
			Expect(filters).NotTo(BeEmpty())
		})

		It("should not produce filters when no upstreams are present", func() {
			filters, err := awsPlugin.(plugins.HttpFilterPlugin).HttpFilters(params, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(filters).To(BeEmpty())
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
		})

	})
})

func getClusterTlsContext(cluster *envoy_config_cluster_v3.Cluster) *envoyauth.UpstreamTlsContext {
	return utils.MustAnyToMessage(cluster.TransportSocket.GetTypedConfig()).(*envoyauth.UpstreamTlsContext)
}
