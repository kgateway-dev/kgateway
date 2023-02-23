package grpcweb_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGrpcweb(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Grpcweb Suite")
}
