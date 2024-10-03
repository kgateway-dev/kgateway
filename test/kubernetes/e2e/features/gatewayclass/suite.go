package gatewayclass

import (
	"context"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/pkg/consts"

	"github.com/solo-io/gloo/projects/gateway2/crds"
	"github.com/solo-io/gloo/projects/gateway2/wellknown"
	"github.com/solo-io/gloo/test/kubernetes/e2e"
)

var _ e2e.NewSuiteFunc = NewTestingSuite

// testingSuite is the entire Suite of tests for the "gatewayclass" feature
// The "gatewayclass" code can be found here: /projects/gateway2/controller
type testingSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewTestingSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &testingSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

// TODO: Remove test when a version of Gateway API that includes https://github.com/kubernetes-sigs/gateway-api/pull/3368 is supported
func (s *testingSuite) TestGatewayClassConditions() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, gwParametersManifestFile)
		s.NoError(err, "can delete gatewayparameters manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, gwParams)

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, gcManifestFile)
		s.NoError(err, "can delete gatewayclass manifest")
		s.testInstallation.Assertions.EventuallyObjectsNotExist(s.ctx, gc)

		err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, supportedCrdsManifestFile)
		s.NoError(err, "can apply gateway api crd manifest")
	})

	// Apply gatewayparams and gatewayclass manifests
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, gwParametersManifestFile)
	s.Require().NoError(err, "can apply gatewayparameters manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, gwParams)
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, gcManifestFile)
	s.Require().NoError(err, "can apply gatewayclass manifest")
	s.testInstallation.Assertions.EventuallyObjectsExist(s.ctx, gc)

	// Assert that the gatewayclass has the expected status conditions set to true
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		gcName := types.NamespacedName{Name: gc.Name, Namespace: gc.Namespace}
		err = s.testInstallation.ClusterContext.Client.Get(s.ctx, gcName, gc)
		assert.NoError(c, err, "gatewayclass not found")

		accepted, supportedVersion := false, false
		for _, conditions := range gc.Status.Conditions {
			if conditions.Type == string(gwv1.GatewayClassConditionStatusAccepted) && conditions.Status == metav1.ConditionTrue {
				accepted = true
			}
			if conditions.Type == string(gwv1.GatewayClassConditionStatusSupportedVersion) && conditions.Status == metav1.ConditionTrue {
				supportedVersion = true
			}
		}
		assert.True(c, accepted, "gatewayclass does not include expected accepted=true status condition")
		assert.True(c, supportedVersion, "gatewayclass does not include expected supportedversion=true status condition")
	}, 10*time.Second, 1*time.Second)

	// Update the bundle-version annotation of the gatewayclass crd to an unsupported version
	crd := &apiextv1.CustomResourceDefinition{}
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		crdName := types.NamespacedName{Name: crds.GatewayClass, Namespace: "default"}
		err = s.testInstallation.ClusterContext.Client.Get(s.ctx, crdName, crd)
		s.Require().NoError(err, "failed to retrieve the gatewayclass crd")
		s.Require().NotNil(crd.Annotations)
		crd.Annotations[consts.BundleVersionAnnotation] = "v0.0.1"
		err = s.testInstallation.ClusterContext.Client.Update(s.ctx, crd)
		s.Require().NoError(err, "failed to update the gatewayclass crd bundle-version annotation to an unsupported version")
	}, 10*time.Second, 1*time.Second)

	// Assert that the gatewayclass has the expected status conditions set to false
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		gcNN := types.NamespacedName{Name: gc.Name, Namespace: gc.Namespace}
		err = s.testInstallation.ClusterContext.Client.Get(s.ctx, gcNN, gc)
		assert.NoError(c, err, "gatewayclass not found")

		accepted, supportedVersion := true, true
		for _, conditions := range gc.Status.Conditions {
			if conditions.Type == string(gwv1.GatewayClassConditionStatusAccepted) && conditions.Status == metav1.ConditionFalse {
				accepted = false
			}
			if conditions.Type == string(gwv1.GatewayClassConditionStatusSupportedVersion) && conditions.Status == metav1.ConditionFalse {
				supportedVersion = false
			}
		}
		assert.False(c, accepted, "gatewayclass does not include expected accepted=false status conditions")
		assert.False(c, supportedVersion, "gatewayclass does not include expected supportedversion=false status conditions")
	}, 10*time.Second, 1*time.Second)

	// Update the bundle-version annotation of the gatewayclass crd to a supported version
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		crd.Annotations[consts.BundleVersionAnnotation] = wellknown.SupportedVersions[0]
		err = s.testInstallation.ClusterContext.Client.Update(s.ctx, crd)
		s.Require().NoError(err, "failed to update the gatewayclass crd bundle-version annotation to a supported version")
	}, 10*time.Second, 1*time.Second)

	// Assert that the gatewayclass has the expected status conditions set to true
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		gcNN := types.NamespacedName{Name: gc.Name, Namespace: gc.Namespace}
		err = s.testInstallation.ClusterContext.Client.Get(s.ctx, gcNN, gc)
		assert.NoError(c, err, "gatewayclass not found")

		accepted, supportedVersion := false, false
		for _, conditions := range gc.Status.Conditions {
			if conditions.Type == string(gwv1.GatewayClassConditionStatusAccepted) && conditions.Status == metav1.ConditionTrue {
				accepted = true
			}
			if conditions.Type == string(gwv1.GatewayClassConditionStatusSupportedVersion) && conditions.Status == metav1.ConditionTrue {
				supportedVersion = true
			}
		}
		assert.True(c, accepted, "gatewayclass does not include expected accepted=true status conditions")
		assert.True(c, supportedVersion, "gatewayclass does not include expected supportedversion=true status conditions")
	}, 10*time.Second, 1*time.Second)

	// Remove the bundle-version annotation from the gatewayclass crd
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		delete(crd.Annotations, consts.BundleVersionAnnotation)
		err = s.testInstallation.ClusterContext.Client.Update(s.ctx, crd)
		s.Require().NoError(err, "failed to remove the gatewayclass crd bundle-version annotation")
	}, 10*time.Second, 1*time.Second)

	// Assert that the gatewayclass has the expected status conditions set to false
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		gcNN := types.NamespacedName{Name: gc.Name, Namespace: gc.Namespace}
		err = s.testInstallation.ClusterContext.Client.Get(s.ctx, gcNN, gc)
		assert.NoError(c, err, "gatewayclass not found")

		accepted, supportedVersion := true, true
		for _, conditions := range gc.Status.Conditions {
			if conditions.Type == string(gwv1.GatewayClassConditionStatusAccepted) && conditions.Status == metav1.ConditionFalse {
				accepted = false
			}
			if conditions.Type == string(gwv1.GatewayClassConditionStatusSupportedVersion) && conditions.Status == metav1.ConditionFalse {
				supportedVersion = false
			}
		}
		assert.False(c, accepted, "gatewayclass does not include expected accepted=false status conditions")
		assert.False(c, supportedVersion, "gatewayclass does not include expected supportedversion=false status conditions")
	}, 10*time.Second, 1*time.Second)

	// Update the bundle-version annotation of the gatewayclass crd to a supported version
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		crd.Annotations[consts.BundleVersionAnnotation] = wellknown.SupportedVersions[1]
		err = s.testInstallation.ClusterContext.Client.Update(s.ctx, crd)
		s.Require().NoError(err, "failed to update the gatewayclass crd bundle-version annotation to a supported version")
	}, 10*time.Second, 1*time.Second)

	// Assert that the gatewayclass has the expected status conditions set to true
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		gcNN := types.NamespacedName{Name: gc.Name, Namespace: gc.Namespace}
		err = s.testInstallation.ClusterContext.Client.Get(s.ctx, gcNN, gc)
		assert.NoError(c, err, "gatewayclass not found")

		accepted, supportedVersion := false, false
		for _, conditions := range gc.Status.Conditions {
			if conditions.Type == string(gwv1.GatewayClassConditionStatusAccepted) && conditions.Status == metav1.ConditionTrue {
				accepted = true
			}
			if conditions.Type == string(gwv1.GatewayClassConditionStatusSupportedVersion) && conditions.Status == metav1.ConditionTrue {
				supportedVersion = true
			}
		}
		assert.True(c, accepted, "gatewayclass does not include expected accepted=true status conditions")
		assert.True(c, supportedVersion, "gatewayclass does not include expected supportedversion=true status conditions")
	}, 10*time.Second, 1*time.Second)
}
