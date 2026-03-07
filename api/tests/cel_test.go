package tests

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/test/crvalidation"
)

func TestCRDs(t *testing.T) {
	v := NewKgatewayValidator(t)
	crvalidation.TestCRValidation(t, v, "testdata")
}
