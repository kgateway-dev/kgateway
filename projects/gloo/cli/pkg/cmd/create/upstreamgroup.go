package create

import (
	"context"
	"strconv"
	"strings"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/prerun"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/argsutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/printers"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/surveyutils"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"

	"github.com/solo-io/solo-kit/pkg/errors"
)

const EmptyUpstreamGroupCreateError = "please provide weighted destinations for your upstream group, or use -i to create the upstream group interactively"

func UpstreamGroup(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.UPSTREAM_GROUP_COMMAND.Use,
		Aliases: constants.UPSTREAM_GROUP_COMMAND.Aliases,
		Short:   "Create an Upstream Group",
		Long: "Upstream groups represent groups of upstreams. An UpstreamGroup addresses an issue of how do you have " +
			"multiple routes or virtual services referencing the same multiple weighted destinations where you want to " +
			"change the weighting consistently for all calling routes. This is a common need for Canary deployments " +
			"where you want all calling routes to forward traffic consistently across the two service versions.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := prerun.CallParentPrerun(cmd, args); err != nil {
				return err
			}
			if err := prerun.EnableConsulClients(opts); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if !opts.Top.Interactive && len(opts.Create.InputUpstreamGroup.WeightedDestinations.Entries) == 0 {
				return errors.Errorf(EmptyUpstreamGroupCreateError)
			}
			if len(opts.Create.InputUpstreamGroup.WeightedDestinations.Entries) == 0 {
				if err := surveyutils.AddUpstreamGroupFlagsInteractive(opts.Top.Ctx, &opts.Create.InputUpstreamGroup); err != nil {
					return err
				}
			}
			if err := argsutils.MetadataArgsParse(opts, args); err != nil {
				return err
			}
			return createUpstreamGroup(opts)
		},
	}
	cliutils.ApplyOptions(cmd, optionsFunc)

	flags := cmd.Flags()
	flags.StringSliceVar(
		&opts.Create.InputUpstreamGroup.WeightedDestinations.Entries,
		"weighted-upstreams",
		[]string{},
		"comma-separated list of weighted upstream key=value entries (namespace.upstreamName=weight)")
	return cmd
}

func createUpstreamGroup(opts *options.Options) error {
	ug, err := upstreamGroupFromOpts(opts)
	if err != nil {
		return err
	}

	if !opts.Create.DryRun {
		ug, err = helpers.MustNamespacedUpstreamGroupClient(opts.Top.Ctx, opts.Metadata.GetNamespace()).Write(ug, clients.WriteOpts{})
		if err != nil {
			return err
		}
	}

	_ = printers.PrintUpstreamGroups(v1.UpstreamGroupList{ug}, opts.Top.Output)

	return nil
}

func upstreamGroupFromOpts(opts *options.Options) (*v1.UpstreamGroup, error) {
	dest, err := upstreamGroupDestinationsFromOpts(opts.Top.Ctx, opts.Create.InputUpstreamGroup)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid upstream spec")
	}
	return &v1.UpstreamGroup{
		Metadata:     &opts.Metadata,
		Destinations: dest,
	}, nil
}

func upstreamGroupDestinationsFromOpts(ctx context.Context, input options.InputUpstreamGroup) ([]*v1.WeightedDestination, error) {
	// collect upstreams list
	ussByKey := make(map[string]*v1.Upstream)
	var usKeys []string
	for _, ns := range helpers.MustGetNamespaces(ctx) {
		usList, err := helpers.MustNamespacedUpstreamClient(ctx, ns).List(ns, clients.ListOpts{})
		if err != nil {
			return nil, err
		}
		for _, us := range usList {
			ref := us.GetMetadata().Ref()
			ussByKey[ref.Key()] = us
			usKeys = append(usKeys, ref.Key())
		}
	}
	if len(usKeys) == 0 {
		return nil, errors.Errorf("no upstreams found. create an upstream first or enable discovery.")
	}

	weightedDestinations := make([]*v1.WeightedDestination, 0)
	for namespacedUpstream, usWeight := range input.WeightedDestinations.MustMap() {
		weight := uint32(1)
		if providedWeight, err := strconv.ParseUint(usWeight, 10, 64); err == nil && providedWeight > 0 {
			weight = uint32(providedWeight)
		}

		if _, ok := ussByKey[namespacedUpstream]; !ok {
			splits := strings.SplitAfter(namespacedUpstream, ".")
			if len(splits) != 2 {
				return nil, errors.Errorf("invalid format: provide namespaced upstream names (namespace.upstreamName)")
			}
			ns := splits[0]
			us := splits[1]
			return nil, errors.Errorf("no upstream found with name %v in namespace %v", us, ns)
		}
		wd := v1.WeightedDestination{
			Destination: &v1.Destination{
				DestinationType: &v1.Destination_Upstream{
					Upstream: ussByKey[namespacedUpstream].GetMetadata().Ref(),
				},
			},
			Weight: weight,
		}
		weightedDestinations = append(weightedDestinations, &wd)
	}
	return weightedDestinations, nil
}
