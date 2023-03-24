package testutils_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/testutils"
	"reflect"
)

var _ = Describe("HttpClientBuilder", func() {

	It("will fail if the client builder has a new top level field", func() {
		// This test is important as it checks whether the client builder has a new top level field.
		// This should happen very rarely, and should be used as an indication that the `Clone` function
		// most likely needs to change to support this new field

		Expect(reflect.TypeOf(testutils.HttpClientBuilder{}).NumField()).To(
			Equal(4),
			"wrong number of fields found",
		)
	})

})
