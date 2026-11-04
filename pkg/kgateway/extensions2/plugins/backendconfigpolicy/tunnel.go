package backendconfigpolicy

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	sharedv1alpha1 "github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/extensions2/pluginutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/cmputils"
)

// redactedValue replaces inline header values in KRT diagnostics.
const redactedValue = "[REDACTED]"

// tunnelHeaderValuePattern is the CRD pattern for inline header values. Secret
// values skip admission, so they are checked against it here.
var tunnelHeaderValuePattern = regexp.MustCompile(`^[!-~]+([\t ]?[!-~]+)*$`)

// tunnelIR holds the effective proxy reference and resolved CONNECT headers,
// whose values are served over SDS.
type tunnelIR struct {
	// Resolve references and attribute errors using the owning policy.
	source  ir.ObjectSource
	proxy   gwv1.BackendObjectReference
	headers []gwv1.HTTPHeader
}

// Equals compares tunnel configuration and its owning policy.
func (t *tunnelIR) Equals(other *tunnelIR) bool {
	if t == nil || other == nil {
		return t == other
	}
	return t.source == other.source &&
		t.proxy.Name == other.proxy.Name &&
		cmputils.PointerValsEqual(t.proxy.Group, other.proxy.Group) &&
		cmputils.PointerValsEqual(t.proxy.Kind, other.proxy.Kind) &&
		cmputils.PointerValsEqual(t.proxy.Namespace, other.proxy.Namespace) &&
		cmputils.PointerValsEqual(t.proxy.Port, other.proxy.Port) &&
		slices.Equal(t.headers, other.headers)
}

// translateTunnel resolves CONNECT headers into IR, retaining the IR on error
// so invalid tunnels are not treated as absent.
func translateTunnel(
	krtctx krt.HandlerContext,
	secrets *krtcollections.SecretIndex,
	namespace, name string,
	spec *kgateway.Tunnel,
) (*tunnelIR, error) {
	gk := wellknown.BackendConfigPolicyGVK.GroupKind()
	out := &tunnelIR{
		source: ir.ObjectSource{Group: gk.Group, Kind: gk.Kind, Namespace: namespace, Name: name},
		proxy:  spec.Proxy.BackendRef,
	}
	from := krtcollections.From{GroupKind: gk, Namespace: namespace}
	resolved, err := pluginutils.ConvertHeaderFilter(krtctx, from, secrets, &sharedv1alpha1.HTTPHeaderFilter{Set: spec.Headers})
	if err == nil && resolved != nil {
		out.headers = resolved.Set
		err = normalizeTunnelHeaders(out.headers)
	}
	if err != nil {
		return out, fmt.Errorf("tunnel headers: %w", err)
	}
	return out, nil
}

// normalizeTunnelHeaders trims and validates CONNECT header values without
// exposing them in errors. Envoy inserts SDS values verbatim, so this is the
// only check a Secret value gets.
func normalizeTunnelHeaders(headers []gwv1.HTTPHeader) error {
	for i, h := range headers {
		// Secret files often end with a newline.
		value := strings.TrimSpace(h.Value)
		switch {
		case value == "":
			return fmt.Errorf("header %s has an empty value", h.Name)
		case !tunnelHeaderValuePattern.MatchString(value):
			return fmt.Errorf("header %s value must be printable ASCII, with single spaces or tabs between words", h.Name)
		}
		headers[i].Value = value
	}
	return nil
}

// redactPolicyCredentials redacts inline headers and stale apply annotations
// without mutating the policy.
func redactPolicyCredentials(pol *kgateway.BackendConfigPolicy) *kgateway.BackendConfigPolicy {
	out := pol
	if _, ok := pol.Annotations[corev1.LastAppliedConfigAnnotation]; ok {
		out = pol.DeepCopy()
		delete(out.Annotations, corev1.LastAppliedConfigAnnotation)
	}
	if pol.Spec.Tunnel == nil {
		return out
	}
	for i, h := range pol.Spec.Tunnel.Headers {
		if h.Value == nil {
			continue
		}
		if out == pol {
			out = pol.DeepCopy()
		}
		out.Spec.Tunnel.Headers[i].Value = new(redactedValue)
	}
	return out
}
