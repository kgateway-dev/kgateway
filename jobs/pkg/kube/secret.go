package kube

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"time"

	"github.com/solo-io/gloo/jobs/pkg/certgen"

	errors "github.com/rotisserie/eris"
	"github.com/solo-io/go-utils/contextutils"
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
func GetExistingValidTlsSecret(ctx context.Context, kube kubernetes.Interface, secretName string, secretNamespace string,
	svcName string, svcNamespace string, renewBeforeDuration time.Duration) (*v1.Secret, bool, error) {

	logger := contextutils.LoggerFrom(ctx)
	logger.Infof("GetExistingValidTlsSecret " + secretName)
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
	logger.Infof("==new decode: %v, err=%v\n", len(decodedCerts), err)

	matchesSvc := false
	now := time.Now().UTC()
	for _, cert := range decodedCerts {
		// if any one of the certs is not currently valid, need to create a new secret
		if now.Before(cert.NotBefore) || now.After(cert.NotAfter.Add(-renewBeforeDuration)) {
			logger.Info("return 2 (cert not currently valid)")
			return nil, false, nil
		}

		// // if the cert is already expired or not yet valid, requests aren't working so don't try to use it while rotating
		// if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		// 	return nil, false, nil
		// }
		// // Create new certificate if old one is expiring soon
		// // If the old one is ok then we should use it while rotating
		// if now.After(cert.NotAfter.Add(-renewBeforeDuration)) {
		// 	return existing, true, nil
		// }

		// check if the cert is valid for this service
		logger.Infof("checking cert validity: dnsNames=%v, svcName=%s, svcNamespace=%s\n", cert.DNSNames, svcName, svcNamespace)
		if !matchesSvc && certgen.ValidForService(cert.DNSNames, svcName, svcNamespace) {
			matchesSvc = true
		}
	}

	// require at least one cert to match service
	if !matchesSvc {
		logger.Info("return 3 (does not match service)")
		return nil, false, nil
	}

	// cert is valid!
	logger.Info("return 4 (valid)")
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

// SwapSecrets by updating making sure that everything has all the right
// certs in the bundle. If it fails mid flight it should make our pods overly permissive.
// In these failure cases an extra rotation should be the maximal fix if in a bad state.
// ctx: context
// currentSecret: current secret (ca bundle both A B), this will be mutated throughout
// nextSecret: The currently unused secret with a CA supported by our current secret
// futureSecret: the future nextSecret. Used to update the ca bundle of the nextSecret once it's persisted
// returns the updated currentSecret or an error if something went wrong
func SwapSecrets(ctx context.Context, gracePeriod time.Duration, kube kubernetes.Interface, currentSecret, nextSecret, futureSecret TlsSecret) (*v1.Secret, error) {

	logger := contextutils.LoggerFrom(ctx)
	// initially, we have currentSecret with currentSecret server cert + caBundle from currentSecret + nextSecret
	secretClient := kube.CoreV1().Secrets(currentSecret.SecretNamespace)
	// Move the tls key/cert from nextSecret -> currentSecret
	currentSecret.Cert = nextSecret.Cert
	currentSecret.PrivateKey = nextSecret.PrivateKey
	secretToWrite := makeTlsSecret(currentSecret)
	_, err := secretClient.Update(ctx, secretToWrite, metav1.UpdateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed updating current private key")
	}

	// now we have written secret with (nextSecret server cert) + (caBundle from currentSecret + nextSecret)

	// wait for all pods to pick up above secret with both caBundles
	// wait for SDS
	logger.Infow("Wrote new cert, waiting to rotate CaBundles")

	ticker := time.NewTicker(1 * time.Second)
	end := time.Now().Add(gracePeriod)
	logger.Infof("Starting a graceperiod for all pods to settle: %v seconds remaining", int(time.Until(end).Seconds()))
	for {
		select {
		case <-ctx.Done():
			logger.Info("context cancelled, next rotation will not break trust consider rotating an extra time")
			goto AFTER
		case t := <-ticker.C:
			if t.After(end) {
				logger.Info("finished waiting for mtls to settle proceeding to break trust in original ca")
				goto AFTER
			}
			// find the remaining integer amount of seconds remaining
			secRemains := int(end.Sub(t).Seconds())
			if secRemains%5 == 0 {
				logger.Infof("%v seconds remaining remaining", secRemains)
			}
		}
	}
AFTER: // label to break out of the ticker loop

	// now we try to go to new servert cert + new caBundle

	// Now that every pod is using the key/cert from nextSecret, overwrite the CaBundle from currentSecret
	// DO_NOT_SUBMIT: This is how we can validate that the multi ca bundle works
	//currentSecret.CaBundle = append(append(currentSecret.CaBundle, nextSecret.CaBundle...), futureSecret.CaBundle...)

	// now we have new cert, caBundle = new + next
	currentSecret.CaBundle = append(nextSecret.CaBundle, futureSecret.CaBundle...)
	secretToWrite = makeTlsSecret(currentSecret)

	_, err = secretClient.Update(ctx, secretToWrite, metav1.UpdateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed updating caBundle")
	}

	logger.Infow("rotated out old CA bundle")
	//Put the new secret in
	// now we persist next cert, caBundle = new + next
	secretToWrite2 := makeTlsSecret(futureSecret)
	_, err = secretClient.Update(ctx, secretToWrite2, metav1.UpdateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed updating next secret")
	}

	return secretToWrite, nil
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
