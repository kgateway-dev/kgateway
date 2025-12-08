package trafficpolicy

import (
	"fmt"
	"strings"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyroutev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoy_basic_auth_v3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/basic_auth/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/util/sets"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	basicAuthFilterName = "envoy.filters.http.basic_auth"
	defaultSecretKey    = ".htpasswd"
	shaPrefix           = "{SHA}"
)

type basicAuthIR struct {
	policy  *envoy_basic_auth_v3.BasicAuthPerRoute
	disable bool
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
	if b.disable != otherBasicAuth.disable {
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
	if basicAuth.disable {
		pCtxTypedFilterConfig.AddTypedConfig(basicAuthFilterName, &envoyroutev3.FilterConfig{Config: &anypb.Any{}, Disabled: true})
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
	in *kgateway.TrafficPolicy,
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
			disable: true,
		}
		return nil
	}

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

	// Validate and filter users to only include SHA hashed passwords
	validUsers, invalidUsers := validateAndFilterSHAUsers(htpasswdData)

	// If there are no valid users after filtering, return an error
	if len(validUsers) == 0 {
		return fmt.Errorf("basic auth: no valid users with {SHA} hash format found")
	}

	// Report invalid users if any were found
	if len(invalidUsers) > 0 {
		err = fmt.Errorf("basic auth: dropped %d user(s) with invalid hash format (only {SHA} is supported) or duplicate usernames.", len(invalidUsers))
	}

	allUsers := strings.Join(validUsers, "\n")
	if len(allUsers) == 0 {
		allUsers = "#"
	}

	// Build the basic auth configuration
	out.basicAuth = &basicAuthIR{
		policy: &envoy_basic_auth_v3.BasicAuthPerRoute{
			// Set the users data source with validated users
			Users: &envoycorev3.DataSource{
				Specifier: &envoycorev3.DataSource_InlineString{
					InlineString: allUsers,
				},
			},
		},
	}

	return err
}

// fetchHtpasswdFromSecret retrieves htpasswd data from a Kubernetes secret
func fetchHtpasswdFromSecret(
	krtctx krt.HandlerContext,
	secrets *krtcollections.SecretIndex,
	secretRef *kgateway.SecretReference,
	policyNamespace string,
) (string, error) {
	// Determine namespace - use secret's namespace if specified, otherwise policy's namespace
	namespace := gwv1.Namespace(policyNamespace)
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
		Name:      secretRef.Name,
		Namespace: &namespace,
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

// validateAndFilterSHAUsers validates htpasswd entries and filters out users with non-SHA hash formats.
// Returns a slice of valid users, a slice of invalid usernames, and an error if validation fails.
// Envoy only supports {SHA} hash format for basic auth.
func validateAndFilterSHAUsers(htpasswdData string) (validUsers []string, invalidUsernames []string) {
	lines := strings.Split(htpasswdData, "\n")
	validUsers = make([]string, 0, len(lines))
	validUsernames := sets.New[string]()
	invalidUsernames = make([]string, 0)

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// htpasswd format is "username:password_hash"
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			// Don't log the entire line to avoid leaking sensitive info
			logger.Warn("malformed htpasswd entry, missing colon", "line", i+1)
			invalidUsernames = append(invalidUsernames, line)
			continue
		}

		username := parts[0]
		passwordHash := parts[1]

		// 5=len("{SHA}"), 28=SHA1 base64 length. these validations are copied from envoy source code.
		validHash := strings.HasPrefix(passwordHash, shaPrefix) && len(passwordHash) == (28+5)
		isDuplicate := validUsernames.Has(username)

		// Check if the password hash uses {SHA} format
		if validHash && !isDuplicate {
			validUsers = append(validUsers, line)
			validUsernames.Insert(username)
		} else {
			logger.Warn("invalid basic auth user", "user", username, "isDuplicate", isDuplicate, "validHash", validHash)
			invalidUsernames = append(invalidUsernames, username)
		}
	}

	return validUsers, invalidUsernames
}
