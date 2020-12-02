package shadowing_test

import (
	"testing"

	"github.com/solo-io/k8s-utils/testutils"

	. "github.com/onsi/ginkgo"
)

func TestTracing(t *testing.T) {
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Shadowing Suite")
}
