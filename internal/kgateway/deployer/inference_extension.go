package deployer

import (
	"context"
	"fmt"

	"helm.sh/helm/v3/pkg/chart"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	infextv1a2 "sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/helm"
	"github.com/kgateway-dev/kgateway/v2/pkg/deployer"
)

type InferenceExtension struct{}

func LoadInferencePoolChart() (*chart.Chart, error) {
	return loadChart(helm.InferenceExtensionHelmChart)
}

func InferencePoolGVKsToWatch(ctx context.Context, d *deployer.Deployer) ([]schema.GroupVersionKind, error) {
	return d.GetGvksToWatch(ctx, map[string]any{
		"inferenceExtension": map[string]any{
			"endpointPicker": map[string]any{},
		},
	})
}

func (ie *InferenceExtension) GetValues(_ context.Context, obj client.Object) (map[string]any, error) {
	if obj == nil {
		return nil, fmt.Errorf("inference pool is not defined for deployer")
	}
	pool, ok := obj.(*infextv1a2.InferencePool)
	if !ok {
		return nil, fmt.Errorf("client.Object that is not an inference pool has been passed in")
	}

	// construct the default values
	vals := &deployer.HelmConfig{
		InferenceExtension: &deployer.HelmInferenceExtension{
			EndpointPicker: &deployer.HelmEndpointPickerExtension{
				PoolName:      pool.Name,
				PoolNamespace: pool.Namespace,
			},
		},
	}

	var convertedVals map[string]any
	err := deployer.JsonConvert(vals, &convertedVals)
	if err != nil {
		return nil, fmt.Errorf("failed to convert inference extension helm values: %w", err)
	}

	return convertedVals, nil
}

// Use a unique release name for the endpoint picker child objects.
func InferenceExtensionReleaseNameAndNamespace(obj client.Object) (string, string) {
	return fmt.Sprintf("%s-endpoint-picker", obj.GetName()), obj.GetNamespace()
}
