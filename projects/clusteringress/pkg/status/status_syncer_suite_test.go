package status_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestStatusSyncer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "StatusSyncer Suite")
}
