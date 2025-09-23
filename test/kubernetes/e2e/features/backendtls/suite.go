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

	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/kubeutils"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/requestutils/curl"
	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
)

var (
	configMapManifest = filepath.Join(fsutils.MustGetThisDir(), "testdata/configmap.yaml")

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
	nginx2Meta = metav1.ObjectMeta{
		Name:      "nginx2",
		Namespace: "default",
	}
	svcGroup = ""
	svcKind  = "Service"
)

type tsuite struct {
	suite.Suite
	ctx              context.Context
	testInstallation *e2e.TestInstallation
	baseManifests    []string
	agentgateway     bool
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &tsuite{
		ctx:              ctx,
		testInstallation: testInst,
		baseManifests: []string{
			filepath.Join(fsutils.MustGetThisDir(), "testdata/base.yaml"),
			filepath.Join(fsutils.MustGetThisDir(), "testdata/nginx.yaml"),
			defaults.CurlPodManifest,
		},
	}
}

func NewAgentgatewayTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &tsuite{
		ctx:              ctx,
		testInstallation: testInst,
		agentgateway:     true,
		baseManifests: []string{
			filepath.Join(fsutils.MustGetThisDir(), "testdata/base-agw.yaml"),
			filepath.Join(fsutils.MustGetThisDir(), "testdata/nginx.yaml"),
			defaults.CurlPodManifest,
		},
	}
}

func (s *tsuite) TestBackendTLSPolicyAndStatus() {
	s.T().Cleanup(func() {
		for _, manifest := range s.baseManifests {
			err := s.testInstallation.Actions.Kubectl().DeleteFileSafe(s.ctx, manifest)
			s.Require().NoError(err)
		}
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, proxyService, proxyDeployment, backendTlsPolicy)
	})

	toCreate := append(s.baseManifests, configMapManifest)
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
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, nginx2Meta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=nginx2",
	})
	s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, proxyObjMeta.GetNamespace(), metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=gw",
	})

	tt := []struct {
		host string
	}{
		{
			host: "example.com",
		},
		{
			host: "example2.com",
		},
	}
	for _, tc := range tt {
		s.testInstallation.Assertions.AssertEventualCurlResponse(
			s.ctx,
			defaults.CurlPodExecOpt,
			[]curl.Option{
				curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
				curl.WithHostHeader(tc.host),
				curl.WithPath("/"),
			},
			&matchers.HttpResponse{
				StatusCode: http.StatusOK,
				Body:       gomega.ContainSubstring(defaults.NginxResponse),
			},
		)
	}

	s.testInstallation.Assertions.AssertEventualCurlResponse(
		s.ctx,
		defaults.CurlPodExecOpt,
		[]curl.Option{
			curl.WithHost(kubeutils.ServiceFQDN(proxyService.ObjectMeta)),
			curl.WithHostHeader("foo.com"),
			curl.WithPath("/"),
		},
		&matchers.HttpResponse{
			// google return 404 this when going to google.com  with host header of "foo.com"
			StatusCode: http.StatusNotFound,
		},
	)

	if s.agentgateway {
		// Agentgateway currently doesn't support Statuses for BackendTLSPolicy
		s.T().Log("Skipping status assertions for Agentgateway as they are not currently supported")
		return
	}
	s.assertPolicyStatus(metav1.Condition{
		Type:               string(v1alpha1.PolicyConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(v1alpha1.PolicyReasonValid),
		Message:            reports.PolicyAcceptedMsg,
		ObservedGeneration: backendTlsPolicy.Generation,
	})
	s.assertPolicyStatus(metav1.Condition{
		Type:               string(v1alpha1.PolicyConditionAttached),
		Status:             metav1.ConditionTrue,
		Reason:             string(v1alpha1.PolicyReasonAttached),
		Message:            reports.PolicyAttachedMsg,
		ObservedGeneration: backendTlsPolicy.Generation,
	})

	// delete configmap so we can assert status updates correctly
	err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, configMapManifest)
	s.Require().NoError(err)

	s.assertPolicyStatus(metav1.Condition{
		Type:               string(gwv1a2.PolicyConditionAccepted),
		Status:             metav1.ConditionFalse,
		Reason:             string(gwv1a2.PolicyReasonInvalid),
		Message:            fmt.Sprintf("%s: default/ca", backendtlspolicy.ErrConfigMapNotFound),
		ObservedGeneration: backendTlsPolicy.Generation,
	})
}

func (s *tsuite) assertPolicyStatus(inCondition metav1.Condition) {
	currentTimeout, pollingInterval := helpers.GetTimeouts()
	p := s.testInstallation.Assertions
	p.Gomega.Eventually(func(g gomega.Gomega) {
		tlsPol := &gwv1a3.BackendTLSPolicy{}
		objKey := client.ObjectKeyFromObject(backendTlsPolicy)
		err := s.testInstallation.ClusterContext.Client.Get(s.ctx, objKey, tlsPol)
		g.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get BackendTLSPolicy %s", objKey)

		g.Expect(tlsPol.Status.Ancestors).To(gomega.HaveLen(2), "ancestors didn't have length of 2")

		expectedAncestorRefs := []gwv1a2.ParentReference{
			{
				Group:     (*gwv1.Group)(&svcGroup),
				Kind:      (*gwv1.Kind)(&svcKind),
				Namespace: ptr.To(gwv1.Namespace(nginxMeta.Namespace)),
				Name:      gwv1.ObjectName(nginxMeta.Name),
			},
			{
				Group:     (*gwv1.Group)(&svcGroup),
				Kind:      (*gwv1.Kind)(&svcKind),
				Namespace: ptr.To(gwv1.Namespace(nginx2Meta.Namespace)),
				Name:      gwv1.ObjectName(nginx2Meta.Name),
			},
		}

		for i, ancestor := range tlsPol.Status.Ancestors {
			expectedRef := expectedAncestorRefs[i]
			g.Expect(ancestor.AncestorRef).To(gomega.BeEquivalentTo(expectedRef))

			g.Expect(ancestor.Conditions).To(gomega.HaveLen(2), "ancestors conditions wasn't length of 2")
			cond := meta.FindStatusCondition(ancestor.Conditions, inCondition.Type)
			g.Expect(cond).NotTo(gomega.BeNil(), "policy should have accepted condition")
			g.Expect(cond.Status).To(gomega.Equal(inCondition.Status), "policy accepted condition should be true")
			g.Expect(cond.Reason).To(gomega.Equal(inCondition.Reason), "policy reason should be accepted")
			g.Expect(cond.Message).To(gomega.Equal(inCondition.Message))
			g.Expect(cond.ObservedGeneration).To(gomega.Equal(inCondition.ObservedGeneration))
		}
	}, currentTimeout, pollingInterval).Should(gomega.Succeed())
}
