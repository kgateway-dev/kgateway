package regexutils_test

import (
	"context"

	"github.com/gogo/protobuf/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/solo-io/gloo/pkg/utils/regexutils"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

var _ = Describe("Regex", func() {
	It("should create regex with default program size", func() {
		regex := NewRegexWithProgramSize("foo", nil)
		Expect(regex.GetRegex()).To(Equal("foo"))
		Expect(regex.GetGoogleRe2().GetMaxProgramSize()).To(BeNil())

		regex = NewRegex(nil, "foo")
		Expect(regex.GetRegex()).To(Equal("foo"))
		Expect(regex.GetGoogleRe2().GetMaxProgramSize()).To(BeNil())
	})
	It("should create regex with a specific program size", func() {
		var number uint32
		number = 123
		regex := NewRegexWithProgramSize("foo", &number)
		Expect(regex.GetRegex()).To(Equal("foo"))
		Expect(regex.GetGoogleRe2().GetMaxProgramSize().GetValue()).To(Equal(number))
	})

	It("should create regex from settings in context", func() {
		ctx := settingsutil.WithSettings(context.Background(), &v1.Settings{
			Gloo: &v1.GlooOptions{RegexMaxProgramSize: &types.UInt32Value{Value: 123}},
		})
		regex := NewRegex(ctx, "foo")
		Expect(regex.GetRegex()).To(Equal("foo"))
		Expect(regex.GetGoogleRe2().GetMaxProgramSize().GetValue()).To(BeEquivalentTo(123))
	})
})
