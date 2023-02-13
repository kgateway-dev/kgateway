package syncutil_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSyncUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sync Utils Suite")
}
