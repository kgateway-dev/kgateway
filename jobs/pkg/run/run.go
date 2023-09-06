package run

import (
	"context"
	"time"

	"github.com/rotisserie/eris"
	"github.com/solo-io/gloo/jobs/pkg/certgen"
	"github.com/solo-io/gloo/jobs/pkg/kube"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	"github.com/solo-io/go-utils/contextutils"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
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

	RenewBefore string
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
	renewBeforeDuration, err := time.ParseDuration(opts.RenewBefore)
	if err != nil {
		return err
	}

	kubeClient := helpers.MustKubeClient()

	var secret *v1.Secret
	if !opts.ForceRotation {
		// check if there is an existing valid TLS secret
		secret, err = kube.GetExistingValidTlsSecret(ctx, kubeClient, opts.SecretName, opts.SecretNamespace,
			opts.SvcName, opts.SvcNamespace, renewBeforeDuration)
		if err != nil {
			return eris.Wrapf(err, "failed validating existing secret")
		}

		if secret != nil {
			contextutils.LoggerFrom(ctx).Infow("existing TLS secret found, skipping update to TLS secret since the old TLS secret is still valid",
				zap.String("secretName", opts.SecretName),
				zap.String("secretNamespace", opts.SecretNamespace))
		}
	}
	// if ForceRotation=true or there is no existing valid secret, generate one
	if secret == nil {
		certs, err := certgen.GenCerts(opts.SvcName, opts.SvcNamespace)
		if err != nil {
			return eris.Wrapf(err, "generating self-signed certs and key")
		}

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
		secret, err = kube.CreateTlsSecret(ctx, kubeClient, secretConfig)
		if err != nil {
			return eris.Wrapf(err, "failed creating secret")
		}
	}

	vwcName := opts.ValidatingWebhookConfigurationName
	if vwcName == "" {
		contextutils.LoggerFrom(ctx).Infof("no ValidatingWebhookConfiguration provided. finished successfully.")
		return nil
	}

	vwcConfig := kube.WebhookTlsConfig{
		ServiceName:      opts.SvcName,
		ServiceNamespace: opts.SvcNamespace,
		CaBundle:         secret.Data[opts.ServerCertAuthorityFileName],
	}

	if err := kube.UpdateValidatingWebhookConfigurationCaBundle(ctx, kubeClient, vwcName, vwcConfig); err != nil {
		return eris.Wrapf(err, "failed patching validating webhook config")
	}

	contextutils.LoggerFrom(ctx).Infof("finished successfully.")

	return nil
}
