package stateful_session_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBasicRoute(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Stateful Session Suite")
}
