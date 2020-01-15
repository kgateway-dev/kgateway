package ui

import (
	"io"
	"os"
	"os/exec"
	"strconv"

	"github.com/rotisserie/eris"

	"github.com/solo-io/go-utils/kubeutils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
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
		Use:     constants.UI_COMMAND.Use,
		Aliases: constants.UI_COMMAND.Aliases,
		Short:   constants.UI_COMMAND.Short,
		Long:    constants.UI_COMMAND.Long,
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

			deployments, err := client.AppsV1().Deployments(opts.Metadata.Namespace).List(metav1.ListOptions{
				FieldSelector: fields.OneTermEqualSelector("metadata.name", "api-server").String(),
			})
			if err != nil {
				return err
			}
			if len(deployments.Items) != 1 {
				return eris.Errorf("Could not find 'api-server' deployment in the '%s' namespace", opts.Metadata.Namespace)
			}

			var staticPort string
			for _, container := range deployments.Items[0].Spec.Template.Spec.Containers {
				if container.Name == "apiserver-ui" {
					for _, port := range container.Ports {
						if port.Name == "static" {
							staticPort = strconv.Itoa(int(port.ContainerPort))
						}
					}
				}
			}
			if staticPort == "" {
				return eris.Errorf("Could not find static port for 'apiserver-ui' container in the 'api-server' deployment")
			}

			/** port-forward command **/

			portFwd := exec.Command("kubectl", "port-forward", "-n", opts.Metadata.Namespace,
				"deployment/api-server", staticPort)

			err = cliutil.Initialize()
			if err != nil {
				return err
			}
			logger := cliutil.GetLogger()

			portFwd.Stderr = io.MultiWriter(logger, os.Stderr)
			if opts.Top.Verbose {
				portFwd.Stdout = io.MultiWriter(logger, os.Stdout)
			} else {
				portFwd.Stdout = logger
			}

			if err := portFwd.Start(); err != nil {
				return err
			}
			defer portFwd.Wait()

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
