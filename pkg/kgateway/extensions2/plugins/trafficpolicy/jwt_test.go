package trafficpolicy

import (
	"errors"
	"testing"
	"time"

	jwtauthnv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func TestTranslateKey(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		expectedError bool
		expectedKeys  int
	}{
		{
			name: "valid JWKS",
			key: `{
				"keys": [
					{
						"kty": "RSA",
						"kid": "test-key",
						"use": "sig",
						"alg": "RS256",
						"n": "test-n",
						"e": "AQAB"
					}
				]
			}`,
			expectedError: false,
			expectedKeys:  1,
		},
		{
			name: "valid single JWK",
			key: `{
				"kty": "RSA",
				"kid": "test-key",
				"use": "sig",
				"alg": "RS256",
				"n": "test-n",
				"e": "AQAB"
			}`,
			expectedError: false,
			expectedKeys:  1,
		},
		{
			name:          "invalid JSON",
			key:           "{invalid json}",
			expectedError: true,
			expectedKeys:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyset, err := TranslateKey(tt.key)
			if tt.expectedError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedKeys, len(keyset.Keys))
		})
	}
}

func TestBuildJwtRequirementFromProviders(t *testing.T) {
	tests := []struct {
		name            string
		routeName       string
		providers       map[string]*jwtauthnv3.JwtProvider
		validationMode  *kgateway.ValidationMode
		expectedType    string
		expectedCount   int
		hasAllowMissing bool
	}{
		{
			name:      "single provider strict mode",
			routeName: "test-route",
			providers: map[string]*jwtauthnv3.JwtProvider{
				"provider1": {Issuer: "test-issuer"},
			},
			validationMode:  nil,
			expectedType:    "provider_name",
			expectedCount:   1,
			hasAllowMissing: false,
		},
		{
			name:      "multiple providers strict mode",
			routeName: "test-route",
			providers: map[string]*jwtauthnv3.JwtProvider{
				"provider1": {Issuer: "test-issuer-1"},
				"provider2": {Issuer: "test-issuer-2"},
			},
			validationMode:  nil,
			expectedType:    "requires_any",
			expectedCount:   2,
			hasAllowMissing: false,
		},
		{
			name:      "single provider allow missing mode",
			routeName: "test-route",
			providers: map[string]*jwtauthnv3.JwtProvider{
				"provider1": {Issuer: "test-issuer"},
			},
			validationMode:  ptr.To(kgateway.ValidationModeAllowMissing),
			expectedType:    "requires_any",
			expectedCount:   2, // provider requirement + allow missing
			hasAllowMissing: true,
		},
		{
			name:      "multiple providers allow missing mode",
			routeName: "test-route",
			providers: map[string]*jwtauthnv3.JwtProvider{
				"provider1": {Issuer: "test-issuer-1"},
				"provider2": {Issuer: "test-issuer-2"},
			},
			validationMode:  ptr.To(kgateway.ValidationModeAllowMissing),
			expectedType:    "requires_any",
			expectedCount:   2, // requires_any with providers + allow missing
			hasAllowMissing: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := buildJwtRequirementFromProviders(tt.providers, tt.validationMode)
			if tt.hasAllowMissing {
				// When allow missing is enabled, the top level should be RequiresAny
				assert.NotNil(t, req.GetRequiresAny())
				assert.Equal(t, tt.expectedCount, len(req.GetRequiresAny().Requirements))

				// Check that one of the requirements is AllowMissing
				hasAllowMissing := false
				for _, r := range req.GetRequiresAny().Requirements {
					if r.GetAllowMissing() != nil {
						hasAllowMissing = true
						break
					}
				}
				assert.True(t, hasAllowMissing, "expected AllowMissing requirement")
			} else if tt.expectedType == "provider_name" {
				assert.NotNil(t, req.GetProviderName())
				assert.Equal(t, "provider1", req.GetProviderName())
			} else {
				assert.NotNil(t, req.GetRequiresAny())
				assert.Equal(t, tt.expectedCount, len(req.GetRequiresAny().Requirements))
			}
		})
	}
}

func TestTranslateJwksConfigMap(t *testing.T) {
	tests := []struct {
		name          string
		cm            *corev1.ConfigMap
		expectedError bool
	}{
		{
			name: "valid configmap",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cm",
				},
				Data: map[string]string{
					"jwks": `{"keys":[{"kty":"RSA","kid":"test-key","use":"sig","alg":"RS256","n":"test-n","e":"AQAB"}]}`,
				},
			},
			expectedError: false,
		},
		{
			name: "missing key in configmap",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cm",
				},
				Data: map[string]string{},
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jwks, err := translateJwksConfigMap(tt.cm)
			if tt.expectedError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, jwks)
			assert.NotNil(t, jwks.LocalJwks)
		})
	}
}

func TestConvertJwtValidationConfig(t *testing.T) {
	tests := []struct {
		name           string
		providers      []kgateway.NamedJWTProvider
		expectedError  bool
		expectedConfig *jwtauthnv3.JwtAuthentication
	}{
		{
			name: "basic provider with inline JWKS",
			providers: []kgateway.NamedJWTProvider{
				{
					Name: "test-provider",
					JWTProvider: kgateway.JWTProvider{
						Issuer: "test-issuer",
						JWKS: kgateway.JWKS{
							LocalJWKS: &kgateway.LocalJWKS{
								Inline: new(`{"keys":[{"kty":"RSA","kid":"test-key","use":"sig","alg":"RS256","n":"test-n","e":"AQAB"}]}`),
							},
						},
						ClaimsToHeaders: []kgateway.JWTClaimToHeader{
							{
								Name:   "sub",
								Header: "X-Subject",
							},
						},
						ForwardToken: new(true),
					},
				},
			},
			expectedError: false,
			expectedConfig: &jwtauthnv3.JwtAuthentication{
				Providers: map[string]*jwtauthnv3.JwtProvider{
					"test-policy_test-ns_test-provider": {
						Issuer:            "test-issuer",
						Audiences:         nil,
						PayloadInMetadata: PayloadInMetadata,
						ClaimToHeaders: []*jwtauthnv3.JwtClaimToHeader{
							{
								ClaimName:  "sub",
								HeaderName: "X-Subject",
							},
						},
						Forward:         true,
						ClearRouteCache: true,
					},
				},
			},
		},
		{
			name: "missing inline key for inline JWKS",
			providers: []kgateway.NamedJWTProvider{
				{
					Name: "test-provider",
					JWTProvider: kgateway.JWTProvider{
						Issuer: "test-issuer",
						JWKS: kgateway.JWKS{
							LocalJWKS: &kgateway.LocalJWKS{
								Inline: new("abc"),
							},
						},
					},
				},
			},
			expectedError:  true,
			expectedConfig: nil,
		},
		{
			name: "multiple providers",
			providers: []kgateway.NamedJWTProvider{
				{
					Name: "provider1",
					JWTProvider: kgateway.JWTProvider{
						Issuer: "test-issuer-1",
						JWKS: kgateway.JWKS{
							LocalJWKS: &kgateway.LocalJWKS{
								Inline: new(`{"keys":[{"kty":"RSA","kid":"test-key-1","use":"sig","alg":"RS256","n":"test-n-1","e":"AQAB"}]}`),
							},
						},
					},
				},
				{
					Name: "provider2",
					JWTProvider: kgateway.JWTProvider{
						Issuer: "test-issuer-2",
						JWKS: kgateway.JWKS{
							LocalJWKS: &kgateway.LocalJWKS{
								Inline: new(`{"keys":[{"kty":"RSA","kid":"test-key-2","use":"sig","alg":"RS256","n":"test-n-2","e":"AQAB"}]}`),
							},
						},
					},
				},
			},
			expectedError: false,
			expectedConfig: &jwtauthnv3.JwtAuthentication{
				Providers: map[string]*jwtauthnv3.JwtProvider{
					"test-policy_test-ns_provider1": {
						Issuer:            "test-issuer-1",
						Audiences:         nil,
						PayloadInMetadata: PayloadInMetadata,
					},
					"test-policy_test-ns_provider2": {
						Issuer:            "test-issuer-2",
						Audiences:         nil,
						PayloadInMetadata: PayloadInMetadata,
					},
				},
			},
		},
		{
			name: "provider with audiences",
			providers: []kgateway.NamedJWTProvider{
				{
					Name: "test-provider",
					JWTProvider: kgateway.JWTProvider{
						Issuer:    "test-issuer",
						Audiences: []string{"aud1", "aud2"},
						JWKS: kgateway.JWKS{
							LocalJWKS: &kgateway.LocalJWKS{
								Inline: new(`{"keys":[{"kty":"RSA","kid":"test-key","use":"sig","alg":"RS256","n":"test-n","e":"AQAB"}]}`),
							},
						},
					},
				},
			},
			expectedError: false,
			expectedConfig: &jwtauthnv3.JwtAuthentication{
				Providers: map[string]*jwtauthnv3.JwtProvider{
					"test-policy_test-ns_test-provider": {
						Issuer:            "test-issuer",
						Audiences:         []string{"aud1", "aud2"},
						PayloadInMetadata: PayloadInMetadata,
					},
				},
			},
		},
		{
			name: "provider with token source",
			providers: []kgateway.NamedJWTProvider{
				{
					Name: "test-provider",
					JWTProvider: kgateway.JWTProvider{
						Issuer: "test-issuer",
						TokenSource: &kgateway.JWTTokenSource{
							HeaderSource: &kgateway.HeaderSource{
								Header: "Authorization",
							},
						},
						JWKS: kgateway.JWKS{
							LocalJWKS: &kgateway.LocalJWKS{
								Inline: new(`{"keys":[{"kty":"RSA","kid":"test-key","use":"sig","alg":"RS256","n":"test-n","e":"AQAB"}]}`),
							},
						},
					},
				},
			},
			expectedError: false,
			expectedConfig: &jwtauthnv3.JwtAuthentication{
				Providers: map[string]*jwtauthnv3.JwtProvider{
					"test-policy_test-ns_test-provider": {
						Issuer:            "test-issuer",
						Audiences:         nil,
						PayloadInMetadata: PayloadInMetadata,
					},
				},
			},
		},
		{
			name: "provider with query params",
			providers: []kgateway.NamedJWTProvider{
				{
					Name: "test-provider",
					JWTProvider: kgateway.JWTProvider{
						Issuer: "test-issuer",
						TokenSource: &kgateway.JWTTokenSource{
							QueryParameter: new("jwt"),
						},
						JWKS: kgateway.JWKS{
							LocalJWKS: &kgateway.LocalJWKS{
								Inline: new(`{"keys":[{"kty":"RSA","kid":"test-key","use":"sig","alg":"RS256","n":"test-n","e":"AQAB"}]}`),
							},
						},
					},
				},
			},
			expectedError: false,
			expectedConfig: &jwtauthnv3.JwtAuthentication{
				Providers: map[string]*jwtauthnv3.JwtProvider{
					"test-policy_test-ns_test-provider": {
						Issuer:            "test-issuer",
						Audiences:         nil,
						PayloadInMetadata: PayloadInMetadata,
						FromParams:        []string{"jwt"},
					},
				},
			},
		},
		{
			name: "provider with remove token",
			providers: []kgateway.NamedJWTProvider{
				{
					Name: "test-provider",
					JWTProvider: kgateway.JWTProvider{
						Issuer: "test-issuer",
						JWKS: kgateway.JWKS{
							LocalJWKS: &kgateway.LocalJWKS{
								Inline: new(`{"keys":[{"kty":"RSA","kid":"test-key","use":"sig","alg":"RS256","n":"test-n","e":"AQAB"}]}`),
							},
						},
						ForwardToken: new(false),
					},
				},
			},
			expectedError: false,
			expectedConfig: &jwtauthnv3.JwtAuthentication{
				Providers: map[string]*jwtauthnv3.JwtProvider{
					"test-policy_test-ns_test-provider": {
						Issuer:            "test-issuer",
						Audiences:         nil,
						PayloadInMetadata: PayloadInMetadata,
						Forward:           false,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jwt := &kgateway.JWT{
				Providers: tt.providers,
			}
			config, err := resolveJwtProviders(nil, nil, nil, ir.ObjectSource{}, "test-policy", "test-ns", jwt)
			if tt.expectedError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, config)
			assert.Equal(t, len(tt.expectedConfig.Providers), len(config.Providers))
			for providerName, expectedProvider := range tt.expectedConfig.Providers {
				actualProvider, ok := config.Providers[providerName]
				require.True(t, ok, "provider %s not found", providerName)
				assert.Equal(t, expectedProvider.Issuer, actualProvider.Issuer)
				assert.Equal(t, expectedProvider.Audiences, actualProvider.Audiences)
				assert.Equal(t, expectedProvider.PayloadInMetadata, actualProvider.PayloadInMetadata)
				assert.Equal(t, expectedProvider.Forward, actualProvider.Forward)

				// Check claim to headers
				assert.Equal(t, len(expectedProvider.ClaimToHeaders), len(actualProvider.ClaimToHeaders))
				for i, expectedClaim := range expectedProvider.ClaimToHeaders {
					actualClaim := actualProvider.ClaimToHeaders[i]
					assert.Equal(t, expectedClaim.ClaimName, actualClaim.ClaimName)
					assert.Equal(t, expectedClaim.HeaderName, actualClaim.HeaderName)
				}

				// Check token source
				if expectedProvider.FromHeaders != nil {
					assert.Equal(t, len(expectedProvider.FromHeaders), len(actualProvider.FromHeaders))
					for i, expectedHeader := range expectedProvider.FromHeaders {
						actualHeader := actualProvider.FromHeaders[i]
						assert.Equal(t, expectedHeader.Name, actualHeader.Name)
						assert.Equal(t, expectedHeader.ValuePrefix, actualHeader.ValuePrefix)
					}
				}
				assert.Equal(t, expectedProvider.FromParams, actualProvider.FromParams)

				// Check JWKS source
				if expectedProvider.JwksSourceSpecifier != nil {
					assert.NotNil(t, actualProvider.JwksSourceSpecifier)
					expectedJwks := expectedProvider.JwksSourceSpecifier.(*jwtauthnv3.JwtProvider_LocalJwks)
					actualJwks := actualProvider.JwksSourceSpecifier.(*jwtauthnv3.JwtProvider_LocalJwks)
					assert.NotNil(t, expectedJwks)
					assert.NotNil(t, actualJwks)
					assert.NotNil(t, actualJwks.LocalJwks)
				}
			}
		})
	}
}

func TestResolveJwtProvidersWithValidationMode(t *testing.T) {
	tests := []struct {
		name                    string
		jwt                     *kgateway.JWT
		expectedHasAllowMissing bool
	}{
		{
			name: "strict mode (nil validation mode)",
			jwt: &kgateway.JWT{
				ValidationMode: nil,
				Providers: []kgateway.NamedJWTProvider{
					{
						Name: "test-provider",
						JWTProvider: kgateway.JWTProvider{
							Issuer: "test-issuer",
							JWKS: kgateway.JWKS{
								LocalJWKS: &kgateway.LocalJWKS{
									Inline: new(`{"keys":[{"kty":"RSA","kid":"test-key","use":"sig","alg":"RS256","n":"test-n","e":"AQAB"}]}`),
								},
							},
						},
					},
				},
			},
			expectedHasAllowMissing: false,
		},
		{
			name: "allow missing mode",
			jwt: &kgateway.JWT{
				ValidationMode: ptr.To(kgateway.ValidationModeAllowMissing),
				Providers: []kgateway.NamedJWTProvider{
					{
						Name: "test-provider",
						JWTProvider: kgateway.JWTProvider{
							Issuer: "test-issuer",
							JWKS: kgateway.JWKS{
								LocalJWKS: &kgateway.LocalJWKS{
									Inline: new(`{"keys":[{"kty":"RSA","kid":"test-key","use":"sig","alg":"RS256","n":"test-n","e":"AQAB"}]}`),
								},
							},
						},
					},
				},
			},
			expectedHasAllowMissing: true,
		},
		{
			name: "allow missing mode with multiple providers",
			jwt: &kgateway.JWT{
				ValidationMode: ptr.To(kgateway.ValidationModeAllowMissing),
				Providers: []kgateway.NamedJWTProvider{
					{
						Name: "provider1",
						JWTProvider: kgateway.JWTProvider{
							Issuer: "test-issuer-1",
							JWKS: kgateway.JWKS{
								LocalJWKS: &kgateway.LocalJWKS{
									Inline: new(`{"keys":[{"kty":"RSA","kid":"test-key-1","use":"sig","alg":"RS256","n":"test-n-1","e":"AQAB"}]}`),
								},
							},
						},
					},
					{
						Name: "provider2",
						JWTProvider: kgateway.JWTProvider{
							Issuer: "test-issuer-2",
							JWKS: kgateway.JWKS{
								LocalJWKS: &kgateway.LocalJWKS{
									Inline: new(`{"keys":[{"kty":"RSA","kid":"test-key-2","use":"sig","alg":"RS256","n":"test-n-2","e":"AQAB"}]}`),
								},
							},
						},
					},
				},
			},
			expectedHasAllowMissing: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := resolveJwtProviders(nil, nil, nil, ir.ObjectSource{}, "test-policy", "test-ns", tt.jwt)
			require.NoError(t, err)
			assert.NotNil(t, config)
			assert.NotNil(t, config.RequirementMap)

			requirementsName := "test-policy_test-ns_requirements"
			req, ok := config.RequirementMap[requirementsName]
			require.True(t, ok, "requirements not found in map")

			if tt.expectedHasAllowMissing {
				// Should have RequiresAny at top level with AllowMissing
				assert.NotNil(t, req.GetRequiresAny())
				hasAllowMissing := false
				for _, r := range req.GetRequiresAny().Requirements {
					if r.GetAllowMissing() != nil {
						hasAllowMissing = true
						break
					}
				}
				assert.True(t, hasAllowMissing, "expected AllowMissing requirement")
			} else {
				// Strict mode: should have provider name directly (single provider) or RequiresAny (multiple providers)
				// but no AllowMissing
				if req.GetRequiresAny() != nil {
					for _, r := range req.GetRequiresAny().Requirements {
						assert.Nil(t, r.GetAllowMissing(), "should not have AllowMissing in strict mode")
					}
				}
			}
		})
	}
}

type fakeBackendResolver struct {
	backend *ir.BackendObjectIR
	err     error
}

func (f *fakeBackendResolver) GetBackendFromRef(krtctx krt.HandlerContext, src ir.ObjectSource, ref gwv1.BackendObjectReference) (*ir.BackendObjectIR, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.backend, nil
}

func TestTranslateJwksRemote(t *testing.T) {
	makeBackendRef := func(name, namespace string, portNumber int32) gwv1.BackendObjectReference {
		ns := gwv1.Namespace(namespace)
		port := gwv1.PortNumber(portNumber)
		return gwv1.BackendObjectReference{
			Name:      gwv1.ObjectName(name),
			Namespace: &ns,
			Port:      &port,
		}
	}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		backend := &ir.BackendObjectIR{
			ObjectSource: ir.ObjectSource{
				Kind:      "Service",
				Namespace: "backend-ns",
				Name:      "backend",
			},
			GvPrefix: "svc",
			Port:     8443,
		}
		resolver := &fakeBackendResolver{backend: backend}
		out := &jwtauthnv3.JwtProvider{}
		cacheDuration := metav1.Duration{Duration: time.Minute}

		err := translateJwks(
			nil,
			kgateway.JWKS{
				RemoteJWKS: &kgateway.RemoteJWKS{
					URL:           "https://example.com/jwks",
					BackendRef:    makeBackendRef("backend", "backend-ns", 8443),
					CacheDuration: &cacheDuration,
				},
			},
			"policy-ns",
			out,
			nil,
			resolver,
			ir.ObjectSource{Namespace: "policy-ns"},
		)
		require.NoError(t, err)

		remote, ok := out.JwksSourceSpecifier.(*jwtauthnv3.JwtProvider_RemoteJwks)
		require.True(t, ok, "expected remote jwks config to be set")
		assert.Equal(t, "https://example.com/jwks", remote.RemoteJwks.GetHttpUri().GetUri())
		assert.Equal(t, backend.ClusterName(), remote.RemoteJwks.GetHttpUri().GetCluster())
		require.NotNil(t, remote.RemoteJwks.GetCacheDuration())
		assert.Equal(t, time.Minute, remote.RemoteJwks.GetCacheDuration().AsDuration())
	})

	t.Run("missing backend ref errors", func(t *testing.T) {
		t.Parallel()
		resolver := &fakeBackendResolver{err: errors.New("backend missing")}
		out := &jwtauthnv3.JwtProvider{}

		err := translateJwks(
			nil,
			kgateway.JWKS{
				RemoteJWKS: &kgateway.RemoteJWKS{
					URL:        "https://example.com/jwks",
					BackendRef: makeBackendRef("backend", "backend-ns", 80),
				},
			},
			"policy-ns",
			out,
			nil,
			resolver,
			ir.ObjectSource{Namespace: "policy-ns"},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "remote jwks: unresolved backend ref")
	})
}
