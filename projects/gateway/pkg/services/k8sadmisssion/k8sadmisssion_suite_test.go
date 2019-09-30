package k8sadmisssion_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestK8sadmisssion(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "K8sadmisssion Suite")
}
