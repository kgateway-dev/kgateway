//go:build e2e

package dualstack

import (
	"context"
	"net/http"
	"net/netip"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	setup = base.TestCase{
		Manifests: []string{defaults.HttpbinManifest, defaults.CurlPodManifest},
	}

	// test cases
	testCases = map[string]*base.TestCase{
		"TestDualStackService": {
			Manifests: []string{gatewayDualStackService},
		},
	}
)

// testingSuite covers the Service IP-family plumbing, which only has anything to
// assert on a cluster configured with both families. It has its own e2e matrix
// entry (`cluster-dual-stack`) for that reason: every other cluster in the matrix
// is single-stack.
type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, setup, testCases),
	}
}

// SetupSuite skips the whole suite on a single-stack cluster before any manifest
// is applied. A RequireDualStack Service is rejected outright there, so the proxy
// would never be provisioned and the base implementation would sit waiting for
// resources that cannot appear.
func (s *testingSuite) SetupSuite() {
	if !s.clusterHasBothIPFamilies() {
		s.T().Skip("cluster is not dual-stack; re-run against a cluster created with IP_FAMILY=dual")
	}
	s.BaseTestingSuite.SetupSuite()
}

// clusterHasBothIPFamilies reports whether the cluster is configured for IPv4 and
// IPv6, read off the default kubernetes Service: a single-stack cluster lists one
// family there, a dual-stack cluster both.
func (s *testingSuite) clusterHasBothIPFamilies() bool {
	var svc corev1.Service
	err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx,
		client.ObjectKey{Name: "kubernetes", Namespace: metav1.NamespaceDefault}, &svc)
	s.Require().NoError(err, "failed to read the default kubernetes Service to detect the cluster's IP families")
	return len(svc.Spec.IPFamilies) > 1
}

// TestDualStackService exercises the Service ipFamilies/ipFamilyPolicy fields on
// a live dual-stack cluster: the proxy Service comes up with a cluster IP of each
// family in the configured order, MetalLB's dual-family pool gives it a
// LoadBalancer address of each family, and requests reach the backend over both
// of its cluster IPs.
func (s *testingSuite) TestDualStackService() {
	s.TestInstallation.AssertionsT(s.T()).EventuallyReadyReplicas(s.Ctx, proxyObjectMeta, gomega.Equal(1))

	proxySvcKey := client.ObjectKey{Name: proxyObjectMeta.Name, Namespace: proxyObjectMeta.Namespace}

	var svc corev1.Service
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, proxySvcKey, &svc)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "the proxy Service should exist")

		g.Expect(svc.Spec.IPFamilyPolicy).NotTo(gomega.BeNil(),
			"ipFamilyPolicy should be set on the proxy Service")
		g.Expect(*svc.Spec.IPFamilyPolicy).To(gomega.Equal(corev1.IPFamilyPolicyRequireDualStack))
		// The configured order, not the cluster's default order: ipFamilies[0]
		// decides which family the primary cluster IP comes from.
		g.Expect(svc.Spec.IPFamilies).To(gomega.Equal([]corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}),
			"the proxy Service should carry the configured families, in order")

		v4, v6 := partitionIPsByFamily(svc.Spec.ClusterIPs)
		g.Expect(v4).To(gomega.HaveLen(1), "expected one IPv4 cluster IP, got %v", svc.Spec.ClusterIPs)
		g.Expect(v6).To(gomega.HaveLen(1), "expected one IPv6 cluster IP, got %v", svc.Spec.ClusterIPs)
	}).
		WithContext(s.Ctx).
		WithTimeout(time.Minute).
		WithPolling(time.Second).
		Should(gomega.Succeed())

	// MetalLB only assigns an address per family when a single pool holds a range
	// of each, which is what the dual branch of hack/kind/setup-metalllb-on-kind.sh
	// builds. Nothing else in the e2e matrix covers that branch.
	s.TestInstallation.AssertionsT(s.T()).Gomega.Eventually(func(g gomega.Gomega) {
		var lbSvc corev1.Service
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, proxySvcKey, &lbSvc)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "the proxy Service should exist")

		var assigned []string
		for _, ingress := range lbSvc.Status.LoadBalancer.Ingress {
			if ingress.IP != "" {
				assigned = append(assigned, ingress.IP)
			}
		}
		v4, v6 := partitionIPsByFamily(assigned)
		g.Expect(v4).NotTo(gomega.BeEmpty(),
			"the LoadBalancer should get an IPv4 address, got %v", assigned)
		g.Expect(v6).NotTo(gomega.BeEmpty(),
			"the LoadBalancer should get an IPv6 address, got %v", assigned)
	}).
		WithContext(s.Ctx).
		WithTimeout(2 * time.Minute).
		WithPolling(2 * time.Second).
		Should(gomega.Succeed())

	// Both legs of the Service should actually carry traffic. The IPv6 one is the
	// interesting half: the proxy has to accept connections on a family its own
	// bootstrap listeners bind, and the curl URL has to bracket the literal.
	v4ClusterIPs, v6ClusterIPs := partitionIPsByFamily(svc.Spec.ClusterIPs)
	for _, clusterIP := range []string{v4ClusterIPs[0], v6ClusterIPs[0]} {
		s.TestInstallation.AssertionsT(s.T()).AssertEventualCurlResponse(
			s.Ctx,
			defaults.CurlPodExecOpt,
			[]curl.Option{
				curl.WithHost(clusterIP),
				curl.WithPort(8080),
				curl.WithHostHeader("example.com"),
				curl.WithPath("/status/200"),
			},
			&matchers.HttpResponse{StatusCode: http.StatusOK},
			time.Minute,
		)
	}
}

// partitionIPsByFamily splits addresses by IP family, dropping anything that does
// not parse as an address.
func partitionIPsByFamily(addrs []string) (v4, v6 []string) {
	for _, addr := range addrs {
		ip, err := netip.ParseAddr(addr)
		if err != nil {
			continue
		}
		if ip.Is4() || ip.Is4In6() {
			v4 = append(v4, addr)
		} else {
			v6 = append(v6, addr)
		}
	}
	return v4, v6
}
