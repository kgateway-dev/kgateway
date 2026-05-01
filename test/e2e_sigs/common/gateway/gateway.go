//go:build e2e

package gateway

import (
	"context"
	"fmt"

	"sigs.k8s.io/e2e-framework/pkg/envconf"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// GetAddress returns the first address from the status of the Gateway with the
// given name and namespace.
func GetAddress(ctx context.Context, cfg *envconf.Config, name, namespace string) (string, error) {
	gw := &gwv1.Gateway{}
	if err := cfg.Client().Resources().Get(ctx, name, namespace, gw); err != nil {
		return "", err
	}

	if len(gw.Status.Addresses) == 0 {
		return "", fmt.Errorf("gateway has no addresses in status")
	}

	address := gw.Status.Addresses[0].Value
	if address == "" {
		return "", fmt.Errorf("gateway address is empty")
	}

	return address, nil
}
