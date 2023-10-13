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

type Options struct {
	SvcName      string
	SvcNamespace string

	SecretName      string
	SecretNamespace string
	NextSecretName  string

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
	if opts.NextSecretName == "" {
		return eris.Errorf("must provide next-secret-name")
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
	// check if there is an existing valid TLS secret
	secret, renewCurrent, err := kube.GetExistingValidTlsSecret(ctx, kubeClient, opts.SecretName, opts.SecretNamespace,
		opts.SvcName, opts.SvcNamespace, renewBeforeDuration)
	if err != nil {
		return eris.Wrapf(err, "failed validating existing secret")
	}
	nextSecret, renewNext, err := kube.GetExistingValidTlsSecret(ctx, kubeClient, opts.NextSecretName, opts.SecretNamespace,
		opts.SvcName, opts.SvcNamespace, renewBeforeDuration)
	// If either secret is empty or invalid, generate two new secrets and save them.
	if secret == nil || nextSecret == nil {
		certs, err := certgen.GenCerts(opts.SvcName, opts.SvcNamespace)
		if err != nil {
			return eris.Wrapf(err, "failed creating secret")
		}
		nextCerts, err := certgen.GenCerts(opts.SvcName, opts.SvcNamespace)
		if err != nil {
			return eris.Wrapf(err, "failed creating next secret")
		}
		certs.CaCertificate = append(certs.CaCertificate, nextCerts.CaCertificate...)

		newSecretConfig := kube.TlsSecret{
			SecretName:         opts.SecretName,
			SecretNamespace:    opts.SecretNamespace,
			PrivateKeyFileName: opts.ServerKeySecretFileName,
			CertFileName:       opts.ServerCertSecretFileName,
			CaBundleFileName:   opts.ServerCertAuthorityFileName,
			PrivateKey:         certs.ServerCertKey,
			Cert:               certs.ServerCertificate,
			CaBundle:           certs.CaCertificate,
		}
		secret, err = kube.CreateTlsSecret(ctx, kubeClient, newSecretConfig)
		if err != nil {
			return eris.Wrapf(err, "error saving secret")
		}
		nextSecretConfig := kube.TlsSecret{
			SecretName:         opts.NextSecretName,
			SecretNamespace:    opts.SecretNamespace,
			PrivateKeyFileName: opts.ServerKeySecretFileName,
			CertFileName:       opts.ServerCertSecretFileName,
			CaBundleFileName:   opts.ServerCertAuthorityFileName,
			PrivateKey:         nextCerts.ServerCertKey,
			Cert:               nextCerts.ServerCertificate,
			CaBundle:           nextCerts.CaCertificate,
		}
		_, err = kube.CreateTlsSecret(ctx, kubeClient, nextSecretConfig)
		if err != nil {
			return eris.Wrapf(err, "error saving secret")
		}
		return persistWebhook(ctx, opts, kubeClient, secret)
	}
	// Rotate out the older cert and add a newer one
	if opts.ForceRotation || renewCurrent || renewNext {
		contextutils.LoggerFrom(ctx).Infow("Rotating secrets regardless of expiration")
		certs, err := certgen.GenCerts(opts.SvcName, opts.SvcNamespace)
		if err != nil {
			return eris.Wrapf(err, "generating self-signed certs and key")
		}
		nextSecretConfig := parseTlsSecret(nextSecret, opts.ServerKeySecretFileName, opts.ServerCertSecretFileName, opts.ServerCertAuthorityFileName)
		secretConfig := parseTlsSecret(secret, opts.ServerKeySecretFileName, opts.ServerCertSecretFileName, opts.ServerCertAuthorityFileName)
		caCert := append(certs.ServerCertificate, certs.CaCertificate...)
		newSecretConfig := kube.TlsSecret{
			SecretName:         opts.NextSecretName,
			SecretNamespace:    opts.SecretNamespace,
			PrivateKeyFileName: opts.ServerKeySecretFileName,
			CertFileName:       opts.ServerCertSecretFileName,
			CaBundleFileName:   opts.ServerCertAuthorityFileName,
			PrivateKey:         certs.ServerCertKey,
			Cert:               caCert,
			CaBundle:           certs.CaCertificate,
		}
		secret, err = kube.SwapSecrets(ctx, kubeClient, secretConfig, nextSecretConfig, newSecretConfig)
		if err != nil {
			return eris.Wrapf(err, "failed creating or rotating secret")
		}
		return persistWebhook(ctx, opts, kubeClient, secret)
	} else {
		contextutils.LoggerFrom(ctx).Infow("existing TLS secret found, skipping update to TLS secret since the old TLS secret is still valid",
			zap.String("secretName", opts.SecretName),
			zap.String("secretNamespace", opts.SecretNamespace))
	}

	return nil
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
func parseTlsSecret(args *v1.Secret, privateKeyFilename, certFileName, caBundleFileName string) kube.TlsSecret {
	return kube.TlsSecret{
		SecretName:         args.GetObjectMeta().GetName(),
		SecretNamespace:    args.GetObjectMeta().GetNamespace(),
		PrivateKeyFileName: privateKeyFilename,
		CertFileName:       certFileName,
		CaBundleFileName:   caBundleFileName,
		PrivateKey:         args.Data[privateKeyFilename],
		Cert:               args.Data[certFileName],
		CaBundle:           args.Data[caBundleFileName],
	}
}
