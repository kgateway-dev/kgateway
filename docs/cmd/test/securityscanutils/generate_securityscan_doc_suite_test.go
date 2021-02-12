package securityscanutils_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/go-utils/testutils"
)

func TestGenerateSecuritScanDoc(t *testing.T) {
	RegisterFailHandler(Fail)
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Generate Security Scan Docs Suite")
}
