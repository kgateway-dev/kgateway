package regexutils

import (
	"context"

	"github.com/solo-io/solo-kit/pkg/errors"

	envoy_type_matcher_v3 "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	wrappers "github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	v32 "github.com/solo-io/gloo/projects/gloo/pkg/api/external/envoy/type/matcher/v3"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

func NewRegex(ctx context.Context, regex string) *envoy_type_matcher_v3.RegexMatcher {
	settings := settingsutil.MaybeFromContext(ctx)
	return NewRegexFromSettings(settings, regex)
}

func NewRegexFromSettings(settings *v1.Settings, regex string) *envoy_type_matcher_v3.RegexMatcher {
	var programsize *uint32
	if settings != nil {
		if max_size := settings.GetGloo().GetRegexMaxProgramSize(); max_size != nil {
			programsize = &max_size.Value
		}
	}
	return NewRegexWithProgramSize(regex, programsize)
}

func NewRegexWithProgramSize(regex string, programsize *uint32) *envoy_type_matcher_v3.RegexMatcher {

	var maxProgramSize *wrappers.UInt32Value
	if programsize != nil {
		maxProgramSize = &wrappers.UInt32Value{
			Value: *programsize,
		}
	}

	return &envoy_type_matcher_v3.RegexMatcher{
		EngineType: &envoy_type_matcher_v3.RegexMatcher_GoogleRe2{
			GoogleRe2: &envoy_type_matcher_v3.RegexMatcher_GoogleRE2{MaxProgramSize: maxProgramSize},
		},
		Regex: regex,
	}
}

// ConvertRegexMatchAndSubstitute into safe variant consumable by envoy.
// By default we use the RegexMatcher_GoogleRe2 matcher which as of
// envoy 1.21 is the only engine supported.
func ConvertRegexMatchAndSubstitute(ctx context.Context, in *v32.RegexMatchAndSubstitute) (*envoy_type_matcher_v3.RegexMatchAndSubstitute, error) {
	if in == nil {
		return nil, nil
	}

	out := &envoy_type_matcher_v3.RegexMatchAndSubstitute{
		Pattern:      NewRegex(ctx, in.GetPattern().GetRegex()),
		Substitution: in.GetSubstitution(),
	}
	switch inET := in.GetPattern().GetEngineType().(type) {
	case nil:
		// we do nothing here as it defaults to googleRe2 but we dont have any of the extra settings on it.
	case *v32.RegexMatcher_GoogleRe2:
		// note that this is likely to be deprecated soon anyways... So if engines no longer matter we may eventually
		// be able to remove most of this.
		outET := out.GetPattern().GetEngineType().(*envoy_type_matcher_v3.RegexMatcher_GoogleRe2)
		if inET.GoogleRe2.GetMaxProgramSize() != nil && (outET.GoogleRe2.GetMaxProgramSize() == nil || inET.GoogleRe2.GetMaxProgramSize().GetValue() < outET.GoogleRe2.GetMaxProgramSize().GetValue()) {
			out.Pattern = NewRegexWithProgramSize(in.GetPattern().GetRegex(), &inET.GoogleRe2.GetMaxProgramSize().Value)
		}

	default:
		// this will only happen if there is a new type of engine that our current envoy implementation does not know how to handle.
		// This should be thrown as we are unsure that we are passing the right info to envoy which could cause crashes.
		return nil, errors.Errorf("Invalid regex EngineType: %v", in.GetPattern().GetEngineType())
	}

	return out, nil
}
