package consul

import (
	envoyapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"

	"github.com/pkg/errors"

	defaultv1 "github.com/solo-io/gloo/pkg/api/defaults/v1"
	"github.com/solo-io/gloo/pkg/secretwatcher"

	"github.com/solo-io/gloo/pkg/api/types/v1"
	"github.com/solo-io/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/pkg/endpointdiscovery"
	"github.com/solo-io/gloo/pkg/plugins"
)

func init() {
	plugins.Register(&Plugin{})
}

func (p *Plugin) SetupEndpointDiscovery(opts bootstrap.Options) (endpointdiscovery.Interface, error) {
	cfg := opts.ConsulOptions.ToConsulConfig()
	disc, err := NewEndpointController(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start consul endpoint discovery")
	}
	return disc, err
}

type Plugin struct{}

const (
	// define Upstream type name
	UpstreamTypeConsul = "consul"
)

func (p *Plugin) GetDependencies(cfg *v1.Config) *plugins.Dependencies {
	deps := new(plugins.Dependencies)
	for _, us := range cfg.Upstreams {
		if us.Type != UpstreamTypeConsul {
			continue
		}
		spec, err := DecodeUpstreamSpec(us.Spec)
		if err != nil {
			continue
		}
		if spec.Connect == nil || spec.Connect.TlsSecretRef == "" {
			continue
		}
		deps.SecretRefs = append(deps.SecretRefs, spec.Connect.TlsSecretRef)
	}
	return deps
}

func (p *Plugin) ProcessUpstream(params *plugins.UpstreamPluginParams, in *v1.Upstream, out *envoyapi.Cluster) error {
	if in.Type != UpstreamTypeConsul {
		return nil
	}
	// decode does validation for us
	spec, err := DecodeUpstreamSpec(in.Spec)
	if err != nil {
		return errors.Wrap(err, "invalid consul upstream spec")
	}

	// consul upstreams use EDS
	out.Type = envoyapi.Cluster_EDS
	out.EdsClusterConfig = &envoyapi.Cluster_EdsClusterConfig{
		EdsConfig: &envoycore.ConfigSource{
			ConfigSourceSpecifier: &envoycore.ConfigSource_Ads{
				Ads: &envoycore.AggregatedConfigSource{},
			},
		},
	}
	if p.connect {

		_, ok := params.Secrets[CertitificateSecretName]
		if ok {
			certChain, privateKey, rootCa, err := getSslSecrets(CertitificateSecretName, params.Secrets)
			if err != nil {
				return err
			}
			certChainData := &envoycore.DataSource{
				Specifier: &envoycore.DataSource_InlineString{
					InlineString: certChain,
				},
			}
			privateKeyData := &envoycore.DataSource{
				Specifier: &envoycore.DataSource_InlineString{
					InlineString: privateKey,
				},
			}
			rootCaData := &envoycore.DataSource{
				Specifier: &envoycore.DataSource_InlineString{
					InlineString: rootCa,
				},
			}

			var validationContext *envoyauth.CertificateValidationContext
			if rootCa != "" {
				validationContext = &envoyauth.CertificateValidationContext{
					TrustedCa: rootCaData,
				}
			}
			out.TlsContext = &envoyauth.UpstreamTlsContext{
				CommonTlsContext: &envoyauth.CommonTlsContext{
					TlsParams: &envoyauth.TlsParameters{},
					TlsCertificates: []*envoyauth.TlsCertificate{
						{
							CertificateChain: certChainData,
							PrivateKey:       privateKeyData,
						},
					},
					ValidationContext: validationContext,
				},
			}
		}
	}
	return nil
}

// TODO(yuval-k): un-copy-paste this from route_config.go
func getSslSecrets(ref string, secrets secretwatcher.SecretMap) (string, string, string, error) {
	sslSecrets, ok := secrets[ref]
	if !ok {
		return "", "", "", errors.Errorf("ssl secret not found for ref %v", ref)
	}
	certChain, ok := sslSecrets.Data[defaultv1.SslCertificateChainKey]
	privateKey, ok := sslSecrets.Data[defaultv1.SslPrivateKeyKey]
	rootCa := sslSecrets.Data[defaultv1.SslRootCaKey]
	return certChain, privateKey, rootCa, nil
}
