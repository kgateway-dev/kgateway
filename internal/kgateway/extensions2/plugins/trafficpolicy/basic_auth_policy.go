package trafficpolicy

import (
	"fmt"
	"strings"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_basic_auth_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/basic_auth/v3"
	"google.golang.org/protobuf/proto"
	"istio.io/istio/pkg/kube/krt"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	basicAuthFilterName = "envoy.filters.http.basic_auth"
	defaultSecretKey    = ".htpasswd"
)

type basicAuthIR struct {
	policy     *envoy_basic_auth_v3.BasicAuthPerRoute
	disableAll bool
}

var _ PolicySubIR = &basicAuthIR{}

func (b *basicAuthIR) Equals(other PolicySubIR) bool {
	otherBasicAuth, ok := other.(*basicAuthIR)
	if !ok {
		return false
	}
	if b == nil || otherBasicAuth == nil {
		return b == nil && otherBasicAuth == nil
	}
	if b.disableAll != otherBasicAuth.disableAll {
		return false
	}
	return proto.Equal(b.policy, otherBasicAuth.policy)
}

func (b *basicAuthIR) Validate() error {
	if b == nil || b.policy == nil {
		return nil
	}
	return b.policy.Validate()
}

// handleBasicAuth configures the per-route basic auth configuration and registers the disabled global filter
func (p *trafficPolicyPluginGwPass) handleBasicAuth(
	fcn string,
	pCtxTypedFilterConfig *ir.TypedFilterConfigMap,
	basicAuth *basicAuthIR,
) {
	if basicAuth == nil {
		return
	}

	// Handle disable case - enable the filter with empty config to override parent policy
	if basicAuth.disableAll {
		pCtxTypedFilterConfig.AddTypedConfig(basicAuthFilterName, EnableFilterPerRoute)
		return
	}

	// Add per-route config using BasicAuthPerRoute
	pCtxTypedFilterConfig.AddTypedConfig(basicAuthFilterName, basicAuth.policy)

	// Register the disabled global filter in the chain
	if p.basicAuthInChain == nil {
		p.basicAuthInChain = make(map[string]*envoy_basic_auth_v3.BasicAuth)
	}
	if _, ok := p.basicAuthInChain[fcn]; !ok {
		// Create a disabled filter with empty users - it will be enabled per-route
		p.basicAuthInChain[fcn] = &envoy_basic_auth_v3.BasicAuth{
			Users: &envoycorev3.DataSource{
				Specifier: &envoycorev3.DataSource_InlineString{
					// If the data source is empty, envoy will NACK. so instead we use a comment.
					InlineString: "#",
				},
			},
		}
	}
}

// constructBasicAuth translates the basic auth spec into an envoy basic auth policy
func constructBasicAuth(
	krtctx krt.HandlerContext,
	in *v1alpha1.TrafficPolicy,
	out *trafficPolicySpecIr,
	secrets *krtcollections.SecretIndex,
) error {
	spec := in.Spec.BasicAuth
	if spec == nil {
		return nil
	}

	// Handle disable case
	if spec.Disable != nil {
		out.basicAuth = &basicAuthIR{
			disableAll: true,
		}
		return nil
	}

	// Build the basic auth configuration
	policy := &envoy_basic_auth_v3.BasicAuthPerRoute{}

	// Handle users data source
	var htpasswdData string
	var err error

	if len(spec.Users) > 0 {
		// Inline users - join with newlines to create htpasswd format
		htpasswdData = strings.Join(spec.Users, "\n")
	} else if spec.SecretRef != nil {
		// Fetch from secret
		htpasswdData, err = fetchHtpasswdFromSecret(krtctx, secrets, spec.SecretRef, in.Namespace)
		if err != nil {
			return fmt.Errorf("basic auth: %w", err)
		}
	} else {
		// This shouldn't happen due to CEL validation
		return fmt.Errorf("basic auth: either users or secretRef must be specified")
	}

	// Set the users data source
	policy.Users = &envoycorev3.DataSource{
		Specifier: &envoycorev3.DataSource_InlineString{
			InlineString: htpasswdData,
		},
	}

	out.basicAuth = &basicAuthIR{
		policy: policy,
	}

	return nil
}

// fetchHtpasswdFromSecret retrieves htpasswd data from a Kubernetes secret
func fetchHtpasswdFromSecret(
	krtctx krt.HandlerContext,
	secrets *krtcollections.SecretIndex,
	secretRef *v1alpha1.SecretReference,
	policyNamespace string,
) (string, error) {
	// Determine namespace - use secret's namespace if specified, otherwise policy's namespace
	namespace := policyNamespace
	if secretRef.Namespace != nil {
		namespace = *secretRef.Namespace
	}

	// Determine the key to use
	key := defaultSecretKey
	if secretRef.Key != nil {
		key = *secretRef.Key
	}

	// Build the secret reference
	secretObjRef := gwv1.SecretObjectReference{
		Name:      gwv1.ObjectName(secretRef.Name),
		Namespace: (*gwv1.Namespace)(&namespace),
	}

	// Use TrafficPolicy as the source for reference grants
	from := krtcollections.From{
		GroupKind: wellknown.TrafficPolicyGVK.GroupKind(),
		Namespace: policyNamespace,
	}

	// Fetch the secret
	secret, err := secrets.GetSecret(krtctx, from, secretObjRef)
	if err != nil {
		return "", fmt.Errorf("failed to fetch secret %s/%s: %w", namespace, secretRef.Name, err)
	}

	// Extract the htpasswd data from the secret
	data, exists := secret.Data[key]
	if !exists {
		return "", fmt.Errorf("secret %s/%s does not contain key '%s'", namespace, secretRef.Name, key)
	}

	if len(data) == 0 {
		return "", fmt.Errorf("secret %s/%s key '%s' is empty", namespace, secretRef.Name, key)
	}

	return strings.TrimSpace(string(data)), nil
}
