package kuberesource_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestKuberesource(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kuberesource Suite")
}
