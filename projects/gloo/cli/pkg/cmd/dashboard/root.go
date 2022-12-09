package dashboard

import (
	"fmt"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/rotisserie/eris"

	"github.com/solo-io/k8s-utils/kubeutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/pkg/browser"
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/spf13/cobra"
)

func RootCmd(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:     constants.DASHBOARD_COMMAND.Use,
		Aliases: constants.DASHBOARD_COMMAND.Aliases,
		Short:   constants.DASHBOARD_COMMAND.Short,
		Long:    constants.DASHBOARD_COMMAND.Long,
		RunE: func(cmd *cobra.Command, args []string) error {

			/** Get the port **/

			cfg, err := kubeutils.GetConfig("", "")
			if err != nil {
				// kubecfg is missing, therefore no cluster is present, only print client version
				return err
			}
			client, err := kubernetes.NewForConfig(cfg)
			if err != nil {
				return err
			}

			deployment, err := client.AppsV1().Deployments(opts.Metadata.GetNamespace()).Get(opts.Top.Ctx, "gloo-fed-console", metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					fmt.Printf("No Gloo dashboard found as part of the installation in namespace %s. The full dashboard is part of Gloo Enterprise by default. ", opts.Metadata.GetNamespace())
				}
				return err
			}

			var staticPort string
			for _, container := range deployment.Spec.Template.Spec.Containers {
				if container.Name == "console" {
					for _, port := range container.Ports {
						if port.Name == "static" {
							staticPort = strconv.Itoa(int(port.ContainerPort))
						}
					}
				}
			}
			if staticPort == "" {
				return eris.Errorf("Could not find static port for 'console' container in the 'gloo-fed-console' deployment")
			}

			/** port-forward command **/

			_, portFwdCmd, err := cliutil.PortForwardGet(opts.Top.Ctx, opts.Metadata.GetNamespace(), "deployment/gloo-fed-console",
				staticPort, staticPort, opts.Top.Verbose, "")
			if err != nil {
				return err
			}
			defer portFwdCmd.Wait()

			/** open in browser **/

			if err := browser.OpenURL("http://localhost:" + staticPort); err != nil {
				return err
			}

			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddNamespaceFlag(pflags, &opts.Metadata.Namespace)
	flagutils.AddVerboseFlag(pflags, opts)

	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}
