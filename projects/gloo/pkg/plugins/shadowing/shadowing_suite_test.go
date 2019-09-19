package shadowing_test

import (
	"testing"

	"github.com/solo-io/go-utils/testutils"

	. "github.com/onsi/ginkgo"
)

func TestTracing(t *testing.T) {
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Shadowing Suite")
}
