package nack

import (
	"context"

	"istio.io/istio/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/schemes"
)

var log = logging.New("nack/publisher")

// Event reasons for Kubernetes Events created by agentgateway NACK detection
const (
	ReasonNack = "AgentGatewayNackError"
)

// Publisher converts NACK events from the agentgateway xDS server into Kubernetes Events.
type Publisher struct {
	client        kube.Client
	eventRecorder record.EventRecorder
}

// newPublisher creates a new NACK event publisher that will publish k8s events
func newPublisher(client kube.Client) *Publisher {
	eventBroadcaster := record.NewBroadcaster()
	eventRecorder := eventBroadcaster.NewRecorder(
		schemes.DefaultScheme(),
		corev1.EventSource{Component: wellknown.DefaultAgwControllerName},
	)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{
		Interface: client.Kube().CoreV1().Events(""),
	})

	return &Publisher{
		client:        client,
		eventRecorder: eventRecorder,
	}
}

// onNack publishes a NACK event as a k8s event.
func (p *Publisher) onNack(ctx context.Context, event NackEvent) {
	var gatewayUID, deployUID types.UID
	gw, err := p.client.GatewayAPI().GatewayV1().Gateways(event.Gateway.Namespace).Get(ctx, event.Gateway.Name, metav1.GetOptions{})
	if err != nil {
		log.Error("failed to get gateway", "error", err)
		return
	}
	gatewayUID = gw.GetUID()
	dep, err := p.client.Kube().AppsV1().Deployments(event.Gateway.Namespace).Get(ctx, event.Gateway.Name, metav1.GetOptions{})
	if err != nil {
		log.Error("failed to get deployment", "error", err)
		return
	}
	deployUID = dep.GetUID()

	gatewayRef := &corev1.ObjectReference{
		Kind:       wellknown.GatewayKind,
		APIVersion: wellknown.GatewayGVK.GroupVersion().String(),
		Name:       event.Gateway.Name,
		Namespace:  event.Gateway.Namespace,
		UID:        gatewayUID,
	}
	deploymentRef := &corev1.ObjectReference{
		Kind:       wellknown.DeploymentGVK.Kind,
		APIVersion: wellknown.DeploymentGVK.GroupVersion().String(),
		Name:       event.Gateway.Name,
		Namespace:  event.Gateway.Namespace,
		UID:        deployUID,
	}

	p.eventRecorder.Eventf(gatewayRef, corev1.EventTypeWarning, ReasonNack, event.ErrorMsg)
	p.eventRecorder.Eventf(deploymentRef, corev1.EventTypeWarning, ReasonNack, event.ErrorMsg)

	log.Debug("published NACK event for Gateway", "gateway", event.Gateway, "typeURL", event.TypeUrl)
}
