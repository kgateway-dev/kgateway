package matchers_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ = Describe("JSON", func() {
	DescribeTable("JSONContains results",
		func(expected []byte, result bool) {
			actual := []byte(`{"a": "b", "d": {"e": 42}, "g": {"h": "i", "j": "k"}}`)

			if result {
				Expect(actual).Should(matchers.JSONContains(expected))
			} else {
				Expect(actual).ShouldNot(matchers.JSONContains(expected))
			}
		},
		Entry("empty map", []byte(`{}`), true),
		Entry("has a:b", []byte(`{"a": "b"}`), true),
		Entry("does not have a:c", []byte(`{"a": "c"}`), false),
		Entry("has d.e:f", []byte(`{"d": {"e": 42}}`), true),
		Entry("does not have d.e:bad", []byte(`{"d": {"e": "bad"}}`), false),
		Entry("has g.h:i", []byte(`{"g": {"h": "i"}}`), true),
		Entry("has g object", []byte(`{"g": {}}`), true),
		Entry("has g.j:k", []byte(`{"g": {"l": 9}}`), false),
	)
})
