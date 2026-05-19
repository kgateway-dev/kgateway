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

	fromCurl    = kubectl.PodExecOptions{Name: "curl", Namespace: testNamespace, Container: "curl"}
	fromNotCurl = kubectl.PodExecOptions{Name: "notcurl", Namespace: testNamespace, Container: "notcurl"}
)

func (s *testingSuite) assertCurlService(
	from kubectl.PodExecOptions,
	svcName, svcNs string,
	matchers matchers.HttpResponse,
	path ...string, //nolint:unparam // The variadic params might cause false positive
) {
	s.assertCurlInner(from, fqdn(svcName, svcNs), "", matchers, "GET", path...)
}

func (s *testingSuite) assertStableCurlService(
	from kubectl.PodExecOptions,
	svcName, svcNs string, //nolint:unparam // current callers all use "svc-a"/testNamespace, but kept parameterized to match the non-stable variant
	matchers matchers.HttpResponse,
	path ...string, //nolint:unparam // The variadic params might cause false positive
) {
	s.assertStableCurlInner(from, fqdn(svcName, svcNs), "", matchers, "GET", path...)
}

// assertCurlServicePost is a helper function to assert a POST request to a service
func (s *testingSuite) assertCurlServicePost(
	from kubectl.PodExecOptions,
	svcName, svcNs string,
	matchers matchers.HttpResponse,
	path ...string, //nolint:unparam // The variadic params might cause false positive
) {
	s.assertCurlInner(from, fqdn(svcName, svcNs), "", matchers, "POST", path...)
}

func fqdn(name, ns string) string {
	return kubeutils.GetServiceHostname(name, ns)
}

func (s *testingSuite) assertCurlHost(
	from kubectl.PodExecOptions,
	targetHost string,
	matchers matchers.HttpResponse,
	path ...string, //nolint:unparam // The variadic params might cause false positive
) {
	s.assertCurlInner(from, targetHost, "", matchers, "GET", path...)
}

func (s *testingSuite) assertStableCurlHost(
	from kubectl.PodExecOptions,
	targetHost string, //nolint:unparam // current callers all use "se-a.serviceentry.com", but kept parameterized to match the non-stable variant
	matchers matchers.HttpResponse,
	path ...string, //nolint:unparam // The variadic params might cause false positive
) {
	s.assertStableCurlInner(from, targetHost, "", matchers, "GET", path...)
}

// assertCurlHostPost is a helper function to assert a POST request to a host
func (s *testingSuite) assertCurlHostPost(
	from kubectl.PodExecOptions,
	targetHost string,
	matchers matchers.HttpResponse,
	path ...string, //nolint:unparam // The variadic params might cause false positive
) {
	s.assertCurlInner(from, targetHost, "", matchers, "POST", path...)
}

func (s *testingSuite) assertCurlInner(
	from kubectl.PodExecOptions,
	targetHost string,
	hostHeader string, //nolint:unparam // hostHeader is wired through buildCurlOptions for future use; current callers all pass ""
	matchers matchers.HttpResponse,
	method string,
	path ...string,
) {
	curlOpts := buildCurlOptions(targetHost, hostHeader, method, path...)

	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		from,
		curlOpts,
		&matchers,
	)
}

func (s *testingSuite) assertStableCurlInner(
	from kubectl.PodExecOptions,
	targetHost string,
	hostHeader string,
	matchers matchers.HttpResponse,
	method string,
	path ...string,
) {
	curlOpts := buildCurlOptions(targetHost, hostHeader, method, path...)

	// Waypoint dataplane startup can briefly return the right response before
	// becoming stable, especially around AuthorizationPolicy setup.
	s.testInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
		s.ctx,
		from,
		curlOpts,
		&matchers,
		time.Minute,
	)
	s.testInstallation.AssertionsT(s.T()).AssertEventuallyConsistentCurlResponse(
		s.ctx,
		from,
		curlOpts,
		&matchers,
	)
}

func buildCurlOptions(
	targetHost string,
	hostHeader string,
	method string,
	path ...string,
) []curl.Option {
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

	return curlOpts
}

// assertCurlGeneric is added to unify testing of mutlivalued iterating tests
func (s *testingSuite) assertCurlGeneric(
	from kubectl.PodExecOptions,
	svc, method, path string,
	expected matchers.HttpResponse,
) {
	s.assertCurlInner(from, fqdn(svc, testNamespace), "", expected, method, path)
}

func (s *testingSuite) assertStableCurlGeneric(
	from kubectl.PodExecOptions,
	svc, method, path string,
	expected matchers.HttpResponse,
) {
	s.assertStableCurlInner(from, fqdn(svc, testNamespace), "", expected, method, path)
}
