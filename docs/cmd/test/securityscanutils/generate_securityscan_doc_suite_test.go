package securityscanutils_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/test/gomega"
	"github.com/solo-io/go-utils/testutils"
)

func TestGenerateSecurityScanDoc(t *testing.T) {
	SetAsyncAssertionDefaults(AsyncAssertionDefaults{})
	RegisterFailHandler(Fail)

	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Generate Security Scan Docs Suite")
}
