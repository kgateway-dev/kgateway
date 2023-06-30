package services_test

import (
	"testing"

	"github.com/solo-io/go-utils/testutils"

	. "github.com/onsi/ginkgo/v2"
)

func TestServices(t *testing.T) {
	testutils.RegisterCommonFailHandlers()
	RunSpecs(t, "Services Suite")
}
