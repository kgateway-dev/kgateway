package server_tls

import (
	"context"
	"time"

	gatewayv1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/defaults"
	"github.com/solo-io/gloo/test/gomega/matchers"
	"github.com/solo-io/gloo/test/kube2e/helper"
	testdefaults "github.com/solo-io/gloo/test/kubernetes/e2e/defaults"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/solo-io/gloo/pkg/utils/kubeutils"
	"github.com/solo-io/gloo/pkg/utils/requestutils/curl"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// serverTlsTestingSuite is the entire Suite of tests for gloo gateway proxy serving terminated TLS.
// The assertions in these tests assume that the warnMissingTlsSecret setting is "false"
type serverTlsTestingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation

	// ns is the namespace in which the feature suite is being executed.
	ns string

	tlsSecret1, tlsSecret2, tlsSecretWithCa []byte
	vs1, vs2, vsWithOneWay, vsWithoutOneWay []byte
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &serverTlsTestingSuite{
		ctx:              ctx,
		testInstallation: testInst,
		ns:               testInst.Metadata.InstallNamespace,
	}
}

func (s *serverTlsTestingSuite) SetupSuite() {
	var err error
	// These functions each substitute our namespace for placeholders in the given manifest
	// file via os.ExpandEnv in order to place our referenced resources in our NS.
	s.tlsSecret1, err = tlsSecret1Manifest()
	s.NoError(err, "can substitute NS in tlsSecret1")
	s.tlsSecret2, err = tlsSecret2Manifest()
	s.NoError(err, "can substitute NS in tlsSecret2")
	s.tlsSecretWithCa, err = tlsSecretWithCaManifest()
	s.NoError(err, "can substitute NS in tlsSecretWithCa")
	s.vs1, err = vs1Manifest()
	s.NoError(err, "can substitute NS in vs1")
	s.vs2, err = vs2Manifest()
	s.NoError(err, "can substitute NS in vs2")
	s.vsWithOneWay, err = vsWithOneWayManifest()
	s.NoError(err, "can substitute NS in vsWithOneWay")
	s.vsWithoutOneWay, err = vsWithoutOneWayManifest()
	s.NoError(err, "can substitute NS in vsWithoutOneWay")

	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can apply Curl setup manifest")

}

func (s *serverTlsTestingSuite) TearDownSuite() {
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, testdefaults.CurlPodManifest)
	s.NoError(err, "can delete Curl setup manifest")
}

// TestOneVirtualService validates the happy path that a VirtualService referencing an existent TLS secret
// terminates TLS and responds appropriately.
func (s *serverTlsTestingSuite) TestOneVirtualService() {
	vs1 := vs1(s.ns)
	s.T().Cleanup(func() {
		// ordering here matters if strict validation enabled
		err := s.testInstallation.Actions.Kubectl().Delete(s.ctx, s.vs1, "-n", s.ns)
		s.NoError(err, "can delete vs1 manifest file")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, vs1)
		s.eventuallyNotInSnapshot(gatewayv1.VirtualServiceGVK, vs1.ObjectMeta)
		err = s.testInstallation.Actions.Kubectl().Delete(s.ctx, s.tlsSecret1)
		s.NoError(err, "can delete tls secret manifest file")
		// tlsSecret2 is deleted in the process of the test.
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, tlsSecret1(s.ns))
	})

	// ordering here matters if strict validation enabled
	err := s.testInstallation.Actions.Kubectl().Apply(s.ctx, s.tlsSecret1)
	s.NoError(err, "can apply tls secret 1 manifest file")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, tlsSecret1(s.ns))
	s.eventuallyInSnapshot(coreSecretGVK, tlsSecret1(s.ns).ObjectMeta)
	err = s.testInstallation.Actions.Kubectl().Apply(s.ctx, s.vs1, "-n", s.ns)
	s.NoError(err, "can apply vs1 manifest file")

	// Assert that we have traffic working on our VS.
	s.assertEventualResponse(vs1.GetName(), expectedHealthyResponse1)
}

// TestTwoVirtualServices validates the happy path that two VirtualServices referencing existent TLS secrets
// terminate TLS and respond appropriately.
func (s *serverTlsTestingSuite) TestTwoVirtualServices() {
	vs1 := vs1(s.ns)
	vs2 := vs2(s.ns)
	s.T().Cleanup(func() {
		// ordering here matters if strict validation enabled
		err := s.testInstallation.Actions.Kubectl().Delete(s.ctx, s.vs1, "-n", s.ns)
		s.NoError(err, "can delete vs1 manifest file")
		err = s.testInstallation.Actions.Kubectl().Delete(s.ctx, s.vs2, "-n", s.ns)
		s.NoError(err, "can delete vs2 manifest file")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, vs1, vs2)
		s.eventuallyNotInSnapshot(gatewayv1.VirtualServiceGVK, vs1.ObjectMeta)
		s.eventuallyNotInSnapshot(gatewayv1.VirtualServiceGVK, vs2.ObjectMeta)
		err = s.testInstallation.Actions.Kubectl().Delete(s.ctx, s.tlsSecret1)
		s.NoError(err, "can delete tls secret 1 manifest file")
		err = s.testInstallation.Actions.Kubectl().Delete(s.ctx, s.tlsSecret2)
		s.NoError(err, "can delete tls secret 2 manifest file")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, tlsSecret1(s.ns), tlsSecret2(s.ns))
	})

	// ordering here matters if strict validation enabled
	err := s.testInstallation.Actions.Kubectl().Apply(s.ctx, s.tlsSecret1)
	s.NoError(err, "can apply tls secret 1 manifest file")
	err = s.testInstallation.Actions.Kubectl().Apply(s.ctx, s.tlsSecret2)
	s.NoError(err, "can apply tls secret 2 manifest file")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, tlsSecret1(s.ns), tlsSecret2(s.ns))
	s.eventuallyInSnapshot(coreSecretGVK, tlsSecret1(s.ns).ObjectMeta)
	s.eventuallyInSnapshot(coreSecretGVK, tlsSecret2(s.ns).ObjectMeta)
	err = s.testInstallation.Actions.Kubectl().Apply(s.ctx, s.vs1, "-n", s.ns)
	s.NoError(err, "can apply vs1 manifest file")
	err = s.testInstallation.Actions.Kubectl().Apply(s.ctx, s.vs2, "-n", s.ns)
	s.NoError(err, "can apply vs2 manifest file")

	// Assert that we have traffic working on both VS.
	s.assertEventualResponse(vs1.GetName(), expectedHealthyResponse1)
	s.assertEventualResponse(vs2.GetName(), expectedHealthyResponse2)
}

// TestTwoVirtualServicesOneMissingTlsSecret validates that when we have two VirtualServices referencing TLS
// secrets, but one secret is missing, the traffic routing defined by the other VirtualService is not affected. In order
// to test this properly, we require persistProxySpec to be off, validating that both VS are working correctly,
// then we delete the secret for one of the VS and restart the Gloo pod. This ensures that we are still
// serving on the other VS.
func (s *serverTlsTestingSuite) TestTwoVirtualServicesOneMissingTlsSecret() {
	vs1 := vs1(s.ns)
	vs2 := vs2(s.ns)
	s.T().Cleanup(func() {
		// ordering here matters if strict validation enabled
		err := s.testInstallation.Actions.Kubectl().Delete(s.ctx, s.vs1, "-n", s.ns)
		s.NoError(err, "can delete vs1 manifest file")
		err = s.testInstallation.Actions.Kubectl().Delete(s.ctx, s.vs2, "-n", s.ns)
		s.NoError(err, "can delete vs2 manifest file")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, vs1, vs2)
		s.eventuallyNotInSnapshot(gatewayv1.VirtualServiceGVK, vs1.ObjectMeta)
		s.eventuallyNotInSnapshot(gatewayv1.VirtualServiceGVK, vs2.ObjectMeta)
		err = s.testInstallation.Actions.Kubectl().Delete(s.ctx, s.tlsSecret1)
		s.NoError(err, "can delete tls secret manifest file")
		// tlsSecret2 is deleted in the process of the test.
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, tlsSecret1(s.ns), tlsSecret2(s.ns))
	})

	// ordering here matters if strict validation enabled
	err := s.testInstallation.Actions.Kubectl().Apply(s.ctx, s.tlsSecret1)
	s.NoError(err, "can apply tls secret 1 manifest file")
	err = s.testInstallation.Actions.Kubectl().Apply(s.ctx, s.tlsSecret2)
	s.NoError(err, "can apply tls secret 2 manifest file")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, tlsSecret1(s.ns), tlsSecret2(s.ns))
	s.eventuallyInSnapshot(coreSecretGVK, tlsSecret1(s.ns).ObjectMeta)
	s.eventuallyInSnapshot(coreSecretGVK, tlsSecret2(s.ns).ObjectMeta)
	err = s.testInstallation.Actions.Kubectl().Apply(s.ctx, s.vs1, "-n", s.ns)
	s.NoError(err, "can apply vs1 manifest file")
	err = s.testInstallation.Actions.Kubectl().Apply(s.ctx, s.vs2, "-n", s.ns)
	s.NoError(err, "can apply vs2 manifest file")

	// Assert that we have traffic working on both VS.
	s.assertEventualResponse(vs1.GetName(), expectedHealthyResponse1)
	s.assertEventualResponse(vs2.GetName(), expectedHealthyResponse2)

	// Delete the secret referenced by VS 2.
	err = s.testInstallation.Actions.Kubectl().Delete(s.ctx, s.tlsSecret2)
	s.NoError(err, "can delete tls secret 2 manifest file")
	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, tlsSecret2(s.ns))
	s.eventuallyNotInSnapshot(coreSecretGVK, tlsSecret2(s.ns).ObjectMeta)

	// Assert that we have traffic working on VS 1 but failed traffic on VS 2.
	s.assertEventualResponse(vs1.GetName(), expectedHealthyResponse1)
	s.assertEventualError(vs2.GetName(), expectedFailedResponseCertRequested)

	// Restart the Gloo deployment
	err = s.testInstallation.Actions.Kubectl().RestartDeploymentAndWait(s.ctx, "gloo", "-n", s.ns)
	s.NoError(err, "can restart gloo deployment")

	timeout, polling := helper.GetTimeouts(time.Second*10, time.Second)

	// Assert that we have traffic working on VS 1 but failed traffic on VS 2.
	s.assertEventuallyConsistentResponse(vs1.GetName(), expectedHealthyResponse1, timeout, polling)
	s.assertEventualError(vs2.GetName(), expectedFailedResponseCertRequested)

}

// TestOneWayServerTlsFailsWithoutOneWayTls validates that one-way server TLS traffic fails when CA data
// is provided in the TLS secret. This is because the Gloo translation loop assumes that mTLS is desired
// if the secret contains a CA cert.
func (s *serverTlsTestingSuite) TestOneWayServerTlsFailsWithoutOneWayTls() {
	vs := vsWithoutOneWay(s.ns)
	s.T().Cleanup(func() {
		// ordering here matters if strict validation enabled
		err := s.testInstallation.Actions.Kubectl().Delete(s.ctx, s.vsWithoutOneWay, "-n", s.ns)
		s.NoError(err, "can delete vs manifest file")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, vs)
		s.eventuallyNotInSnapshot(gatewayv1.VirtualServiceGVK, vs.ObjectMeta)
		err = s.testInstallation.Actions.Kubectl().Delete(s.ctx, s.tlsSecretWithCa)
		s.NoError(err, "can delete tls secret manifest file")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, tlsSecretWithCa(s.ns))
	})

	// ordering here matters if strict validation enabled
	err := s.testInstallation.Actions.Kubectl().Apply(s.ctx, s.tlsSecretWithCa)
	s.NoError(err, "can apply tls secret manifest file")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, tlsSecretWithCa(s.ns))
	s.eventuallyInSnapshot(coreSecretGVK, tlsSecretWithCa(s.ns).ObjectMeta)
	err = s.testInstallation.Actions.Kubectl().Apply(s.ctx, s.vsWithoutOneWay, "-n", s.ns)
	s.NoError(err, "can apply vs manifest file")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, vsWithoutOneWay(s.ns))

	s.assertEventualError(vs.GetName(), expectedFailedResponseCodeInvalidVs)
}

// TestOneWayServerTlsWorksWithOneWayTls validates that one-way server TLS traffic succeeds when CA data
// is provided in the TLS secret IF oneWayTls is set on the sslConfig. This is because the Gloo translation
// loop assumes that mTLS is desired if the secret contains a CA cert unless oneWayTls is set.
func (s *serverTlsTestingSuite) TestOneWayServerTlsWorksWithOneWayTls() {
	vs := vsWithOneWay(s.ns)
	s.T().Cleanup(func() {
		// ordering here matters if strict validation enabled
		err := s.testInstallation.Actions.Kubectl().Delete(s.ctx, s.vsWithOneWay, "-n", s.ns)
		s.NoError(err, "can delete vs manifest file")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, vs)
		s.eventuallyNotInSnapshot(gatewayv1.VirtualServiceGVK, vs.ObjectMeta)
		err = s.testInstallation.Actions.Kubectl().Delete(s.ctx, s.tlsSecretWithCa)
		s.NoError(err, "can delete tls secret manifest file")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, tlsSecretWithCa(s.ns))
	})

	// ordering here matters if strict validation enabled
	err := s.testInstallation.Actions.Kubectl().Apply(s.ctx, s.tlsSecretWithCa)
	s.NoError(err, "can apply tls secret manifest file")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, tlsSecretWithCa(s.ns))
	s.eventuallyInSnapshot(coreSecretGVK, tlsSecretWithCa(s.ns).ObjectMeta)
	err = s.testInstallation.Actions.Kubectl().Apply(s.ctx, s.vsWithOneWay, "-n", s.ns)
	s.NoError(err, "can apply vs manifest file")

	s.assertEventualResponse(vs.GetName(), expectedHealthyResponseWithOneWay)
}

func curlOptions(ns, hostHeaderValue string) []curl.Option {
	return []curl.Option{
		curl.WithHost(kubeutils.ServiceFQDN(metav1.ObjectMeta{Name: defaults.GatewayProxyName, Namespace: ns})),
		// The host header must match the domain in the VirtualService
		curl.WithHostHeader(hostHeaderValue),
		curl.WithPort(443),
		curl.IgnoreServerCert(),
		curl.WithScheme("https"),
		curl.WithSni(hostHeaderValue),
	}
}

func (s *serverTlsTestingSuite) assertEventualResponse(hostHeaderValue string, matcher *matchers.HttpResponse) {

	// Make sure our proxy pod is running
	listOpts := metav1.ListOptions{
		LabelSelector: "gloo=gateway-proxy",
	}
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, s.testInstallation.Metadata.InstallNamespace, listOpts, time.Minute*2)

	// Check curl works against expected response
	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		curlOptions(s.ns, hostHeaderValue),
		matcher)
}

func (s *serverTlsTestingSuite) assertEventuallyConsistentResponse(hostHeaderValue string, matcher *matchers.HttpResponse, timeouts ...time.Duration) {

	// Make sure our proxy pod is running
	listOpts := metav1.ListOptions{
		LabelSelector: "gloo=gateway-proxy",
	}
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, s.testInstallation.Metadata.InstallNamespace, listOpts, time.Minute*2)

	// Check curl works against expected response
	s.testInstallation.Assertions.AssertEventuallyConsistentCurlResponse(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		curlOptions(s.ns, hostHeaderValue),
		matcher,
		timeouts...)
}

func (s *serverTlsTestingSuite) assertEventualError(hostHeaderValue string, code int) {

	// Make sure our proxy pod is running
	listOpts := metav1.ListOptions{
		LabelSelector: "gloo=gateway-proxy",
	}
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, s.testInstallation.Metadata.InstallNamespace, listOpts, time.Minute*2)

	// Check curl works against expected response
	s.testInstallation.Assertions.AssertEventualCurlError(
		s.ctx,
		testdefaults.CurlPodExecOpt,
		curlOptions(s.ns, hostHeaderValue),
		code)
}

func (s *serverTlsTestingSuite) eventuallyInSnapshot(gvk schema.GroupVersionKind, meta metav1.ObjectMeta) {
	s.testInstallation.Assertions.AssertGlooAdminApi(
		s.ctx,
		metav1.ObjectMeta{
			Name:      kubeutils.GlooDeploymentName,
			Namespace: s.testInstallation.Metadata.InstallNamespace,
		},
		s.testInstallation.Assertions.InputSnapshotContainsElement(gvk, meta),
	)
}
func (s *serverTlsTestingSuite) eventuallyNotInSnapshot(gvk schema.GroupVersionKind, meta metav1.ObjectMeta) {
	s.testInstallation.Assertions.AssertGlooAdminApi(
		s.ctx,
		metav1.ObjectMeta{
			Name:      kubeutils.GlooDeploymentName,
			Namespace: s.testInstallation.Metadata.InstallNamespace,
		},
		s.testInstallation.Assertions.InputSnapshotDoesNotContainElement(gvk, meta),
	)
}
