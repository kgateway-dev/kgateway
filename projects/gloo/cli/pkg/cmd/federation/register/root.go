package register

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
	v1 "k8s.io/api/rbac/v1"
)

var glooFederationPolicyRules = []v1.PolicyRule{
	{
		Verbs:     []string{"*"},
		APIGroups: []string{"gloo.solo.io", "gateway.solo.io", "enterprise.gloo.solo.io", "graphql.gloo.solo.io", "ratelimit.solo.io"},
		Resources: []string{"*"},
	},
	{
		Verbs:     []string{"get", "list", "watch"},
		APIGroups: []string{"apps"},
		Resources: []string{"deployments", "daemonsets"},
	},
	{
		Verbs:     []string{"get", "list", "watch"},
		APIGroups: []string{""},
		Resources: []string{"pods", "nodes", "services"},
	},
}

var glooFederationReadConfigPolicyRules = []v1.PolicyRule{
	{
		Verbs:     []string{"get"},
		APIGroups: []string{""},
		Resources: []string{"services/proxy"},
	},
}

func RegisterCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.CLUSTER_REGISTER_COMMAND.Use,
		Aliases: constants.CLUSTER_REGISTER_COMMAND.Aliases,
		Short:   constants.CLUSTER_REGISTER_COMMAND.Short,
		Long:    constants.CLUSTER_REGISTER_COMMAND.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			return Register(opts)
		},
	}

	pflags := cmd.PersistentFlags()
	flagutils.AddClusterFlags(pflags, &opts.Cluster)
	flagutils.AddRegisterFlags(pflags, &opts.Cluster.Register)
	// this flag is mainly for demo, testing, and debugging purposes
	pflags.Lookup(flagutils.LocalClusterDomainOverride).Hidden = true
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func DeregisterCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.CLUSTER_DEREGISTER_COMMAND.Use,
		Short: constants.CLUSTER_DEREGISTER_COMMAND.Short,
		Long:  constants.CLUSTER_DEREGISTER_COMMAND.Long,
		RunE: func(cmd *cobra.Command, args []string) error {
			return Deregister(opts)
		},
	}

	pflags := cmd.PersistentFlags()
	flagutils.AddClusterFlags(pflags, &opts.Cluster)
	flagutils.AddDeregisterFlags(pflags, &opts.Cluster.Deregister)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
