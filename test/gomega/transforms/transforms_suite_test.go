package transforms_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/solo-io/go-utils/testutils"
)

func TestTransforms(t *testing.T) {
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Transforms Suite")
}
