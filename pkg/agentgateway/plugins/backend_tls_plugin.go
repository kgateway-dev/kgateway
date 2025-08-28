package plugins

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/agentgateway/agentgateway/go/api"
	"google.golang.org/protobuf/types/known/wrapperspb"
	"istio.io/istio/pkg/kube/krt"
	"istio.io/istio/pkg/ptr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/pkg/agentgateway/utils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

// NewBackendTLSPlugin creates a new BackendTLSPolicy plugin
func NewBackendTLSPlugin(agw *AgwCollections) AgentgatewayPlugin {
	clusterDomain := kubeutils.GetClusterDomainName()
	policyCol := krt.NewManyCollection(agw.BackendTLSPolicies, func(krtctx krt.HandlerContext, btls *gwv1alpha3.BackendTLSPolicy) []ADPPolicy {
		return translatePoliciesForBackendTLS(krtctx, agw.ConfigMaps, btls, clusterDomain)
	})
	return AgentgatewayPlugin{
		ContributesPolicies: map[schema.GroupKind]PolicyPlugin{
			wellknown.BackendTLSPolicyGVK.GroupKind(): {
				Policies: policyCol,
			},
		},
		ExtraHasSynced: func() bool {
			return policyCol.HasSynced()
		},
	}
}

// translatePoliciesForService generates backend TLS policies
func translatePoliciesForBackendTLS(
	krtctx krt.HandlerContext,
	cfgmaps krt.Collection[*corev1.ConfigMap],
	btls *gwv1alpha3.BackendTLSPolicy,
	clusterDomain string,
) []ADPPolicy {
	logger := logger.With("plugin_kind", "backendtls")
	var policies []ADPPolicy

	for idx, target := range btls.Spec.TargetRefs {
		var policyTarget *api.PolicyTarget

		switch string(target.Kind) {
		case wellknown.BackendGVK.Kind:
			// The target defaults to <backend-namespace>/<backend-name>.
			// If SectionName is specified to select a specific target in the Backend,
			// the target becomes <backend-namespace>/<backend-name>/<section-name>
			policyTarget = &api.PolicyTarget{
				Kind: &api.PolicyTarget_Backend{
					Backend: utils.InternalBackendName(btls.Namespace, string(target.Name), string(ptr.OrEmpty(target.SectionName))),
				},
			}
		case wellknown.ServiceKind:
			hostname := fmt.Sprintf("%s.%s.svc.%s", target.Name, btls.Namespace, clusterDomain)
			// If SectionName is specified to select the port, use service/<namespace>/<hostname>:<port>
			if port := ptr.OrEmpty(target.SectionName); port != "" {
				policyTarget = &api.PolicyTarget{
					Kind: &api.PolicyTarget_Backend{
						Backend: fmt.Sprintf("service/%s/%s:%s", btls.Namespace, hostname, port),
					},
				}
			} else {
				// Select the entire service with <namespace>/<hostname>
				policyTarget = &api.PolicyTarget{
					Kind: &api.PolicyTarget_Service{
						Service: fmt.Sprintf("%s/%s", btls.Namespace, hostname),
					},
				}
			}

		default:
			logger.Warn("unsupported target kind", "kind", target.Kind, "policy", btls.Name)
			continue
		}
		caCert, err := getBackendTLSCACert(krtctx, cfgmaps, btls)
		if err != nil {
			logger.Error("error getting backend TLS CA cert", "policy", client.ObjectKeyFromObject(btls), "error", err)
			return nil
		}

		policy := &api.Policy{
			Name:   btls.Namespace + "/" + btls.Name + ":" + strconv.Itoa(idx) + ":backendtls",
			Target: policyTarget,
			Spec: &api.PolicySpec{Kind: &api.PolicySpec_BackendTls{
				BackendTls: &api.PolicySpec_BackendTLS{
					Root: caCert,
					// Used for mTLS, not part of the spec currently
					Cert: nil,
					Key:  nil,
					// Not currently in the spec.
					Insecure: nil,
					// Validation.Hostname is a required value and validated with CEL
					Hostname: wrapperspb.String(string(btls.Spec.Validation.Hostname)),
				},
			}},
		}
		policies = append(policies, ADPPolicy{policy})
	}

	return policies
}

func getBackendTLSCACert(
	krtctx krt.HandlerContext,
	cfgmaps krt.Collection[*corev1.ConfigMap],
	btls *gwv1alpha3.BackendTLSPolicy,
) (*wrapperspb.BytesValue, error) {
	validation := btls.Spec.Validation
	if wk := validation.WellKnownCACertificates; wk != nil {
		switch kind := *wk; kind {
		case gwv1alpha3.WellKnownCACertificatesSystem:
			return nil, nil

		default:
			return nil, fmt.Errorf("unsupported wellKnownCACertificates: %v", kind)
		}
	}

	// One of WellKnownCACertificates or CACertificateRefs will always be specified (CEL validated)
	if len(validation.CACertificateRefs) == 0 {
		// should never happen as this is CEL validated. Only here to prevent panic in tests
		return nil, errors.New("BackendTLSPolicy must specify either wellKnownCACertificates or caCertificateRefs")
	}
	var sb strings.Builder
	for _, ref := range validation.CACertificateRefs {
		if ref.Group != gwv1.Group(wellknown.ConfigMapGVK.Group) || ref.Kind != gwv1.Kind(wellknown.ConfigMapGVK.Kind) {
			return nil, fmt.Errorf("BackendTLSPolicy's validation.caCertificateRefs must be a ConfigMap reference; got %s", ref)
		}
		nn := types.NamespacedName{
			Name:      string(ref.Name),
			Namespace: btls.Namespace,
		}
		cfgmap := krt.FetchOne(krtctx, cfgmaps, krt.FilterObjectName(nn))
		if cfgmap == nil {
			return nil, fmt.Errorf("ConfigMap %s not found", nn)
		}
		caCert, err := extractCARoot(ptr.Flatten(cfgmap))
		if err != nil {
			return nil, fmt.Errorf("error extracting CA cert from ConfigMap %s: %w", nn, err)
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(caCert)
	}
	return wrapperspb.Bytes([]byte(sb.String())), nil
}

func extractCARoot(cm *corev1.ConfigMap) (string, error) {
	caCrt, ok := cm.Data["ca.crt"]
	if !ok {
		return "", errors.New("ca.crt key missing")
	}

	return caCrt, nil
}
