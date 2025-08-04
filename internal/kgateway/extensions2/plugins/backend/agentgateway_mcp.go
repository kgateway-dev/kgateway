package backend

import (
	"fmt"

	"github.com/agentgateway/agentgateway/go/api"
	"istio.io/istio/pkg/kube/krt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
)

const (
	mcpProtocol = "kgateway.dev/mcp"
)

func processMCPBackendForAgentGateway(ctx krt.HandlerContext, nsCol krt.Collection[*corev1.Namespace], svcCol krt.Collection[*corev1.Service], be *v1alpha1.Backend) ([]*api.Backend, []*api.Policy, error) {
	// Convert Kubernetes MCP targets to agentgateway format
	var mcpTargets []*api.MCPTarget
	var backends []*api.Backend

	if be.Spec.MCP != nil {
		for _, targetSelector := range be.Spec.MCP.Targets {
			if targetSelector.StaticTarget != nil {
				staticBackendRef := be.Namespace + "/" + targetSelector.StaticTarget.Name
				// TODO: check if the backend exists first to dedup
				staticBackend := &api.Backend{
					Name: staticBackendRef,
					Kind: &api.Backend_Static{
						Static: &api.StaticBackend{
							Host: targetSelector.StaticTarget.Host,
							Port: targetSelector.StaticTarget.Port,
						},
					},
				}
				backends = append(backends, staticBackend)

				mcpTarget := &api.MCPTarget{
					Name: targetSelector.StaticTarget.Name,
					Backend: &api.BackendReference{
						Kind: &api.BackendReference_Backend{
							Backend: staticBackendRef,
						},
						Port: uint32(targetSelector.StaticTarget.Port),
					},
				}

				// Convert protocol if specified
				switch targetSelector.StaticTarget.Protocol {
				case v1alpha1.MCPProtocolSSE:
					mcpTarget.Protocol = api.MCPTarget_SSE
				case v1alpha1.MCPProtocolStreamableHTTP:
					mcpTarget.Protocol = api.MCPTarget_STREAMABLE_HTTP
				default:
					mcpTarget.Protocol = api.MCPTarget_UNDEFINED
				}

				mcpTargets = append(mcpTargets, mcpTarget)
			}

			if targetSelector.Selectors != nil {
				// Filter services based on service selector and namespace selector
				var filters []krt.FetchOption

				// Apply service label selector
				if targetSelector.Selectors.ServiceSelector != nil {
					// Convert metav1.LabelSelector to labels.Selector
					serviceSelector, err := metav1.LabelSelectorAsSelector(targetSelector.Selectors.ServiceSelector)
					if err != nil {
						logger.Warn("invalid service selector", "error", err, "backend", be.Name)
						continue
					}
					// Use the selector to filter services with matching labels
					if !serviceSelector.Empty() {
						filters = append(filters, krt.FilterGeneric(func(svc any) bool {
							service := svc.(*corev1.Service)
							return serviceSelector.Matches(labels.Set(service.Labels))
						}))
					}
				}

				// Apply namespace selector if specified
				if targetSelector.Selectors.NamespaceSelector != nil {
					namespaceSelector, err := metav1.LabelSelectorAsSelector(targetSelector.Selectors.NamespaceSelector)
					if err != nil {
						logger.Warn("invalid namespace selector", "error", err, "backend", be.Name)
						continue
					}
					if !namespaceSelector.Empty() {
						// Get all namespaces and find those matching the selector
						allNamespaces := krt.Fetch(ctx, nsCol)
						matchingNamespaces := make(map[string]bool)
						for _, ns := range allNamespaces {
							if namespaceSelector.Matches(labels.Set(ns.Labels)) {
								matchingNamespaces[ns.Name] = true
							}
						}
						// Filter services to only those in matching namespaces
						filters = append(filters, krt.FilterGeneric(func(svc any) bool {
							service := svc.(*corev1.Service)
							return matchingNamespaces[service.Namespace]
						}))
					}
				} else {
					// If no namespace selector, limit to same namespace as backend
					filters = append(filters, krt.FilterGeneric(func(svc any) bool {
						service := svc.(*corev1.Service)
						return service.Namespace == be.Namespace
					}))
				}

				// Fetch matching services
				matchingServices := krt.Fetch(ctx, svcCol, filters...)

				// Create MCP targets for each matching service
				for _, service := range matchingServices {
					for _, port := range service.Spec.Ports {
						if port.AppProtocol == nil || *port.AppProtocol != mcpProtocol {
							continue
						}
						targetName := service.Name + fmt.Sprintf("-%d", port.Port)
						if port.Name != "" {
							targetName = service.Name + "-" + port.Name
						}
						// For each mcp port on the service, create an MCP target
						mcpTarget := &api.MCPTarget{
							Name: targetName,
							Backend: &api.BackendReference{
								Kind: &api.BackendReference_Service{Service: service.Namespace + "/" + service.Name},
								Port: uint32(port.Port),
							},
						}

						// TODO: Determine protocol from service annotations or other metadata
						// For now, default to undefined protocol
						mcpTarget.Protocol = api.MCPTarget_UNDEFINED

						mcpTargets = append(mcpTargets, mcpTarget)
					}
				}
			}
		}
	}

	mcpBackend := &api.Backend{
		Name: be.Namespace + "/" + be.Name,
		Kind: &api.Backend_Mcp{
			Mcp: &api.MCPBackend{
				Targets: mcpTargets,
			},
		},
	}
	backends = append(backends, mcpBackend)
	// TODO: add support for backend auth policy for mcp
	return backends, nil, nil
}
