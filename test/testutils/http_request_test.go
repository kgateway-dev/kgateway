package testutils_test

import (
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/testutils"
)

var _ = Describe("HttpRequestBuilder", func() {

	It("will fail if the request builder has a new top level field", func() {
		// This test is important as it checks whether the request builder has a new top level field.
		// This should happen very rarely, and should be used as an indication that the `Clone` function
		// most likely needs to change to support this new field

		Expect(reflect.TypeOf(testutils.HttpRequestBuilder{}).NumField()).To(
			Equal(9),
			"wrong number of fields found",
		)
	})

})
