package del_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDel(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Del Suite")
}
