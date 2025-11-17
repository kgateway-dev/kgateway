package nack

import (
	"context"
	"time"

	"istio.io/istio/pkg/kube"
	"k8s.io/apimachinery/pkg/types"
)

type NackEventPublisher struct {
	nackPublisher *Publisher
	ctx           context.Context
}

// NackEvent represents a NACK received from an agentgateway gateway
type NackEvent struct {
	Gateway   types.NamespacedName
	TypeUrl   string
	ErrorMsg  string
	Timestamp time.Time
}

func NewNackEventPublisher(ctx context.Context, client kube.Client) *NackEventPublisher {
	return &NackEventPublisher{
		nackPublisher: newPublisher(client),
		ctx:           ctx,
	}
}

// PublishNack publishes a NACK event to the Kubernetes Event API.
func (n *NackEventPublisher) PublishNack(nackEvent *NackEvent) {
	n.nackPublisher.onNack(n.ctx, *nackEvent)
}
