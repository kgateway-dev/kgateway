package options

import (
	"github.com/solo-io/solo-kit/pkg/errors"
	"io/ioutil"
)

type Secret struct {
	TlsSecret   TlsSecret
	AwsSecret   AwsSecret
	AzureSecret AzureSecret
}

type AwsSecret struct {
	AccessKey string
	SecretKey string
}

type AzureSecret struct {
	ApiKeys InputMapStringString
}

type TlsSecret struct {
	RootCaFilename     string
	PrivateKeyFilename string
	CertChainFilename  string
	// non-user facing value for test purposes
	// if set, Read() will just return the filenames
	Mock bool
}

// ReadFiles provides a way to sidestep file io during testing
func (t *TlsSecret) ReadFiles() (string, string, string, error) {
	if t.Mock {
		return t.RootCaFilename, t.PrivateKeyFilename, t.CertChainFilename, nil
	}
	var rootCa []byte
	if t.RootCaFilename != "" {
		var err error
		rootCa, err = ioutil.ReadFile(t.RootCaFilename)
		if err != nil {
			return "", "", "", errors.Wrapf(err, "reading root ca file: %v", t.RootCaFilename)
		}
	}
	privateKey, err := ioutil.ReadFile(t.PrivateKeyFilename)
	if err != nil {
		return "", "", "", errors.Wrapf(err, "reading private key file: %v", t.PrivateKeyFilename)
	}
	certChain, err := ioutil.ReadFile(t.CertChainFilename)
	if err != nil {
		return "", "", "", errors.Wrapf(err, "reading cert chain file: %v", t.CertChainFilename)
	}
	return string(rootCa), string(privateKey), string(certChain), nil
}
