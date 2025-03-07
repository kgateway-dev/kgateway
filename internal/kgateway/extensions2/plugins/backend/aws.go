package backend

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"unicode/utf8"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_aws_common_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/aws/v3"
	envoy_lambda_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/aws_lambda/v3"
	envoy_request_signing_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/aws_request_signing/v3"
	envoy_upstream_codec "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/upstream_codec/v3"
	envoy_hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoy_upstreams_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	translatorutils "github.com/kgateway-dev/kgateway/v2/internal/kgateway/translator/utils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/arnutils"
)

const (
	// accessKey is the key name for in the secret data for the access key id.
	accessKey = "accessKey"
	// sessionToken is the key name for in the secret data for the session token.
	sessionToken = "sessionToken"
	// secretKey is the key name for in the secret data for the secret access key.
	secretKey = "secretKey"
	// lambdaServiceName is the service name for the lambda filter.
	lambdaServiceName = "lambda"
	// lambdaFilterName is the name of the lambda filter.
	lambdaFilterName = "envoy.filters.http.aws_lambda"
	// awsRequestSigningFilterName is the name of the aws request signing filter.
	awsRequestSigningFilterName = "envoy.filters.http.aws_request_signing"
	// upstreamCodecFilterName is the name of the upstream codec filter.
	upstreamCodecFilterName = "envoy.filters.http.upstream_codec"
	// defaultAWSRegion is the default AWS region.
	defaultAWSRegion = "us-east-1"
)

// processAws processes an AWS backend and returns an envoy cluster.
func processAws(ctx context.Context, in *v1alpha1.AwsBackend, ir *BackendIr, out *envoy_config_cluster_v3.Cluster) error {
	out.ClusterDiscoveryType = &envoy_config_cluster_v3.Cluster_Type{
		Type: envoy_config_cluster_v3.Cluster_LOGICAL_DNS,
	}
	out.DnsLookupFamily = envoy_config_cluster_v3.Cluster_AUTO

	endpointConfig, err := configureEndpoint(in)
	if err != nil {
		return err
	}
	if endpointConfig.useTLS {
		if err := configureTLS(out, endpointConfig.hostname); err != nil {
			return err
		}
	}
	if err := configureUpstreamHTTPFilters(out, in, ir); err != nil {
		return err
	}

	pluginutils.EnvoySingleEndpointLoadAssignment(out, endpointConfig.hostname, endpointConfig.port)
	return nil
}

// endpointConfig is a helper struct to store the endpoint configuration for the Lambda backend.
type endpointConfig struct {
	hostname string
	port     uint32
	useTLS   bool
}

// configureEndpoint parses the endpoint URL and returns the endpoint configuration
func configureEndpoint(in *v1alpha1.AwsBackend) (*endpointConfig, error) {
	config := &endpointConfig{
		hostname: getLambdaHostname(in),
		port:     443,
		useTLS:   true,
	}
	if in.Lambda.EndpointURL == "" {
		// no custom endpoint specified, use the default lambda hostname.
		return config, nil
	}

	parsedURL, err := url.Parse(in.Lambda.EndpointURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse endpoint URL: %v", err)
	}
	config.useTLS = parsedURL.Scheme == "https"
	if !config.useTLS {
		config.port = 80
	}
	config.hostname = parsedURL.Hostname()
	if parsedURL.Port() != "" {
		if p, err := strconv.ParseUint(parsedURL.Port(), 10, 32); err == nil {
			config.port = uint32(p)
		}
	}

	return config, nil
}

// configureTLS configures TLS for the cluster.
func configureTLS(out *envoy_config_cluster_v3.Cluster, hostname string) error {
	// TODO(yuval-k): Add verification context
	typedConfig, err := utils.MessageToAny(&envoyauth.UpstreamTlsContext{
		Sni: hostname,
	})
	if err != nil {
		return fmt.Errorf("failed to create tls context: %v", err)
	}
	out.TransportSocket = &envoy_config_core_v3.TransportSocket{
		Name: wellknown.TransportSocketTls,
		ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{
			TypedConfig: typedConfig,
		},
	}
	return nil
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
// If the qualifier is not specified, the $LATEST qualifier is used. An error is returned if the arn
// is not a valid lambda arn.
func buildLambdaARN(in *v1alpha1.AwsBackend, region string) (string, error) {
	qualifier := "$LATEST"
	if in.Lambda.Qualifier != "" {
		qualifier = in.Lambda.Qualifier
	}
	// TODO(tim): url.QueryEscape(...)?
	arnStr := fmt.Sprintf("arn:aws:lambda:%s:%s:function:%s:%s", region, in.AccountId, in.Lambda.FunctionName, qualifier)
	parsedARN, err := arnutils.Parse(arnStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse lambda arn: %v", err)
	}
	return parsedARN.String(), nil
}

// configureAWSAuth configures AWS authentication for the given backend.
func configureAWSAuth(in *v1alpha1.AwsBackend, ir *BackendIr, region string) (*envoy_request_signing_v3.AwsRequestSigning, error) {
	// when no auth is specified, use the default aws auth provider documented by the lambda filter:
	// https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/aws_lambda_filter#credentials.
	if in.Auth == nil || in.Auth.Type != v1alpha1.AwsAuthTypeSecret {
		return &envoy_request_signing_v3.AwsRequestSigning{
			ServiceName: lambdaServiceName,
			Region:      region,
		}, nil
	}

	// handle secret-based auth. configure inline credentials.
	derived, err := deriveStaticSecret(ir.AwsSecret)
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

// configureUpstreamHTTPFilters configures HTTP filters for the cluster
func configureUpstreamHTTPFilters(
	out *envoy_config_cluster_v3.Cluster,
	in *v1alpha1.AwsBackend,
	ir *BackendIr,
) error {
	region := getRegion(in)
	lambdaARN, err := buildLambdaARN(in, region)
	if err != nil {
		return err
	}

	lambdaConfigAny, err := utils.MessageToAny(&envoy_lambda_v3.Config{
		Arn:            lambdaARN,
		InvocationMode: getLambdaInvocationMode(in),
	})
	if err != nil {
		return fmt.Errorf("failed to create lambda config: %v", err)
	}

	awsRequestSigning, err := configureAWSAuth(in, ir, region)
	if err != nil {
		return err
	}
	awsRequestSigningAny, err := utils.MessageToAny(awsRequestSigning)
	if err != nil {
		return fmt.Errorf("failed to create aws request signing config: %v", err)
	}

	codecConfigAny, err := utils.MessageToAny(&envoy_upstream_codec.UpstreamCodec{})
	if err != nil {
		return fmt.Errorf("failed to create upstream codec config: %v", err)
	}

	if err := translatorutils.MutateHttpOptions(out, func(opts *envoy_upstreams_v3.HttpProtocolOptions) {
		opts.UpstreamProtocolOptions = &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &envoy_upstreams_v3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
					Http2ProtocolOptions: &envoy_core_v3.Http2ProtocolOptions{},
				},
			},
		}
		opts.CommonHttpProtocolOptions = &envoy_core_v3.HttpProtocolOptions{
			IdleTimeout: &durationpb.Duration{
				Seconds: 30,
			},
		}
		opts.HttpFilters = append(opts.GetHttpFilters(), &envoy_hcm.HttpFilter{
			Name: lambdaFilterName,
			ConfigType: &envoy_hcm.HttpFilter_TypedConfig{
				TypedConfig: lambdaConfigAny,
			},
		})
		opts.HttpFilters = append(opts.GetHttpFilters(), &envoy_hcm.HttpFilter{
			Name: awsRequestSigningFilterName,
			ConfigType: &envoy_hcm.HttpFilter_TypedConfig{
				TypedConfig: awsRequestSigningAny,
			},
		})
		opts.HttpFilters = append(opts.GetHttpFilters(), &envoy_hcm.HttpFilter{
			Name: upstreamCodecFilterName,
			ConfigType: &envoy_hcm.HttpFilter_TypedConfig{
				TypedConfig: codecConfigAny,
			},
		})
	}); err != nil {
		return fmt.Errorf("failed to mutate http options: %v", err)
	}

	return nil
}

// getLambdaHostname returns the hostname for the lambda function. When using a custom endpoint
// has been specified, it will be returned. Otherwise, the default lambda hostname is returned.
func getLambdaHostname(in *v1alpha1.AwsBackend) string {
	if in.Lambda.EndpointURL != "" {
		return in.Lambda.EndpointURL
	}
	return fmt.Sprintf("lambda.%s.amazonaws.com", getRegion(in))
}

// processEndpointsAws processes the endpoints for the aws backend.
func processEndpointsAws(_ *v1alpha1.AwsBackend) *ir.EndpointsForBackend {
	return nil
}

// getRegion returns the region for the aws backend. If a region is specified, it will be returned.
// Otherwise, the default region is returned.
func getRegion(in *v1alpha1.AwsBackend) string {
	if in.Region != nil {
		return *in.Region
	}
	return defaultAWSRegion
}

// staticSecretDerivation is a helper struct to store the decoded secret values
// from an AWS Kubernetes Secret reference.
type staticSecretDerivation struct {
	access, session, secret string
}

// deriveStaticSecret from ingest if we are using a kubernetes secretref
// Named returns with the derived string contents or an error due to retrieval or format.
func deriveStaticSecret(awsSecrets *ir.Secret) (*staticSecretDerivation, error) {
	var errs []error
	// validate that the secret has field in string format and has an access_key and secret_key
	if awsSecrets.Data[accessKey] == nil || !utf8.Valid(awsSecrets.Data[accessKey]) {
		// err is nil here but this is still safe
		errs = append(errs, errors.New("access_key is not a valid string"))
	}
	if awsSecrets.Data[secretKey] == nil || !utf8.Valid(awsSecrets.Data[secretKey]) {
		errs = append(errs, errors.New("secret_key is not a valid string"))
	}
	// Session key is optional, but if it is present, it must be a valid string.
	if awsSecrets.Data[sessionToken] != nil && !utf8.Valid(awsSecrets.Data[sessionToken]) {
		errs = append(errs, errors.New("session_key is not a valid string"))
	}
	return &staticSecretDerivation{
		access:  string(awsSecrets.Data[accessKey]),
		session: string(awsSecrets.Data[sessionToken]),
		secret:  string(awsSecrets.Data[secretKey]),
	}, errors.Join(errs...)
}
