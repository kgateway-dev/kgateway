package surveyutils

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

func AddUpstreamGroupFlagsInteractive(upstreamGroup *options.InputUpstreamGroup) error {
	// TODO this was copied -- refactor and prevent duplication
	// collect secrets list
	usClient := helpers.MustUpstreamClient()
	ussByKey := make(map[string]*v1.Upstream)
	var usKeys []string
	for _, ns := range helpers.MustGetNamespaces() {
		usList, err := usClient.List(ns, clients.ListOpts{})
		if err != nil {
			return err
		}
		for _, us := range usList {
			ref := us.Metadata.Ref()
			ussByKey[ref.Key()] = us
			usKeys = append(usKeys, ref.Key())
		}
	}
	if len(usKeys) == 0 {
		return errors.Errorf("no upstreams found. create an upstream first or enable " +
			"discovery.")
	}
	// TODO end of copied code

	var chosenUpstreams []string
	if err := cliutil.MultiChooseFromList(
		"Choose upstreams to add to your upstream group: ",
		&chosenUpstreams,
		usKeys,
	); err != nil {
		return err
	}

	upstreamGroup.WeightedDestinations = make([]v1.WeightedDestination, len(chosenUpstreams))
	for i, us := range chosenUpstreams {
		var weight = uint32(0)
		if err := cliutil.GetUint32InputDefault(fmt.Sprintf("Weight for the %v upstream?", us), &weight,1); err != nil {
			return err
		}
		// TODO ensure weight is nonzero

		// TODO handle all destination types, including multi?
		upstreamGroup.WeightedDestinations[i] = v1.WeightedDestination{
			Destination: &v1.Destination{
				DestinationType: &v1.Destination_Upstream{
					Upstream: utils.ResourceRefPtr(ussByKey[us].Metadata.Ref()),
				},
			},
			Weight: uint32(weight),
		}
	}
	return nil
}
