//go:build e2e

package quota

import (
	"context"
	"net/http"

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
		BaseTestingSuite: base.NewBaseTestingSuite(
			ctx,
			testInst,
			base.TestCase{Manifests: []string{rlqsServerManifest}},
			map[string]*base.TestCase{
				"TestRateLimitQuotaProtocol": {
					Manifests:       []string{quotaPolicyManifest},
					MinGwApiVersion: base.GwApiRequireRouteNames,
				},
			},
		),
	}
}

func (s *testingSuite) TestRateLimitQuotaProtocol() {
	// A missing dynamic bucket value bypasses RLQS instead of creating an
	// incomplete bucket.
	s.send("/deny", "", http.StatusOK)

	// The first request subscribes the bucket. Send retries until Envoy has
	// received and applied the server's DENY_ALL assignment.
	s.send("/deny", "tenant-a", http.StatusTooManyRequests)

	// The matcher must not leak the deny bucket onto another route.
	s.send("/allow", "tenant-a", http.StatusOK)
	s.send("/unscoped", "tenant-a", http.StatusOK)
}

func (s *testingSuite) send(path, tenant string, status int) {
	opts := []curl.Option{
		curl.WithPath(path),
		curl.WithHostHeader("example.com"),
		curl.WithPort(80),
	}
	if tenant != "" {
		opts = append(opts, curl.WithHeader("x-tenant-id", tenant))
	}
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{StatusCode: status},
		opts...,
	)
}
