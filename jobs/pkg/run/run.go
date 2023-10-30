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
	"k8s.io/client-go/kubernetes"
)

// Options to configure the rotation job
// Certgen job yaml files have opinionated defaults for these options
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

	// The duration waited after first updating a secret's cabundle before
	// updating the actual secrets.
	// Lower values make the rotation job run faster
	// Higher values make the rotation job more resilient to errors
	RotationDuration string
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
	rotationDuration, err := time.ParseDuration(opts.RotationDuration)
	if err != nil {
		return err
	}

	logger := contextutils.LoggerFrom(ctx)
	logger.Info("starting certgen job, validated inputs")

	kubeClient := helpers.MustKubeClient()

	// check if there is an existing valid TLS secret
	secret, expiringSoon, err := kube.GetExistingValidTlsSecret(ctx, kubeClient, opts.SecretName, opts.SecretNamespace,
		opts.SvcName, opts.SvcNamespace, renewBeforeDuration)
	if err != nil {
		return eris.Wrapf(err, "failed validating existing secret")
	}

	if secret == nil {
		logger.Info("no existing valid secret, generating a new one...")
		// generate a new one
		certs, err := certgen.GenCerts(opts.SvcName, opts.SvcNamespace)
		if err != nil {
			return eris.Wrapf(err, "generating self-signed certs and key")
		}

		secretConfig := kube.TlsSecret{
			SecretName:         opts.SecretName,
			SecretNamespace:    opts.SecretNamespace,
			PrivateKeyFileName: opts.ServerKeySecretFileName,
			CertFileName:       opts.ServerCertSecretFileName,
			CaBundleFileName:   opts.ServerCertAuthorityFileName,
			PrivateKey:         certs.ServerCertKey,
			Cert:               certs.ServerCertificate,
			CaBundle:           certs.CaCertificate,
		}
		secret, err = kube.CreateTlsSecret(ctx, kubeClient, secretConfig)
		if err != nil {
			return eris.Wrapf(err, "failed creating secret")
		}
	} else if expiringSoon || opts.ForceRotation {
		logger.Info("secret exists but need to rotate...")
		// current secret
		tlsSecret := parseTlsSecret(secret, opts)
		// newly generated certs to rotate in
		nextCerts, err := certgen.GenCerts(opts.SvcName, opts.SvcNamespace)
		if err != nil {
			return eris.Wrapf(err, "generating self-signed certs and key")
		}
		secret, err = kube.RotateCerts(ctx, kubeClient, tlsSecret, nextCerts, rotationDuration)
		if err != nil {
			return eris.Wrapf(err, "failed to rotate certs")
		}
	} else {
		// cert is still good
		contextutils.LoggerFrom(ctx).Infow("existing TLS secret found, skipping update to TLS secret since the old TLS secret is still valid",
			zap.String("secretName", opts.SecretName),
			zap.String("secretNamespace", opts.SecretNamespace))
	}

	return persistWebhook(ctx, opts, kubeClient, secret)
}

func persistWebhook(ctx context.Context, opts Options, kubeClient kubernetes.Interface, secret *v1.Secret) error {
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

// construct a TlsSecret from an existing Secret
func parseTlsSecret(secret *v1.Secret, opts Options) kube.TlsSecret {
	return kube.TlsSecret{
		SecretName:         secret.GetObjectMeta().GetName(),
		SecretNamespace:    secret.GetObjectMeta().GetNamespace(),
		PrivateKeyFileName: opts.ServerKeySecretFileName,
		CertFileName:       opts.ServerCertSecretFileName,
		CaBundleFileName:   opts.ServerCertAuthorityFileName,
		PrivateKey:         secret.Data[opts.ServerKeySecretFileName],
		Cert:               secret.Data[opts.ServerCertSecretFileName],
		CaBundle:           secret.Data[opts.ServerCertAuthorityFileName],
	}
}
