package deployer

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Deployer uses HelmValueGenerator implementation to generate a set of helm values
// when rendering a Helm chart
type HelmValuesGenerator interface {
	GetValues(ctx context.Context, obj client.Object) (map[string]any, error)
}
