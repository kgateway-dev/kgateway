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
	vsClient := helpers.MustVirtualServiceClient()
	vs, err := vsClient.Read(opts.Metadata.Namespace, opts.Metadata.Name, clients.ReadOpts{})
	if err != nil {
		return errors.Wrapf(err, "Error reading vhost")
	}

	if opts.ResourceVersion != "" {
		if vs.Metadata.ResourceVersion != opts.ResourceVersion {
			return fmt.Errorf("conflict - resource version does not match")
		}
	}

	if int(opts.Index) >= len(vs.VirtualHost.Routes) {
		return fmt.Errorf("invalid route index")
	}

	route := vs.VirtualHost.Routes[opts.Index]

	err = modify(route)
	if err != nil {
		return err
	}

	_, err = vsClient.Write(vs, clients.WriteOpts{OverwriteExisting: true})
	return err
}

func EditRoutePreRunE(opts *RouteEditInput) error {

	if opts.Top.Interactive {

		vsclient := helpers.MustVirtualServiceClient()
		vsvc, err := vsclient.Read(opts.Metadata.Namespace, opts.Metadata.Name, clients.ReadOpts{})
		if err != nil {
			return err
		}

		if idx, err := surveyutils.SelectRouteFromVirtualServiceInteractive(vsvc, "Choose the route you wish to change: "); err != nil {
			return err
		} else {
			opts.Index = uint32(idx)
			opts.ResourceVersion = vsvc.Metadata.ResourceVersion
		}
	}

	return nil
}
