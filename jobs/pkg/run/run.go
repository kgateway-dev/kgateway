package run

import (
	"context"

	"go.uber.org/zap"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/jobs/pkg/certgen"
	"github.com/solo-io/gloo/jobs/pkg/kube"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/go-utils/contextutils"
)

type Options struct {
	SvcName      string
	SvcNamespace string

	SecretName      string
	SecretNamespace string

	ServerCertSecretFileName    string
	ServerCertAuthorityFileName string
	ServerKeySecretFileName     string

	ValidatingWebhookConfigurationName string

	ForceRotation bool
}

func Run(ctx context.Context, opts Options) error {
	if opts.SvcNamespace == "" {
		return eris.Errorf("must provide svc-namespace")
	}
	if opts.SvcName == "" {
		return eris.Errorf("must provide svc-name")
	}
	if opts.SecretNamespace == "" {
		return eris.Errorf("must provide secret-namespace")
	}
	if opts.SecretName == "" {
		return eris.Errorf("must provide secret-name")
	}
	if opts.ServerCertSecretFileName == "" {
		return eris.Errorf("must provide name for the server cert entry in the secret data")
	}
	if opts.ServerCertAuthorityFileName == "" {
		return eris.Errorf("must provide name for the cert authority entry in the secret data")
	}
	if opts.ServerKeySecretFileName == "" {
		return eris.Errorf("must provide name for the server key entry in the secret data")
	}
	certs, err := certgen.GenCerts(opts.SvcName, opts.SvcNamespace)
	if err != nil {
		return eris.Wrapf(err, "generating self-signed certs and key")
	}
	kubeClient := helpers.MustKubeClient()

	caCert := append(certs.ServerCertificate, certs.CaCertificate...)
	secretConfig := kube.TlsSecret{
		SecretName:         opts.SecretName,
		SecretNamespace:    opts.SecretNamespace,
		PrivateKeyFileName: opts.ServerKeySecretFileName,
		CertFileName:       opts.ServerCertSecretFileName,
		CaBundleFileName:   opts.ServerCertAuthorityFileName,
		PrivateKey:         certs.ServerCertKey,
		Cert:               caCert,
		CaBundle:           certs.CaCertificate,
	}

	if !opts.ForceRotation {
		existAndValid, err := kube.SecretExistsAndIsValidTlsSecret(ctx, kubeClient, secretConfig)
		if err != nil {
			return eris.Wrapf(err, "failed validating existing secret")
		}

		if existAndValid {
			contextutils.LoggerFrom(ctx).Infow("existing TLS secret found, skipping update to TLS secret and ValidatingWebhookConfiguration since the old TLS secret is still existAndValid",
				zap.String("secretName", secretConfig.SecretName),
				zap.String("secretNamespace", secretConfig.SecretNamespace))
			return nil
		}
	}

	if err := kube.CreateTlsSecret(ctx, kubeClient, secretConfig); err != nil {
		return eris.Wrapf(err, "failed creating secret")
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
		return eris.Wrapf(err, "failed patching validating webhook config")
	}

	contextutils.LoggerFrom(ctx).Infof("finished successfully.")

	return nil
}
