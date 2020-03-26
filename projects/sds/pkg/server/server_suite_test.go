package server

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSDSServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SDS Server Suite")
}
