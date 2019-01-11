package clusteringress_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestClusteringress(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Clusteringress Suite")
}
