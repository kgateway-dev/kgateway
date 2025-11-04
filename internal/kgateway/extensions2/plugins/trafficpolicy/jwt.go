package trafficpolicy

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"sort"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	jwtauthnv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	"github.com/go-jose/go-jose/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	PayloadInMetadata string = "payload"
)

type jwtIr struct {
	provider    *ir.GatewayExtension
	jwtPerRoute *jwtauthnv3.PerRouteConfig
}

func (j *jwtIr) Equals(other *jwtIr) bool {
	if j == nil && other == nil {
		return true
	}
	if j == nil || other == nil {
		return false
	}

	// Compare providers
	if (j.provider == nil) != (other.provider == nil) {
		return false
	}
	if j.provider != nil && !j.provider.Equals(*other.provider) {
		return false
	}

	return proto.Equal(j.jwtPerRoute, other.jwtPerRoute)
}

// handleJwt configures the filter JwtAuthentication and per-route JWT configuration for a specific route
func (p *trafficPolicyPluginGwPass) handleJwt(fcn string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, jwtIr *jwtIr) {
	if jwtIr == nil || jwtIr.jwtPerRoute == nil {
		return
	}

	providerName := jwtIr.provider.ResourceName()
	jwtName := jwtFilterName(providerName)
	if jwtIr.jwtPerRoute != nil {
		pCtxTypedFilterConfig.AddTypedConfig(jwtName, jwtIr.jwtPerRoute)
	}

	p.jwtPerProvider.Add(fcn, providerName, jwtIr.provider)
}

func translatePerRouteConfig(requirementsName string) (*jwtauthnv3.PerRouteConfig, error) {
	perRouteConfig := &jwtauthnv3.PerRouteConfig{
		RequirementSpecifier: &jwtauthnv3.PerRouteConfig_RequirementName{
			RequirementName: requirementsName,
		},
	}
	return perRouteConfig, nil
}

// constructJwt translates the jwt spec into an envoy jwt policy and stores it in the traffic policy IR
func constructJwt(
	krtctx krt.HandlerContext,
	in *v1alpha1.TrafficPolicy,
	out *trafficPolicySpecIr, // todo move down
	fetchGatewayExtension FetchGatewayExtensionFunc,
) error {
	spec := in.Spec.JWT
	if spec == nil {
		return nil
	}

	provider, err := fetchGatewayExtension(krtctx, *spec.ExtensionRef, in.GetNamespace())
	if err != nil {
		return fmt.Errorf("jwt: %w", err)
	}
	if provider.ExtType != v1alpha1.GatewayExtensionTypeJWTProvider || provider.JwtProviders == nil {
		return pluginutils.ErrInvalidExtensionType(v1alpha1.GatewayExtensionTypeJWTProvider, provider.ExtType)
	}

	requirementsName := fmt.Sprintf("%s_%s_requirements", spec.ExtensionRef.Name, in.Namespace)
	perRouteConfig, err := translatePerRouteConfig(requirementsName)
	if err != nil {
		return err
	}

	out.jwt = &jwtIr{
		provider:    provider,
		jwtPerRoute: perRouteConfig,
	}
	return nil
}

// Validate performs validation on the jwt component.
func (j *jwtIr) Validate() error {
	return j.validate()
}

func (j *jwtIr) validate() error {
	if j == nil {
		return nil
	}

	var errs []error

	err := j.provider.Validate()
	if err != nil {
		errs = append(errs, err)
	}

	err = j.jwtPerRoute.Validate()
	if err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// ProviderName returns a unique name for a provider in the context of a route
func ProviderName(resourceName, providerName string) string {
	return fmt.Sprintf("%s_%s", resourceName, providerName)
}

func translateProvider(krtctx krt.HandlerContext, provider v1alpha1.JWTProvider, policyNs string, secrets *krtcollections.SecretIndex) (*jwtauthnv3.JwtProvider, error) {
	var claimToHeaders []*jwtauthnv3.JwtClaimToHeader
	for _, claim := range provider.ClaimsToHeaders {
		claimToHeaders = append(claimToHeaders, &jwtauthnv3.JwtClaimToHeader{
			ClaimName:  claim.Name,
			HeaderName: claim.Header,
		})
	}
	var shouldForward bool
	if provider.KeepToken != nil && *provider.KeepToken == v1alpha1.TokenForward {
		shouldForward = true
	}
	jwtProvider := &jwtauthnv3.JwtProvider{
		Issuer:            provider.Issuer,
		Audiences:         provider.Audiences,
		PayloadInMetadata: PayloadInMetadata,
		ClaimToHeaders:    claimToHeaders,
		Forward:           shouldForward,
		// TODO(npolshak): Do we want to set NormalizePayload  to support https://datatracker.ietf.org/doc/html/rfc8693#name-scope-scopes-claim
	}
	translateTokenSource(provider, jwtProvider)
	err := translateJwks(krtctx, provider.JWKS, secrets, policyNs, jwtProvider)

	if err != nil {
		return nil, err
	}
	return jwtProvider, nil
}

func translateTokenSource(provider v1alpha1.JWTProvider, out *jwtauthnv3.JwtProvider) {
	if provider.TokenSource == nil {
		return
	}
	if provider.TokenSource.HeaderSource != nil {
		if headers := provider.TokenSource.HeaderSource; len(headers) != 0 {
			for _, header := range headers {
				var headerStr, prefixStr string
				if header.Header != nil {
					headerStr = *header.Header
				}
				if header.Prefix != nil {
					prefixStr = *header.Prefix
				}
				out.FromHeaders = append(out.GetFromHeaders(), &jwtauthnv3.JwtHeader{
					Name:        headerStr,
					ValuePrefix: prefixStr,
				})
			}
		}
	}
	if provider.TokenSource.QueryParams != nil {
		out.FromParams = provider.TokenSource.QueryParams
	}
}

func translateJwks(krtctx krt.HandlerContext, jwkConfig v1alpha1.JWKS, secrets *krtcollections.SecretIndex, policyNs string, out *jwtauthnv3.JwtProvider) error {
	var err error
	var secret *ir.Secret
	var jwkSource *jwtauthnv3.JwtProvider_LocalJwks
	if jwkConfig.LocalJWKS.File != nil {
		jwkSource, err = translateJwksFile(*jwkConfig.LocalJWKS.File)
	} else if jwkConfig.LocalJWKS.InlineKey != nil {
		jwkSource, err = translateJwksInline(*jwkConfig.LocalJWKS.InlineKey)
	} else if jwkConfig.LocalJWKS.SecretRef != nil {
		secret, err = GetSecretIr(secrets, krtctx, jwkConfig.LocalJWKS.SecretRef.Name, policyNs)
		if err != nil {
			return errors.New("failed to find secret " + jwkConfig.LocalJWKS.SecretRef.Name)
		}
		jwkSource, err = translateJwksSecret(jwkConfig.LocalJWKS.SecretRef, secret)
	}
	out.JwksSourceSpecifier = jwkSource
	return err
}

func translateJwksSecret(ref *corev1.LocalObjectReference, secret *ir.Secret) (*jwtauthnv3.JwtProvider_LocalJwks, error) {
	k8sSecret := secret.Obj.(*corev1.Secret)
	secretKey := k8sSecret.Data[ref.Name]
	if secretKey == nil {
		return nil, errors.New("secret key not found")
	}
	return translateJwksInline(string(secretKey))
}

func translateJwksFile(filename string) (*jwtauthnv3.JwtProvider_LocalJwks, error) {
	return &jwtauthnv3.JwtProvider_LocalJwks{
		LocalJwks: &envoycorev3.DataSource{
			Specifier: &envoycorev3.DataSource_Filename{
				Filename: filename,
			},
		},
	}, nil
}

func translateJwksInline(inlineKey string) (*jwtauthnv3.JwtProvider_LocalJwks, error) {
	keyset, err := TranslateKey(inlineKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse inline jwks: %v", err)
	}

	keysetJson, err := json.Marshal(keyset)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize inline jwks: %v", err)
	}

	return &jwtauthnv3.JwtProvider_LocalJwks{
		LocalJwks: &envoycorev3.DataSource{
			Specifier: &envoycorev3.DataSource_InlineString{
				InlineString: string(keysetJson),
			},
		},
	}, nil
}

func TranslateKey(key string) (*jose.JSONWebKeySet, error) {
	// key can be an individual key, a key set or a pem block public key:
	// is it a pem block?
	var multierr error
	ks, err := parsePem(key)
	if err == nil {
		return ks, nil
	}
	multierr = errors.Join(multierr, fmt.Errorf("PEM %v", err))

	ks, err = parseKeySet(key)
	if err == nil {
		if len(ks.Keys) != 0 {
			return ks, nil
		}
		err = errors.New("no keys in set")
	}
	multierr = errors.Join(multierr, fmt.Errorf("JWKS %v", err))

	ks, err = parseKey(key)
	if err == nil {
		return ks, nil
	}
	multierr = errors.Join(multierr, fmt.Errorf("JWK %v", err))

	return nil, fmt.Errorf("cannot parse local jwks: %v", multierr)
}

func parseKeySet(key string) (*jose.JSONWebKeySet, error) {
	var keyset jose.JSONWebKeySet
	err := json.Unmarshal([]byte(key), &keyset)
	return &keyset, err
}

func parseKey(key string) (*jose.JSONWebKeySet, error) {
	var jwk jose.JSONWebKey
	err := json.Unmarshal([]byte(key), &jwk)
	return &jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{jwk},
	}, err
}

func parsePem(key string) (*jose.JSONWebKeySet, error) {
	block, _ := pem.Decode([]byte(key))
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	var err error
	var publicKey interface{}
	publicKey, err = x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		publicKey, err = x509.ParsePKIXPublicKey(block.Bytes) // Parses both RS256 and PS256
		if err != nil {
			return nil, err
		}
	}

	alg := ""
	switch publicKey.(type) {
	// RS256 implied for hash
	case *rsa.PublicKey:
		alg = "RS256"

	case *ecdsa.PublicKey:
		alg = "ES256"

	case ed25519.PublicKey:
		alg = "EdDSA"

	default:
		// HS256 is not supported as this is only used by HMAC, which doesn't use public keys
		return nil, errors.New("unsupported public key. only RSA, ECDSA, and Ed25519 public keys are supported in PEM format")
	}

	jwk := jose.JSONWebKey{
		Key:       publicKey,
		Algorithm: alg,
		Use:       "sig",
	}
	keySet := &jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{jwk},
	}
	return keySet, nil
}

func buildJwtRequirementFromProviders(providersMap map[string]*jwtauthnv3.JwtProvider) *jwtauthnv3.JwtRequirement {
	var reqs []*jwtauthnv3.JwtRequirement
	for providerName := range providersMap {
		reqs = append(reqs, &jwtauthnv3.JwtRequirement{
			RequiresType: &jwtauthnv3.JwtRequirement_ProviderName{
				ProviderName: providerName,
			},
		})
	}

	// sort for idempotency
	sort.Slice(reqs, func(i, j int) bool { return reqs[i].GetProviderName() < reqs[j].GetProviderName() })

	// if there is only one requirement, return it directly
	if len(reqs) == 1 {
		return reqs[0]
	}
	// if there are multiple requirements, return a RequiresAny requirement. Requires Any will OR the requirements
	return &jwtauthnv3.JwtRequirement{
		RequiresType: &jwtauthnv3.JwtRequirement_RequiresAny{
			RequiresAny: &jwtauthnv3.JwtRequirementOrList{
				Requirements: reqs,
			},
		},
	}
}

func GetSecretIr(secrets *krtcollections.SecretIndex, krtctx krt.HandlerContext, secretName, ns string) (*ir.Secret, error) {
	secretRef := gwv1.SecretObjectReference{
		Name: gwv1.ObjectName(secretName),
	}
	from := krtcollections.From{
		GroupKind: ir.GatewayExtensionGVK.GroupKind(),
		Namespace: ns,
	}
	secret, err := secrets.GetSecret(krtctx, from, secretRef)
	if err != nil {
		return nil, fmt.Errorf("failed to find secret %s: %v", secretName, err)
	}
	return secret, nil
}

func jwtFilterName(name string) string {
	if name == "" {
		return jwtFilterNamePrefix
	}
	return fmt.Sprintf("%s/%s", jwtFilterNamePrefix, name)
}
