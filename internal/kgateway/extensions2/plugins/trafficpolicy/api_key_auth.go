package trafficpolicy

import (
	"fmt"

	envoyapikeyauthv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/api_key_auth/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	apiKeyAuthFilterNamePrefix = "envoy.filters.http.api_key_auth" //nolint:gosec
)

// apiKeyAuthIR is the internal representation of an API key authentication policy.
type apiKeyAuthIR struct {
	config *envoyapikeyauthv3.ApiKeyAuth
}

func (a *apiKeyAuthIR) Equals(other *apiKeyAuthIR) bool {
	if a == nil && other == nil {
		return true
	}
	if a == nil || other == nil {
		return false
	}
	if a.config == nil && other.config == nil {
		return true
	}
	if a.config == nil || other.config == nil {
		return false
	}
	// Compare the serialized configs for equality using proto.Equal
	return proto.Equal(a.config, other.config)
}

// Validate performs validation on the API key auth component.
func (a *apiKeyAuthIR) Validate() error {
	if a == nil {
		return nil
	}
	if a.config == nil {
		return nil
	}
	return a.config.Validate()
}

// constructAPIKeyAuth translates the API key authentication spec into an Envoy API key auth filter configuration
func constructAPIKeyAuth(
	krtctx krt.HandlerContext,
	policy *v1alpha1.TrafficPolicy,
	commoncol *collections.CommonCollections,
	out *trafficPolicySpecIr,
) error {
	spec := policy.Spec
	if spec.APIKeyAuthentication == nil {
		return nil
	}

	ak := spec.APIKeyAuthentication

	// Resolve secrets using SecretIndex
	var secrets []*ir.Secret
	secretGK := schema.GroupKind{Group: "", Kind: "Secret"}
	secretCol := commoncol.Secrets.GetSecretCollection(secretGK)
	if secretCol == nil {
		return fmt.Errorf("secret collection not found")
	}

	if ak.SecretRef != nil {
		// Use ResourceName format: Group/Kind/Namespace/Name
		secretObjSource := ir.ObjectSource{
			Group:     "",
			Kind:      "Secret",
			Namespace: policy.Namespace,
			Name:      ak.SecretRef.Name,
		}
		secret := krt.FetchOne(krtctx, secretCol, krt.FilterKey(secretObjSource.ResourceName()))
		if secret == nil {
			return fmt.Errorf("API key secret %s not found in namespace %s", ak.SecretRef.Name, policy.Namespace)
		}
		secrets = []*ir.Secret{secret}
	} else if ak.SecretSelector != nil {
		// Fetch secrets matching labels, then filter by namespace
		allSecrets := krt.Fetch(krtctx, secretCol, krt.FilterLabel(ak.SecretSelector.MatchLabels))
		for i := range allSecrets {
			secret := &allSecrets[i]
			if secret.Namespace == policy.Namespace {
				secrets = append(secrets, secret)
			}
		}
		if len(secrets) == 0 {
			return fmt.Errorf("no secrets found matching selector %v in namespace %s", ak.SecretSelector.MatchLabels, policy.Namespace)
		}
	} else {
		return fmt.Errorf("either secretRef or secretSelector must be specified")
	}

	// Parse secrets and build credentials
	var credentials []*envoyapikeyauthv3.Credential
	var errs []error

	for _, secret := range secrets {
		for keyName, keyValue := range secret.Data {
			// Skip empty values
			if len(keyValue) == 0 {
				continue
			}

			// The value is expected to be a plain string representing the API key
			// The secret key name becomes the client identifier
			apiKey := string(keyValue)
			if apiKey == "" {
				errs = append(errs, fmt.Errorf("secret %s key %s has empty API key value", secret.ObjectSource.Name, keyName))
				continue
			}

			credentials = append(credentials, &envoyapikeyauthv3.Credential{
				Key:    apiKey,
				Client: keyName,
			})
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors processing API key secrets: %v", errs)
	}

	if len(credentials) == 0 {
		return fmt.Errorf("no valid API keys found in secrets")
	}

	// Convert API KeySources to Envoy KeySource format
	var envoyKeySources []*envoyapikeyauthv3.KeySource
	if len(ak.KeySources) > 0 {
		for _, keySource := range ak.KeySources {
			envoyKeySource := &envoyapikeyauthv3.KeySource{}
			if keySource.Header != nil && *keySource.Header != "" {
				envoyKeySource.Header = *keySource.Header
			}
			if keySource.Query != nil && *keySource.Query != "" {
				envoyKeySource.Query = *keySource.Query
			}
			if keySource.Cookie != nil && *keySource.Cookie != "" {
				envoyKeySource.Cookie = *keySource.Cookie
			}
			// Only add if at least one source is specified
			if envoyKeySource.Header != "" || envoyKeySource.Query != "" || envoyKeySource.Cookie != "" {
				envoyKeySources = append(envoyKeySources, envoyKeySource)
			}
		}
	}

	// If no key sources were specified, default to "api-key" header
	if len(envoyKeySources) == 0 {
		envoyKeySources = []*envoyapikeyauthv3.KeySource{
			{
				Header: "api-key",
			},
		}
	}

	// Determine hide credentials (default to false)
	hideCredentials := false
	if ak.HideAPIKey != nil {
		hideCredentials = *ak.HideAPIKey
	}

	// Build Envoy API key auth filter configuration
	apiKeyAuthConfig := &envoyapikeyauthv3.ApiKeyAuth{
		Credentials: credentials,
		KeySources:  envoyKeySources,
		Forwarding: &envoyapikeyauthv3.Forwarding{
			Header:          "x-client-id",
			HideCredentials: hideCredentials,
		},
	}

	out.apiKeyAuth = &apiKeyAuthIR{
		config: apiKeyAuthConfig,
	}

	return nil
}

// handleAPIKeyAuth configures the API key auth filter and per-route API key auth configuration.
// This follows the same pattern as RBAC: add an empty filter to the chain and put the actual config
// in the typedPerFilterConfig. The per-route config will be applied at RouteConfiguration level for
// gateway-level policies, and at Route level for route-level policies (which will override the
// RouteConfiguration level config).
//
// IMPORTANT: For route-level-only policies (no gateway-level policy), we add FilterConfig with
// disabled: true at the RouteConfiguration level in ApplyRouteConfigPlugin. This disables the filter
// for all routes by default. Routes with policies will override this with ApiKeyAuthPerRoute, which
// enables the filter for those specific routes.
func (p *trafficPolicyPluginGwPass) handleAPIKeyAuth(
	fcn string,
	pCtxTypedFilterConfig *ir.TypedFilterConfigMap,
	apiKeyAuthIr *apiKeyAuthIR,
) {
	if apiKeyAuthIr == nil || apiKeyAuthIr.config == nil {
		return
	}

	// Always add the filter to the chain if not already present.
	// For route-level-only policies, it will be disabled at RouteConfiguration level,
	// and enabled per-route via ApiKeyAuthPerRoute for routes with policies.
	if p.apiKeyAuthInChain == nil {
		p.apiKeyAuthInChain = make(map[string]*envoyapikeyauthv3.ApiKeyAuth)
	}
	if _, ok := p.apiKeyAuthInChain[fcn]; !ok {
		p.apiKeyAuthInChain[fcn] = &envoyapikeyauthv3.ApiKeyAuth{}
	}

	// Always add the per-route API key auth configuration to the typed filter config
	// This will be applied at RouteConfiguration level for gateway-level policies,
	// and at Route level for route-level policies (overriding RouteConfiguration level)
	perRouteConfig := &envoyapikeyauthv3.ApiKeyAuthPerRoute{
		Credentials: apiKeyAuthIr.config.Credentials,
		KeySources:  apiKeyAuthIr.config.KeySources,
		Forwarding:  apiKeyAuthIr.config.Forwarding,
	}

	pCtxTypedFilterConfig.AddTypedConfig(apiKeyAuthFilterNamePrefix, perRouteConfig)
}
