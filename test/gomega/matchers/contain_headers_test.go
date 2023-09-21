package matchers_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/solo-io/gloo/test/gomega/matchers"
)

var _ = Describe("ContainHeaders", func() {

	DescribeTable("HttpResponse contains headers",
		func(expectedHeaders http.Header) {
			actualHeaders := http.Header{}
			actualHeaders.Add("east", "east-1")
			actualHeaders.Add("east", "east-2")
			actualHeaders.Add("west", "west-1")
			actualHeaders.Add("west", "west-2")

			httpResponse := &http.Response{
				StatusCode: http.StatusOK,
				Header:     actualHeaders,
			}
			Expect(httpResponse).To(matchers.ContainHeaders(expectedHeaders))
		},
		Entry("empty headers", http.Header{}),
		Entry("nil headers", nil),
		Entry("subset of headers", http.Header{
			"east": []string{"east-1", "east-2"},
		}),
		Entry("multiple subset of headers", http.Header{
			"east": []string{"east-1"},
			"west": []string{"west-2"},
		}),
	)

	DescribeTable("HttpResponse does not contain headers",
		func(expectedHeaders http.Header) {
			actualHeaders := http.Header{}
			actualHeaders.Add("east", "east-1")
			actualHeaders.Add("east", "east-2")
			actualHeaders.Add("west", "west-1")
			actualHeaders.Add("west", "west-2")

			httpResponse := &http.Response{
				StatusCode: http.StatusOK,
				Header:     actualHeaders,
			}
			Expect(httpResponse).NotTo(matchers.ContainHeaders(expectedHeaders))
		},
		Entry("missing header", http.Header{
			"south": []string{""},
		}),
		Entry("empty header value", http.Header{
			"east": []string{""},
		}),
		Entry("missing header value", http.Header{
			"east": []string{"east-missing"},
		}),
	)

})

var _ = Describe("ConsistOfHeaders", func() {
	DescribeTable("HttpResponse contains headers",
		func(expectedHeaders http.Header, pass bool) {
			actualHeaders := http.Header{}
			actualHeaders.Add("east", "east-1")
			actualHeaders.Add("east", "east-2")
			actualHeaders.Add("west", "west-1")
			actualHeaders.Add("west", "west-2")

			httpResponse := &http.Response{
				StatusCode: http.StatusOK,
				Header:     actualHeaders,
			}

			match := matchers.ConsistOfHeaders(expectedHeaders)
			// if we expect the match to fail, we wrap the ConsistOfHeaders matcher in a Not matcher
			if !pass {
				match = Not(match)
			}

			Expect(httpResponse).To(match)
		},
		Entry("pass on empty headers", http.Header{}, true),
		Entry("pass on nil header", nil, true),
		Entry("pass if header with all values exists", http.Header{
			"east": []string{"east-1", "east-2"},
		}, true),
		Entry("fail when header values are missing", http.Header{
			"east": []string{"east-1"},
		}, false),
	)

	DescribeTable("HttpResponse does not contain headers",
		func(expectedHeaders http.Header) {
			actualHeaders := http.Header{}
			actualHeaders.Add("east", "east-1")
			actualHeaders.Add("east", "east-2")
			actualHeaders.Add("west", "west-1")
			actualHeaders.Add("west", "west-2")

			httpResponse := &http.Response{
				StatusCode: http.StatusOK,
				Header:     actualHeaders,
			}
			Expect(httpResponse).NotTo(matchers.ContainHeaders(expectedHeaders))
		},
		Entry("missing header", http.Header{
			"south": []string{""},
		}),
		Entry("empty header value", http.Header{
			"east": []string{""},
		}),
		Entry("missing header value", http.Header{
			"east": []string{"east-missing"},
		}),
	)
})
