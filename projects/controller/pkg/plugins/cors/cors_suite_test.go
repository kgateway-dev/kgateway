package cors

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCors(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cors Suite")
}
