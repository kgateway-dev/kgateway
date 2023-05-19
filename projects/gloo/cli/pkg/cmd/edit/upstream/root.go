package upstream

import (
	"fmt"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/edit/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/ssl"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type EditUpstream struct {
	SslSecretRef core.ResourceRef
	Sni          string
	Remove       bool
}

func RootCmd(opts *options.EditOptions, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	optsExt := &EditUpstream{}

	cmd := &cobra.Command{
		Use:     constants.UPSTREAM_COMMAND.Use,
		Aliases: constants.UPSTREAM_COMMAND.Aliases,
		Short:   "edit an upstream in a namespace",
		Long:    "usage: glooctl edit upstream [NAME] [--namespace=namespace]",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.Top.Interactive {
				if err := addEditUpstreamInteractiveFlags(optsExt); err != nil {
					return err
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return editUpstream(opts, optsExt, args)
		},
	}

	addEditUpstreamOptions(cmd.Flags(), optsExt)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func addEditUpstreamOptions(set *pflag.FlagSet, edit *EditUpstream) {
	set.StringVar(&edit.SslSecretRef.Name, "ssl-secret-name", "", "name of the ssl secret for this upstream")
	set.StringVar(&edit.SslSecretRef.Namespace, "ssl-secret-namespace", "", "namespace of the ssl secret for this upstream")
	set.StringVar(&edit.Sni, "ssl-sni", "", "SNI value to provide when contacting this upstream")
	set.BoolVar(&edit.Remove, "ssl-remove", false, "Remove SSL configuration from this upstream")
}

func addEditUpstreamInteractiveFlags(opts *EditUpstream) error {

	if err := cliutil.GetStringInput("name of the ssl secret for this upstream: ", &opts.SslSecretRef.Name); err != nil {
		return err
	}
	if err := cliutil.GetStringInput("namespace of the ssl secret for this upstream: ", &opts.SslSecretRef.Namespace); err != nil {
		return err
	}
	if err := cliutil.GetStringInput("SNI value to provide when contacting this upstream: ", &opts.Sni); err != nil {
		return err
	}

	return nil
}

func editUpstream(opts *options.EditOptions, optsExt *EditUpstream, args []string) error {
	upClient := helpers.MustNamespacedUpstreamClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	up, err := upClient.Read(opts.Metadata.GetNamespace(), opts.Metadata.GetName(), clients.ReadOpts{})
	if err != nil {
		return errors.Wrapf(err, "Error reading upstream")
	}

	if opts.ResourceVersion != "" {
		if up.GetMetadata().GetResourceVersion() != opts.ResourceVersion {
			return fmt.Errorf("conflict - resource version does not match")
		}
	}

	if optsExt.Remove {
		up.SslConfig = nil
	} else {
		if up.GetSslConfig() == nil {
			up.SslConfig = &ssl.UpstreamSslConfig{}
		}

		hasBoth := (optsExt.SslSecretRef.GetName() != "") && (optsExt.SslSecretRef.GetNamespace() != "")
		hasNone := (optsExt.SslSecretRef.GetName() == "") && (optsExt.SslSecretRef.GetNamespace() == "")

		if hasBoth {
			up.GetSslConfig().SslSecrets = &ssl.UpstreamSslConfig_SecretRef{
				SecretRef: &optsExt.SslSecretRef,
			}
		} else if !hasNone {
			return fmt.Errorf("both --ssl-secret-name and --ssl-secret-namespace must be provided")
		}
		if optsExt.Sni != "" {
			up.GetSslConfig().Sni = optsExt.Sni
		}
	}
	_, err = upClient.Write(up, clients.WriteOpts{OverwriteExisting: true})
	return err
}
