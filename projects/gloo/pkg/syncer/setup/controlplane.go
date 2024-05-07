package setup

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	skkube "github.com/solo-io/solo-kit/pkg/api/v1/resources/common/kubernetes"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	NoXdsPortFoundError = eris.New("failed to find xds port")
	noXdsPortFoundError = func(portName string, svcNamespace string, svcName string) error {
		return eris.Wrapf(NoXdsPortFoundError, "no port with the name %s found in service %s.%s", portName, svcNamespace, svcName)
	}
	NoGlooSvcFoundError = eris.New("failed to find Gloo service")
	noGlooSvcFoundError = func(err error, svcNamespace string, svcName string) error {
		wrapped := eris.Wrap(err, NoGlooSvcFoundError.Error())
		return eris.Wrapf(wrapped, "service %s.%s", svcNamespace, svcName)
	}
)

// GetControlPlaneXdsPort gets the xDS port from the gloo Service.
func GetControlPlaneXdsPort(ctx context.Context, svcClient skkube.ServiceClient) (int32, error) {
	// When this code is invoked from within the running Pod, this will contain the namespace where Gloo is running
	svcNamespace := utils.GetPodNamespace()
	return GetNamespacedControlPlaneXdsPort(ctx, svcNamespace, svcClient)
}

// GetNamespacedControlPlaneXdsPort gets the xDS port from the Gloo Service, provided the namespace the Service is running in
func GetNamespacedControlPlaneXdsPort(ctx context.Context, svcNamespace string, svcClient skkube.ServiceClient) (int32, error) {
	glooSvc, err := svcClient.Read(svcNamespace, kubeutils.GlooServiceName, clients.ReadOpts{Ctx: ctx})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return 0, noGlooSvcFoundError(err, svcNamespace, kubeutils.GlooServiceName)
		}
		return 0, err
	}

	// find the xds port on the Gloo Service
	for _, port := range glooSvc.Spec.Ports {
		if port.Name == kubeutils.GlooXdsPortName {
			return port.Port, nil
		}
	}
	return 0, noXdsPortFoundError(kubeutils.GlooXdsPortName, svcNamespace, kubeutils.GlooServiceName)
}

// GetControlPlaneXdsHost gets the xDS address from the gloo Service.
func GetControlPlaneXdsHost() string {
	return kubeutils.ServiceFQDN(metav1.ObjectMeta{
		Name:      kubeutils.GlooServiceName,
		Namespace: utils.GetPodNamespace(),
	})
}
