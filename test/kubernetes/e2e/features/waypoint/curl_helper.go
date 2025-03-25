package waypoint

import (
	"fmt"
	"time"

	"knative.dev/pkg/network"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var (
	testAppPort = 8080

	fromCurl = kubectl.PodExecOptions{Name: "curl", Namespace: testNamespace, Container: "curl"}
	// TODO this will be used for testing AuthorizationPolicy
	// fromNotCurl = kubectl.PodExecOptions{Name: "notcurl", Namespace: testNamespace, Container: "notcurl"}
)

func (s *testingSuite) assertCurlService(
	from kubectl.PodExecOptions,
	svcName, svcNs string,
	matchers matchers.HttpResponse,
) {
	s.assertCurlInner(from, fqdn(svcName, svcNs), matchers, "")
}

func fqdn(name, ns string) string {
	// TODO: reevaluate knative dep, dedupe with pkg/utils/kubeutils/dns.go
	return fmt.Sprintf("%s.%s.svc.%s", name, ns, network.GetClusterDomainName())
}

func (s *testingSuite) assertCurlHost(
	from kubectl.PodExecOptions,
	targetHost string,
	matchers matchers.HttpResponse,
) {
	s.assertCurlInner(from, targetHost, matchers, "")
}

func (s *testingSuite) assertCurlInner(
	from kubectl.PodExecOptions,
	targetHost string,
	matchers matchers.HttpResponse,
	authHeader string,
) {
	curlOpts := []curl.Option{
		curl.WithHost(targetHost),
		curl.WithPort(testAppPort),
	}
	if authHeader != "" {
		curlOpts = append(curlOpts, curl.WithHeader("Authorization", authHeader))
	}

	// even though the next assert contains an Eventually, we can't adjust the timeout
	// for the initial successful request.20 seconds is going to cause flakes.
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		from,
		curlOpts,
		&matchers,
		time.Minute,
	)

	// now make sure we didn't get lucky by using consistently
	s.testInstallation.Assertions.AssertEventuallyConsistentCurlResponse(
		s.ctx,
		from,
		curlOpts,
		&matchers,
	)
}
