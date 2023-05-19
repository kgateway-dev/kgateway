package list

import (
	"fmt"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	clusterSecretType = "solo.io/kubeconfig"
)

func RootCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   constants.CLUSTER_LIST_COMMAND.Use,
		Short: constants.CLUSTER_LIST_COMMAND.Short,
		RunE: func(cmd *cobra.Command, args []string) error {
			secretClient := helpers.MustKubeClientWithKubecontext(opts.Top.KubeContext).CoreV1().Secrets(opts.Cluster.FederationNamespace)
			secrets, err := secretClient.List(opts.Top.Ctx, metav1.ListOptions{})
			if err != nil {
				return errors.Wrapf(err, "Failed to list clusters.")
			}
			for _, s := range secrets.Items {
				if string(s.Type) == clusterSecretType {
					fmt.Printf("%s\n", s.GetName())
				}
			}
			return nil
		},
	}
	flagutils.AddClusterFlags(cmd.PersistentFlags(), &opts.Cluster)
	return cmd
}
