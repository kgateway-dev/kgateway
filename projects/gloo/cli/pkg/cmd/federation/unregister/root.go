package unregister

import (
	"fmt"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

var EmptyClusterNameError = eris.New("please provide a cluster name to unregister")

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.CLUSTER_UNREGISTER_COMMAND.Use,
		Short: constants.CLUSTER_UNREGISTER_COMMAND.Short,
		Long:  constants.CLUSTER_UNREGISTER_COMMAND.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			secretClient := helpers.MustSecretClient()
			for _, clusterName := range args {
				err := secretClient.Delete(opts.Cluster.FederationNamespace, clusterName, clients.DeleteOpts{})
				if err != nil {
					fmt.Printf("Error unregistering cluster %s", clusterName)
				}
			}
			if len(args) == 0 {
				if opts.Cluster.Unregister.ClusterName == "" {
					return EmptyClusterNameError
				}
				return secretClient.Delete(opts.Cluster.FederationNamespace, opts.Cluster.Unregister.ClusterName, clients.DeleteOpts{})
			}
			return nil
		},
	}

	pflags := cmd.PersistentFlags()
	flagutils.AddClusterFlags(pflags, &opts.Cluster)
	flagutils.AddUnregisterFlags(pflags, &opts.Cluster.Unregister)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
