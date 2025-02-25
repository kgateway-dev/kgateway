package matchers_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ = Describe("ContainMapElements", func() {

	DescribeTable("Map contains all Key/Value pairs",
		func(expectedMap map[string]string) {
			actualMap := map[string]string{
				"key1": "value1",
				"key2": "value2",
			}

			Expect(actualMap).To(matchers.ContainMapElements(expectedMap))
		},
		Entry("empty map", map[string]string{}),
		Entry("nil map", nil),
		Entry("all values in actual in expected", map[string]string{
			"key1": "value1",
			"key2": "value2",
		}),
		Entry("some values in actual in expected", map[string]string{
			"key1": "value1",
		}),
	)

	DescribeTable("Map does not contain all Key/Value pairs",
		func(expectedMap map[string]string) {
			actualMap := map[string]string{
				"key1": "value1",
				"key2": "value2",
			}

			Expect(actualMap).ToNot(matchers.ContainMapElements(expectedMap))
		},
		Entry("all values in actual in expected plus non-matching values", map[string]string{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		}),
		Entry("some values not in actual in expected", map[string]string{
			"key3": "value3",
		}),
		Entry("key in actual with non-matching value", map[string]string{
			"key1": "value3",
		}),
	)

	When("actual is nil", func() {
		var (
			actual map[string]any = nil
		)
		It("never matches when expected is nil", func() {
			var expected map[string]string = nil
			Expect(actual).ToNot(matchers.ContainMapElements(expected))
		})
		It("never matches when expected is empty", func() {
			expected := map[string]string{}
			Expect(actual).ToNot(matchers.ContainMapElements(expected))
		})
		It("never matches when expected is non-empty", func() {
			expected := map[string]string{"foo": "bar"}
			Expect(actual).ToNot(matchers.ContainMapElements(expected))
		})
	})
	When("actual is empty", func() {
		var (
			actual map[string]any = map[string]any{}
		)
		It("never matches when expected is nil", func() {
			var expected map[string]string = nil
			Expect(actual).ToNot(matchers.ContainMapElements(expected))
		})
		It("never matches when expected is empty", func() {
			expected := map[string]string{}
			Expect(actual).ToNot(matchers.ContainMapElements(expected))
		})
		It("never matches when expected is non-empty", func() {
			expected := map[string]string{"foo": "bar"}
			Expect(actual).ToNot(matchers.ContainMapElements(expected))
		})
	})
})

var _ = Describe("ContainsDeepMapElements", func() {
	DescribeTable("Map contains all Key/Value pairs",
		func(expectedMap map[string]any) {
			actualMap := map[string]any{
				"key1": "value1",
				"key2": "value2",
				"key3": map[string]any{
					"key4": "value4",
					"key5": "value5",
				},
			}

			Expect(actualMap).To(matchers.ContainsDeepMapElements(expectedMap))
		},
		Entry("empty map", map[string]any{}),
		Entry("nil map", nil),
		Entry("all values in actual in expected", map[string]any{
			"key1": "value1",
			"key2": "value2",
		}),
		Entry("some values in actual in expected", map[string]any{
			"key1": "value1",
		}),
		Entry("nested map", map[string]any{
			"key3": map[string]any{
				"key4": "value4",
				"key5": "value5",
			},
		}),
		Entry("nested map with some of the nested values", map[string]any{
			"key3": map[string]any{
				"key5": "value5",
			},
		}),
	)

	When("actual is nil", func() {
		var (
			actual map[string]any = nil
		)
		It("never matches when expected is nil", func() {
			var expected map[string]any = nil
			Expect(actual).ToNot(matchers.ContainsDeepMapElements(expected))
		})
		It("never matches when expected is empty", func() {
			expected := map[string]any{}
			Expect(actual).ToNot(matchers.ContainsDeepMapElements(expected))
		})
		It("never matches when expected is non-empty", func() {
			expected := map[string]any{"foo": "bar"}
			Expect(actual).ToNot(matchers.ContainsDeepMapElements(expected))
		})
	})

	When("actual is empty", func() {
		var (
			actual map[string]any = map[string]any{}
		)
		It("never matches when expected is nil", func() {
			var expected map[string]any = nil
			Expect(actual).ToNot(matchers.ContainsDeepMapElements(expected))
		})
		It("never matches when expected is empty", func() {
			expected := map[string]any{}
			Expect(actual).ToNot(matchers.ContainsDeepMapElements(expected))
		})
		It("never matches when expected is non-empty", func() {
			expected := map[string]any{"foo": "bar"}
			Expect(actual).ToNot(matchers.ContainsDeepMapElements(expected))
		})
	})
})
