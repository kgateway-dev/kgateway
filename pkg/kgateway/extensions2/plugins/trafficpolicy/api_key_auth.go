package trafficpolicy

import (
	"errors"
	"fmt"
	"slices"
	"strings"

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

	APIKeyAuthEnabledFilterName = "api_key_auth_enabled" //nolint:gosec // G101: Potential hardcoded credentials
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

// parsedAPIKey is one credential paired with the Secret it came from, so that duplicate
// reporting can name the source without echoing the key value.
type parsedAPIKey struct {
	client string
	key    string
	// secret is the "namespace/name" of the Secret the credential was read from.
	secret string
}

// constructAPIKeyAuth translates the API key authentication spec into an Envoy API key auth per-route configuration
func constructAPIKeyAuth(
	krtctx krt.HandlerContext,
	policy *kgateway.TrafficPolicy,
	commoncol *collections.CommonCollections,
	out *trafficPolicySpecIr,
) error {
	spec := policy.Spec
	if spec.APIKeyAuth == nil {
		return nil
	}

	ak := spec.APIKeyAuth

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
		return errors.New("either secretRef or secretSelector must be specified")
	}

	// Parse secrets into credentials, then de-duplicate before emitting.
	var parsed []parsedAPIKey
	for _, secret := range secrets {
		for keyName, keyValue := range secret.Data {
			// Skip empty values
			if len(keyValue) == 0 {
				continue
			}

			// The value is expected to be a plain string representing the API key
			// The secret key name becomes the client identifier
			parsed = append(parsed, parsedAPIKey{
				client: keyName,
				key:    string(keyValue),
				secret: secret.ObjectSource.Namespace + "/" + secret.ObjectSource.Name,
			})
		}
	}

	credentials, errs := dedupeAPIKeyCredentials(parsed, policy.Namespace+"/"+policy.Name)
	if len(errs) > 0 {
		return fmt.Errorf("errors processing API key secrets: %w", errors.Join(errs...))
	}

	if len(credentials) == 0 {
		return errors.New("no valid API keys found in secrets")
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

// dedupeAPIKeyCredentials orders the parsed credentials and collapses duplicate key values.
//
// Envoy requires credential keys to be unique within a single ApiKeyAuth config
// (source/extensions/filters/http/api_key_auth/api_key_auth.h) and NACKs the RouteConfiguration
// that carries a repeated one. Because ApiKeyAuthPerRoute rides in typed_per_filter_config, that
// rejection takes the whole RouteConfiguration with it and freezes config delivery for the entire
// listener, while the policy and the route both keep reporting Accepted. The uniqueness rule is
// knowable here, so never emit a list that violates it.
//
// Two duplicates are not the same mistake:
//   - The same client with the same key is the same credential listed twice, which is what a Secret
//     copied to a second namespace looks like once both copies are selected. Dropping the repeat is
//     a no-op, so do it quietly.
//   - Two clients sharing a key value is ambiguous. The client name is forwarded as the client id
//     header and recorded in auth metadata, so keeping one of them silently would be picking an
//     identity on an authn path. Report it: the caller turns that into Accepted=False on the policy
//     and a 500 on the affected route, which is a smaller blast radius than a frozen listener.
func dedupeAPIKeyCredentials(parsed []parsedAPIKey, policyRef string) ([]*envoyapikeyauthv3.Credential, []error) {
	// Secret.Data is a map and neither GetSecretsBySelector nor krt.Fetch define an order, so sort
	// before emitting. An unstable credential order makes apiKeyAuthIR.Equals report a change on
	// every recompute, and it would make which duplicate survives, and which one is named in the
	// error below, vary between translations of identical input.
	slices.SortFunc(parsed, func(a, b parsedAPIKey) int {
		if c := strings.Compare(a.client, b.client); c != 0 {
			return c
		}
		if c := strings.Compare(a.key, b.key); c != 0 {
			return c
		}
		return strings.Compare(a.secret, b.secret)
	})

	credentials := make([]*envoyapikeyauthv3.Credential, 0, len(parsed))
	seen := make(map[string]parsedAPIKey, len(parsed))
	var errs []error
	for _, c := range parsed {
		prev, dup := seen[c.key]
		if !dup {
			seen[c.key] = c
			credentials = append(credentials, &envoyapikeyauthv3.Credential{
				Key:    c.key,
				Client: c.client,
			})
			continue
		}

		if prev.client == c.client {
			logger.Debug("dropping duplicate api key credential",
				"policy", policyRef,
				"client", c.client,
				"secret", c.secret,
				"duplicate_of", prev.secret,
			)
			continue
		}

		// This message reaches policy status, so it must never contain the key value itself.
		errs = append(errs, fmt.Errorf(
			"duplicate API key value shared by client %q in secret %s and client %q in secret %s",
			prev.client, prev.secret, c.client, c.secret))
	}

	return credentials, errs
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

		// Explicitly set the APIKeyAuthEnabledFilterName to a blank transformation.
		// This ensures that the metadata is not set if auth is not configured on the route
		AddBlankTransformationIfNeeded(pCtxTypedFilterConfig, APIKeyAuthEnabledFilterName, p.enableAuthMetadata)
		return
	}

	if apiKeyAuthIr.config == nil {
		return
	}

	// Adds the ApiKeyAuthPerRoute to the typed_per_filter_config.
	// Also requires API key auth http_filter to be added to the filter chain.
	pCtxTypedFilterConfig.AddTypedConfig(apiKeyAuthFilterNamePrefix, apiKeyAuthIr.config)

	// Set the AuthSucceeded metadata field to indicate that the request has successfully been authed
	AddAuthMetadataIfNeeded(pCtxTypedFilterConfig, APIKeyAuthEnabledFilterName, p.enableAuthMetadata)

	// Add a filter to the chain. When having an api key auth policy for a route we need to also have a
	// globally api key auth http filter in the chain otherwise it will be ignored.
	if p.apiKeyAuthInChain == nil {
		p.apiKeyAuthInChain = make(map[string]*envoyapikeyauthv3.ApiKeyAuth)
	}
	if _, ok := p.apiKeyAuthInChain[fcn]; !ok {
		p.apiKeyAuthInChain[fcn] = &envoyapikeyauthv3.ApiKeyAuth{}
	}
}
