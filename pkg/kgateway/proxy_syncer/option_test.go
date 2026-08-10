package proxy_syncer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
	krtutil "github.com/kgateway-dev/kgateway/v2/pkg/utils/krtutil"
)

type testStatusWriter struct{}

func (testStatusWriter) ApplyStatus(context.Context, statussync.Resource) {}

func TestWithStatusRegistration(t *testing.T) {
	contributions := krt.NewStaticCollection[reports.StatusContribution](nil, nil)
	contributionsByTarget := krtutil.UnnamedIndex(contributions, func(contribution reports.StatusContribution) []reports.StatusKey {
		return []reports.StatusKey{contribution.Target.Key()}
	})
	objects := krt.NewStaticCollection(nil, []*gwv1.Gateway{{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
	}})
	collections := statussync.NewStatusCollections()
	writers := map[schema.GroupVersionKind]statussync.ResourceStatusSyncer{}
	gvk := schema.GroupVersionKind{Group: "example.io", Version: "v1", Kind: "Example"}
	called := false

	statusSyncer := NewStatusSyncer(StatusSyncerConfig{
		Plugins:                     pluginsdk.Plugin{},
		ControllerName:              "controller-name",
		StatusCollections:           collections,
		StatusWriters:               writers,
		StatusContributions:         contributions,
		StatusContributionsByTarget: contributionsByTarget,
		// The proxy syncer contributes StatusCollections.HasSynced exactly once; the status
		// syncer must carry it through rather than adding a second copy of its own.
		CacheSyncs: []cache.InformerSynced{collections.HasSynced},
	}, WithStatusRegistration(func(in StatusRegistrationInputs) {
		called = true
		assert.Same(t, collections, in.Collections)
		assert.NotNil(t, in.StatusContributions)
		assert.NotNil(t, in.ContributionsByTarget)
		resourceReports := statussync.NewResourceReports(
			objects,
			in.StatusContributions,
			in.ContributionsByTarget,
			func(object *gwv1.Gateway) statussync.Resource {
				return statussync.Resource{
					GroupVersionKind: gvk,
					NamespacedName: types.NamespacedName{
						Name: object.Name, Namespace: object.Namespace,
					},
				}
			},
			in.KrtOpts.ToOptions("ExampleStatusReports")...,
		)
		statussync.RegisterResource(in.Collections, gvk, objects)
		statussync.RegisterResourceReports(in.Collections, resourceReports)
		in.RegisterWriter(gvk, testStatusWriter{})
	}))

	assert.True(t, called)
	// One entry, not two: StatusCollections.HasSynced re-reads its registration set on every
	// call, so the entry inherited from CacheSyncs already covers reducers registered here.
	assert.Len(t, statusSyncer.cacheSyncs, 1)
	assert.Eventually(t, collections.HasSynced, 5*time.Second, 10*time.Millisecond,
		"registered reducer should be part of the sync barrier")
	assert.IsType(t, testStatusWriter{}, statusSyncer.writers[gvk])
}
