package assertions

import (
	"fmt"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ExpectNumFields asserts that the given struct contains the given number of fields
func ExpectNumFields(obj interface{}, numFields int) {
	GinkgoHelper()
	Expect(reflect.TypeOf(obj).NumField()).To(
		Equal(numFields),
		fmt.Sprintf("wrong number of fields found in %T", obj),
	)
}
