package server_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSDSServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SDS Server Suite")
}
