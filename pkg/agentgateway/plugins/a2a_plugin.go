package plugins

import (
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
)

const (
	a2aProtocol   = "kgateway.dev/a2a"
	a2aPluginName = "a2a-policy-plugin"
)

// A2APlugin converts an a2a annotated service to an agentgateway a2a policy
type A2APlugin struct{}

// NewA2APlugin creates a new A2A policy plugin
func NewA2APlugin() *A2APlugin {
	return &A2APlugin{}
}

// GroupKind returns the GroupKind of the policy this plugin handles
func (p *A2APlugin) GroupKind() schema.GroupKind {
	return schema.GroupKind{
		Group: wellknown.ServiceGVK.GroupKind().Group,
		Kind:  wellknown.ServiceGVK.GroupKind().Kind,
	}
}

// Name returns the name of this plugin
func (p *A2APlugin) Name() string {
	return a2aPluginName
}

// GeneratePolicies generates ADP policies for services with a2a protocol
func (p *A2APlugin) GeneratePolicies(ctx krt.HandlerContext, agw *AgwCollections) ([]ADPPolicy, error) {
	logger := logging.New("agentgateway/plugins/a2a")

	services := agw.Services
	if services == nil {
		logger.Warn("services collection is nil, skipping A2A policy generation")
		return nil, nil
	}

	return p.GenerateA2APolicies(ctx, services)
}

// GenerateA2APolicies generates A2A policies for services with a2a protocol
func (p *A2APlugin) GenerateA2APolicies(ctx krt.HandlerContext, services krt.Collection[*corev1.Service]) ([]ADPPolicy, error) {
	logger := logging.New("agentgateway/plugins/a2a")
	logger.Debug("generating A2A policies")

	var a2aPolicies []ADPPolicy

	// Fetch all services and process them
	allServices := krt.Fetch(ctx, services)

	for _, svc := range allServices {
		policies := p.generatePoliciesForService(svc)
		a2aPolicies = append(a2aPolicies, policies...)
	}

	logger.Info("generated A2A policies", "count", len(a2aPolicies))
	return a2aPolicies, nil
}

// generatePoliciesForService generates A2A policies for a single service
func (p *A2APlugin) generatePoliciesForService(svc *corev1.Service) []ADPPolicy {
	logger := logging.New("agentgateway/plugins/a2a")
	var a2aPolicies []ADPPolicy

	for _, port := range svc.Spec.Ports {
		if port.AppProtocol != nil && *port.AppProtocol == a2aProtocol {
			logger.Debug("found A2A service", "service", svc.Name, "namespace", svc.Namespace, "port", port.Port)

			svcRef := fmt.Sprintf("%v/%v", svc.Namespace, svc.Name)
			policy := &api.Policy{
				Name:   fmt.Sprintf("a2a/%s/%s/%d", svc.Namespace, svc.Name, port.Port),
				Target: &api.PolicyTarget{Kind: &api.PolicyTarget_Backend{Backend: svcRef}},
				Spec: &api.PolicySpec{Kind: &api.PolicySpec_A2A_{
					A2A: &api.PolicySpec_A2A{},
				}},
			}

			a2aPolicies = append(a2aPolicies, ADPPolicy{Policy: policy})
		}
	}

	return a2aPolicies
}

// Verify that A2APlugin implements the required interfaces
var _ PolicyPlugin = (*A2APlugin)(nil)
