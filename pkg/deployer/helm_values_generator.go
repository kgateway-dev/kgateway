package deployer

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Deployer uses HelmValueGenerator implementation to generate a set of helm values
// when rendering a Helm chart
type HelmValuesGenerator interface {
	// GetValues returns the helm values used to render the dynamically provisioned resources
	// for the given object (e.g. Gateway). If the values returned are nil, it indicates that
	// the object is self-managed and no resources should be provisioned.
	GetValues(ctx context.Context, obj client.Object) (map[string]any, error)

	// GetCacheSyncHandlers returns the cache sync handlers for the HelmValuesGenerator controller
	GetCacheSyncHandlers() []cache.InformerSynced
}

// ObjectPostProcessor is an optional interface that can be implemented by HelmValuesGenerator
// to post-process rendered objects before they are deployed. This is used for applying
// strategic merge patch overlays from GatewayParameters.
type ObjectPostProcessor interface {
	// PostProcessObjects applies any post-processing to the rendered objects.
	// This is called after helm rendering but before deployment.
	// It returns the (potentially modified) slice of objects, as new objects may be added
	// (e.g., PodDisruptionBudget, HorizontalPodAutoscaler).
	PostProcessObjects(ctx context.Context, obj client.Object, rendered []client.Object) ([]client.Object, error)
}

// ReportingObjectPostProcessor is an optional interface for an ObjectPostProcessor that
// also reports status for the object the rendered objects belong to. When a
// HelmValuesGenerator implements it, the Deployer calls it instead of PostProcessObjects.
type ReportingObjectPostProcessor interface {
	PostProcessObjectsWithReport(ctx context.Context, obj client.Object, rendered []client.Object) ([]client.Object, *PostProcessReport, error)
}

// PostProcessReport is status a post-processor computed for the object the rendered
// objects belong to, such as a Gateway.
type PostProcessReport struct {
	// Conditions are set on the object's status.
	Conditions []metav1.Condition
	// OwnedConditionTypes are the condition types the post-processor manages. A type
	// listed here and absent from Conditions is removed from the object's status.
	OwnedConditionTypes []string
	// Events are recorded on the object when the reported conditions change its status.
	Events []ReportEvent
}

// ReportEvent is a Kubernetes Event to record on the object the rendered objects
// belong to.
type ReportEvent struct {
	// Type is corev1.EventTypeNormal or corev1.EventTypeWarning.
	Type string
	// Reason is a short CamelCase reason.
	Reason string
	// Action is what the controller did, or failed to do, to the object.
	Action string
	// Note is the human-readable message.
	Note string
}
