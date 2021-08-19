package surveyutils

import (
	"context"
	"fmt"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

func AddUpstreamGroupFlagsInteractive(ctx context.Context, upstreamGroup *options.InputUpstreamGroup) error {

	// collect upstreams list
	ussByKey := make(map[string]*v1.Upstream)
	var usKeys []string
	for _, ns := range helpers.MustGetNamespaces(ctx) {
		usList, err := helpers.MustNamespacedUpstreamClient(ctx, ns).List(ns, clients.ListOpts{})
		if err != nil {
			return err
		}
		for _, us := range usList {
			ref := us.GetMetadata().Ref()
			ussByKey[ref.Key()] = us
			usKeys = append(usKeys, ref.Key())
		}
	}
	if len(usKeys) == 0 {
		return errors.Errorf("no upstreams found. create an upstream first or enable discovery.")
	}

	var chosenUpstreams []string
	if err := cliutil.MultiChooseFromList(
		"Choose upstreams to add to your upstream group: ",
		&chosenUpstreams,
		usKeys,
	); err != nil {
		return err
	}

	upstreamGroup.WeightedDestinations.Entries = make([]string, len(chosenUpstreams))
	for i, us := range chosenUpstreams {
		var weight = uint32(0)
		if err := cliutil.GetUint32InputDefault(fmt.Sprintf("Weight for the %v upstream?", us), &weight, 1); err != nil {
			return err
		}
		upstreamGroup.WeightedDestinations.Entries[i] = us + "=" + fmt.Sprint(weight)
	}
	return nil
}
