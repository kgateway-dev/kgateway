package install

import (
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/go-utils/errors"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/spf13/cobra"
)

func ingressCmd(opts *options.Options) *cobra.Command {
	const glooIngressUrlTemplate = "https://github.com/solo-io/gloo/releases/download/v%s/gloo-ingress.yaml"
	cmd := &cobra.Command{
		Use:   "ingress",
		Short: "install the Gloo Ingress Controller on kubernetes",
		Long:  "requires kubectl to be installed",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := preInstall(opts.Install.Namespace); err != nil {
				return errors.Wrapf(err, "pre-install failed")
			}
			if err := installFromUri(opts, opts.Install.GlooManifestOverride, glooIngressUrlTemplate, nil); err != nil {
				return errors.Wrapf(err, "installing ingress from manifest")
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddInstallFlags(pflags, &opts.Install)
	return cmd
}
