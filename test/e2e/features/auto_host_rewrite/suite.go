//go:build e2e

package auto_host_rewrite

import (
	"context"
	"net/http"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var _ e2e.NewSuiteFunc = NewTestingSuite // makes the suite discoverable

type testingSuite struct {
	suite.Suite

	ctx context.Context
	ti  *e2e.TestInstallation

	commonManifests []string
	commonResources []client.Object
}

func NewTestingSuite(ctx context.Context, ti *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:             ctx,
		ti:              ti,
		commonManifests: []string{defaults.HttpbinManifest, autoHostRewriteManifest},
		commonResources: []client.Object{route, trafficPolicy},
	}
}

/* ───────────────────────── Set-up / Tear-down ───────────────────────── */

func (s *testingSuite) SetupSuite() {
	for _, mf := range s.commonManifests {
		s.Require().NoError(
			s.ti.Actions.Kubectl().ApplyFile(s.ctx, mf),
			"apply "+mf,
		)
	}
	s.ti.AssertionsT(s.T()).EventuallyObjectsExist(s.ctx, s.commonResources...)

	// wait for httpbin to come up
	s.ti.AssertionsT(s.T()).EventuallyPodsRunning(s.ctx, defaults.HttpbinDeployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.HttpbinLabelSelector,
	})
}

func (s *testingSuite) TearDownSuite() {
	if testutils.ShouldSkipCleanup(s.T()) {
		return
	}
	for _, mf := range s.commonManifests {
		_ = s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, mf)
	}
	s.ti.AssertionsT(s.T()).EventuallyObjectsNotExist(s.ctx, s.commonResources...)
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
