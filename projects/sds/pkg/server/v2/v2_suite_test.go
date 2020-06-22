package sds_server_v2

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSDSServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SDS Server V2 Suite")
}
