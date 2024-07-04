package kubegatewayutils

import (
	"context"
	"strconv"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	cliconstants "github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

// Returns true if Kubernetes Gateway API CRDs are on the cluster.
// Note: this doesn't check for specific CRD names; it returns true if *any* k8s Gateway CRD is detected
func DetectKubeGatewayCrds(cfg *rest.Config) (bool, error) {
	discClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return false, err
	}

	groups, err := discClient.ServerGroups()
	if err != nil {
		return false, err
	}

	// Check if gateway group exists
	for _, group := range groups.Groups {
		if group.Name == wellknown.GatewayGroup {
			return true, nil
		}
	}

	return false, nil
}

// Returns true if the GG_K8S_GW_CONTROLLER env var is true in the gloo deployment.
// Note: This is tied up with the GG implementation and will need to be updated if it changes
func DetectKubeGatewayEnabled(ctx context.Context, opts *options.Options) (bool, error) {
	glooDeploymentName, err := helpers.GetGlooDeploymentName(opts.Top.Ctx, opts.Metadata.GetNamespace())
	if err != nil {
		return false, err
	}

	// check if Kubernetes Gateway integration is enabled by checking if the controller env variable is set in the
	// gloo deployment
	client, err := helpers.GetKubernetesClient(opts.Top.KubeContext)
	if err != nil {
		return false, eris.Wrapf(err, "could not get kubernetes client")
	}

	glooDeployment, err := client.AppsV1().Deployments(opts.Metadata.GetNamespace()).Get(ctx, glooDeploymentName, metav1.GetOptions{})
	if err != nil {
		return false, eris.Wrapf(err, "could not get gloo deployment")
	}

	var glooContainer *corev1.Container
	for _, container := range glooDeployment.Spec.Template.Spec.Containers {
		if container.Name == cliconstants.GlooContainerName {
			glooContainer = &container
			break
		}
	}
	if glooContainer == nil {
		return false, eris.New("could not find gloo container in gloo deployment")
	}

	for _, envVar := range glooContainer.Env {
		if envVar.Name == constants.GlooGatewayEnableK8sGwControllerEnv {
			val, err := strconv.ParseBool(envVar.Value)
			if err != nil {
				return false, eris.Wrapf(err, "could not parse value of %s env var in gloo deployment", constants.GlooGatewayEnableK8sGwControllerEnv)
			}
			return val, nil
		}
	}
	return false, nil
}
