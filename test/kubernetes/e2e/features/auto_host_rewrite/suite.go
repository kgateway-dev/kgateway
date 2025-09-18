package auto_host_rewrite

import (
	"context"
	"net/http"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	testmatchers "github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"
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
		commonManifests: []string{defaults.CurlPodManifest, defaults.HttpbinManifest, autoHostRewriteManifest},
		commonResources: []client.Object{proxyDeployment, proxyService, route, trafficPolicy},
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
	s.ti.Assertions.EventuallyObjectsExist(s.ctx, s.commonResources...)

	// wait for all pods to actually come up
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, defaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.CurlPodLabelSelector,
	})
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, defaults.HttpbinDeployment.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.HttpbinLabelSelector,
	})
	s.ti.Assertions.EventuallyPodsRunning(s.ctx, proxyObjectMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=" + proxyObjectMeta.GetName(),
	})
}
func (s *testingSuite) TearDownSuite() {
	for _, mf := range s.commonManifests {
		_ = s.ti.Actions.Kubectl().DeleteFileSafe(s.ctx, mf)
	}
	s.ti.Assertions.EventuallyObjectsNotExist(s.ctx, s.commonResources...)
}

/* ──────────────────────────── Test Cases ──────────────────────────── */

func (s *testingSuite) TestHostHeader() {
	// test basic route with autoHostRewrite
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithPath("/headers"),
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("foo.local"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			// `/headers` output should have `Host` header set with the DNS name of the service
			// due to autoHostRewrite=true
			Body: gomega.ContainSubstring("httpbin.default.svc"),
		},
	)

	// test specific rule with URLRewrite.hostname set, which overrides the autoHostRewrite from TrafficPolicy
	s.ti.Assertions.AssertEventuallyConsistentCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithPath("/headers-override"),
			curl.WithHost(kubeutils.ServiceFQDN(proxyObjectMeta)),
			curl.WithHostHeader("foo.local"),
			curl.WithPort(8080),
		},
		&testmatchers.HttpResponse{
			StatusCode: http.StatusOK,
			// `/headers` output should have `Host` header set to the urlRwrite.hostname value
			Body: gomega.ContainSubstring("foo.override"),
		},
	)
}
