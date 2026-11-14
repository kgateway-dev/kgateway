//go:build e2e

package buffer

import (
	"context"
	"net/http"
	"strings"

	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, base.TestCase{}, testCases),
	}
}

// Keep this in sync with maxRequestSize in testdata/trafficpolicy.yaml.
const bufferLimit = 1024

func (s *testingSuite) TestBufferLimit() {
	for _, tc := range []struct {
		name           string
		hostname       string
		bodySize       int
		expectedStatus int
	}{
		{
			name:           "empty body",
			hostname:       "buffer.example.com",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "below limit",
			hostname:       "buffer.example.com",
			bodySize:       bufferLimit - 1,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "at limit",
			hostname:       "buffer.example.com",
			bodySize:       bufferLimit,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "above limit",
			hostname:       "buffer.example.com",
			bodySize:       bufferLimit + 1,
			expectedStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:           "untargeted route accepts oversized body",
			hostname:       "no-buffer.example.com",
			bodySize:       bufferLimit * 2,
			expectedStatus: http.StatusOK,
		},
	} {
		s.Run(tc.name, func() {
			common.BaseGateway.Send(
				s.T(),
				&testmatchers.HttpResponse{StatusCode: tc.expectedStatus},
				curl.WithMethod(http.MethodPost),
				curl.WithPath("/"),
				curl.WithHostHeader(tc.hostname),
				curl.WithPort(80),
				curl.WithBody(strings.Repeat("a", tc.bodySize)),
			)
		})
	}
}
