package backendtls

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/onsi/gomega"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwv1a3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/backendtlspolicy"
	reports "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/reporter"
	"github.com/kgateway-dev/kgateway/v2/test/gomega/matchers"
	"github.com/kgateway-dev/kgateway/v2/test/helpers"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e/defaults"

	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
)

var (
	baseManifests = []string{
		filepath.Join(fsutils.MustGetThisDir(), "inputs/base.yaml"),
		filepath.Join(fsutils.MustGetThisDir(), "inputs/nginx.yaml"),
		defaults.CurlPodManifest,
	}
	configMapManifest = filepath.Join(fsutils.MustGetThisDir(), "inputs/configmap.yaml")

	proxyObjMeta = metav1.ObjectMeta{
		Name:      "gw",
		Namespace: "default",
	}
	proxyDeployment  = &appsv1.Deployment{ObjectMeta: proxyObjMeta}
	proxyService     = &corev1.Service{ObjectMeta: proxyObjMeta}
	backendTlsPolicy = &gwv1a3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-policy",
			Namespace: "default",
		},
	}
	configMap = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ca",
			Namespace: "default",
		},
	}
	nginxMeta = metav1.ObjectMeta{
		Name:      "nginx",
		Namespace: "default",
	}
	svcGroup = ""
	svcKind  = "Service"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

type clientTlsTestingSuite struct {
	suite.Suite
	ctx              context.Context
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &clientTlsTestingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *clientTlsTestingSuite) TestBackendTLSPolicyAndStatus() {
	s.T().Cleanup(func() {
		for _, manifest := range baseManifests {
			err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
			s.Require().NoError(err)
		}
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment, backendTlsPolicy)
	})

	toCreate := append(baseManifests, configMapManifest)
	for _, manifest := range toCreate {
		err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, manifest)
		s.Require().NoError(err)
	}

	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, proxyService, proxyDeployment, backendTlsPolicy, configMap)
	// TODO: make this a specific assertion to remove the need for c/p the label selector
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, defaults.CurlPod.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=curl",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, nginxMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gw",
	})

	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("example.com"),
			curl.WithPath("/"),
		},
		&matchers.HttpResponse{
			StatusCode: http.StatusOK,
			Body:       gomega.ContainSubstring(defaults.NginxResponse),
		},
	)

	s.assertPolicyStatus(metav1.Condition{
		Type:    string(gwv1a2.PolicyConditionAccepted),
		Status:  metav1.ConditionTrue,
		Reason:  string(gwv1a2.PolicyReasonAccepted),
		Message: reports.PolicyAcceptedAndAttachedMsg,
	})

	// delete configmap so we can assert status updates correctly
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, configMapManifest)
	s.Require().NoError(err)

	s.assertPolicyStatus(metav1.Condition{
		Type:    string(gwv1a2.PolicyConditionAccepted),
		Status:  metav1.ConditionFalse,
		Reason:  string(gwv1a2.PolicyReasonInvalid),
		Message: fmt.Sprintf("%s: default/ca", backendtlspolicy.ErrConfigMapNotFound),
	})
}

func (s *clientTlsTestingSuite) assertPolicyStatus(inCondition metav1.Condition) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	p := s.testInstallation.Assertions
	p.Gomega.Eventually(func(g gomega.Gomega) {
		tlsPol := &gwv1a3.BackendTLSPolicy{}
		objKey := client.ObjectKeyFromObject(backendTlsPolicy)
		err := s.testInstallation.ClusterContext.Client.Get(s.ctx, objKey, tlsPol)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get BackendTLSPolicy %s", objKey)

		g.Expect(tlsPol.Status.Ancestors).To(gomega.HaveLen(1), "ancestors didn't have length of 1")
		ancestor := tlsPol.Status.Ancestors[0]

		expectedAncestorRef := gwv1a2.ParentReference{
			Group:     (*gwv1.Group)(&svcGroup),
			Kind:      (*gwv1.Kind)(&svcKind),
			Namespace: ptr.To(gwv1.Namespace(nginxMeta.Namespace)),
			Name:      gwv1.ObjectName(nginxMeta.Name),
		}
		g.Expect(ancestor.AncestorRef).To(gomega.BeEquivalentTo(expectedAncestorRef))

		g.Expect(ancestor.Conditions).To(gomega.HaveLen(1), "ancestors conditions wasn't length of 1")
		cond := meta.FindStatusCondition(ancestor.Conditions, inCondition.Type)
		g.Expect(cond).NotTo(gomega.BeNil(), "policy should have accepted condition")
		g.Expect(cond.Status).To(gomega.Equal(inCondition.Status), "policy accepted condition should be true")
		g.Expect(cond.Reason).To(gomega.Equal(inCondition.Reason), "policy reason should be accepted")
		g.Expect(cond.Message).To(gomega.Equal(inCondition.Message))
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}
