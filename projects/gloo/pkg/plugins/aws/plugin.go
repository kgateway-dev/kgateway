package aws

import (
	"context"
	"fmt"
	"net/url"
	"unicode/utf8"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/hashicorp/go-multierror"
	. "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/aws"
	envoy_transform "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/extensions/transformation"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options/aws"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/pluginutils"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/transformation"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/upstreams"
	"github.com/solo-io/gloo/projects/gloo/pkg/utils"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/errors"
)

var (
	_ plugins.Plugin           = new(plugin)
	_ plugins.UpstreamPlugin   = new(plugin)
	_ plugins.RoutePlugin      = new(plugin)
	_ plugins.HttpFilterPlugin = new(plugin)
)

const (
	ExtensionName = "aws_lambda"
	// filter info
	FilterName = "io.solo.aws_lambda"
)

var pluginStage = plugins.DuringStage(plugins.OutAuthStage)

type plugin struct {
	recordedUpstreams map[string]*aws.UpstreamSpec
	ctx               context.Context
	// earlyTransformsAdded is intended to point to the RequireEarlyTransformation property
	// in the transformation plugin, which controls whether early-stage transforms will be processed
	// see AWS plugin instantiation at the following link as an example:
	// https://github.com/solo-io/gloo/blob/2168dff1344d2b488d74cb2c1baabe10a9301757/projects/gloo/pkg/plugins/registry/registry.go#L61
	earlyTransformsAdded *bool
	settings             *v1.GlooOptions_AWSOptions
	upstreamOptions      *v1.UpstreamOptions
}

func NewPlugin(earlyTransformsAdded *bool) plugins.Plugin {
	return &plugin{
		earlyTransformsAdded: earlyTransformsAdded,
	}
}

func (p *plugin) Name() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) error {
	p.ctx = params.Ctx
	p.recordedUpstreams = make(map[string]*aws.UpstreamSpec)
	p.settings = params.Settings.GetGloo().GetAwsOptions()
	p.upstreamOptions = params.Settings.GetUpstreamOptions()
	return nil
}

func getLambdaHostname(s *aws.UpstreamSpec) string {
	return fmt.Sprintf("lambda.%s.amazonaws.com", s.GetRegion())
}

func (p *plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {
	upstreamSpec, ok := in.GetUpstreamType().(*v1.Upstream_Aws)
	if !ok {
		// not ours
		return nil
	}
	// even if it failed, route should still be valid
	p.recordedUpstreams[translator.UpstreamToClusterName(in.GetMetadata().Ref())] = upstreamSpec.Aws

	lambdaHostname := getLambdaHostname(upstreamSpec.Aws)

	// configure Envoy cluster routing info
	out.ClusterDiscoveryType = &envoy_config_cluster_v3.Cluster_Type{
		Type: envoy_config_cluster_v3.Cluster_LOGICAL_DNS,
	}
	// TODO(yuval-k): why do we need to make sure we use ipv4 only dns?
	out.DnsLookupFamily = envoy_config_cluster_v3.Cluster_V4_ONLY
	pluginutils.EnvoySingleEndpointLoadAssignment(out, lambdaHostname, 443)

	commonTlsContext, err := utils.GetCommonTlsContextFromUpstreamOptions(p.upstreamOptions)
	if err != nil {
		return err
	}
	tlsContext := &envoyauth.UpstreamTlsContext{
		CommonTlsContext: commonTlsContext,
		// TODO(yuval-k): Add verification context
		Sni: lambdaHostname,
	}
	out.TransportSocket = &envoy_config_core_v3.TransportSocket{
		Name:       wellknown.TransportSocketTls,
		ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: utils.MustMessageToAny(tlsContext)},
	}

	var accessKey, sessionToken, secretKey string
	if upstreamSpec.Aws.GetSecretRef() == nil &&
		!p.settings.GetEnableCredentialsDiscovey() &&
		p.settings.GetServiceAccountCredentials() == nil {
		return errors.Errorf("no aws secret provided. consider setting enableCredentialsDiscovey to true or enabling service account credentials if running in EKS")
	}

	if upstreamSpec.Aws.GetSecretRef() != nil {

		secret, err := params.Snapshot.Secrets.Find(upstreamSpec.Aws.GetSecretRef().Strings())
		if err != nil {
			return errors.Wrapf(err, "retrieving aws secret")
		}

		awsSecrets, ok := secret.GetKind().(*v1.Secret_Aws)
		if !ok {
			return errors.Errorf("secret (%s.%s) is not an AWS secret", secret.GetMetadata().GetName(), secret.GetMetadata().GetNamespace())
		}

		var secretErrs error

		accessKey = awsSecrets.Aws.GetAccessKey()
		secretKey = awsSecrets.Aws.GetSecretKey()
		sessionToken = awsSecrets.Aws.GetSessionToken()
		if accessKey == "" || !utf8.Valid([]byte(accessKey)) {
			secretErrs = multierror.Append(secretErrs, errors.Errorf("access_key is not a valid string"))
		}
		if secretKey == "" || !utf8.Valid([]byte(secretKey)) {
			secretErrs = multierror.Append(secretErrs, errors.Errorf("secret_key is not a valid string"))
		}
		// Session key is optional
		if sessionToken != "" && !utf8.Valid([]byte(sessionToken)) {
			secretErrs = multierror.Append(secretErrs, errors.Errorf("session_key is not a valid string"))
		}

		if secretErrs != nil {
			return secretErrs
		}

	}

	lpe := &AWSLambdaProtocolExtension{
		Host:         lambdaHostname,
		Region:       upstreamSpec.Aws.GetRegion(),
		AccessKey:    accessKey,
		SecretKey:    secretKey,
		SessionToken: sessionToken,
		RoleArn:      upstreamSpec.Aws.GetRoleArn(),
	}

	if err := pluginutils.SetExtensionProtocolOptions(out, FilterName, lpe); err != nil {
		return errors.Wrapf(err, "converting aws protocol options to struct")
	}
	return nil
}

func (p *plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	err := pluginutils.MarkPerFilterConfig(p.ctx, params.Snapshot, in, out, FilterName,
		func(spec *v1.Destination) (proto.Message, error) {
			// check if it's aws destination
			if spec.GetDestinationSpec() == nil {
				return nil, nil
			}
			awsDestinationSpec, ok := spec.GetDestinationSpec().GetDestinationType().(*v1.DestinationSpec_Aws)
			if !ok {
				return nil, nil
			}

			// get upstream
			upstreamRef, err := upstreams.DestinationToUpstreamRef(spec)
			if err != nil {
				contextutils.LoggerFrom(p.ctx).Error(err)
				return nil, err
			}
			lambdaSpec, ok := p.recordedUpstreams[translator.UpstreamToClusterName(upstreamRef)]
			if !ok {
				err := errors.Errorf("%v is not an AWS upstream", *upstreamRef)
				contextutils.LoggerFrom(p.ctx).Error(err)
				return nil, err
			}
			// should be aws upstream

			// get function
			logicalName := awsDestinationSpec.Aws.GetLogicalName()
			for _, lambdaFunc := range lambdaSpec.GetLambdaFunctions() {
				if lambdaFunc.GetLogicalName() == logicalName {

					lambdaRouteFunc := &AWSLambdaPerRoute{
						Async: awsDestinationSpec.Aws.GetInvocationStyle() == aws.DestinationSpec_ASYNC,
						// we need to query escape per AWS spec:
						// see the CanonicalQueryString section in here: https://docs.aws.amazon.com/general/latest/gr/sigv4-create-canonical-request.html
						Qualifier:   url.QueryEscape(lambdaFunc.GetQualifier()),
						Name:        lambdaFunc.GetLambdaFunctionName(),
						UnwrapAsAlb: awsDestinationSpec.Aws.GetUnwrapAsAlb(),
					}

					return lambdaRouteFunc, nil
				}
			}
			return nil, errors.Errorf("unknown function %v", logicalName)
		},
	)

	if err != nil {
		return err
	}
	return pluginutils.MarkPerFilterConfig(p.ctx, params.Snapshot, in, out, transformation.FilterName,
		func(spec *v1.Destination) (proto.Message, error) {
			// check if it's aws destination
			if spec.GetDestinationSpec() == nil {
				return nil, nil
			}
			awsDestinationSpec, ok := spec.GetDestinationSpec().GetDestinationType().(*v1.DestinationSpec_Aws)
			if !ok {
				return nil, nil
			}

			transformations := []*envoy_transform.RouteTransformations_RouteTransformation{}

			requesttransform := awsDestinationSpec.Aws.GetRequestTransformation()
			if requesttransform {
				// Early stage transform: place all headers in the request body
				transformations = append(transformations, &envoy_transform.RouteTransformations_RouteTransformation{
					Stage: transformation.EarlyStageNumber,
					Match: &envoy_transform.RouteTransformations_RouteTransformation_RequestMatch_{
						RequestMatch: &envoy_transform.RouteTransformations_RouteTransformation_RequestMatch{
							RequestTransformation: &envoy_transform.Transformation{
								TransformationType: &envoy_transform.Transformation_HeaderBodyTransform{
									HeaderBodyTransform: &envoy_transform.HeaderBodyTransform{},
								},
							},
						},
					},
				})

				// Regular stage transform: extract the path and querystring
				transformations = append(transformations, &envoy_transform.RouteTransformations_RouteTransformation{
					Match: &envoy_transform.RouteTransformations_RouteTransformation_RequestMatch_{
						RequestMatch: &envoy_transform.RouteTransformations_RouteTransformation_RequestMatch{
							RequestTransformation: &envoy_transform.Transformation{
								TransformationType: &envoy_transform.Transformation_TransformationTemplate{
									TransformationTemplate: &envoy_transform.TransformationTemplate{
										Extractors: map[string]*envoy_transform.Extraction{
											"path": {
												Source:   &envoy_transform.Extraction_Header{Header: ":path"},
												Regex:    `([^\?]+)(\?.*)?`,
												Subgroup: uint32(1),
											},
											"queryString": {
												Source:   &envoy_transform.Extraction_Header{Header: ":path"},
												Regex:    `([^\?]+)(\?(.*))?`,
												Subgroup: uint32(3),
											},
											"httpMethod": {
												Source:   &envoy_transform.Extraction_Header{Header: ":method"},
												Regex:    `(.*)`,
												Subgroup: uint32(1),
											},
										},
										BodyTransformation: &envoy_transform.TransformationTemplate_MergeExtractorsToBody{
											MergeExtractorsToBody: &envoy_transform.MergeExtractorsToBody{},
										},
									},
								},
							},
						},
					},
				})

				// Tell the transformation filter to process early-stage transformations
				*p.earlyTransformsAdded = true
			}

			repsonsetransform := awsDestinationSpec.Aws.GetResponseTransformation()
			if repsonsetransform {
				transformations = append(transformations, &envoy_transform.RouteTransformations_RouteTransformation{
					Match: &envoy_transform.RouteTransformations_RouteTransformation_ResponseMatch_{
						ResponseMatch: &envoy_transform.RouteTransformations_RouteTransformation_ResponseMatch{
							ResponseTransformation: &envoy_transform.Transformation{
								TransformationType: &envoy_transform.Transformation_TransformationTemplate{
									TransformationTemplate: &envoy_transform.TransformationTemplate{
										BodyTransformation: &envoy_transform.TransformationTemplate_Body{
											Body: &envoy_transform.InjaTemplate{
												Text: "{{body}}",
											},
										},
										Headers: map[string]*envoy_transform.InjaTemplate{
											"content-type": {
												Text: "text/html",
											},
										},
									},
								},
							},
						},
					},
				})
			}

			return &envoy_transform.RouteTransformations{
				Transformations: transformations,
			}, nil
		},
	)
}

func (p *plugin) HttpFilters(_ plugins.Params, _ *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	if len(p.recordedUpstreams) == 0 {
		// no upstreams no filter
		return nil, nil
	}
	filterconfig := &AWSLambdaConfig{}
	switch typedFetcher := p.settings.GetCredentialsFetcher().(type) {
	case *v1.GlooOptions_AWSOptions_EnableCredentialsDiscovey:
		filterconfig.CredentialsFetcher = &AWSLambdaConfig_UseDefaultCredentials{
			UseDefaultCredentials: &wrappers.BoolValue{
				Value: typedFetcher.EnableCredentialsDiscovey,
			},
		}
	case *v1.GlooOptions_AWSOptions_ServiceAccountCredentials:
		filterconfig.CredentialsFetcher = &AWSLambdaConfig_ServiceAccountCredentials_{
			ServiceAccountCredentials: typedFetcher.ServiceAccountCredentials,
		}
	}
	filterconfig.CredentialRefreshDelay = p.settings.GetCredentialRefreshDelay()
	filterconfig.PropagateOriginalRouting = p.settings.GetPropagateOriginalRouting().GetValue()

	f, err := plugins.NewStagedFilterWithConfig(FilterName, filterconfig, pluginStage)
	if err != nil {
		return nil, err
	}

	filters := []plugins.StagedHttpFilter{
		f,
	}

	return filters, nil
}
