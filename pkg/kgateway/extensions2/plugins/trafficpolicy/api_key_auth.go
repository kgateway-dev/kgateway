package trafficpolicy

import (
	"fmt"

	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyapikeyauthv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/api_key_auth/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	apiKeyAuthFilterNamePrefix = "envoy.filters.http.api_key_auth" //nolint:gosec
)

// apiKeyAuthIR is the internal representation of an API key authentication policy.
type apiKeyAuthIR struct {
	config  *envoyapikeyauthv3.ApiKeyAuthPerRoute
	disable bool
}

func (a *apiKeyAuthIR) Equals(other *apiKeyAuthIR) bool {
	if a == nil && other == nil {
		return true
	}
	if a == nil || other == nil {
		return false
	}
	if a.disable != other.disable {
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

// constructAPIKeyAuth translates the API key authentication spec into an Envoy API key auth per-route configuration
func constructAPIKeyAuth(
	krtctx krt.HandlerContext,
	policy *kgateway.TrafficPolicy,
	commoncol *collections.CommonCollections,
	out *trafficPolicySpecIr,
) error {
	spec := policy.Spec
	if spec.APIKeyAuthentication == nil {
		return nil
	}

	ak := spec.APIKeyAuthentication

	// Handle disable case
	if ak.Disable != nil {
		out.apiKeyAuth = &apiKeyAuthIR{
			disable: true,
		}
		return nil
	}

	// Resolve secrets using SecretIndex with ReferenceGrant validation
	var secrets []ir.Secret
	secretGK := schema.GroupKind{Group: "", Kind: "Secret"}
	policyGK := wellknown.TrafficPolicyGVK.GroupKind()
	from := krtcollections.From{
		GroupKind: policyGK,
		Namespace: policy.Namespace,
	}

	if ak.SecretRef != nil {
		secret, err := commoncol.Secrets.GetSecret(krtctx, from, *ak.SecretRef)
		if err != nil {
			return fmt.Errorf("API key secret %s: %w", ak.SecretRef.Name, err)
		}
		secrets = []ir.Secret{*secret}
	} else if ak.SecretSelector != nil {
		// Fetch secrets matching labels and namespace with ReferenceGrant validation
		var err error
		secrets, err = commoncol.Secrets.GetSecretsBySelector(
			krtctx,
			from,
			secretGK,
			ak.SecretSelector.MatchLabels,
		)
		if err != nil {
			return fmt.Errorf("failed to get secrets by selector: %w", err)
		}
		if len(secrets) == 0 {
			return fmt.Errorf("no secrets found matching selector %v in namespace %s", ak.SecretSelector.MatchLabels, policy.Namespace)
		}
	} else {
		// We shouldn't get here because the spec validation should catch this
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

	// Determine hide credentials (default to true since ForwardCredential defaults to false)
	hideCredentials := true
	if ak.ForwardCredential != nil {
		hideCredentials = !(*ak.ForwardCredential)
	}

	// Build Envoy API key auth per-route configuration
	apiKeyAuthPolicy := &envoyapikeyauthv3.ApiKeyAuthPerRoute{
		Credentials: credentials,
		KeySources:  envoyKeySources,
		Forwarding: &envoyapikeyauthv3.Forwarding{
			HideCredentials: hideCredentials,
		},
	}

	// Only set client ID header forwarding if ClientIdHeader is specified
	if ak.ClientIdHeader != nil {
		apiKeyAuthPolicy.Forwarding.Header = *ak.ClientIdHeader
	}

	out.apiKeyAuth = &apiKeyAuthIR{
		config: apiKeyAuthPolicy,
	}

	return nil
}

// handleAPIKeyAuth configures the API key auth filter and per-route API key auth configuration.
// This follows the same pattern as CORS: add the policy to the typed_per_filter_config.
// Also requires API key auth http_filter to be added to the filter chain.
func (p *trafficPolicyPluginGwPass) handleAPIKeyAuth(
	fcn string,
	pCtxTypedFilterConfig *ir.TypedFilterConfigMap,
	apiKeyAuthIr *apiKeyAuthIR,
) {
	if apiKeyAuthIr == nil {
		return
	}

	// Handle disable case - set disabled flag to override parent policy
	if apiKeyAuthIr.disable {
		pCtxTypedFilterConfig.AddTypedConfig(apiKeyAuthFilterNamePrefix, &envoyroutev3.FilterConfig{Disabled: true})
		return
	}

	if apiKeyAuthIr.config == nil {
		return
	}

	// Adds the ApiKeyAuthPerRoute to the typed_per_filter_config.
	// Also requires API key auth http_filter to be added to the filter chain.
	pCtxTypedFilterConfig.AddTypedConfig(apiKeyAuthFilterNamePrefix, apiKeyAuthIr.config)

	// Add a filter to the chain. When having an api key auth policy for a route we need to also have a
	// globally api key auth http filter in the chain otherwise it will be ignored.
	if p.apiKeyAuthInChain == nil {
		p.apiKeyAuthInChain = make(map[string]*envoyapikeyauthv3.ApiKeyAuth)
	}
	if _, ok := p.apiKeyAuthInChain[fcn]; !ok {
		p.apiKeyAuthInChain[fcn] = &envoyapikeyauthv3.ApiKeyAuth{}
	}
}
