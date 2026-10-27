//go:build e2e

package waypoint

import (
	"time"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils/kubectl"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var (
	testAppPort = 8080

	fromCurl = kubectl.PodExecOptions{Name: "curl", Namespace: testNamespace, Container: "curl"}
)

func (s *testingSuite) assertCurlService(
	from kubectl.PodExecOptions,
	svcName, svcNs string,
	matchers matchers.HttpResponse,
	path ...string, //nolint:unparam // The variadic params might cause false positive
) {
	s.assertCurlInner(from, fqdn(svcName, svcNs), "", matchers, "GET", path...)
}

func fqdn(name, ns string) string {
	return kubeutils.GetServiceHostname(name, ns)
}

func (s *testingSuite) assertCurlInner(
	from kubectl.PodExecOptions,
	targetHost string,
	hostHeader string,
	matchers matchers.HttpResponse,
	method string,
	path ...string,
) {
	curlOpts := []curl.Option{
		curl.WithHost(targetHost),
		curl.WithPort(testAppPort),
	}
	if hostHeader != "" {
		curlOpts = append(curlOpts, curl.WithHostHeader(hostHeader))
	}

	// keeping for the backward compatibility when method is not set in the test
	if len(method) > 0 && method != "" {
		curlOpts = append(curlOpts, curl.WithMethod(method))
	}

	// keeping for the backward compatibility when path is not set in the test
	if len(path) > 0 && path[0] != "" {
		curlOpts = append(curlOpts, curl.WithPath(path[0]))
	}

	// wait for 1 good response
	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		from,
		curlOpts,
		&matchers,
		time.Minute,
	)

	// then ensure it's consistently working
	s.testInstallation.AssertionsT(s.T()).AssertEventuallyConsistentCurlResponse(
		s.ctx,
		from,
		curlOpts,
		&matchers,
	)
}
