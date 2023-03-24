package testutils_test

import (
	"github.com/solo-io/go-utils/testutils"
	"testing"

	. "github.com/onsi/ginkgo/v2"
)

func TestTestUtils(t *testing.T) {
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "TestUtils Suite")
}
