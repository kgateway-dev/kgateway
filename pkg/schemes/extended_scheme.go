package schemes

import (
	"context"
	"fmt"

	istiokube "istio.io/istio/pkg/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/solo-io/gloo/projects/gateway2/crds"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
)

// AddGatewayV1A2Scheme adds the gwv1a2 scheme to the provided scheme if the TCPRoute CRD exists.
func AddGatewayV1A2Scheme(ctx context.Context, cli istiokube.Client, scheme *runtime.Scheme) error {
	exists, err := crdExists(ctx, cli, crds.TCPRoute)
	if err != nil {
		return fmt.Errorf("error checking if %s CRD exists: %w", wellknown.TCPRouteKind, err)
	}

	if exists {
		if err := gwv1a2.Install(scheme); err != nil {
			return fmt.Errorf("error adding Gateway API v1alpha2 to scheme: %w", err)
		}
	}

	return nil
}

// crdExists queries the Kubernetes API for the provided CRD name.
func crdExists(ctx context.Context, cli istiokube.Client, name string) (bool, error) {
	_, err := cli.Dynamic().Resource(crds.GVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	return true, nil
}
