package regexutils_test

import (
	"context"

	"github.com/golang/protobuf/ptypes/wrappers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/pkg/utils/regexutils"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	v32 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
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
			Gloo: &v1.GlooOptions{RegexMaxProgramSize: &wrappers.UInt32Value{Value: 123}},
		})
		regex := NewRegex(ctx, "foo")
		Expect(regex.GetRegex()).To(Equal("foo"))
		Expect(regex.GetGoogleRe2().GetMaxProgramSize().GetValue()).To(BeEquivalentTo(123))
	})
	It("should create regex even without engine", func() {
		ctx := settingsutil.WithSettings(context.Background(), &v1.Settings{
			Gloo: &v1.GlooOptions{RegexMaxProgramSize: &wrappers.UInt32Value{Value: 123}},
		})
		subPattern := v32.RegexMatcher{
			Regex: "(.*)",
		}
		in := v32.RegexMatchAndSubstitute{
			Substitution: "123",
			Pattern:      &subPattern,
		}
		out, err := ConvertRegexMatchAndSubstitute(ctx, &in)
		Expect(err).To(BeNil())
		Expect(out.Pattern.Regex).To(Equal(in.Pattern.Regex))
		Expect(out.Substitution).To(Equal(in.Substitution))
	})
})
