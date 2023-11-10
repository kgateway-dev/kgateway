package reports_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTranslator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Reporter Suite")
}
