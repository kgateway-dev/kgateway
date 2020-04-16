package regexutils

import (
	"context"

	envoy_type_matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher"
	wrappers "github.com/golang/protobuf/ptypes/wrappers"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
)

func NewRegex(ctx context.Context, regex string) *envoy_type_matcher.RegexMatcher {

	settings := settingsutil.MaybeFromContext(ctx)
	var programsize *uint32
	if settings != nil {
		if max_size := settings.GetGloo().GetRegexMaxProgramSize(); max_size != nil {
			programsize = &max_size.Value
		}
	}

	return NewRegexWithProgramSize(regex, programsize)
}

func NewRegexFromSettings(settings *v1.Settings, regex string) *envoy_type_matcher.RegexMatcher {
	var programsize *uint32

	if max_size := settings.GetGloo().GetRegexMaxProgramSize(); max_size != nil {
		programsize = &max_size.Value
	}

	return NewRegexWithProgramSize(regex, programsize)
}

func NewRegexWithProgramSize(regex string, programsize *uint32) *envoy_type_matcher.RegexMatcher {

	var maxProgramSize *wrappers.UInt32Value
	if programsize != nil {
		maxProgramSize = &wrappers.UInt32Value{
			Value: *programsize,
		}
	}

	return &envoy_type_matcher.RegexMatcher{
		EngineType: &envoy_type_matcher.RegexMatcher_GoogleRe2{GoogleRe2: &envoy_type_matcher.RegexMatcher_GoogleRE2{MaxProgramSize: maxProgramSize}},
		Regex:      regex,
	}
}
