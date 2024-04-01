package setup

import (
	"context"
	"fmt"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
)

var (
	NoXdsPortFoundError = func(portName string, svcNamespace string, svcName string) error {
		return eris.Errorf("no port with the name %s found in service %s.%s", portName, svcNamespace, svcName)
	}
)

// GetControlPlaneXdsPort gets the xDS port from the gloo Service.
func GetControlPlaneXdsPort(ctx context.Context, svcClient skkube.ServiceClient) (int32, error) {
	// this is the namespace where gloo is running
	svcNamespace := utils.GetPodNamespace()
	glooSvc, err := svcClient.Read(svcNamespace, kubeutils.GlooServiceName, clients.ReadOpts{Ctx: ctx})
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

// GetControlPlaneXdsHost gets the xDS address from the gloo Service.
func GetControlPlaneXdsHost() string {
	return fmt.Sprintf("%s.%s.svc.%s", kubeutils.GlooServiceName, utils.GetPodNamespace(), "cluster.local")
}
