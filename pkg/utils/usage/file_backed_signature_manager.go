package usage

import (
	"io/ioutil"
	"os"
	"path"

	"github.com/google/uuid"
	report_signature "github.com/solo-io/reporting-client/pkg/signature"
)

const (
	signatureFileName = "usage-signature"
	filePermissions   = 0644
)

type fileBackedSignatureManager struct {
	configDir string
}

var _ report_signature.SignatureManager = &fileBackedSignatureManager{}

// expects a path to the glooctl config dir, usually ~/.gloo
func NewFileBackedSignatureManager(configDir string) report_signature.SignatureManager {
	return &fileBackedSignatureManager{
		configDir: configDir,
	}
}

func (f *fileBackedSignatureManager) GetSignature() (string, error) {
	signatureFilePath := path.Join(f.configDir, signatureFileName)

	return f.getOrGenerateSignature(signatureFilePath)
}

func (f *fileBackedSignatureManager) getOrGenerateSignature(signatureFilePath string) (string, error) {
	if _, err := os.Stat(signatureFilePath); err != nil {
		return f.writeNewSignatureFile(signatureFilePath)
	}

	signatureBytes, err := ioutil.ReadFile(signatureFileName)
	if err != nil {
		return "", err
	}

	signature := string(signatureBytes)

	if signature == "" {
		return f.writeNewSignatureFile(signatureFilePath)
	}

	return signature, nil
}

// returns the generated signature
func (f *fileBackedSignatureManager) writeNewSignatureFile(signatureFilePath string) (string, error) {
	signature, err := f.generateSignature()
	if err != nil {
		return "", err
	}

	return signature, ioutil.WriteFile(signatureFilePath, []byte(signature), filePermissions)
}

func (f *fileBackedSignatureManager) generateSignature() (string, error) {
	newUuid, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}

	return newUuid.String(), nil
}
