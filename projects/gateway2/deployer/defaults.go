package deployer

import (
	"context"
	"fmt"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// This file contains helper functions that get values that can be derived from the running Gloo instance.
// These values are used by the deployer to render the dynamically-generated proxy resources (deployment, etc)

var (
	NoXdsPortFoundError = func(portName string, svcNamespace string, svcName string) error {
		return eris.Errorf("no port with the name %s found in service %s.%s", portName, svcNamespace, svcName)
	}
)

// GetDefaultXdsPort gets the xDS port from the gloo Service.
func GetDefaultXdsPort(ctx context.Context, cli client.Client) (int32, error) {
	glooSvc := &corev1.Service{}
	// this is the namespace where gloo is running
	svcNamespace := utils.GetPodNamespace()
	err := cli.Get(ctx, client.ObjectKey{Namespace: svcNamespace, Name: kubeutils.GlooServiceName}, glooSvc)
	if err != nil {
		return 0, err
	}

	// find the xds port on the gloo service
	for _, port := range glooSvc.Spec.Ports {
		if port.Name == kubeutils.GlooXdsPortName {
			return port.Port, nil
		}
	}
	return 0, NoXdsPortFoundError(kubeutils.GlooXdsPortName, svcNamespace, kubeutils.GlooServiceName)
}

// GetDefaultXdsHost gets the xDS address from the gloo Service.
func GetDefaultXdsHost() string {
	return fmt.Sprintf("%s.%s.svc.%s", kubeutils.GlooServiceName, utils.GetPodNamespace(), "cluster.local")
}
