package license

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLicense(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "License Suite")
}
