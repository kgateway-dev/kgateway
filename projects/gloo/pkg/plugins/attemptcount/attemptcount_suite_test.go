package attemptcount_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestAttemptCount(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Attempt Count Suite")
}
