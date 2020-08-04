package aws_test

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/api/v2/route"
	gogoproto "github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/aws"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	awsapi "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	. "github.com/solo-io/gloo/projects/gloo/pkg/plugins/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/gloo/test/matchers"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

const (
	accessKeyValue    = "some acccess value"
	secretKeyValue    = "some secret value"
	sessionTokenValue = "some session token value"
)

var _ = Describe("Plugin", func() {
	var (
		params      plugins.Params
		vhostParams plugins.VirtualHostParams
		awsPlugin   plugins.Plugin
		upstream    *v1.Upstream
		route       *v1.Route
		out         *envoyapi.Cluster
		outroute    *envoyroute.Route
		lpe         *AWSLambdaProtocolExtension
	)
	BeforeEach(func() {
		var b bool
		awsPlugin = NewPlugin(&b)
		awsPlugin.Init(plugins.InitParams{})
		upstreamName := "up"
		clusterName := upstreamName
		funcName := "foo"
		upstream = &v1.Upstream{
			Metadata: core.Metadata{
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
			Expect(err).To(MatchError("secret {secretref ns} is not an AWS secret"))
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
	})

	Context("routes", func() {

		var destination *v1.Destination

		BeforeEach(func() {
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

			awsPlugin.Init(plugins.InitParams{
				Settings: &v1.Settings{
					Gloo: &v1.GlooOptions{
						AwsOptions: &v1.GlooOptions_AWSOptions{
							CredentialsFetcher: &v1.GlooOptions_AWSOptions_EnableCredentialsDiscovey{
								EnableCredentialsDiscovey: true,
							},
						},
					},
				},
			})
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
			gogoTypedConfig := &types.Any{TypeUrl: goTypedConfig.TypeUrl, Value: goTypedConfig.Value}
			err = types.UnmarshalAny(gogoTypedConfig, cfg)
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
				Timeout: &types.Duration{
					Seconds: 5,
					Nanos:   5,
				},
			}

			roleArn = "role_arn"
		)

		BeforeEach(func() {
			cfg = &AWSLambdaConfig{}

			awsPlugin.Init(plugins.InitParams{
				Settings: &v1.Settings{
					Gloo: &v1.GlooOptions{
						AwsOptions: &v1.GlooOptions_AWSOptions{
							CredentialsFetcher: &v1.GlooOptions_AWSOptions_ServiceAccountCredentials{
								ServiceAccountCredentials: saCredentials,
							},
						},
					},
				},
			})
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
			gogoTypedConfig := &types.Any{TypeUrl: goTypedConfig.TypeUrl, Value: goTypedConfig.Value}
			err = types.UnmarshalAny(gogoTypedConfig, cfg)
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
