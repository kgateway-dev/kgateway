package discovery_watchlabels

import (
	"context"

	"github.com/onsi/gomega"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1/kube/apis/gloo.solo.io/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/v1/options"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/kubernetes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/solo-io/gloo/test/kubernetes/e2e"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/stretchr/testify/suite"
)

var _ e2e.NewSuiteFunc = NewDiscoveryWatchlabelsSuite

// discoveryWatchlabelsSuite is the Suite of tests for validating Upstream discovery behavior when watchLabels are enabled
// This suite replaces the "upstream discovery" Context block from kube2e gateway tests
type discoveryWatchlabelsSuite struct {
	suite.Suite

	ctx context.Context

	// testInstallation contains all the metadata/utilities necessary to execute a series of tests
	// against an installation of Gloo Gateway
	testInstallation *e2e.TestInstallation
}

func NewDiscoveryWatchlabelsSuite(ctx context.Context, testInst *e2e.TestInstallation) suite.TestingSuite {
	return &discoveryWatchlabelsSuite{
		ctx:              ctx,
		testInstallation: testInst,
	}
}

func (s *discoveryWatchlabelsSuite) TestDiscoverUpstreamMatchingWatchLabels() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, serviceWithLabelsManifest, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.Assertions.NoError(err, "can delete service")

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, serviceWithoutLabelsManifest, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.Assertions.NoError(err, "can delete service")

		err = s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, serviceWithNoMatchingLabelsManifest, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.Assertions.NoError(err, "can delete service")
	})

	// add one service with labels matching our watchLabels
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, serviceWithLabelsManifest, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err, "can apply service")

	// add one service without labels matching our watchLabels
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, serviceWithoutLabelsManifest, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err, "can apply service")

	// add one service with a label matching our watchLabels but with an unwatched value
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, serviceWithNoMatchingLabelsManifest, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err, "can apply service")

	// eventually an Upstream should be created for the Service with matching labels
	// Upstreams no longer report status if they have not been translated at all to avoid conflicting with
	// other syncers that have translated them, so we can only detect that the objects exist here
	labeledUsName := kubernetes.UpstreamName(s.testInstallation.Metadata.InstallNamespace, "example-svc", 8000)
	s.testInstallation.Assertions.EventuallyResourceExists(
		func() (resources.Resource, error) {
			return s.testInstallation.ResourceClients.UpstreamClient().Read(s.testInstallation.Metadata.InstallNamespace, labeledUsName, clients.ReadOpts{Ctx: s.ctx})
		},
	)

	// the Upstream should have DiscoveryMetadata labels matching the parent Service
	us, err := s.testInstallation.ResourceClients.UpstreamClient().Read(s.testInstallation.Metadata.InstallNamespace, labeledUsName, clients.ReadOpts{Ctx: s.ctx})
	s.Assert().NoError(err, "can read upstream")

	s.Assert().Equal(map[string]string{
		"watchedKey": "watchedValue",
		"bonusKey":   "bonusValue",
	}, us.GetDiscoveryMetadata().GetLabels())

	// no Upstream should be created for the Service that does not have the watchLabels
	noLabelsUsName := kubernetes.UpstreamName(s.testInstallation.Metadata.InstallNamespace, "example-svc-no-labels", 8000)
	s.testInstallation.Assertions.ConsistentlyObjectsNotExist(
		s.ctx, &v1.Upstream{
			ObjectMeta: metav1.ObjectMeta{
				Name:      noLabelsUsName,
				Namespace: s.testInstallation.Metadata.InstallNamespace,
			},
		},
	)

	// no Upstream should be created for the Service that has a watched label without a watched value
	noMatchingLabelsUsName := kubernetes.UpstreamName(s.testInstallation.Metadata.InstallNamespace, "example-svc-no-matching-labels", 8000)
	s.testInstallation.Assertions.ConsistentlyObjectsNotExist(
		s.ctx, &v1.Upstream{
			ObjectMeta: metav1.ObjectMeta{
				Name:      noMatchingLabelsUsName,
				Namespace: s.testInstallation.Metadata.InstallNamespace,
			},
		},
	)

	// modify the non-watched label on the labeled service
	err = s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, serviceWithModifiedLabelsManifest, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err, "can re-apply service")

	// expect the Upstream's DiscoveryMeta to eventually match the modified labels from the parent Service
	s.testInstallation.Assertions.Gomega.Eventually(func() (map[string]string, error) {
		us, err = s.testInstallation.ResourceClients.UpstreamClient().Read(s.testInstallation.Metadata.InstallNamespace, labeledUsName, clients.ReadOpts{Ctx: s.ctx})
		return us.GetDiscoveryMetadata().GetLabels(), err
	}).Should(gomega.Equal(map[string]string{
		"watchedKey": "watchedValue",
		"bonusKey":   "bonusValue-modified",
	}))
}

func (s *discoveryWatchlabelsSuite) TestDiscoverySpecPreserved() {
	s.T().Cleanup(func() {
		err := s.testInstallation.Actions.Kubectl().DeleteFile(s.ctx, serviceWithLabelsManifest, "-n", s.testInstallation.Metadata.InstallNamespace)
		s.Assertions.NoError(err, "can delete service")
	})

	// add one service with labels matching our watchLabels
	err := s.testInstallation.Actions.Kubectl().ApplyFile(s.ctx, serviceWithLabelsManifest, "-n", s.testInstallation.Metadata.InstallNamespace)
	s.Assert().NoError(err, "can apply service")

	// eventually an Upstream should be created for the Service with matching labels
	// Upstreams no longer report status if they have not been translated at all to avoid conflicting with
	// other syncers that have translated them, so we can only detect that the objects exist here
	labeledUsName := kubernetes.UpstreamName(s.testInstallation.Metadata.InstallNamespace, "example-svc", 8000)
	s.testInstallation.Assertions.EventuallyResourceExists(
		func() (resources.Resource, error) {
			return s.testInstallation.ResourceClients.UpstreamClient().Read(s.testInstallation.Metadata.InstallNamespace, labeledUsName, clients.ReadOpts{Ctx: s.ctx})
		},
	)

	// the Upstream should have DiscoveryMetadata labels matching the parent Service
	us, err := s.testInstallation.ResourceClients.UpstreamClient().Read(s.testInstallation.Metadata.InstallNamespace, labeledUsName, clients.ReadOpts{Ctx: s.ctx})
	s.Assert().NoError(err, "can read upstream")

	s.Assert().NotNil(us.GetKube())
	s.Assert().Nil(us.GetKube().GetServiceSpec())

	// modify the Upstream to have a ServiceSpec
	us.GetKube().ServiceSpec = &options.ServiceSpec{
		PluginType: &options.ServiceSpec_GrpcJsonTranscoder{},
	}
	updatedUs, err := s.testInstallation.ResourceClients.UpstreamClient().Write(us, clients.WriteOpts{Ctx: s.ctx, OverwriteExisting: true})
	s.Assert().NoError(err, "can update upstream")
	s.Assert().NotNil(updatedUs.GetKube().GetServiceSpec())

	// expect the Upstream to consistently have the modified Spec
	s.testInstallation.Assertions.Gomega.Consistently(func() (*options.ServiceSpec, error) {
		us, err := s.testInstallation.ResourceClients.UpstreamClient().Read(us.GetMetadata().GetNamespace(), us.GetMetadata().GetName(), clients.ReadOpts{Ctx: s.ctx})
		return us.GetKube().GetServiceSpec(), err
	}).Should(gomega.Not(gomega.BeNil()))
}
