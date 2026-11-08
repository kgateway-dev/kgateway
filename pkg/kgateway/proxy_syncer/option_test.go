package proxy_syncer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube/krt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/statussync"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

type testStatusWriter struct{}

func (testStatusWriter) ApplyStatus(context.Context, statussync.Resource) {}

type otherStatusWriter struct{}

func (otherStatusWriter) ApplyStatus(context.Context, statussync.Resource) {}

func TestWithStatusRegistration(t *testing.T) {
	contributions := krt.NewStaticCollection[reports.StatusContribution](nil, nil)
	objects := krt.NewStaticCollection(nil, []*gwv1.Gateway{{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
	}})
	gvk := schema.GroupVersionKind{Group: "example.io", Version: "v1", Kind: "Example"}
	builtinGVK := schema.GroupVersionKind{Group: "example.io", Version: "v1", Kind: "Builtin"}
	called := false

	cfg := processStatusSyncerOptions(WithStatusRegistration(func(in statussync.RegistrationInputs) {
		called = true
		assert.NotNil(t, in.Collections)
		assert.Same(t, contributions, in.StatusContributions)
		assert.NotNil(t, in.ContributionsByTarget)
		assert.Equal(t, "controller-name", in.ControllerName)
		statussync.Register(in, gvk, objects)
		in.RegisterWriter(gvk, testStatusWriter{})
		// A downstream registration runs after the built-in kinds, so it cannot take over
		// the writer of one.
		in.RegisterWriter(builtinGVK, otherStatusWriter{})
	}))
	pipeline := &statusPipeline{registrations: cfg.statusRegistrations}
	syncer := &StatusSyncer{pipeline: pipeline}
	assert.EqualError(t, syncer.Start(t.Context()), "status syncer started before ProxySyncer.Init built its status pipeline")

	pipeline.init("controller-name", contributions, krtutil.KrtOptions{})
	pipeline.in.RegisterWriter(builtinGVK, testStatusWriter{})
	pipeline.registerDownstream()

	assert.True(t, called)
	assert.Eventually(t, pipeline.in.Collections.HasSynced, 5*time.Second, 10*time.Millisecond,
		"registered reducer should be part of the sync barrier")
	assert.IsType(t, testStatusWriter{}, pipeline.writers[gvk])
	assert.IsType(t, testStatusWriter{}, pipeline.writers[builtinGVK], "the first writer registered for a GVK keeps it")
}
