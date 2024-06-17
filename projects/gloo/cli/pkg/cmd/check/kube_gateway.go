package check

import (
	"context"

	"github.com/hashicorp/go-multierror"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/projects/gateway2/pkg/api/gateway.gloo.solo.io/v1alpha1"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check/internal"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/kubegatewayutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
		printer.AppendMessage("\nSkipping Kubernetes Gateway resources check -- Kube Gateway integration not enabled")
		return nil
	}

	printer.AppendMessage("\nDetected Kubernetes Gateway integration!")

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

// check if kube gateway integration is enabled by checking if the Gateway API CRDs are installed and
// whether a GatewayParameters CR exists in the install namespace
func isKubeGatewayEnabled(ctx context.Context, opts *options.Options) (bool, error) {
	cfg, err := kubeutils.GetRestConfigWithKubeContext(opts.Top.KubeContext)
	if err != nil {
		return false, err
	}

	hasCrds, err := kubegatewayutils.DetectKubeGatewayCrds(cfg)
	if err != nil {
		return false, eris.Wrapf(err, "could not determine if kubernetes gateway crds are applied")
	}
	if !hasCrds {
		return false, nil
	}

	// look for default GatewayParameters
	scheme := scheme.Scheme
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return false, err
	}
	cli, err := client.New(cfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return false, err
	}

	gwp := v1alpha1.GatewayParameters{}
	err = cli.Get(ctx, client.ObjectKey{Name: wellknown.DefaultGatewayParametersName, Namespace: opts.Metadata.GetNamespace()}, &gwp)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
