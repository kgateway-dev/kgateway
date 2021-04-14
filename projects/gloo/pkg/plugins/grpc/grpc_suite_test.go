package grpc

import (
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	"github.com/solo-io/go-utils/log"
)

func TestGrpc(t *testing.T) {
	RegisterFailHandler(Fail)
	log.DefaultOut = GinkgoWriter
	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "Grpc Suite", []Reporter{junitReporter})
}
