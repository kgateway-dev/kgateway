package syncer

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/solo-io/solo-kit/test/helpers"
)

func TestSyncer(t *testing.T) {
	RegisterFailHandler(Fail)
	helpers.SetupLog()
	RunSpecs(t, "Syncer Suite")
}
