package aws

import (
	"fmt"
	"net/url"
	"os"
	"unicode/utf8"

	"github.com/hashicorp/go-multierror"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/imdario/mergo"
	v3 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/config/core/v3"
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
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
)

var (
	_ plugins.Plugin           = new(Plugin)
	_ plugins.UpstreamPlugin   = new(Plugin)
	_ plugins.RoutePlugin      = new(Plugin)
	_ plugins.HttpFilterPlugin = new(Plugin)
)

const (
	ExtensionName                 = "aws_lambda"
	FilterName                    = "io.solo.aws_lambda"
	ResponseTransformationName    = "io.solo.api_gateway.api_gateway_transformer"
	ResponseTransformationTypeUrl = "type.googleapis.com/envoy.config.transformer.aws_lambda.v2.ApiGatewayTransformation"
)

// PerRouteConfigGenerator defines how to build the Per Route Configuration for a Lambda upstream
// This enables the open source and enterprise definitions to differ, but still share the same core plugin functionality
type PerRouteConfigGenerator func(options *v1.GlooOptions_AWSOptions,
	destination *aws.DestinationSpec, upstream *aws.UpstreamSpec) (*AWSLambdaPerRoute, error)

var (
	pluginStage          = plugins.DuringStage(plugins.OutAuthStage)
	transformPluginStage = plugins.BeforeStage(plugins.OutAuthStage)
)

type Plugin struct {
	perRouteConfigGenerator      PerRouteConfigGenerator
	recordedUpstreams            map[string]*aws.UpstreamSpec
	settings                     *v1.GlooOptions_AWSOptions
	upstreamOptions              *v1.UpstreamOptions
	requiresTransformationFilter bool
}

// NewPlugin creates an instance of the aws plugin and sets the non-per run
// configuration set by the perrouteconfiggenerator.
func NewPlugin(perRouteConfigGenerator PerRouteConfigGenerator) plugins.Plugin {
	return &Plugin{
		perRouteConfigGenerator: perRouteConfigGenerator,
	}
}

// Name is basically a seperate stringer that returns aws_lambda
func (p *Plugin) Name() string {
	return ExtensionName
}

// Init the per run configuration of the plugin including blowing away the known upstreams,
// the current settings for the plugin, and whether we currently need transformation.
func (p *Plugin) Init(params plugins.InitParams) {
	p.recordedUpstreams = make(map[string]*aws.UpstreamSpec)
	p.settings = params.Settings.GetGloo().GetAwsOptions()
	p.upstreamOptions = params.Settings.GetUpstreamOptions()
	p.requiresTransformationFilter = false
}

func getLambdaHostname(s *aws.UpstreamSpec) string {
	return fmt.Sprintf("lambda.%s.amazonaws.com", s.GetRegion())
}

func (p *Plugin) ProcessUpstream(params plugins.Params, in *v1.Upstream, out *envoy_config_cluster_v3.Cluster) error {
	upstreamSpec, ok := in.GetUpstreamType().(*v1.Upstream_Aws)
	if !ok {
		// this is not an aws upstream so we disregard
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
	// TODO(nfuden): Update to reasonable ipv6 https://aws.amazon.com/about-aws/whats-new/2021/12/aws-lambda-ipv6-endpoints-inbound-connections/
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
	typedConfig, err := utils.MessageToAny(tlsContext)
	if err != nil {
		return err
	}
	out.TransportSocket = &envoy_config_core_v3.TransportSocket{
		Name:       wellknown.TransportSocketTls,
		ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
	}

	// To utilize the aws lambda plugin much of the power comes via its secret management
	// Check that one of the supported auth paradigms in enabled.
	// Currently: static secret ref, credential discovery or ServiceAccountCreds such in eks

	if upstreamSpec.Aws.GetSecretRef() == nil &&
		!p.settings.GetEnableCredentialsDiscovey() &&
		p.settings.GetServiceAccountCredentials() == nil {
		return errors.Errorf("no aws secret provided. consider setting enableCredentialsDiscovey to true or enabling service account credentials if running in EKS")
	}

	// If static secret is set retrieve the information needed
	var accessKey, sessionToken, secretKey string
	if upstreamSpec.Aws.GetSecretRef() != nil {
		accessKey, sessionToken, secretKey, err = deriveStaticSecret(params, upstreamSpec.Aws.GetSecretRef())
		if err != nil {
			return err
		}
	}

	lpe := &AWSLambdaProtocolExtension{
		Host:                lambdaHostname,
		Region:              upstreamSpec.Aws.GetRegion(),
		AccessKey:           accessKey,
		SecretKey:           secretKey,
		SessionToken:        sessionToken,
		RoleArn:             upstreamSpec.Aws.GetRoleArn(),
		DisableRoleChaining: upstreamSpec.Aws.GetDisableRoleChaining(),
	}

	if err := pluginutils.SetExtensionProtocolOptions(out, FilterName, lpe); err != nil {
		return errors.Wrapf(err, "converting aws protocol options to struct")
	}
	return nil
}

func (p *Plugin) ProcessRoute(params plugins.RouteParams, in *v1.Route, out *envoy_config_route_v3.Route) error {
	err := pluginutils.MarkPerFilterConfig(params.Ctx, params.Snapshot, in, out, FilterName,
		func(spec *v1.Destination) (proto.Message, error) {
			logger := contextutils.LoggerFrom(params.Ctx)
			// local variable to avoid side effects for calls that are not to aws upstreams
			dest := spec.GetDestinationSpec()
			tryingNonExplicitAWSDest := dest == nil && p.settings.GetFallbackToFirstFunction().GetValue()

			// users do not have to set the aws destination spec on the route if they have fallback enabled.
			// check for this and update the local variable to not cause destination side effects until the end when we
			// are sure that this is pointing to a valid aws upstream
			if tryingNonExplicitAWSDest {
				logger.Debug("no destinationSpec set with fallbackToFirstFunction enabled, processing as aws route")
				dest = &v1.DestinationSpec{
					DestinationType: &v1.DestinationSpec_Aws{
						Aws: &aws.DestinationSpec{},
					},
				}
			}

			// it is incorrect to set lambda functionality on routes that do not have a lambda function specified
			// unless we fallback. and the fallback case has been handled above. Therefore, skip.
			if dest == nil {
				return nil, nil
			}
			// check if the destination is an aws destinationtype and skip the function if not
			awsDestinationSpec, ok := dest.GetDestinationType().(*v1.DestinationSpec_Aws)
			if !ok {
				return nil, nil
			}

			// warn user if they are using deprecated responseTransformation
			if awsDestinationSpec.Aws.GetResponseTransformation() {
				logger.Warn("field responseTransformation is deprecated; consider using unwrapAsApiGateway")
			}

			upstreamRef, err := upstreams.DestinationToUpstreamRef(spec)
			if err != nil {
				logger.Error(err)
				return nil, err
			}

			// validate that the upstream is one that we have previously recorded as an aws upstream
			lambdaSpec, ok := p.recordedUpstreams[translator.UpstreamToClusterName(upstreamRef)]
			if !ok {
				if tryingNonExplicitAWSDest {
					// skip the lambda plugin as the route was not explicitly set to be an aws route
					// so it is fine that this is not an aws upstream
					return nil, nil
				}
				// error as we have a route that `thinks` its pointing to an aws upstream
				// but the upstream does not believe that it is an aws upstream
				err := errors.Errorf("%v is not an AWS upstream", *upstreamRef)
				logger.Error(err)
				return nil, err
			}

			// persist that we are treating this as an aws route due to the
			// upstream being a valid aws upstream
			if tryingNonExplicitAWSDest {
				spec.DestinationSpec = dest
			}

			return p.perRouteConfigGenerator(p.settings, awsDestinationSpec.Aws, lambdaSpec)
		},
	)

	if err != nil {
		return err
	}
	return pluginutils.ModifyPerFilterConfig(params.Ctx, params.Snapshot, in, out, transformation.FilterName,
		func(spec *v1.Destination, existing *any.Any) (proto.Message, error) {
			// check if it's aws destination
			if spec.GetDestinationSpec() == nil {
				return nil, nil
			}
			awsDestinationSpec, ok := spec.GetDestinationSpec().GetDestinationType().(*v1.DestinationSpec_Aws)
			if !ok {
				return nil, nil
			}
			awsDestination := awsDestinationSpec.Aws

			requiresRequestTransformation := awsDestination.GetRequestTransformation()

			// We should have already mutated awsDestination.UnwrapAsApiGateway if awsDestination.ResponseTransformation
			// is set to true. This is done in GenerateAWSLambdaRouteConfig
			requiresUnwrap :=
				// unwrapAsAlb is handled directly in the aws lambda filter rather than through a transformer.
				// If both are set, we error in GenerateAWSLambdaRouteConfig.
				awsDestination.GetUnwrapAsApiGateway()

			if !requiresRequestTransformation && !requiresUnwrap {
				return nil, nil
			}

			p.requiresTransformationFilter = true

			var reqTransform *envoy_transform.Transformation
			if requiresRequestTransformation {
				reqTransform = &envoy_transform.Transformation{
					TransformationType: &envoy_transform.Transformation_HeaderBodyTransform{
						HeaderBodyTransform: &envoy_transform.HeaderBodyTransform{
							AddRequestMetadata: true,
						},
					},
				}
			}

			var transform *envoy_transform.RouteTransformations_RouteTransformation
			if requiresRequestTransformation {
				// Early stage transform: place all headers in the request body
				transform = &envoy_transform.RouteTransformations_RouteTransformation{
					Stage: transformation.AwsStageNumber,
					Match: &envoy_transform.RouteTransformations_RouteTransformation_RequestMatch_{
						RequestMatch: &envoy_transform.RouteTransformations_RouteTransformation_RequestMatch{
							RequestTransformation: reqTransform,
						},
					},
				}
			} else {
				p.requiresTransformationFilter = false
			}

			var transforms envoy_transform.RouteTransformations
			if existing != nil {
				err := existing.UnmarshalTo(&transforms)
				if err != nil {
					// this should never happen
					return nil, err
				}
			}
			if transform != nil {
				transforms.Transformations = append(transforms.GetTransformations(), transform)
			}
			return &transforms, nil
		},
	)
}

func (p *Plugin) HttpFilters(_ plugins.Params, _ *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	if len(p.recordedUpstreams) == 0 {
		// no upstreams no filter
		return nil, nil
	}
	filterConfig := &AWSLambdaConfig{}
	switch typedFetcher := p.settings.GetCredentialsFetcher().(type) {
	case *v1.GlooOptions_AWSOptions_EnableCredentialsDiscovey:
		filterConfig.CredentialsFetcher = &AWSLambdaConfig_UseDefaultCredentials{
			UseDefaultCredentials: &wrappers.BoolValue{
				Value: typedFetcher.EnableCredentialsDiscovey,
			},
		}
	case *v1.GlooOptions_AWSOptions_ServiceAccountCredentials:
		filterConfig.CredentialsFetcher = &AWSLambdaConfig_ServiceAccountCredentials_{
			ServiceAccountCredentials: typedFetcher.ServiceAccountCredentials,
		}
	}
	filterConfig.CredentialRefreshDelay = p.settings.GetCredentialRefreshDelay()
	filterConfig.PropagateOriginalRouting = p.settings.GetPropagateOriginalRouting().GetValue()

	f, err := plugins.NewStagedFilter(FilterName, filterConfig, pluginStage)
	if err != nil {
		return nil, err
	}

	filters := []plugins.StagedHttpFilter{
		f,
	}
	if p.requiresTransformationFilter {
		awsStageConfig := &envoy_transform.FilterTransformations{
			Stage: transformation.AwsStageNumber,
		}
		tf, err := plugins.NewStagedFilter(transformation.FilterName, awsStageConfig, transformPluginStage)
		if err != nil {
			return nil, err
		}
		filters = append(filters, tf)
	}

	return filters, nil
}

// GenerateAWSLambdaRouteConfig is an overridable way to handle destination logic for lambdas.
// Passed in at plugin creation as it fulfills PerRouteConfigGenerator interface
func GenerateAWSLambdaRouteConfig(options *v1.GlooOptions_AWSOptions, destination *aws.DestinationSpec, upstream *aws.UpstreamSpec) (*AWSLambdaPerRoute, error) {

	// merge the non-default values (trues and non-zeros) from default onto destination
	if destination != nil && upstream.GetDestinationOverrides() != nil {
		mergo.Merge(destination, upstream.GetDestinationOverrides())
	}

	logicalName := destination.GetLogicalName()
	if len(upstream.GetLambdaFunctions()) == 0 {
		return nil, errors.Errorf("lambda points to upstream with no functions %v", logicalName)
	}

	// Validate whether there is a function that conforms to our request
	var lambdaFunc *aws.LambdaFunctionSpec
	for _, candidateLambdaFunc := range upstream.GetLambdaFunctions() {
		if candidateLambdaFunc.GetLogicalName() == logicalName {
			lambdaFunc = candidateLambdaFunc
			break
		}
	}

	if lambdaFunc == nil {
		// pull from options to see if we allow not setting the function on a route
		// this is dangerous due to name ordering when discovery is on https://github.com/solo-io/gloo/tree/main/projects/discovery/pkg/fds/discoveries/aws/aws.go#L75
		tryFallback := options.GetFallbackToFirstFunction().GetValue()
		if !tryFallback {
			return nil, errors.Errorf("unknown lambda function %v", logicalName)
		}
		// Check at the start of the function to make sure that there exists at least one function.
		lambdaFunc = upstream.GetLambdaFunctions()[0]
	}

	functionName := lambdaFunc.GetLambdaFunctionName()

	// Update the information to further format the function definition if requested
	// Used for resource based access.
	if upstream.GetAwsAccountId() != "" {
		awsRegion := upstream.GetRegion()
		if awsRegion == "" {
			awsRegion = os.Getenv("AWS_REGION")
		}
		// eg arn:aws:lambda:us-east-2:986112284769:function:simplerhello
		functionName = fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s",
			awsRegion, upstream.GetAwsAccountId(), functionName)
	}

	// transparently use the unwrapAsApiGateway functionality for responseTransformation
	if destination.GetResponseTransformation() {
		destination.UnwrapAsApiGateway = true
	}

	// error if unwrapAsAlb and unwrapAsApiGateway are both configured
	if destination.GetUnwrapAsAlb() && destination.GetUnwrapAsApiGateway() {
		return nil, errors.Errorf("only one of unwrapAsAlb and unwrapAsApiGateway/responseTransformation may be set")
	}

	var transformerConfig *v3.TypedExtensionConfig
	if destination.GetUnwrapAsApiGateway() {
		transformerConfig = &v3.TypedExtensionConfig{
			Name: ResponseTransformationName,
			TypedConfig: &any.Any{
				TypeUrl: ResponseTransformationTypeUrl,
			},
		}
	}

	// Convert the function that has been retrieved into a useable routefunction
	lambdaRouteFunc := &AWSLambdaPerRoute{
		Async: destination.GetInvocationStyle() == aws.DestinationSpec_ASYNC,
		// we need to query escape per AWS spec:
		// see the CanonicalQueryString section in here: https://docs.aws.amazon.com/general/latest/gr/sigv4-create-canonical-request.html
		Qualifier:         url.QueryEscape(lambdaFunc.GetQualifier()),
		Name:              url.QueryEscape(functionName),
		UnwrapAsAlb:       destination.GetUnwrapAsAlb(),
		TransformerConfig: transformerConfig,
	}

	return lambdaRouteFunc, nil

}

// deriveStaticSecret from ingest if we are using a kubernetes secretref
// Named returns with the derived string contents or an error due to retrieval or format.
func deriveStaticSecret(params plugins.Params, secretRef *core.ResourceRef) (access, session, secret string, err error) {
	glooSecret, err := params.Snapshot.Secrets.Find(secretRef.Strings())
	if err != nil {
		err = errors.Wrapf(err, "retrieving aws secret")
		return
	}

	awsSecrets, ok := glooSecret.GetKind().(*v1.Secret_Aws)
	if !ok {
		err = errors.Errorf("secret (%s.%s) is not an AWS secret",
			glooSecret.GetMetadata().GetName(), glooSecret.GetMetadata().GetNamespace())
		return
	}
	// validate that the secret has field in string format and has an access_key and secret_key
	access = awsSecrets.Aws.GetAccessKey()
	secret = awsSecrets.Aws.GetSecretKey()
	session = awsSecrets.Aws.GetSessionToken()
	if access == "" || !utf8.Valid([]byte(access)) {
		// err is nil here but this is still safe
		err = multierror.Append(err, errors.Errorf("access_key is not a valid string"))
	}
	if secret == "" || !utf8.Valid([]byte(secret)) {
		err = multierror.Append(err, errors.Errorf("secret_key is not a valid string"))
	}
	// Session key is optional
	if session != "" && !utf8.Valid([]byte(session)) {
		err = multierror.Append(err, errors.Errorf("session_key is not a valid string"))
	}
	return
}
