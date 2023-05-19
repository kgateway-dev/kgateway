package virtualservice

import (
	"fmt"
	"reflect"

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

type EditVirtualService struct {
	SslSecretRef core.ResourceRef
	SniDomains   []string
	Remove       bool
}

func RootCmd(opts *options.EditOptions, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	optsExt := &EditVirtualService{}

	cmd := &cobra.Command{
		Use:     constants.VIRTUAL_SERVICE_COMMAND.Use,
		Aliases: constants.VIRTUAL_SERVICE_COMMAND.Aliases,
		Short:   "edit a virtualservice in a namespace",
		Long:    "usage: glooctl edit virtualservice [NAME] [--namespace=namespace] [-o FORMAT]",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.Top.Interactive {
				if err := addEditVirtualServiceInteractiveFlags(optsExt); err != nil {
					return err
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return editVirtualService(opts, optsExt, args)
		},
	}

	addEditVirtualServiceOptions(cmd.Flags(), optsExt)
	cliutils.ApplyOptions(cmd, optionsFunc)
	cmd.AddCommand(RateLimitConfig(opts, optionsFunc...))
	return cmd
}

func addEditVirtualServiceOptions(set *pflag.FlagSet, edit *EditVirtualService) {
	set.StringVar(&edit.SslSecretRef.Name, "ssl-secret-name", "", "name of the ssl secret for this virtual service")
	set.StringVar(&edit.SslSecretRef.Namespace, "ssl-secret-namespace", "", "namespace of the ssl secret for this virtual service")
	set.StringArrayVar(&edit.SniDomains, "ssl-sni-domains", nil, "SNI domains for this virtual service")
	set.BoolVar(&edit.Remove, "ssl-remove", false, "Remove SSL configuration from this virtual service")
}

func addEditVirtualServiceInteractiveFlags(opts *EditVirtualService) error {

	if err := cliutil.GetStringInput("name of the ssl secret for this virtual service: ", &opts.SslSecretRef.Name); err != nil {
		return err
	}
	if err := cliutil.GetStringInput("namespace of the ssl secret for this virtual service: ", &opts.SslSecretRef.Namespace); err != nil {
		return err
	}
	if err := cliutil.GetStringSliceInput("SNI domains for this virtual service: ", &opts.SniDomains); err != nil {
		return err
	}

	return nil
}

func editVirtualService(opts *options.EditOptions, optsExt *EditVirtualService, args []string) error {
	vsClient := helpers.MustNamespacedVirtualServiceClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	vs, err := vsClient.Read(opts.Metadata.GetNamespace(), opts.Metadata.GetName(), clients.ReadOpts{})
	if err != nil {
		return errors.Wrapf(err, "Error reading virtual service")
	}

	if opts.ResourceVersion != "" {
		if vs.GetMetadata().GetResourceVersion() != opts.ResourceVersion {
			return fmt.Errorf("conflict - resource version does not match")
		}
	}

	if optsExt.Remove {
		vs.SslConfig = nil
	} else {
		if vs.GetSslConfig() == nil {
			vs.SslConfig = &ssl.SslConfig{}
		}

		if optsExt.SslSecretRef.GetName() != "" {
			vs.GetSslConfig().SslSecrets = &ssl.SslConfig_SecretRef{
				SecretRef: &optsExt.SslSecretRef,
			}
		} else if optsExt.SslSecretRef.GetNamespace() != "" {
			return fmt.Errorf("name must be provided")
		}

		if optsExt.SniDomains != nil {
			vs.GetSslConfig().SniDomains = optsExt.SniDomains
		}
		if reflect.DeepEqual(*vs.GetSslConfig(), ssl.SslConfig{}) {
			vs.SslConfig = nil
		}
	}

	_, err = vsClient.Write(vs, clients.WriteOpts{OverwriteExisting: true})
	return err
}
