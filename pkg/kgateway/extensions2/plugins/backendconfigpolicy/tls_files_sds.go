package backendconfigpolicy

import (
	"context"

	envoyclusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoytlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// tlsFilesSecretPrefix starts the names of the SDS secrets that carry
// tls.files paths.
const tlsFilesSecretPrefix = "tls_files_"

// splitFileTLS moves file-based certificates and trust bundles out of tlsCtx
// into SDS secrets. Envoy reloads file-based TLS material when the files
// change only if it is delivered over SDS; inline file paths are read once.
// Secret names include the cluster name, so two backends sharing a policy never
// publish the same secret.
//
// It returns tlsCtx unchanged, and no secrets, when nothing is file-based.
func splitFileTLS(tlsCtx *envoytlsv3.UpstreamTlsContext, clusterName string) (*envoytlsv3.UpstreamTlsContext, []*envoytlsv3.Secret) {
	common := tlsCtx.GetCommonTlsContext()
	certs := common.GetTlsCertificates()
	hasFileCert := len(certs) == 1 && (isFileDataSource(certs[0].GetCertificateChain()) || isFileDataSource(certs[0].GetPrivateKey()))
	validation := common.GetValidationContext()
	hasFileCA := isFileDataSource(validation.GetTrustedCa())
	if !hasFileCert && !hasFileCA {
		return tlsCtx, nil
	}

	out := proto.Clone(tlsCtx).(*envoytlsv3.UpstreamTlsContext)
	var secrets []*envoytlsv3.Secret
	if hasFileCert {
		name := tlsFilesSecretPrefix + clusterName + "_cert"
		out.CommonTlsContext.TlsCertificates = nil
		out.CommonTlsContext.TlsCertificateSdsSecretConfigs = []*envoytlsv3.SdsSecretConfig{adsSecretConfig(name)}
		secrets = append(secrets, &envoytlsv3.Secret{
			Name: name,
			Type: &envoytlsv3.Secret_TlsCertificate{TlsCertificate: certs[0]},
		})
	}
	if hasFileCA {
		name := tlsFilesSecretPrefix + clusterName + "_ca"
		out.CommonTlsContext.ValidationContextType = &envoytlsv3.CommonTlsContext_ValidationContextSdsSecretConfig{
			ValidationContextSdsSecretConfig: adsSecretConfig(name),
		}
		secrets = append(secrets, &envoytlsv3.Secret{
			Name: name,
			Type: &envoytlsv3.Secret_ValidationContext{ValidationContext: validation},
		})
	}
	return out, secrets
}

func isFileDataSource(ds *envoycorev3.DataSource) bool {
	_, ok := ds.GetSpecifier().(*envoycorev3.DataSource_Filename)
	return ok
}

func adsSecretConfig(name string) *envoytlsv3.SdsSecretConfig {
	return &envoytlsv3.SdsSecretConfig{
		Name: name,
		SdsConfig: &envoycorev3.ConfigSource{
			ResourceApiVersion:    envoycorev3.ApiVersion_V3,
			ConfigSourceSpecifier: &envoycorev3.ConfigSource_Ads{Ads: &envoycorev3.AggregatedConfigSource{}},
		},
	}
}

// effectiveFileTLSSecrets returns the SDS secrets referenced by the TLS context
// processBackend applied to out.
func effectiveFileTLSSecrets(in ir.BackendObjectIR, clusterName string) []*envoytlsv3.Secret {
	policies := in.AttachedPolicies.Policies[wellknown.BackendConfigPolicyGVK.GroupKind()]
	if len(policies) == 0 || hasBackendTLSPolicy(in) {
		return nil
	}
	merged, ok := mergePolicies(policies).PolicyIr.(*BackendConfigPolicyIR)
	if !ok || merged.tlsConfig == nil {
		return nil
	}
	_, secrets := splitFileTLS(merged.tlsConfig, clusterName)
	return secrets
}

// newBaseClusterResourcesHook publishes the secrets for file-based TLS and
// the resources for a CONNECT tunnel.
func newBaseClusterResourcesHook(tunnelHook sdk.ProcessBaseClusterResources) sdk.ProcessBaseClusterResources {
	return func(kctx krt.HandlerContext, ctx context.Context, in ir.BackendObjectIR, out *envoyclusterv3.Cluster) (sdk.BaseClusterResources, error) {
		// Read the name before the tunnel hook rewrites the cluster.
		secrets := effectiveFileTLSSecrets(in, out.GetName())
		resources, err := tunnelHook(kctx, ctx, in, out)
		resources.Secrets = append(resources.Secrets, secrets...)
		return resources, err
	}
}
