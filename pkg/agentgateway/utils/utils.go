package utils

import "fmt"

// SingularLLMProviderSubBackendName is the name of the sub-backend for singular LLM providers.
// If the Backend is ns/foo, the sub-backend will be ns/foo/backend
const SingularLLMProviderSubBackendName = "backend"

// InternalGatewayName returns the name of the internal Gateway corresponding to the
// specified gwv1-api gwv1 and listener. If the listener is not specified, returns internal name without listener.
// Format: gwNs/gwName.listener
func InternalGatewayName(gwNamespace, gwName, lName string) string {
	if lName == "" {
		return fmt.Sprintf("%s/%s", gwNamespace, gwName)
	}
	return fmt.Sprintf("%s/%s.%s", gwNamespace, gwName, lName)
}

// InternalRouteRuleName returns the name of the internal Route Rule corresponding to the
// specified route. If ruleName is not specified, returns the internal name without the route rule.
// Format: routeNs/routeName.ruleName
func InternalRouteRuleName(routeNamespace, routeName, ruleName string) string {
	if ruleName == "" {
		return fmt.Sprintf("%s/%s", routeNamespace, routeName)
	}
	return fmt.Sprintf("%s/%s.%s", routeNamespace, routeName, ruleName)
}

// InternalMCPStaticBackendName returns the name of the internal MCP Static Backend corresponding to the
// specified backend and target.
// Format: backendNamespace/backendName/targetName
func InternalMCPStaticBackendName(backendNamespace, backendName, targetName string) string {
	return backendNamespace + "/" + backendName + "/" + targetName
}

// InternalBackendName returns the name of the internal Backend corresponding to the
// specified backend and target.
// Format: backendNamespace/backendName when targetName is empty, otherwise backendNamespace/backendName/targetName
func InternalBackendName(backendNamespace, backendName, targetName string) string {
	name := backendNamespace + "/" + backendName
	if targetName != "" {
		name += "/" + targetName
	}
	return name
}
