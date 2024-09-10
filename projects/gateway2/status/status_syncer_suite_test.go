package status

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestStatusSyncer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Status Syncer Suite")
}
