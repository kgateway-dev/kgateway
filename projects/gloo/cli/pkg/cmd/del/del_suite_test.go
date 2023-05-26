package del_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDel(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Del Suite")
}
