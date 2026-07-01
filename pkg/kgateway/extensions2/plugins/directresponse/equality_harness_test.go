package directresponse

// equality_harness_test.go verifies that directResponse.Equals detects a change
// in every field and treats specs with equal content but distinct pointers as
// equal. The reflexivity subtest guards against comparing DirectResponseSpec's
// pointer fields (Body, BodyFormat) by identity instead of value.
//
// See test/testutils/equalstest for the harness API.

import (
	"testing"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/test/testutils/equalstest"
)

func baseHarnessDirectResponse() *directResponse {
	return &directResponse{
		spec: kgateway.DirectResponseSpec{
			StatusCode: 503,
			Body:       new("service unavailable"),
			BodyFormat: &kgateway.BodyFormat{
				ContentType: new("text/plain"),
				Text:        new("oops"),
			},
		},
	}
}

func TestHarnessDirectResponseEquals(t *testing.T) {
	cases := []equalstest.Case[*directResponse]{
		{
			Field:  "spec",
			Mutate: func(d **directResponse) { (*d).spec.StatusCode = 500 },
		},
		{
			Field:  "spec",
			Mutate: func(d **directResponse) { (*d).spec.Body = new("different body") },
		},
		{
			Field:  "spec",
			Mutate: func(d **directResponse) { (*d).spec.Body = nil },
		},
		{
			Field:  "spec",
			Mutate: func(d **directResponse) { (*d).spec.BodyFormat.ContentType = new("application/json") },
		},
		{
			Field:  "spec",
			Mutate: func(d **directResponse) { (*d).spec.BodyFormat = nil },
		},
	}

	equalstest.Run(
		t,
		baseHarnessDirectResponse,
		func(a, b *directResponse) bool { return a.Equals(b) },
		cases,
		[]string{"ct"}, // +noKrtEquals, direct_response_plugin.go
		equalstest.IncludeUnexported(),
	)
}
