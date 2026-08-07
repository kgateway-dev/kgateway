//go:build e2e

// Package localstack resolves the endpoint of the localstack service that
// e2e clusters expose for AWS API emulation.
package localstack

import (
	"context"
	"fmt"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// serviceNamespace is the namespace the localstack service is deployed to.
	serviceNamespace = "localstack"
	// serviceName is the name of the localstack service.
	serviceName = "localstack"
)

// EndpointURL resolves the localstack NodePort endpoint from the cluster as
// http://<node-internal-ip>:<nodePort>. It returns found=false (and no error)
// when the localstack service does not exist, so callers can treat localstack
// as optional.
func EndpointURL(ctx context.Context, c client.Client) (endpoint string, found bool, err error) {
	svc := &corev1.Service{}
	err = c.Get(ctx, client.ObjectKey{Namespace: serviceNamespace, Name: serviceName}, svc)
	if apierrors.IsNotFound(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("failed to get localstack service: %w", err)
	}
	if len(svc.Spec.Ports) == 0 || svc.Spec.Ports[0].NodePort == 0 {
		return "", false, fmt.Errorf("localstack service is missing a node port")
	}

	var nodes corev1.NodeList
	if err := c.List(ctx, &nodes); err != nil {
		return "", false, fmt.Errorf("failed to list cluster nodes: %w", err)
	}
	if len(nodes.Items) == 0 {
		return "", false, fmt.Errorf("cluster must have at least one node")
	}

	var nodeIP string
	for _, node := range nodes.Items {
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				nodeIP = addr.Address
				break
			}
		}
		if nodeIP != "" {
			break
		}
	}
	if nodeIP == "" {
		return "", false, fmt.Errorf("failed to determine localstack node internal IP")
	}

	parsed, err := url.Parse(fmt.Sprintf("http://%s:%d", nodeIP, svc.Spec.Ports[0].NodePort))
	if err != nil {
		return "", false, fmt.Errorf("failed to parse localstack URL: %w", err)
	}
	return parsed.String(), true, nil
}
