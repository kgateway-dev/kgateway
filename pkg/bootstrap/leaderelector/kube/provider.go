package kube

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/recorder"

	"github.com/solo-io/go-utils/contextutils"
)

var _ recorder.Provider = new(noopProvider)

func NewNoopProvider() *noopProvider {
	return &noopProvider{}
}

type noopProvider struct {
}

func (n *noopProvider) GetEventRecorderFor(name string) record.EventRecorder {
	return &noopEventRecorder{}
}

type noopEventRecorder struct {
	ctx context.Context
}

func (n *noopEventRecorder) Event(object runtime.Object, eventType, reason, message string) {
	contextutils.LoggerFrom(n.ctx).Debugf("Event callback called")
}

func (n *noopEventRecorder) Eventf(
	object runtime.Object,
	eventType, reason, messageFmt string,
	args ...interface{},
) {
	contextutils.LoggerFrom(n.ctx).Debugf("Eventf callback called")
}

func (n *noopEventRecorder) AnnotatedEventf(
	object runtime.Object,
	annotations map[string]string,
	eventType, reason, messageFmt string,
	args ...interface{},
) {
	contextutils.LoggerFrom(n.ctx).Debugf("AnnotatedEventf callback called")
}
