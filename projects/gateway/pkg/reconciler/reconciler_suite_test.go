package reconciler_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var test *testing.T

func TestReconciler(t *testing.T) {
	test = t
	RegisterFailHandler(Fail)
	RunSpecs(t, "Reconciler Suite")
}
