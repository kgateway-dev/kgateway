package utils

import (
	envoyauth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	gogo_types "github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

type SslConfigTranslator struct {
	secrets v1.SecretList
}

func NewSslConfigTranslator(secrets v1.SecretList) *SslConfigTranslator {
	return &SslConfigTranslator{
		secrets: secrets,
	}
}

func (s *SslConfigTranslator) ResolveUpstreamSslConfig(uc *v1.UpstreamSslConfig) (*envoyauth.UpstreamTlsContext, error) {
	common, err := s.ResolveCommonSslConfig(uc)
	if err != nil {
		return nil, err
	}
	return &envoyauth.UpstreamTlsContext{
		CommonTlsContext: common,
		Sni:              uc.Sni,
	}, nil
}
func (s *SslConfigTranslator) ResolveDownstreamSslConfig(dc *v1.SslConfig) (*envoyauth.DownstreamTlsContext, error) {
	common, err := s.ResolveCommonSslConfig(dc)
	if err != nil {
		return nil, err
	}
	var requireClientCert *gogo_types.BoolValue
	if common.ValidationContextType != nil {
		requireClientCert = &gogo_types.BoolValue{Value: true}
	}
	return &envoyauth.DownstreamTlsContext{
		CommonTlsContext:         common,
		RequireClientCertificate: requireClientCert,
	}, nil
}

type CertSource interface {
	GetSecretRef() *core.ResourceRef
	GetSslFiles() *v1.SSLFiles
}

func dataSourceGenerator(inlineDataSource bool) func(s string) *envoycore.DataSource {
	return func(s string) *envoycore.DataSource {
		if !inlineDataSource {
			return &envoycore.DataSource{
				Specifier: &envoycore.DataSource_Filename{
					Filename: s,
				},
			}
		}
		return &envoycore.DataSource{
			Specifier: &envoycore.DataSource_InlineString{
				InlineString: s,
			},
		}
	}
}

func (s *SslConfigTranslator) ResolveCommonSslConfig(cs CertSource) (*envoyauth.CommonTlsContext, error) {
	var (
		certChain, privateKey, rootCa string
		// if using a Secret ref, we will inline the certs in the tls config
		inlineDataSource bool
	)

	if sslSecrets := cs.GetSecretRef(); sslSecrets != nil {
		var err error
		inlineDataSource = true
		ref := sslSecrets
		certChain, privateKey, rootCa, err = getSslSecrets(*ref, s.secrets)
		if err != nil {
			return nil, err
		}
	} else if sslSecrets := cs.GetSslFiles(); sslSecrets != nil {
		certChain, privateKey, rootCa = sslSecrets.TlsCert, sslSecrets.TlsKey, sslSecrets.RootCa
	} else {
		return nil, errors.New("no certificate information found")
	}

	dataSource := dataSourceGenerator(inlineDataSource)

	var certChainData, privateKeyData, rootCaData *envoycore.DataSource

	if certChain != "" {
		certChainData = dataSource(certChain)
	}
	if privateKey != "" {
		privateKeyData = dataSource(privateKey)
	}
	if rootCa != "" {
		rootCaData = dataSource(rootCa)
	}

	tlsContext := &envoyauth.CommonTlsContext{
		// default params
		TlsParams:     &envoyauth.TlsParameters{},
		AlpnProtocols: []string{"h2", "http/1.1"},
	}

	if certChainData != nil && privateKeyData != nil {
		tlsContext.TlsCertificates = []*envoyauth.TlsCertificate{
			{
				CertificateChain: certChainData,
				PrivateKey:       privateKeyData,
			},
		}
	} else if certChainData != nil || privateKeyData != nil {
		return nil, errors.New("both or none of cert chain and private key must be provided")
	}

	if rootCaData != nil {
		tlsContext.ValidationContextType = &envoyauth.CommonTlsContext_ValidationContext{
			ValidationContext: &envoyauth.CertificateValidationContext{
				TrustedCa: rootCaData,
			},
		}
	}

	return tlsContext, nil
}

func getSslSecrets(ref core.ResourceRef, secrets v1.SecretList) (string, string, string, error) {
	secret, err := secrets.Find(ref.Strings())
	if err != nil {
		return "", "", "", errors.Wrapf(err, "SSL secret not found")
	}

	sslSecret, ok := secret.Kind.(*v1.Secret_Tls)
	if !ok {
		return "", "", "", errors.Errorf("%v is not a TLS secret", secret.GetMetadata().Ref())
	}

	certChain := sslSecret.Tls.CertChain
	privateKey := sslSecret.Tls.PrivateKey
	rootCa := sslSecret.Tls.RootCa
	return certChain, privateKey, rootCa, nil
}
