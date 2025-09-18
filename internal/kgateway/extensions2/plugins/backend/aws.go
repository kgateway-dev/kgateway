package backend

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"unicode/utf8"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_aws_common_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/aws/v3"
	envoy_lambda_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/aws_lambda/v3"
	envoy_request_signing_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/aws_request_signing/v3"
	envoy_upstream_codec "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/upstream_codec/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	translatorutils "github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/arnutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

const (
	// lambdaServiceName is the service name for the lambda filter.
	lambdaServiceName = "lambda"
	// lambdaFilterName is the name of the lambda filter.
	lambdaFilterName = "envoy.filters.http.aws_lambda"
	// awsRequestSigningFilterName is the name of the aws request signing filter.
	awsRequestSigningFilterName = "envoy.filters.http.aws_request_signing"
	// upstreamCodecFilterName is the name of the upstream codec filter.
	upstreamCodecFilterName = "envoy.filters.http.upstream_codec"
)

// AwsIr is the internal representation of an AWS backend.
type AwsIr struct {
	lambdaFilters         *lambdaFilters
	lambdaEndpoint        *lambdaEndpointConfig
	lambdaTransportSocket *envoycorev3.TransportSocket
}

// Equals checks if two AwsIr objects are equal.
func (u *AwsIr) Equals(other any) bool {
	otherAws, ok := other.(*AwsIr)
	if !ok {
		return false
	}
	if u == nil && otherAws != nil {
		return false
	}
	if u != nil {
		if otherAws == nil {
			return false
		}
		if !u.lambdaEndpoint.Equals(otherAws.lambdaEndpoint) {
			return false
		}
		if !u.lambdaFilters.Equals(otherAws.lambdaFilters) {
			return false
		}
		if !proto.Equal(u.lambdaTransportSocket, otherAws.lambdaTransportSocket) {
			return false
		}
	}
	return true
}

// processAws processes an AWS backend and returns an envoy cluster.
func processAws(ir *AwsIr, out *envoyclusterv3.Cluster) error {
	// defensive check; this should never happen with union types
	if ir == nil {
		return fmt.Errorf("aws ir is nil")
	}

	out.ClusterDiscoveryType = &envoyclusterv3.Cluster_Type{
		Type: envoyclusterv3.Cluster_LOGICAL_DNS,
	}
	if ir.lambdaTransportSocket != nil {
		out.TransportSocket = ir.lambdaTransportSocket
	}

	if err := translatorutils.MutateHttpOptions(out, func(opts *envoy_upstreams_v3.HttpProtocolOptions) {
		opts.UpstreamProtocolOptions = &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
					Http2ProtocolOptions: &envoycorev3.Http2ProtocolOptions{},
				},
			},
		}
		opts.CommonHttpProtocolOptions = &envoycorev3.HttpProtocolOptions{
			IdleTimeout: &durationpb.Duration{
				Seconds: 30,
			},
		}
		opts.HttpFilters = append(opts.GetHttpFilters(), &envoy_hcm.HttpFilter{
			Name: lambdaFilterName,
			ConfigType: &envoy_hcm.HttpFilter_TypedConfig{
				TypedConfig: ir.lambdaFilters.lambdaConfigAny,
			},
		})
		opts.HttpFilters = append(opts.GetHttpFilters(), &envoy_hcm.HttpFilter{
			Name: awsRequestSigningFilterName,
			ConfigType: &envoy_hcm.HttpFilter_TypedConfig{
				TypedConfig: ir.lambdaFilters.awsRequestSigningAny,
			},
		})
		opts.HttpFilters = append(opts.GetHttpFilters(), &envoy_hcm.HttpFilter{
			Name: upstreamCodecFilterName,
			ConfigType: &envoy_hcm.HttpFilter_TypedConfig{
				TypedConfig: ir.lambdaFilters.codecConfigAny,
			},
		})
	}); err != nil {
		return fmt.Errorf("failed to mutate http options: %v", err)
	}

	pluginutils.EnvoySingleEndpointLoadAssignment(out, ir.lambdaEndpoint.hostname, ir.lambdaEndpoint.port)
	return nil
}

// configureAWSAuth configures AWS authentication for the given backend.
func configureAWSAuth(secret *ir.Secret, region string) (*envoy_request_signing_v3.AwsRequestSigning, error) {
	// when no auth is specified, use the default aws auth provider documented by the lambda filter:
	// https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/aws_lambda_filter#credentials.
	if secret == nil || secret.Data == nil {
		return &envoy_request_signing_v3.AwsRequestSigning{
			ServiceName: lambdaServiceName,
			Region:      region,
		}, nil
	}
	// handle secret-based auth. configure inline credentials.
	derived, err := deriveStaticSecret(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to derive static secret: %v", err)
	}

	return &envoy_request_signing_v3.AwsRequestSigning{
		ServiceName: lambdaServiceName,
		Region:      region,
		CredentialProvider: &envoy_aws_common_v3.AwsCredentialProvider{
			InlineCredential: &envoy_aws_common_v3.InlineCredentialProvider{
				AccessKeyId:     derived.access,
				SecretAccessKey: derived.secret,
				SessionToken:    derived.session,
			},
		},
	}, nil
}

// lambdaFilters is a helper struct to store the lambda filters for the given backend.
type lambdaFilters struct {
	lambdaConfigAny      *anypb.Any
	awsRequestSigningAny *anypb.Any
	codecConfigAny       *anypb.Any
}

// Equals checks if two lambdaFilters objects are equal.
func (u *lambdaFilters) Equals(other *lambdaFilters) bool {
	return cmputils.CompareWithNils(u, other, func(a, b *lambdaFilters) bool {
		return proto.Equal(a.lambdaConfigAny, b.lambdaConfigAny) &&
			proto.Equal(a.awsRequestSigningAny, b.awsRequestSigningAny) &&
			proto.Equal(a.codecConfigAny, b.codecConfigAny)
	})
}

// buildLambdaFilters configures cluster's upstream HTTP filters for the given backend.
func buildLambdaFilters(
	arn string,
	region string,
	secret *ir.Secret,
	invokeMode envoy_lambda_v3.Config_InvocationMode,
	payloadTransformMode v1alpha1.AWSLambdaPayloadTransformMode,
) (*lambdaFilters, error) {
	var payloadPassthrough bool
	switch payloadTransformMode {
	case v1alpha1.AWSLambdaPayloadTransformNone:
		payloadPassthrough = true
	case v1alpha1.AWSLambdaPayloadTransformEnvoy:
		payloadPassthrough = false
	}

	lambdaConfigAny, err := utils.MessageToAny(&envoy_lambda_v3.Config{
		Arn:                arn,
		InvocationMode:     invokeMode,
		PayloadPassthrough: payloadPassthrough,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create lambda config: %v", err)
	}

	awsRequestSigning, err := configureAWSAuth(secret, region)
	if err != nil {
		return nil, fmt.Errorf("failed to create aws request signing config: %v", err)
	}
	awsRequestSigningAny, err := utils.MessageToAny(awsRequestSigning)
	if err != nil {
		return nil, fmt.Errorf("failed to create aws request signing config: %v", err)
	}

	codecConfigAny, err := utils.MessageToAny(&envoy_upstream_codec.UpstreamCodec{})
	if err != nil {
		return nil, fmt.Errorf("failed to create upstream codec config: %v", err)
	}

	return &lambdaFilters{
		lambdaConfigAny:      lambdaConfigAny,
		awsRequestSigningAny: awsRequestSigningAny,
		codecConfigAny:       codecConfigAny,
	}, nil
}

// getRegion returns the region for the aws backend. If a region is specified, it will be returned.
// Otherwise, the default region is returned.
func getRegion(in *v1alpha1.AwsBackend) string {
	if in.Region != nil {
		return *in.Region
	}
	return wellknown.DefaultAWSRegion
}

// getLambdaHostname returns the hostname for the lambda function. When using a custom endpoint
// has been specified, it will be returned. Otherwise, the default lambda hostname is returned.
func getLambdaHostname(in *v1alpha1.AwsBackend) string {
	if in.Lambda.EndpointURL != nil {
		return *in.Lambda.EndpointURL
	}
	return fmt.Sprintf("lambda.%s.amazonaws.com", getRegion(in))
}

// getLambdaInvocationMode returns the Lambda invocation mode. Default is synchronous.
func getLambdaInvocationMode(in *v1alpha1.AwsBackend) envoy_lambda_v3.Config_InvocationMode {
	invokeMode := envoy_lambda_v3.Config_SYNCHRONOUS
	if in.Lambda.InvocationMode == v1alpha1.AwsLambdaInvocationModeAsynchronous {
		invokeMode = envoy_lambda_v3.Config_ASYNCHRONOUS
	}
	return invokeMode
}

// buildLambdaARN attempts to build a fully qualified lambda arn from the given backend configuration.
// CEL validation should reject invalid `qualifier` values and handle defaulting, so we can assume
// the qualifier passed here is valid.
// An error is returned if the arn is not a valid lambda arn.
func buildLambdaARN(in *v1alpha1.AwsBackend, region string) (string, error) {
	// TODO(tim): url.QueryEscape(...)?
	arnStr := fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s:%s", region, in.AccountId, in.Lambda.FunctionName, in.Lambda.Qualifier)
	parsedARN, err := arnutils.Parse(arnStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse lambda arn: %v", err)
	}
	return parsedARN.String(), nil
}

// lambdaEndpointConfig is a helper struct to store the endpoint configuration for the Lambda backend.
type lambdaEndpointConfig struct {
	hostname string
	port     uint32
	useTLS   bool
}

// Equals checks if two lambdaEndpointConfig objects are equal.
func (u *lambdaEndpointConfig) Equals(other *lambdaEndpointConfig) bool {
	return u.hostname == other.hostname && u.port == other.port && u.useTLS == other.useTLS
}

// configureLambdaEndpoint parses the endpoint URL and returns the endpoint configuration.
func configureLambdaEndpoint(in *v1alpha1.AwsBackend) (*lambdaEndpointConfig, error) {
	config := &lambdaEndpointConfig{
		hostname: getLambdaHostname(in),
		port:     443,
		useTLS:   true,
	}

	if in.Lambda.EndpointURL == nil {
		// no custom endpoint specified, use the default lambda hostname.
		return config, nil
	}

	inUrl := *in.Lambda.EndpointURL
	parsedURL, err := url.Parse(inUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse endpoint URL: %v", err)
	}
	config.useTLS = parsedURL.Scheme == "https"
	config.hostname = parsedURL.Hostname()

	port, err := strconv.ParseUint(parsedURL.Port(), 10, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to parse port: %v", err)
	}
	config.port = uint32(port)

	return config, nil
}

// processEndpointsAws processes the endpoints for the aws backend.
func processEndpointsAws(_ *v1alpha1.AwsBackend) *ir.EndpointsForBackend {
	return nil
}

// staticSecretDerivation is a helper struct to store the decoded secret values
// from an AWS Kubernetes Secret reference.
type staticSecretDerivation struct {
	access, session, secret string
}

// deriveStaticSecret derives the static secret from the given secret.
func deriveStaticSecret(awsSecrets *ir.Secret) (*staticSecretDerivation, error) {
	var errs []error
	// validate that the secret has field in string format and has an access_key and secret_key
	if awsSecrets.Data[wellknown.AccessKey] == nil || !utf8.Valid(awsSecrets.Data[wellknown.AccessKey]) {
		// err is nil here but this is still safe
		errs = append(errs, errors.New("access_key is not a valid string"))
	}
	if awsSecrets.Data[wellknown.SecretKey] == nil || !utf8.Valid(awsSecrets.Data[wellknown.SecretKey]) {
		errs = append(errs, errors.New("secret_key is not a valid string"))
	}
	// Session key is optional, but if it is present, it must be a valid string.
	if awsSecrets.Data[wellknown.SessionToken] != nil && !utf8.Valid(awsSecrets.Data[wellknown.SessionToken]) {
		errs = append(errs, errors.New("session_key is not a valid string"))
	}
	return &staticSecretDerivation{
		access:  string(awsSecrets.Data[wellknown.AccessKey]),
		session: string(awsSecrets.Data[wellknown.SessionToken]),
		secret:  string(awsSecrets.Data[wellknown.SecretKey]),
	}, errors.Join(errs...)
}
