//go:build e2e

package backends

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	"istio.io/istio/pkg/test/util/retry"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/e2e"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/common"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/defaults"
	"github.com/kgateway-dev/kgateway/v2/test/e2e/tests/base"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/testutils"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

var (
	baseManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "base.yaml")
	// backend in separate manifest to allow creation independently of routing config
	backendManifest       = filepath.Join(fsutils.MustGetThisDir(), "testdata", "backend.yaml")
	backendErrorManifest  = filepath.Join(fsutils.MustGetThisDir(), "testdata", "backend-error.yaml")
	backendUpdateManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "backend-update-error.yaml")
	priorityGroupManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata", "priority-groups.yaml")

	proxyObjMeta = metav1.ObjectMeta{
		Name:      "gateway",
		Namespace: "kgateway-base",
	}

	testCases = map[string]*base.TestCase{
		"TestConfigureBackingDestinationsWithUpstream": {
			Manifests: []string{baseManifest, backendManifest},
		},
		"TestBackendWithRuntimeError": {
			Manifests: []string{backendErrorManifest},
		},
		"TestPriorityGroupsFailover": {
			Manifests: []string{defaults.HttpEchoPodManifest, defaults.HttpbinManifest, priorityGroupManifest},
		},
	}
)

type testingSuite struct {
	*base.BaseTestingSuite
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		BaseTestingSuite: base.NewBaseTestingSuite(ctx, testInst, base.TestCase{}, testCases),
	}
}

func (s *testingSuite) TestConfigureBackingDestinationsWithUpstream() {
	backend := &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-static",
			Namespace: proxyObjMeta.GetNamespace(),
		},
	}

	common.BaseGateway.Send(
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(defaults.NginxResponse),
		},
		curl.WithHostHeader("example.com"),
		curl.WithPath("/"),
		curl.WithPort(80),
	)

	s.assertStatus(backend, metav1.Condition{
		Type:    "Accepted",
		Status:  metav1.ConditionTrue,
		Reason:  "Accepted",
		Message: "Backend accepted",
	})
}

// TestBackendWithRuntimeError tests if backend condition is updated with error
func (s *testingSuite) TestBackendWithRuntimeError() {
	backendWithError := &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-aws-backend",
			Namespace: proxyObjMeta.GetNamespace(),
		},
	}

	s.assertStatus(backendWithError, metav1.Condition{
		Type:    "Accepted",
		Status:  metav1.ConditionFalse,
		Reason:  "Invalid",
		Message: `Backend error: "Secret kgateway-base/lambda-secret not found"`,
	})

	// apply the secret mid-test: it must not exist during the first assertion,
	// and its unsupported key surfaces a different status update error
	testutils.Cleanup(s.T(), func() {
		err := s.TestInstallation.Actions.Kubectl().DeleteFileSafe(s.Ctx, backendUpdateManifest)
		s.Require().NoError(err)
	})

	err := s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, backendUpdateManifest)
	s.Require().NoError(err)

	s.assertStatus(backendWithError, metav1.Condition{
		Type:   "Accepted",
		Status: metav1.ConditionFalse,
		Reason: "Invalid",
		Message: `Backend error: "failed to create aws request signing config: failed to derive static secret: access_key is not a valid string
secret_key is not a valid string"`,
	})
}

// TestPriorityGroupsFailover verifies that a priority groups Backend sends
// traffic to the first group, fails over to the second group (whose members
// share a priority and are load balanced together) when the first group's
// backend dies, and recovers once it returns. Failover is driven by the
// active health check configured via BackendConfigPolicy: the referenced
// backends' endpoints are merged into the cluster's load assignment, so the
// health checks probe the real services directly.
func (s *testingSuite) TestPriorityGroupsFailover() {
	pgBackend := &kgateway.Backend{
		ObjectMeta: metav1.ObjectMeta{Name: "priority-groups", Namespace: proxyObjMeta.GetNamespace()},
	}

	testutils.Cleanup(s.T(), func() {
		// restore the shared nginx backend for the suites that run after this one
		s.Require().NoError(s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, defaults.NginxPodManifest))
		s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx, common.SharedNginxNamespace, metav1.ListOptions{
			LabelSelector: defaults.WellKnownAppLabel + "=nginx",
		})
	})

	// all backendRefs resolve
	s.assertStatus(pgBackend, metav1.Condition{
		Type:    "Accepted",
		Status:  metav1.ConditionTrue,
		Reason:  "Accepted",
		Message: "Backend accepted",
	})

	curlOpts := []curl.Option{
		curl.WithHostHeader("failover.example.com"),
		curl.WithPath("/"),
		curl.WithPort(80),
	}
	// the health check needs a couple of intervals to observe a state change,
	// and pod restarts add latency; give the eventual matches a generous window
	failoverRetry := []retry.Option{retry.Timeout(2 * time.Minute)}

	// group 0 (nginx) serves all traffic
	common.BaseGateway.SendEventuallyConsistent(
		s.Ctx,
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(defaults.NginxResponse),
		},
		curlOpts...,
	)

	// kill group 0 by deleting the shared nginx pod. Its Service keeps the
	// ClusterIP, so the priority-0 endpoint stays resolvable but the health
	// check starts failing.
	nginxPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "nginx", Namespace: common.SharedNginxNamespace},
	}
	s.Require().NoError(s.TestInstallation.ClusterContext.Client.Delete(s.Ctx, nginxPod))
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsNotExist(s.Ctx, common.SharedNginxNamespace, metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=nginx",
	})

	// traffic fails over to group 1 with no outage: every response is a 200.
	// Both group members share priority 1 and are load balanced together, so
	// both response signatures must be observed.
	common.BaseGateway.SendWithRetry(
		s.Ctx,
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring("http-echo"),
		},
		failoverRetry,
		curlOpts...,
	)
	common.BaseGateway.SendWithRetry(
		s.Ctx,
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring("httpbin"),
		},
		failoverRetry,
		curlOpts...,
	)

	// restore group 0; one passing health check marks it healthy again and
	// traffic drains back to the higher priority group
	s.Require().NoError(s.TestInstallation.Actions.Kubectl().ApplyFile(s.Ctx, defaults.NginxPodManifest))
	s.TestInstallation.AssertionsT(s.T()).EventuallyPodsRunning(s.Ctx, common.SharedNginxNamespace, metav1.ListOptions{
		LabelSelector: defaults.WellKnownAppLabel + "=nginx",
	})
	common.BaseGateway.SendEventuallyConsistent(
		s.Ctx,
		s.T(),
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(defaults.NginxResponse),
		},
		curlOpts...,
	)
}

func (s *testingSuite) assertStatus(backend *kgateway.Backend, expected metav1.Condition) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	p := s.TestInstallation.AssertionsT(s.T())
	p.Gomega.Eventually(func(g gomega.Gomega) {
		be := &kgateway.Backend{}
		objKey := client.ObjectKeyFromObject(backend)
		err := s.TestInstallation.ClusterContext.Client.Get(s.Ctx, objKey, be)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get Backend %s", objKey)

		actual := be.Status.Conditions
		g.Expect(actual).To(gomega.HaveLen(1), "condition should have length of 1")
		cond := meta.FindStatusCondition(actual, expected.Type)
		g.Expect(cond).NotTo(gomega.BeNil())
		g.Expect(cond.Status).To(gomega.Equal(expected.Status))
		g.Expect(cond.Reason).To(gomega.Equal(expected.Reason))
		g.Expect(cond.Message).To(gomega.Equal(expected.Message))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}
