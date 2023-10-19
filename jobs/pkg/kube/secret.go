package kube

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"time"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/jobs/pkg/certgen"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/k8s-utils/certutils"
	"go.uber.org/zap"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type TlsSecret struct {
	SecretName, SecretNamespace                        string
	PrivateKeyFileName, CertFileName, CaBundleFileName string
	PrivateKey, Cert, CaBundle                         []byte
}

// If there is a currently valid TLS secret with the given name and namespace, that is valid for the given
// service name/namespace, then return it. Otherwise return nil.
// The second return value is a bool indicating whether the secret is expiring soon (i.e. within the renewBeforeDuration).
func GetExistingValidTlsSecret(ctx context.Context, kube kubernetes.Interface, secretName string, secretNamespace string,
	svcName string, svcNamespace string, renewBeforeDuration time.Duration) (*v1.Secret, bool, error) {

	logger := contextutils.LoggerFrom(ctx)
	logger.Infow("GetExistingValidTlsSecret for secret",
		zap.String("secretName", secretName),
		zap.String("secretNamespace", secretNamespace))

	secretClient := kube.CoreV1().Secrets(secretNamespace)
	existing, err := secretClient.Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Warnw("failed to retrieve existing secret",
				zap.String("secretName", secretName),
				zap.String("secretNamespace", secretNamespace))
			// necessary to return no errors in this case so we don't short circuit certgen on the first run
			return nil, false, nil
		}
		return nil, false, errors.Wrapf(err, "failed to retrieve existing secret")
	}

	if existing.Type != v1.SecretTypeTLS {
		return nil, false, errors.Errorf("unexpected secret type, expected %s and got %s", v1.SecretTypeTLS, existing.Type)
	}

	// decode the server cert(s)
	certPemBytes := existing.Data[v1.TLSCertKey]
	decodedCerts, err := decodeCertChain(certPemBytes)
	if err != nil {
		return nil, false, errors.Wrapf(err, "failed to decode cert chain")
	}
	logger.Infof("found %v certs\n", len(decodedCerts))

	matchesSvc := false
	now := time.Now().UTC()
	for _, cert := range decodedCerts {
		// if the cert is already expired or not yet valid, requests aren't working so don't try to use it while rotating
		if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
			logger.Info("cert is expired or not yet valid")
			return nil, false, nil
		}

		// check if the cert is valid for this service
		certMatchesSvc := certgen.ValidForService(cert.DNSNames, svcName, svcNamespace)
		// if the cert is valid but expiring soon, then use it while rotating certs
		if certMatchesSvc && now.After(cert.NotAfter.Add(-renewBeforeDuration)) {
			logger.Info("cert is valid but expiring soon")
			return existing, true, nil
		}
		if certMatchesSvc {
			matchesSvc = true
		}
	}

	// require at least one cert to match service
	if !matchesSvc {
		logger.Infow("cert is not valid for given service",
			zap.String("svcName", svcName),
			zap.String("svcNamespace", svcNamespace))
		return nil, false, nil
	}

	// cert is valid!
	logger.Info("existing cert is valid!")
	return existing, false, nil
}

// Returns the created or updated secret
func CreateTlsSecret(ctx context.Context, kube kubernetes.Interface, secretCfg TlsSecret) (*v1.Secret, error) {
	secret := makeTlsSecret(secretCfg)

	secretClient := kube.CoreV1().Secrets(secret.Namespace)

	logger := contextutils.LoggerFrom(ctx)
	logger.Infow("creating TLS secret", zap.String("secret", secret.Name))

	createdSecret, err := secretClient.Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			logger.Infow("existing TLS secret found, attempting to update",
				zap.String("secretName", secret.Name),
				zap.String("secretNamespace", secret.Namespace))

			existing, err := secretClient.Get(ctx, secret.Name, metav1.GetOptions{})
			if err != nil {
				return nil, errors.Wrapf(err, "failed to retrieve existing secret after receiving AlreadyExists error on Create")
			}

			secret.ResourceVersion = existing.ResourceVersion

			updatedSecret, err := secretClient.Update(ctx, secret, metav1.UpdateOptions{})
			if err != nil {
				return nil, errors.Wrapf(err, "failed updating existing secret")
			}
			return updatedSecret, nil
		}
		return nil, errors.Wrapf(err, "failed creating secret")
	}

	return createdSecret, nil
}

// RotateSecrets rotates secrets in a few steps.
//
// We start with:
//   - The current secret (currentTlsSecret) which will be rotated out. It initially
//     contains the current server cert/key and ca bundle.
//   - The newly generated certs (nextCerts) which we will switch over to.
//
// The update is done in the following order:
//  1. Set current secret's ca bundle to the current + next ca bundle (so both CAs are accepted temporarily)
//  2. Wait for the change to propagate
//  3. Set the current secret's server cert and private key to those of the newly generated certs
//  4. Wait for the change to propagate
//  5. Set the current secret's ca bundle to the next ca bundle. Now it contains only the next server
//     cert and next ca bundle and the old ones are no longer supported.
func RotateSecrets(ctx context.Context,
	kubeClient kubernetes.Interface,
	currentTlsSecret TlsSecret,
	nextCerts *certutils.Certificates,
	gracePeriod time.Duration) (*v1.Secret, error) {

	logger := contextutils.LoggerFrom(ctx)
	logger.Infow("rotating secret", zap.String("secretName", currentTlsSecret.SecretName), zap.String("secretNamespace", currentTlsSecret.SecretNamespace))

	secretClient := kubeClient.CoreV1().Secrets(currentTlsSecret.SecretNamespace)

	// set secret's caBundle to the combination of current ca + next ca, and persist changes
	currentTlsSecret.CaBundle = append(currentTlsSecret.CaBundle, nextCerts.CaCertificate...)
	secretToWrite := makeTlsSecret(currentTlsSecret)
	logger.Info("updating to both ca bundles")
	_, err := secretClient.Update(ctx, secretToWrite, metav1.UpdateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed updating to both ca bundles")
	}

	// wait for pods to pick up the ca bundle change
	logger.Info("waiting for ca bundle changes to be picked up")
	waitGracePeriod(ctx, gracePeriod)

	// set serverCert to next and persist secret
	currentTlsSecret.Cert = nextCerts.ServerCertificate
	currentTlsSecret.PrivateKey = nextCerts.ServerCertKey
	secretToWrite = makeTlsSecret(currentTlsSecret)
	logger.Info("updating to new server cert")
	_, err = secretClient.Update(ctx, secretToWrite, metav1.UpdateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed updating to new server cert")
	}

	// wait for pods to pick up the server cert change
	logger.Info("waiting for server cert changes to be picked up")
	waitGracePeriod(ctx, gracePeriod)

	// set currentSecret's caBundle to next (now currentSecret contains only next ca and next serverCert) and persist currentSecret
	currentTlsSecret.CaBundle = nextCerts.CaCertificate
	secretToWrite = makeTlsSecret(currentTlsSecret)
	logger.Info("updating to new ca bundle")
	_, err = secretClient.Update(ctx, secretToWrite, metav1.UpdateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed updating to new ca bundle")
	}

	// return the updated secret
	logger.Info("secret has been updated")
	return secretToWrite, nil
}

func waitGracePeriod(ctx context.Context, gracePeriod time.Duration) {
	logger := contextutils.LoggerFrom(ctx)
	ticker := time.NewTicker(1 * time.Second)
	end := time.Now().Add(gracePeriod)
	logger.Infof("Starting a grace period for all pods to settle: %v seconds remaining", int(time.Until(end).Seconds()))
	for {
		select {
		case <-ctx.Done():
			logger.Info("context cancelled, next rotation will not break trust, consider rotating an extra time")
			return
		case t := <-ticker.C:
			if t.After(end) {
				logger.Info("finished waiting for pods to settle")
				return
			}
			// find the remaining integer amount of seconds remaining
			secRemains := int(end.Sub(t).Seconds())
			if secRemains%5 == 0 {
				logger.Infof("%v seconds remaining", secRemains)
			}
		}
	}
}

func makeTlsSecret(args TlsSecret) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      args.SecretName,
			Namespace: args.SecretNamespace,
		},
		Type: v1.SecretTypeTLS,
		Data: map[string][]byte{
			args.PrivateKeyFileName: args.PrivateKey,
			args.CertFileName:       args.Cert,
			args.CaBundleFileName:   args.CaBundle,
		},
	}
}

func decodeCertChain(chain []byte) ([]*x509.Certificate, error) {
	var rootDecoded []byte
	rest := chain
	for {
		var pemBlock *pem.Block
		pemBlock, rest = pem.Decode(rest)
		if pemBlock == nil {
			break
		}
		rootDecoded = append(rootDecoded, pemBlock.Bytes...)
	}

	return x509.ParseCertificates(rootDecoded)
}
