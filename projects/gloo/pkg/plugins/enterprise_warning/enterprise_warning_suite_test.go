package enterprise_warning_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEnterpriseWarning(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "EnterpriseWarning Suite")
}
