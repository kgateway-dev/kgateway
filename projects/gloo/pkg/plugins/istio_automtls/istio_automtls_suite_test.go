package istio_automtls_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestIstioAutoMtls(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Istio Automtls Plugin Suite")
}
