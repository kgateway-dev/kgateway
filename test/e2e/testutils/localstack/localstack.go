//go:build e2e

// Package localstack resolves the endpoint of the localstack service that
// e2e clusters expose for AWS API emulation.
package localstack

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
)

const (
	// serviceNamespace is the namespace the localstack service is deployed to.
	serviceNamespace = "localstack"
	// serviceName is the name of the localstack service.
	serviceName = "localstack"
	// podLabel selects the localstack pod.
	podLabel = "app.kubernetes.io/name=localstack"
	// containerName is the localstack container within that pod.
	containerName = "localstack"
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
		return "", false, errors.New("localstack service is missing a node port")
	}

	var nodes corev1.NodeList
	if err := c.List(ctx, &nodes); err != nil {
		return "", false, fmt.Errorf("failed to list cluster nodes: %w", err)
	}
	if len(nodes.Items) == 0 {
		return "", false, errors.New("cluster must have at least one node")
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
		return "", false, errors.New("failed to determine localstack node internal IP")
	}

	parsed, err := url.Parse(fmt.Sprintf("http://%s:%d", nodeIP, svc.Spec.Ports[0].NodePort))
	if err != nil {
		return "", false, fmt.Errorf("failed to parse localstack URL: %w", err)
	}
	return parsed.String(), true, nil
}

// RequestCount returns how many times localstack has served the given AWS API
// action (e.g. "sts.AssumeRole") with the given HTTP status, according to its
// request log, which records one line per call:
//
//	... localstack.request.aws : AWS sts.AssumeRole => 200
//
// This is the only per-action signal available: Envoy funnels every STS call of
// a region through one internal cluster, so its cluster and credential-provider
// stats cannot distinguish sts:AssumeRole from sts:AssumeRoleWithWebIdentity.
func RequestCount(ctx context.Context, cli *kubectl.Cli, action string, status int) (int, error) {
	pods, err := cli.GetPodsInNsWithLabel(ctx, serviceNamespace, podLabel)
	if err != nil {
		return 0, fmt.Errorf("failed to list localstack pods: %w", err)
	}
	if len(pods) == 0 {
		return 0, errors.New("no localstack pod found")
	}
	pattern := regexp.MustCompile(fmt.Sprintf(`AWS %s => %d\b`, regexp.QuoteMeta(action), status))
	count := 0
	for _, pod := range pods {
		logs, err := cli.GetContainerLogs(ctx, serviceNamespace, pod, kubectl.WithContainer(containerName))
		if err != nil {
			return 0, fmt.Errorf("failed to read logs of localstack pod %s: %w", pod, err)
		}
		count += len(pattern.FindAllStringIndex(logs, -1))
	}
	return count, nil
}
