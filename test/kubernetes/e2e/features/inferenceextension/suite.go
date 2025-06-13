package inferenceextension

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/test/kubernetes/e2e"
)

// testingSuite is the entire Suite of tests for testing K8s Service-specific features/fixes
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against a kgateway installation
	testInstallation *e2e.TestInstallation

	// manifests is a map of manifests keyed by a test name
	manifests map[string][][]byte

	// objects is a list of client objects used by a test
	objects []client.Object
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
		manifests:        map[string][][]byte{},
	}
}

func (s *testingSuite) SetupSuite() {
	s.manifests = map[string][][]byte{
		"TestSingleHTTPRouteSingleInferencePool": {
			basicManifest,
		},
		"TestSingleHTTPRouteMultiInferencePool": {
			singleRouteMultiPoolManifest,
		},
		"TestMultiHTTPRouteSingleInferencePool": {
			multiRouteSinglePoolManifest,
		},
	}
}

func (s *testingSuite) BeforeTest(suiteName, testName string) {
	fmt.Printf("Applying manifests for test %s in suite %s", testName, suiteName)
	for _, manifest := range s.manifests[testName] {
		err := s.testInstallation.Actions.Kubectl().Apply(s.ctx, manifest)
		s.Require().NoError(err)
	}
}

func (s *testingSuite) AfterTest(suiteName, testName string) {
	if s.T().Failed() {
		s.testInstallation.PreFailHandler(s.ctx)
	}

	fmt.Printf("Deleting manifests for test %s in suite %s", testName, suiteName)

	manifests, ok := s.manifests[testName]
	if !ok {
		s.FailNow("no manifests found for %s", testName)
	}

	for _, manifest := range manifests {
		err := s.testInstallation.Actions.Kubectl().Delete(s.ctx, manifest)
		s.NoError(err, "can delete manifest %s", string(manifest))
	}

	s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, s.objects...)
}

func (s *testingSuite) TestSingleHTTPRouteSingleInferencePool() {
	// Track core objects from testdata manifests
	clientPod := &corev1.Pod{ObjectMeta: objectMeta(basicTestNs, clientName)}
	gtwDeployment := &appsv1.Deployment{ObjectMeta: objectMeta(basicTestNs, gtwName)}
	gtwService := &corev1.Service{ObjectMeta: objectMeta(basicTestNs, gtwName)}
	vllmDeployment := &appsv1.Deployment{ObjectMeta: objectMeta(basicTestNs, vllmName)}
	eppDeployment := &appsv1.Deployment{ObjectMeta: objectMeta(basicTestNs, eppName)}
	eppService := &corev1.Service{ObjectMeta: objectMeta(basicTestNs, eppName)}
	s.objects = []client.Object{
		clientPod,
		gtwDeployment,
		gtwService,
		vllmDeployment,
		eppDeployment,
		eppService,
	}

	// Assert test objects exist
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, s.objects...)

	// Assert test pods are running using key=pod_name and value=pod_namespace map.
	for k, v := range map[string]string{
		vllmName:   basicTestNs,
		eppName:    basicTestNs,
		clientName: basicTestNs,
	} {
		s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, v, metav1.ListOptions{
			LabelSelector: "app=" + k,
		}, podRunTimeout)
	}

	// Assert the gateway programmed condition
	s.testInstallation.Assertions.EventuallyGatewayCondition(
		s.ctx,
		gtwDeployment.Name,
		gtwDeployment.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
		gtwProgramTimeout,
	)

	// Assert the route status conditions
	conditions := []gwv1.RouteConditionType{gwv1.RouteConditionAccepted, gwv1.RouteConditionResolvedRefs}
	for _, c := range conditions {
		s.testInstallation.Assertions.EventuallyHTTPRouteCondition(
			s.ctx,
			routeName,
			basicTestNs,
			c,
			metav1.ConditionTrue,
		)
	}

	// TODO [danehans]: Assert InferencePool status when https://github.com/kgateway-dev/kgateway/issues/11379 is fixed

	// Assert the OpenAI API endpoint test cases
	s.runOpenAITests(
		basicTestNs,
		gtwService.ObjectMeta,
		map[string]string{
			"": baseModelName,
		},
	)
}

func (s *testingSuite) TestSingleHTTPRouteMultiInferencePool() {
	// Track core objects from testdata manifests
	clientPod := &corev1.Pod{ObjectMeta: objectMeta(singleRouteMultiPoolNs, clientName)}
	gtwDeployment := &appsv1.Deployment{ObjectMeta: objectMeta(singleRouteMultiPoolNs, gtwName)}
	gtwService := &corev1.Service{ObjectMeta: objectMeta(singleRouteMultiPoolNs, gtwName)}
	vllmDeployment := &appsv1.Deployment{ObjectMeta: objectMeta(singleRouteMultiPoolNs, vllmName)}
	secondVllmDeployment := &appsv1.Deployment{ObjectMeta: objectMeta(singleRouteMultiPoolNs, secondVllmName)}
	eppDeployment := &appsv1.Deployment{ObjectMeta: objectMeta(singleRouteMultiPoolNs, eppName)}
	eppService := &corev1.Service{ObjectMeta: objectMeta(singleRouteMultiPoolNs, eppName)}
	secondEPPDeployment := &appsv1.Deployment{ObjectMeta: objectMeta(singleRouteMultiPoolNs, secondEPPName)}
	secondEPPService := &corev1.Service{ObjectMeta: objectMeta(singleRouteMultiPoolNs, secondEPPName)}
	s.objects = []client.Object{
		clientPod,
		gtwDeployment,
		gtwService,
		vllmDeployment,
		secondVllmDeployment,
		eppDeployment,
		eppService,
		secondEPPDeployment,
		secondEPPService,
	}

	// Assert test objects exist
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, s.objects...)

	// Assert test pods are running using key=pod_name and value=pod_namespace map.
	for k, v := range map[string]string{
		vllmName:       singleRouteMultiPoolNs,
		eppName:        singleRouteMultiPoolNs,
		secondVllmName: singleRouteMultiPoolNs,
		secondEPPName:  singleRouteMultiPoolNs,
		clientName:     singleRouteMultiPoolNs,
	} {
		s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, v, metav1.ListOptions{
			LabelSelector: "app=" + k,
		}, podRunTimeout)
	}

	// Assert the gateway programmed condition
	s.testInstallation.Assertions.EventuallyGatewayCondition(
		s.ctx,
		gtwDeployment.Name,
		gtwDeployment.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
		gtwProgramTimeout,
	)

	// Assert the route status conditions
	conditions := []gwv1.RouteConditionType{gwv1.RouteConditionAccepted, gwv1.RouteConditionResolvedRefs}
	for _, c := range conditions {
		s.testInstallation.Assertions.EventuallyHTTPRouteCondition(
			s.ctx,
			routeName,
			singleRouteMultiPoolNs,
			c,
			metav1.ConditionTrue,
		)
	}

	// TODO [danehans]: Assert InferencePool status when https://github.com/kgateway-dev/kgateway/issues/11379 is fixed

	// Assert the OpenAI API endpoint test cases
	s.runOpenAITests(singleRouteMultiPoolNs,
		gtwService.ObjectMeta,
		headerToModel,
	)
}

func (s *testingSuite) TestMultiHTTPRouteSingleInferencePool() {
	// Track core objects from testdata manifests
	clientPod := &corev1.Pod{ObjectMeta: objectMeta(multiRouteSinglePoolNs, clientName)}
	gtwDeployment := &appsv1.Deployment{ObjectMeta: objectMeta(multiRouteSinglePoolNs, gtwName)}
	gtwService := &corev1.Service{ObjectMeta: objectMeta(multiRouteSinglePoolNs, gtwName)}
	vllmDeployment := &appsv1.Deployment{ObjectMeta: objectMeta(multiRouteSinglePoolNs, vllmName)}
	secondVllmDeployment := &appsv1.Deployment{ObjectMeta: objectMeta(multiRouteSinglePoolNs, secondVllmName)}
	eppDeployment := &appsv1.Deployment{ObjectMeta: objectMeta(multiRouteSinglePoolNs, eppName)}
	eppService := &corev1.Service{ObjectMeta: objectMeta(multiRouteSinglePoolNs, eppName)}
	secondEPPDeployment := &appsv1.Deployment{ObjectMeta: objectMeta(multiRouteSinglePoolNs, secondEPPName)}
	secondEPPService := &corev1.Service{ObjectMeta: objectMeta(multiRouteSinglePoolNs, secondEPPName)}
	s.objects = []client.Object{
		clientPod,
		gtwDeployment,
		gtwService,
		vllmDeployment,
		secondVllmDeployment,
		eppDeployment,
		eppService,
		secondEPPDeployment,
		secondEPPService,
	}

	// Assert test objects exist
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, s.objects...)

	// Assert test pods are running using key=pod_name and value=pod_namespace map.
	for k, v := range map[string]string{
		vllmName:       multiRouteSinglePoolNs,
		eppName:        multiRouteSinglePoolNs,
		secondVllmName: multiRouteSinglePoolNs,
		secondEPPName:  multiRouteSinglePoolNs,
		clientName:     multiRouteSinglePoolNs,
	} {
		s.testInstallation.Assertions.EventuallyPodsRunning(s.ctx, v, metav1.ListOptions{
			LabelSelector: "app=" + k,
		}, podRunTimeout)
	}

	// Assert the gateway programmed condition
	s.testInstallation.Assertions.EventuallyGatewayCondition(
		s.ctx,
		gtwDeployment.Name,
		gtwDeployment.Namespace,
		gwv1.GatewayConditionProgrammed,
		metav1.ConditionTrue,
		gtwProgramTimeout,
	)

	// Assert the route status conditions for each route
	routeConditions := map[string][]gwv1.RouteConditionType{
		routeName: []gwv1.RouteConditionType{
			gwv1.RouteConditionAccepted,
			gwv1.RouteConditionResolvedRefs,
		},
		secondRouteName: []gwv1.RouteConditionType{
			gwv1.RouteConditionAccepted,
			gwv1.RouteConditionResolvedRefs,
		},
	}

	for routeName, conditions := range routeConditions {
		for _, c := range conditions {
			s.testInstallation.Assertions.EventuallyHTTPRouteCondition(
				s.ctx,
				routeName,
				multiRouteSinglePoolNs,
				c,
				metav1.ConditionTrue,
			)
		}
	}

	// TODO [danehans]: Assert InferencePool status when https://github.com/kgateway-dev/kgateway/issues/11379 is fixed

	// Assert the OpenAI API endpoint test cases
	s.runOpenAITests(multiRouteSinglePoolNs,
		gtwService.ObjectMeta,
		headerToModel,
	)
}
