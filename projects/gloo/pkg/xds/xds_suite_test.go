package xds_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestXds(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Xds Suite")
}
