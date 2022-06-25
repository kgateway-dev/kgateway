package translator

import v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"

// Predicate is used to determine how to reduce a list of Gateways
type Predicate interface {
	// ReadGateway returns true if a Gateway should be processed during translation, false otherwise
	ReadGateway(gw *v1.Gateway) bool
}

var (
	_ Predicate = new(SingleNamespacePredicate)
	_ Predicate = new(AllNamespacesPredicate)
)

// FilterGateways filters a GatewayList based on a Predicate, and returns a new list
func FilterGateways(gateways v1.GatewayList, predicate Predicate) v1.GatewayList {
	var filteredGateways v1.GatewayList
	for _, gateway := range gateways {
		if predicate.ReadGateway(gateway) {
			filteredGateways = append(filteredGateways, gateway)
		}
	}
	return filteredGateways
}

// GetPredicate returns a Predicate for determining which Gateways to process
func GetPredicate(writeNamespace string, readGatewaysFromAllNamespaces bool) Predicate {
	if readGatewaysFromAllNamespaces {
		return &AllNamespacesPredicate{}
	}
	return &SingleNamespacePredicate{
		namespace: writeNamespace,
	}
}

// SingleNamespacePredicate returns true if a Gateway is in the same namespace as
// the one Gloo controllers are configured to write to.
// When this predicate is used, Gloo will only read gateways from the same namespace that
// it writes to
type SingleNamespacePredicate struct {
	namespace string
}

func (s *SingleNamespacePredicate) ReadGateway(gw *v1.Gateway) bool {
	return gw.GetMetadata().GetNamespace() == s.namespace
}

// AllNamespacesPredicate returns true for all Gateways, independent of their namespace
// When this predicate is used, Gloo will read gateways from all namespaces, not just
// the one it is configured to write to
type AllNamespacesPredicate struct {
}

func (a *AllNamespacesPredicate) ReadGateway(_ *v1.Gateway) bool {
	return true
}
