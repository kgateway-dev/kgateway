package deployer

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Deployer uses HelmValueGenerator implementation to generate a set of helm values
// when rendering a Helm chart
type HelmValuesGenerator interface {
	// GetValues returns the helm values used to render the dynamically provisioned resources
	// for the given object (e.g. Gateway). If the values returned are nil, it indicates that
	// the object is self-managed and no resources should be provisioned.
	GetValues(ctx context.Context, obj client.Object) (map[string]any, error)

	// IsSelfManaged returns true if the object is self-managed (i.e. no resources should be
	// provisioned dynamically).
	IsSelfManaged(ctx context.Context, obj client.Object) (bool, error)
}
