package plugins

import (
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

const (
	a2aProtocol = "kgateway.dev/a2a"
)

// NewA2APlugin creates a new A2A policy plugin
func NewA2APlugin(agw *AgwCollections) AgwPlugin {
	policyCol := krt.NewManyCollection(agw.Services, func(krtctx krt.HandlerContext, svc *corev1.Service) []AgwPolicy {
		return translatePoliciesForService(svc)
	})
	return AgwPlugin{
		ContributesPolicies: map[schema.GroupKind]PolicyPlugin{
			wellknown.ServiceGVK.GroupKind(): {
				Policies: policyCol,
			},
		},
		ExtraHasSynced: func() bool {
			return policyCol.HasSynced()
		},
	}
}

// translatePoliciesForService generates A2A policies for a single service
func translatePoliciesForService(svc *corev1.Service) []AgwPolicy {
	var a2aPolicies []AgwPolicy

	for _, port := range svc.Spec.Ports {
		if port.AppProtocol != nil && *port.AppProtocol == a2aProtocol {
			logger.Debug("found A2A service", "service", svc.Name, "namespace", svc.Namespace, "port", port.Port)

			svcRef := fmt.Sprintf("%v/%v:%d", svc.Namespace, svc.Name, port.Port)
			policy := &api.Policy{
				Name:   fmt.Sprintf("a2a/%s/%s/%d", svc.Namespace, svc.Name, port.Port),
				Target: &api.PolicyTarget{Kind: &api.PolicyTarget_Backend{Backend: svcRef}},
				Spec: &api.PolicySpec{Kind: &api.PolicySpec_A2A_{
					A2A: &api.PolicySpec_A2A{},
				}},
			}

			a2aPolicies = append(a2aPolicies, AgwPolicy{Policy: policy})
		}
	}

	return a2aPolicies
}
