package check

import (
	"context"

	"github.com/hashicorp/go-multierror"
	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check/internal"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/kubegatewayutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
)

type CheckFunc = func(ctx context.Context, printer printers.P, opts *options.Options) error

func CheckKubeGatewayResources(ctx context.Context, printer printers.P, opts *options.Options) error {
	var multiErr *multierror.Error

	kubeGatewayEnabled, err := isKubeGatewayEnabled(ctx, opts)
	if err != nil {
		multiErr = multierror.Append(multiErr, eris.Wrapf(err, "unable to determine if Kubernetes Gateway integration is enabled"))
		return multiErr
	}

	if !kubeGatewayEnabled {
		printer.AppendMessage("\nSkipping Kubernetes Gateway resources check -- Kubernetes Gateway integration not enabled")
		return nil
	}

	printer.AppendMessage("\nDetected Kubernetes Gateway integration!")

	var checks = []CheckFunc{}

	if included := doesNotContain(opts.Top.CheckName, constants.KubeGatewayClasses); included {
		checks = append(checks, internal.CheckGatewayClass)
	}

	if included := doesNotContain(opts.Top.CheckName, constants.KubeGateways); included {
		checks = append(checks, internal.CheckGateways)
	}

	if included := doesNotContain(opts.Top.CheckName, constants.KubeHTTPRoutes); included {
		checks = append(checks, internal.CheckHTTPRoutes)
	}

	for _, check := range checks {
		if err := check(ctx, printer, opts); err != nil {
			multiErr = multierror.Append(multiErr, err)
		}
	}

	return multiErr.ErrorOrNil()
}

// check if Kubernetes Gateway integration is enabled by checking if the Gateway API CRDs are installed and
// whether the GG_K8S_GW_CONTROLLER env var is true in the gloo deployment.
func isKubeGatewayEnabled(ctx context.Context, opts *options.Options) (bool, error) {
	cfg, err := kubeutils.GetRestConfigWithKubeContext(opts.Top.KubeContext)
	if err != nil {
		return false, err
	}

	gatewayEnabled, err := kubegatewayutils.DetectKubeGatewayEnabled(ctx, opts)
	if err != nil {
		return false, eris.Wrapf(err, "unable to determine if Kubernetes Gateway integration is enabled")
	}

	if !gatewayEnabled {
		return false, nil
	}

	hasCRDs, err := kubegatewayutils.DetectKubeGatewayCrds(cfg)
	if err != nil {
		return false, eris.Wrapf(err, "unable to determine if Kubernetes Gateway CRDs are applied")
	}
	if !hasCRDs {
		return false, eris.New("The Kubernetes Gateway integration is enabled, but the Kubernetes Gateway CRDs are not applied in the cluster. To apply the CRDs, run `kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.0.0/standard-install.yaml`")
	}

	return true, nil
}
