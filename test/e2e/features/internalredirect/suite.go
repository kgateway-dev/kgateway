//go:build e2e

package internalredirect

import (
	"context"
	"net/http"

	"github.com/onsi/gomega"
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

var testCases = map[string]*base.TestCase{
	"TestInternalRedirectFollowed":           {},
	"TestWithoutPolicyRedirectPassedThrough": {},
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// TestInternalRedirectFollowed verifies that when an upstream returns a 303,
// the gateway follows the redirect internally and returns the redirected
// endpoint's response.
func (s *testingSuite) TestInternalRedirectFollowed() {
	// httpbin's /redirect-to?url=<target>&status_code=303 returns a 303 with
	// Location pointing at <target>. The /anything endpoint echoes its path.
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring("/anything/internal-redirect"),
		},
		curl.WithPort(80),
		curl.WithPath("/redirect-to?url=http://www.example.com/anything/internal-redirect&status_code=303"),
		curl.WithHostHeader("www.example.com"),
	)
}

// TestWithoutPolicyRedirectPassedThrough verifies that without an
// internalRedirect policy, the 303 redirect is passed through to the client.
func (s *testingSuite) TestWithoutPolicyRedirectPassedThrough() {
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusSeeOther,
		},
		curl.WithPort(80),
		curl.WithPath("/redirect-to?url=http://no-policy.example.com/status/200&status_code=303"),
		curl.WithHostHeader("no-policy.example.com"),
	)
}
