package main

import (
	"context"
	"os"

	v1 "k8s.io/api/core/v1"

	"github.com/solo-io/gloo/jobs/pkg/run"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/constants"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/utils/statusutils"
	"github.com/spf13/cobra"
)

func main() {
	ctx := contextutils.WithLogger(context.Background(), "gencert")

	if err := cmd(ctx).Execute(); err != nil {
		contextutils.LoggerFrom(ctx).Fatal("execution failed")
	}
}

func cmd(ctx context.Context) *cobra.Command {
	var opts run.Options

	cmd := &cobra.Command{
		Use:     "certgen",
		Aliases: constants.PROXY_COMMAND.Aliases,
		Short:   "generate kube secrets with self-signed certs.",
		Long: "generate kube secrets with self-signed certs. " +
			"certgen can also patch admission webhook configurations with the matching ca bundle for the generated certs..",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run.Run(ctx, opts)
		},
	}

	pFlags := cmd.PersistentFlags()

	podNamespace := os.Getenv(statusutils.PodNamespaceEnvName)

	pFlags.StringVar(&opts.SvcName, "svc-name", "",
		"name of the service for which to generate the certs")
	pFlags.StringVar(&opts.SvcNamespace, "svc-namespace", podNamespace,
		"namespace of the service for which to generate the certs")
	pFlags.StringVar(&opts.SecretName, "secret-name", "",
		"name of the secret to create which holds the certs")
	pFlags.StringVar(&opts.NextSecretName, "next-secret-name", "",
		"name of the secret to store the next set of certs")
	pFlags.StringVar(&opts.SecretNamespace, "secret-namespace", podNamespace,
		"namespace of the secret to create which holds the certs")
	pFlags.StringVar(&opts.ServerCertSecretFileName, "secret-cert-name", v1.TLSCertKey,
		"name of the server cert as it will be stored in the secret data")
	pFlags.StringVar(&opts.ServerKeySecretFileName, "server-key-name", v1.TLSPrivateKeyKey,
		"name of the server key as it will be stored in the secret data")
	pFlags.StringVar(&opts.ServerCertAuthorityFileName, "server-ca-name", v1.ServiceAccountRootCAKey,
		"name of the server cert authority as it will be stored in the secret data")
	pFlags.StringVar(&opts.ValidatingWebhookConfigurationName, "validating-webhook-configuration-name", "",
		"name of the ValidatingWebhookConfiguration to patch with the generated CA bundle. leave empty to skip this step.")
	pFlags.BoolVar(&opts.ForceRotation, "force-rotation", false,
		"if set, will create new certs even if the old one are still valid")
	pFlags.StringVar(&opts.RotationDuration, "rotation-duration", "10s",
		"time duration string value which specifies the total delay expected while swapping certs, if too short will cause requests to potentially be dropped. If activedeadlineseconds is set make sure this is less")
	pFlags.StringVar(&opts.RenewBefore, "renew-before", "2160h",
		"time duration string value which specifies how long before expiry a certificate should be renewed")
	return cmd
}
