package assertions_test

import (
	"testing"

	"github.com/solo-io/go-utils/testutils"

	. "github.com/onsi/ginkgo/v2"
)

func TestAssertions(t *testing.T) {
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Assertions Suite")
}
