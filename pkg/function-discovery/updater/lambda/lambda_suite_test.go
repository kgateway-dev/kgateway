package lambda_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestLambda(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lambda Suite")
}
