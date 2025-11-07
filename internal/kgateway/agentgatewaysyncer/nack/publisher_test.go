package nack

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"istio.io/istio/pkg/kube"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	apiv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	testGateway      = types.NamespacedName{Name: "test-gw", Namespace: "default"}
	testTypeURL      = "type.googleapis.com/agentgateway.dev.resource.Resource"
	testErrorMessage = "test error"
	testNackEvent    = NackEvent{
		Gateway:   testGateway,
		TypeUrl:   testTypeURL,
		ErrorMsg:  testErrorMessage,
		Timestamp: time.Now(),
	}
)

func TestNewPublisher(t *testing.T) {
	client := kube.NewFakeClient()
	publisher := newPublisher(client)

	assert.NotNil(t, publisher)
	assert.NotNil(t, publisher.client)
	assert.NotNil(t, publisher.eventRecorder)
}

func TestPublisher_OnNack(t *testing.T) {
	client := kube.NewFakeClient()
	publisher := newPublisher(client)

	fakeRecorder := record.NewFakeRecorder(10)
	publisher.eventRecorder = fakeRecorder

	ctx := context.TODO()
	// Ensure involved objects exist so UID lookups succeed
	_, _ = client.GatewayAPI().GatewayV1().Gateways(testGateway.Namespace).Create(ctx, &apiv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testGateway.Name,
			Namespace: testGateway.Namespace,
		},
	}, metav1.CreateOptions{})
	_, _ = client.Kube().AppsV1().Deployments(testGateway.Namespace).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testGateway.Name,
			Namespace: testGateway.Namespace,
		},
	}, metav1.CreateOptions{})

	publisher.onNack(ctx, testNackEvent)

	// Verify event was recorded
	select {
	case event := <-fakeRecorder.Events:
		assert.Contains(t, event, "Warning")
		assert.Contains(t, event, ReasonNack)
		assert.Contains(t, event, testErrorMessage)
	default:
		t.Fatal("Expected event to be recorded but none was found")
	}
}
