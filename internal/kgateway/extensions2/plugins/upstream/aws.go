package upstream

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"unicode/utf8"

	envoy_config_cluster_v3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoy_config_core_v3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	awspb "github.com/solo-io/envoy-gloo/go/config/filter/http/aws_lambda/v2"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
)

const (
	AccessKey    = "accessKey"
	SessionToken = "sessionToken"
	SecretKey    = "secretKey"
)

func processAws(ctx context.Context, in *v1alpha1.AwsUpstream, ir *UpstreamIr, out *envoy_config_cluster_v3.Cluster) {

	lambdaHostname := getLambdaHostname(in)

	// configure Envoy cluster routing info
	out.ClusterDiscoveryType = &envoy_config_cluster_v3.Cluster_Type{
		Type: envoy_config_cluster_v3.Cluster_LOGICAL_DNS,
	}
	// TODO(yuval-k): why do we need to make sure we use ipv4 only dns?
	// TODO(nfuden): Update to reasonable ipv6 https://aws.amazon.com/about-aws/whats-new/2021/12/aws-lambda-ipv6-endpoints-inbound-connections/
	out.DnsLookupFamily = envoy_config_cluster_v3.Cluster_V4_ONLY
	pluginutils.EnvoySingleEndpointLoadAssignment(out, lambdaHostname, 443)

	// TODO: this returns nil anyway, so don't worry about temporarily migrating legacy util function
	// that still relied on gloo v1 types
	// commonTlsContext, err := utils.GetCommonTlsContextFromUpstreamOptions(nil)
	// if err != nil {
	// 	// return err
	// 	return
	// }
	tlsContext := &envoyauth.UpstreamTlsContext{
		CommonTlsContext: nil,
		// TODO(yuval-k): Add verification context
		Sni: lambdaHostname,
	}
	typedConfig, err := anypb.New(tlsContext)
	if err != nil {
		// return err
		return
	}
	out.TransportSocket = &envoy_config_core_v3.TransportSocket{
		Name:       wellknown.TransportSocketTls,
		ConfigType: &envoy_config_core_v3.TransportSocket_TypedConfig{TypedConfig: typedConfig},
	}

	// To utilize the aws lambda plugin much of the power comes via its secret management
	// Check that one of the supported auth paradigms in enabled.
	// Currently: static secret ref, credential discovery or ServiceAccountCreds such in eks

	if ir.AwsSecret == nil {
		// return errors.Errorf("no aws secret provided. consider setting enableCredentialsDiscovey to true or enabling service account credentials if running in EKS")
		return
	}

	// If static secret is set retrieve the information needed
	var derivedSecret staticSecretDerivation
	if ir.AwsSecret != nil {
		derivedSecret, err = deriveStaticSecret(ir.AwsSecret)
		if err != nil {
			//			return err
			return
		}
	}

	lpe := &awspb.AWSLambdaProtocolExtension{
		Host:         lambdaHostname,
		Region:       in.Region,
		AccessKey:    derivedSecret.access,
		SecretKey:    derivedSecret.secret,
		SessionToken: derivedSecret.session,
		//		RoleArn:             upstreamSpec.Aws.GetRoleArn(),
		//		DisableRoleChaining: upstreamSpec.Aws.GetDisableRoleChaining(),
	}

	if err := pluginutils.SetExtensionProtocolOptions(out, FilterName, lpe); err != nil {
		//		return errors.Wrapf(err, "converting aws protocol options to struct")
	}
	// return nil
}

func getLambdaHostname(in *v1alpha1.AwsUpstream) string {
	return fmt.Sprintf("lambda.%s.amazonaws.com", in.Region)
}

func processEndpointsAws(in *v1alpha1.AwsUpstream) *ir.EndpointsForUpstream {
	return nil
}

func (p *upstreamPlugin) processBackendAws(
	ctx context.Context,
	pCtx *ir.RouteBackendContext,
	dest *upstreamDestination,
) error {

	functionName := dest.FunctionName
	if p.needFilter == nil {
		p.needFilter = make(map[string]bool)
	}
	p.needFilter[pCtx.FilterChainName] = true

	lambdaRouteFunc := &awspb.AWSLambdaPerRoute{
		Async: false,
		// we need to query escape per AWS spec:
		// see the CanonicalQueryString section in here: https://docs.aws.amazon.com/general/latest/gr/sigv4-create-canonical-request.html
		// Qualifier:         url.QueryEscape(lambdaFunc.GetQualifier()),
		Name: url.QueryEscape(functionName),
		//UnwrapAsAlb:       destination.GetUnwrapAsAlb(),
		//TransformerConfig: transformerConfig,
	}
	lambdaRouteFuncAny, err := anypb.New(lambdaRouteFunc)
	if err != nil {
		return err
	}
	pCtx.AddTypedConfig(FilterName, lambdaRouteFuncAny)
	return nil
}

type staticSecretDerivation struct {
	access, session, secret string
}

// deriveStaticSecret from ingest if we are using a kubernetes secretref
// Named returns with the derived string contents or an error due to retrieval or format.
func deriveStaticSecret(awsSecrets *ir.Secret) (staticSecretDerivation, error) {
	var errs []error
	derived := staticSecretDerivation{
		access:  string(awsSecrets.Data[AccessKey]),
		session: string(awsSecrets.Data[SessionToken]),
		secret:  string(awsSecrets.Data[SecretKey]),
	}

	// validate that the secret has field in string format and has an access_key and secret_key

	if derived.access == "" || !utf8.Valid([]byte(derived.access)) {
		// err is nil here but this is still safe
		errs = append(errs, errors.New("access_key is not a valid string"))
	}

	if derived.secret == "" || !utf8.Valid([]byte(derived.secret)) {
		errs = append(errs, errors.New("secret_key is not a valid string"))
	}

	// Session key is optional

	if derived.session != "" && !utf8.Valid([]byte(derived.session)) {
		errs = append(errs, errors.New("session_key is not a valid string"))
	}

	return derived, errors.Join(errs...)
}
