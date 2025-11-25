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
	"slices"
	"sort"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	jwtauthnv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	"github.com/go-jose/go-jose/v4"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

const (
	PayloadInMetadata   = "payload"
	jwtFilterNamePrefix = "jwt"
	jwtConfigMapKey     = "jwks"

	remoteJWKSTimeoutSecs = 5
)

type jwtIr struct {
	perProviderConfig []*perProviderJwtConfig
}

type perProviderJwtConfig struct {
	provider       *TrafficPolicyGatewayExtensionIR
	perRouteConfig *jwtauthnv3.PerRouteConfig
}

var _ PolicySubIR = &jwtIr{}

func (j *jwtIr) Equals(other PolicySubIR) bool {
	otherJwt, ok := other.(*jwtIr)
	if !ok {
		return false
	}
	if j == nil || otherJwt == nil {
		return j == nil && otherJwt == nil
	}

	return slices.EqualFunc(j.perProviderConfig, otherJwt.perProviderConfig, func(a, b *perProviderJwtConfig) bool {
		return proto.Equal(a.perRouteConfig, b.perRouteConfig) &&
			cmputils.CompareWithNils(a.provider, b.provider, func(a, b *TrafficPolicyGatewayExtensionIR) bool {
				return a.Equals(*b)
			})
	})
}

// handleJwt configures the filter JwtAuthentication and per-route JWT configuration for a specific route
func (p *trafficPolicyPluginGwPass) handleJwt(fcn string, pCtxTypedFilterConfig *ir.TypedFilterConfigMap, jwtIr *jwtIr) {
	if jwtIr == nil {
		return
	}

	for _, cfg := range jwtIr.perProviderConfig {
		if cfg == nil {
			continue
		}
		providerName := providerName(cfg.provider)
		if cfg.perRouteConfig != nil {
			jwtName := jwtFilterName(providerName)
			pCtxTypedFilterConfig.AddTypedConfig(jwtName, cfg.perRouteConfig)
		}
		p.jwtPerProvider.Add(fcn, providerName, cfg.provider)
	}
}

func translatePerRouteConfig(requirementsName string) *jwtauthnv3.PerRouteConfig {
	perRouteConfig := &jwtauthnv3.PerRouteConfig{
		RequirementSpecifier: &jwtauthnv3.PerRouteConfig_RequirementName{
			RequirementName: requirementsName,
		},
	}
	return perRouteConfig
}

// constructJwt translates the jwt spec into an envoy jwt policy and stores it in the traffic policy IR
func constructJwt(
	krtctx krt.HandlerContext,
	in *v1alpha1.TrafficPolicy,
	out *trafficPolicySpecIr,
	fetchGatewayExtension FetchGatewayExtensionFunc,
) error {
	spec := in.Spec.JWT
	if spec == nil {
		return nil
	}

	provider, err := fetchGatewayExtension(krtctx, spec.ExtensionRef, in.GetNamespace())
	if err != nil {
		return fmt.Errorf("jwt: %w", err)
	}
	if provider.Jwt == nil {
		return pluginutils.ErrInvalidExtensionType(v1alpha1.GatewayExtensionTypeJWTProvider)
	}

	requirementsName := fmt.Sprintf("%s_%s_requirements", spec.ExtensionRef.Name, in.Namespace)
	perRouteConfig := translatePerRouteConfig(requirementsName)

	out.jwt = &jwtIr{
		perProviderConfig: []*perProviderJwtConfig{
			{
				provider:       provider,
				perRouteConfig: perRouteConfig,
			},
		},
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

	for _, cfg := range j.perProviderConfig {
		if cfg == nil {
			continue
		}
		if cfg.provider != nil {
			if err := cfg.provider.Validate(); err != nil {
				errs = append(errs, err)
			}
		}
		if cfg.perRouteConfig != nil {
			if err := cfg.perRouteConfig.Validate(); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}

func ProviderName(resourceName, providerName string) string {
	return fmt.Sprintf("%s_%s", resourceName, providerName)
}

func translateProvider(
	krtctx krt.HandlerContext,
	provider v1alpha1.JWTProvider,
	policyNs string,
	configMaps krt.Collection[*corev1.ConfigMap],
	resolver backendResolver,
	gwExtObj ir.ObjectSource,
) (*jwtauthnv3.JwtProvider, error) {
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
	if len(claimToHeaders) > 0 {
		jwtProvider.ClearRouteCache = true
	}
	translateTokenSource(provider, jwtProvider)
	err := translateJwks(krtctx, provider.JWKS, policyNs, jwtProvider, configMaps, resolver, gwExtObj)
	if err != nil {
		return nil, err
	}
	return jwtProvider, nil
}

func translateTokenSource(provider v1alpha1.JWTProvider, out *jwtauthnv3.JwtProvider) {
	if provider.TokenSource == nil {
		return
	}
	if headerSource := provider.TokenSource.HeaderSource; headerSource != nil {
		var prefix string
		if headerSource.Prefix != nil {
			prefix = *headerSource.Prefix
		}
		out.FromHeaders = []*jwtauthnv3.JwtHeader{
			{
				Name:        headerSource.Header,
				ValuePrefix: prefix,
			},
		}
	}
	if queryParams := provider.TokenSource.QueryParameter; queryParams != nil {
		out.FromParams = []string{*queryParams}
	}
}

type backendResolver interface {
	GetBackendFromRef(krt.HandlerContext, ir.ObjectSource, gwv1.BackendObjectReference) (*ir.BackendObjectIR, error)
}

func translateJwks(
	krtctx krt.HandlerContext,
	jwkConfig v1alpha1.JWKS,
	policyNs string,
	out *jwtauthnv3.JwtProvider,
	configMaps krt.Collection[*corev1.ConfigMap],
	resolver backendResolver,
	gwExtObj ir.ObjectSource,
) error {
	switch {
	case jwkConfig.LocalJWKS != nil:
		switch {
		case jwkConfig.LocalJWKS.Inline != nil:
			jwkSource, err := translateJwksInline(*jwkConfig.LocalJWKS.Inline)
			if err != nil {
				return err
			}
			out.JwksSourceSpecifier = jwkSource
		case jwkConfig.LocalJWKS.ConfigMapRef != nil:
			cm, err := GetConfigMap(krtctx, configMaps, jwkConfig.LocalJWKS.ConfigMapRef.Name, policyNs)
			if err != nil {
				return fmt.Errorf("failed to find configmap %s: %v", jwkConfig.LocalJWKS.ConfigMapRef.Name, err)
			}
			jwkSource, err := translateJwksConfigMap(cm)
			if err != nil {
				return err
			}
			out.JwksSourceSpecifier = jwkSource
		}
	case jwkConfig.RemoteJWKS != nil:
		remote := jwkConfig.RemoteJWKS
		backend, err := resolver.GetBackendFromRef(krtctx, gwExtObj, remote.BackendRef)
		if err != nil {
			return fmt.Errorf("remote jwks: unresolved backend ref: %w", err)
		}
		jwksOut := &jwtauthnv3.JwtProvider_RemoteJwks{
			RemoteJwks: &jwtauthnv3.RemoteJwks{
				HttpUri: &envoycorev3.HttpUri{
					Timeout: &durationpb.Duration{Seconds: remoteJWKSTimeoutSecs},
					Uri:     remote.URL,
					HttpUpstreamType: &envoycorev3.HttpUri_Cluster{
						Cluster: backend.ClusterName(),
					},
				},
			},
		}
		if remote.CacheDuration != nil {
			jwksOut.RemoteJwks.CacheDuration = durationpb.New(remote.CacheDuration.Duration)
		}
		out.JwksSourceSpecifier = jwksOut
	}
	return nil
}

func translateJwksConfigMap(cm *corev1.ConfigMap) (*jwtauthnv3.JwtProvider_LocalJwks, error) {
	data := cm.Data[jwtConfigMapKey]
	if data == "" {
		return nil, fmt.Errorf("configmap key '%s' not found", jwtConfigMapKey)
	}
	return translateJwksInline(data)
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
	var publicKey any
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

func GetConfigMap(krtctx krt.HandlerContext, configMaps krt.Collection[*corev1.ConfigMap], cmName, ns string) (*corev1.ConfigMap, error) {
	if configMaps == nil {
		return nil, errors.New("configmaps collection not available")
	}
	obj := krt.FetchOne(krtctx, configMaps, krt.FilterObjectName(types.NamespacedName{Namespace: ns, Name: cmName}))
	if obj == nil {
		return nil, &krtcollections.NotFoundError{NotFoundObj: ir.ObjectSource{Group: "", Kind: "ConfigMap", Namespace: ns, Name: cmName}}
	}
	return *obj, nil
}

func jwtFilterName(name string) string {
	if name == "" {
		return jwtFilterNamePrefix
	}
	return fmt.Sprintf("%s/%s", jwtFilterNamePrefix, name)
}
