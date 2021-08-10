package options

import (
	"fmt"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	editOptions "github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/errors"
)

type RouteEditInput struct {
	*editOptions.EditOptions
	Index uint32
}

func UpdateRoute(opts *RouteEditInput, modify func(*gatewayv1.Route) error) error {
	vsClient := helpers.MustNamespacedVirtualServiceClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	vs, err := vsClient.Read(opts.Metadata.GetNamespace(), opts.Metadata.GetName(), clients.ReadOpts{})
	if err != nil {
		return errors.Wrapf(err, "Error reading vhost")
	}

	if opts.ResourceVersion != "" {
		if vs.GetMetadata().GetResourceVersion() != opts.ResourceVersion {
			return fmt.Errorf("conflict - resource version does not match")
		}
	}

	if int(opts.Index) >= len(vs.GetVirtualHost().GetRoutes()) {
		return fmt.Errorf("invalid route index")
	}

	route := vs.GetVirtualHost().GetRoutes()[opts.Index]

	err = modify(route)
	if err != nil {
		return err
	}

	_, err = vsClient.Write(vs, clients.WriteOpts{OverwriteExisting: true})
	return err
}

func EditRoutePreRunE(opts *RouteEditInput) error {
	if opts.Top.Interactive {
		vsclient := helpers.MustNamespacedVirtualServiceClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
		vsvc, err := vsclient.Read(opts.Metadata.GetNamespace(), opts.Metadata.GetName(), clients.ReadOpts{})
		if err != nil {
			return err
		}

		if idx, err := surveyutils.SelectRouteFromVirtualServiceInteractive(vsvc, "Choose the route you wish to change: "); err != nil {
			return err
		} else {
			opts.Index = uint32(idx)
			opts.ResourceVersion = vsvc.GetMetadata().ResourceVersion
		}
	}
	return nil
}
