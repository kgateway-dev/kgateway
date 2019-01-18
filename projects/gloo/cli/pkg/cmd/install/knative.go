package install

import (
	"bytes"
	"os"
	"os/exec"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

//go:generate sh -c "2gobytes -p install -a knativeManifestBytes -i ${GOPATH}/src/github.com/solo-io/gloo/install/integrations/knative-no-istio-0.3.0.yaml | sed 's@// date.*@@g' > knative.yaml.go"

func KnativeCmd(opts *options.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "knative",
		Short: "install Knative on kubernetes",
		Long:  "requires kubectl to be installed",
		RunE: func(cmd *cobra.Command, args []string) error {
			kubectl := exec.Command("kubectl", "apply", "-f", "-")
			kubectl.Stdin = bytes.NewBuffer(knativeManifestBytes)
			kubectl.Stdout = os.Stdout
			kubectl.Stderr = os.Stderr
			return kubectl.Run()
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddInstallFlags(pflags, &opts.Install)
	return cmd
}
