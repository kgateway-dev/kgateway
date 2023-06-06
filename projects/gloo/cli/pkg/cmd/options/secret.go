package options

import (
	"crypto/tls"
	"os"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"

	"github.com/solo-io/solo-kit/pkg/errors"
)

type Secret struct {
	TlsSecret    TlsSecret
	AwsSecret    AwsSecret
	AzureSecret  AzureSecret
	HeaderSecret HeaderSecret
}

type AwsSecret struct {
	AccessKey    string
	SecretKey    string
	SessionToken string
}

type AzureSecret struct {
	ApiKeys InputMapStringString
}

type HeaderSecret struct {
	Headers InputMapStringString
}

type TlsSecret struct {
	RootCaFilename     string
	PrivateKeyFilename string
	CertChainFilename  string
	OCSPStapleFilename string
	// non-user facing value for test purposes
	// if set, Read() will just return the filenames
	Mock bool
}

// ReadFiles provides a way to sidestep file io during testing
// It reads the files and returns a TlsSecret object which holds the file contents
func (t *TlsSecret) ReadFiles() (*gloov1.TlsSecret, error) {
	// short circuit if testing
	if t.Mock {
		return &gloov1.TlsSecret{
			RootCa:     t.RootCaFilename,
			PrivateKey: t.PrivateKeyFilename,
			CertChain:  t.CertChainFilename,
			OcspStaple: []byte(t.OCSPStapleFilename),
		}, nil
	}

	// ensure that the key pair is valid
	if err := t.validateKeyPairIfExists(); err != nil {
		return &gloov1.TlsSecret{}, errors.Wrapf(err, "invalid key pair (cert chain file: %v, private key file: %v)", t.CertChainFilename, t.PrivateKeyFilename)
	}

	// read files
	var rootCa []byte
	if t.RootCaFilename != "" {
		var err error
		rootCa, err = os.ReadFile(t.RootCaFilename)
		if err != nil {
			return &gloov1.TlsSecret{}, errors.Wrapf(err, "reading root ca file: %v", t.RootCaFilename)
		}
	}
	var ocspStaple []byte
	if t.OCSPStapleFilename != "" {
		var err error
		ocspStaple, err = os.ReadFile(t.OCSPStapleFilename)
		if err != nil {
			return &gloov1.TlsSecret{}, errors.Wrapf(err, "reading ocsp staple file: %v", t.OCSPStapleFilename)
		}
	}
	var privateKey []byte
	var certChain []byte
	if t.keyPairExists() {
		var err error
		privateKey, err = os.ReadFile(t.PrivateKeyFilename)
		if err != nil {
			return &gloov1.TlsSecret{}, errors.Wrapf(err, "reading private key file: %v", t.PrivateKeyFilename)
		}
		certChain, err = os.ReadFile(t.CertChainFilename)
		if err != nil {
			return &gloov1.TlsSecret{}, errors.Wrapf(err, "reading cert chain file: %v", t.CertChainFilename)
		}
	}

	return &gloov1.TlsSecret{
		RootCa:     string(rootCa),
		PrivateKey: string(privateKey),
		CertChain:  string(certChain),
		OcspStaple: ocspStaple,
	}, nil
}

func (t *TlsSecret) keyPairExists() bool {
	return !(t.CertChainFilename == "" && t.PrivateKeyFilename == "")
}

func (t *TlsSecret) validateKeyPairIfExists() error {
	if !t.keyPairExists() {
		return nil
	}
	if _, err := tls.LoadX509KeyPair(t.CertChainFilename, t.PrivateKeyFilename); err != nil {
		return err
	}
	return nil
}
