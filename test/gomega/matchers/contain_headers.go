package matchers

import (
	"net/http"
	"net/textproto"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	"github.com/solo-io/gloo/test/gomega/transforms"
)

// ContainHeaders produces a matcher that will only match if all provided headers
// are completely accounted for, including multi-value headers.
func ContainHeaders(headers http.Header) types.GomegaMatcher {
	if headers == nil {
		// If no headers are defined, we create a matcher that always succeeds
		// If we do not this we will create an And matcher for 0 objects, which leads to a panic
		return gstruct.Ignore()
	}
	headerMatchers := make([]types.GomegaMatcher, 0, len(headers))
	for k, v := range headers {
		headerMatchers = append(headerMatchers, gomega.WithTransform(transforms.WithHeaderValues(k), gomega.ContainElements(v)))
	}
	return gomega.And(headerMatchers...)
}

// ConsistOfHeaders produces a matcher that will only match if all provided headers are completely accounted for, including multi-value headers.
// This matcher will fail if there are any extra headers that are not specified in the headers passed in.
func ConsistOfHeaders(headers http.Header) types.GomegaMatcher {
	if headers == nil {
		// If no headers are defined, we create a matcher that always succeeds
		// If we do not this we will create an And matcher for 0 objects, which leads to a panic
		return gstruct.Ignore()
	}
	headerMatchers := make([]types.GomegaMatcher, 0, len(headers))
	for k, v := range headers {
		headerMatchers = append(headerMatchers, gomega.WithTransform(transforms.WithHeaderValues(k), gomega.ConsistOf(v)))
	}
	return gomega.And(headerMatchers...)
}

// ContainHeaders produces a matcher that will only match if all provided header keys exist.
func ContainHeaderKeys(keys []string) types.GomegaMatcher {
	if len(keys) == 0 {
		// If no keys are defined, we create a matcher that always succeeds
		// If we do not this we will create an And matcher for 0 objects, which leads to a panic
		return gstruct.Ignore()
	}
	for i, key := range keys {
		keys[i] = textproto.CanonicalMIMEHeaderKey(key)
	}
	matcher := gomega.WithTransform(transforms.WithHeaderKeys(), gomega.ContainElements(keys))
	return gomega.And(matcher)
}
