package check

import (
	"context"
	"strconv"

	"github.com/hashicorp/go-multierror"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check/internal"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	cliconstants "github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/gloo/projects/gloo/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CheckFunc = func(ctx context.Context, printer printers.P, opts *options.Options) error

func CheckKubeGatewayResources(ctx context.Context, printer printers.P, opts *options.Options) error {
	var multiErr *multierror.Error

	kubeGatewayEnabled, err := isKubeGatewayEnabled(ctx, opts)
	if err != nil {
		multiErr = multierror.Append(multiErr, eris.Wrapf(err, "unable to determine if kube gateway is enabled"))
		return multiErr
	}

	if !kubeGatewayEnabled {
		printer.AppendMessage("Skipping Kubernetes Gateway resources check -- Kube Gateway integration not enabled")
		return nil
	}

	checks := []CheckFunc{
		internal.CheckGatewayClass,
		internal.CheckGateways,
		internal.CheckHTTPRoutes,
	}

	for _, check := range checks {
		if err := check(ctx, printer, opts); err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	return multiErr.ErrorOrNil()
}

func isKubeGatewayEnabled(ctx context.Context, opts *options.Options) (bool, error) {
	// check if kube gateway integration is enabled by checking if the controller env variable is set in the
	// gloo deployment
	client, err := helpers.GetKubernetesClient(opts.Top.KubeContext)
	if err != nil {
		return false, eris.Wrapf(err, "could not get kubernetes client")
	}
	glooDeployment, err := client.AppsV1().Deployments(opts.Metadata.GetNamespace()).Get(ctx, kubeutils.GlooDeploymentName, metav1.GetOptions{})
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
