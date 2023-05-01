package testutils_test

import (
	"testing"

	"github.com/solo-io/go-utils/testutils"

	. "github.com/onsi/ginkgo"
)

func TestTestUtils(t *testing.T) {
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "TestUtils Suite")
}
