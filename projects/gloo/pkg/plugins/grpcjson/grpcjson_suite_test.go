package grpcjson_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGRPCJson(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GrpcJson Suite")
}
