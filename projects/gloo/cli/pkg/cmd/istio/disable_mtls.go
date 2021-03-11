package istio

import (
	"errors"
	"fmt"

	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/flagutils"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/go-utils/cliutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"

	"github.com/spf13/cobra"
)

var (
	// ErrMTLSAlreadyDisabled occurs when trying to disable mTLS for an upstream that isn't using mTLS
	ErrMTLSAlreadyDisabled = errors.New("upstream already has mTLS disabled")
)

// DisableMTLS removes an sslConfig from the given upstream which was previously
// used by envoy SDS to pick up the mTLS certs
func DisableMTLS(opts *options.Options, optionsFunc ...cliutils.OptionsFunc) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disable-mtls",
		Short: "Disables Istio mTLS for a given upstream",
		Long:  "Disables Istio mTLS for a given upstream, by removing the sslConfig which lets envoy know to get the certs via SDS",
		RunE: func(cmd *cobra.Command, args []string) error {
			err := istioDisableMTLS(args, opts)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return err
			}
			return nil
		},
	}
	pflags := cmd.PersistentFlags()
	flagutils.AddUpstreamFlag(pflags, &opts.Istio.Upstream)
	cliutils.ApplyOptions(cmd, optionsFunc)
	return cmd
}

func istioDisableMTLS(args []string, opts *options.Options) error {
	upClient := helpers.MustNamespacedUpstreamClient(opts.Top.Ctx, opts.Metadata.GetNamespace())
	upstream, err := upClient.Read(opts.Metadata.GetNamespace(), opts.Istio.Upstream, clients.ReadOpts{})
	if err != nil {
		return eris.Wrapf(err, "Error reading upstream")
	}

	return disableMTLSOnUpstream(upClient, upstream)
}

func disableMTLSOnUpstreamList(client v1.UpstreamClient, upstreamList v1.UpstreamList) error {
	for _, us := range upstreamList {
		if err := disableMTLSOnUpstream(client, us); err != nil {
			return err
		}
	}
	return nil
}

func disableMTLSOnUpstream(client v1.UpstreamClient, upstream *v1.Upstream) error {
	if upstream.SslConfig == nil {
		return eris.Wrapf(ErrMTLSAlreadyDisabled, "upstream does not have an sslConfig set")
	}

	upstream.SslConfig = nil

	_, err := client.Write(upstream, clients.WriteOpts{OverwriteExisting: true})
	return err
}
