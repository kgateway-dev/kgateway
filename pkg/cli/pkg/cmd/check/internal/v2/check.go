package v2

import (
	"context"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/check/internal"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
)

type CheckFunc = func(ctx context.Context, printer printers.P, opts *options.Options) error

func Check(ctx context.Context, printer printers.P, opts *options.Options) error {
	checks := []CheckFunc{
		internal.CheckConnection,
		internal.CheckDeployments,
		internal.CheckGatewayClass,
		internal.CheckGatewys,
		internal.CheckHTTPRoutes,
	}

	for _, check := range checks {
		if err := check(ctx, printer, opts); err != nil {
			return err
		}
	}

	return nil

}
