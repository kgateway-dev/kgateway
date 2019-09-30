package run

import (
	"context"

	"github.com/solo-io/gloo/jobs/pkg/certgen"
	"github.com/solo-io/gloo/jobs/pkg/kube"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/errors"
)

type Options struct {
	SvcName      string
	SvcNamespace string

	SecretName      string
	SecretNamespace string

	ServerCertSecretKey string
	ServerKeySecretKey  string

	ValidatingWebhookConfigurationName string
}

func Run(ctx context.Context, opts Options) error {
	if opts.SvcNamespace == "" {
		return errors.Errorf("must provide svc-namespace")
	}
	if opts.SvcName == "" {
		return errors.Errorf("must provide svc-name")
	}
	if opts.SecretNamespace == "" {
		return errors.Errorf("must provide secret-namespace")
	}
	if opts.SecretName == "" {
		return errors.Errorf("must provide secret-name")
	}
	if opts.ServerCertSecretKey == "" {
		return errors.Errorf("must provide secret data key for server cert")
	}
	if opts.ServerKeySecretKey == "" {
		return errors.Errorf("must provide secret data key for server key")
	}
	certs, err := certgen.GenCerts(opts.SvcName, opts.SvcNamespace)
	if err != nil {
		return errors.Wrapf(err, "generating self-signed certs and key")
	}
	kubeClient := helpers.MustKubeClient()

	secretConfig := kube.TlsSecret{
		SecretName:      opts.SecretName,
		SecretNamespace: opts.SecretNamespace,
		PrivateKeyKey:   opts.ServerKeySecretKey,
		CaCertKey:       opts.ServerCertSecretKey,
		PrivateKey:      certs.ServerCertKey,
		CaCert:          certs.ServerCertificate,
	}

	if err := kube.CreateTlsSecret(ctx, kubeClient, secretConfig); err != nil {
		return errors.Wrapf(err, "failed creating secret")
	}

	vwcName := opts.ValidatingWebhookConfigurationName
	if vwcName == "" {
		contextutils.LoggerFrom(ctx).Infof("no ValidatingWebhookConfiguration provided. finished successfully.")
		return nil
	}

	vwcConfig := kube.WebhookTlsConfig{
		ServiceName:      opts.SvcName,
		ServiceNamespace: opts.SvcNamespace,
		CaBundle:         certs.CaCertificate,
	}

	if err := kube.UpdateValidatingWebhookConfigurationCaBundle(ctx, kubeClient, vwcName, vwcConfig); err != nil {
		return errors.Wrapf(err, "failed patching validating webhook config")
	}

	contextutils.LoggerFrom(ctx).Infof("finished successfully.")

	return nil
}
