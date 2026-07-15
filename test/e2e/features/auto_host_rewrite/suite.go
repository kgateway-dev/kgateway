//go:build e2e

package auto_host_rewrite

import (
	"context"
	"net/http"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite // makes the suite discoverable

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, ti *e2e.TestInstallation) suite.TestingSuite {
	setup := base.TestCase{
		Manifests: []string{defaults.HttpbinManifest, autoHostRewriteManifest},
	}
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, ti, setup, nil),
	}
}

/* ──────────────────────────── Test Cases ──────────────────────────── */

func (s *testingSuite) TestHostHeader() {
	// test basic route with autoHostRewrite
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			// `/headers` output should have `Host` header set with the DNS name of the service
			// due to autoHostRewrite=true
			Body: gomega.ContainSubstring("httpbin.default.svc"),
		},
		curl.WithPath("/headers"),
		curl.WithHostHeader("foo.local"),
		curl.WithPort(80),
	)

	// test specific rule with URLRewrite.hostname set, which overrides the autoHostRewrite from TrafficPolicy
	common.BaseGateway.Send(
		s.T(),
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			// `/headers` output should have `Host` header set to the urlRewrite.hostname value
			Body: gomega.ContainSubstring("foo.override"),
		},
		curl.WithPath("/headers-override"),
		curl.WithHostHeader("foo.local"),
		curl.WithPort(80),
	)
}
