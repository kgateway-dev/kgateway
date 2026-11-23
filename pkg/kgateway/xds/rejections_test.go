package xds

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
)

func TestRejectionTrackerAggregatesStreams(t *testing.T) {
	tracker := NewRejectionTracker()
	gw := types.NamespacedName{Namespace: "ns", Name: "gw"}
	const listeners, clusters = "envoy.config.listener.v3.Listener", "envoy.config.cluster.v3.Cluster"

	tracker.Reject(1, gw, listeners, "b-error")
	tracker.Reject(2, gw, listeners, "a-error")
	tracker.Reject(2, gw, clusters, "cluster-error")
	require.Equal(t, []GatewayXdsRejection{{
		Gateway: gw,
		Rejections: []TypeRejection{
			{TypeURL: clusters, Message: "cluster-error"},
			{TypeURL: listeners, Message: "a-error", Others: 1},
		},
	}}, tracker.Rejections().List())

	// Stream 2 accepts listeners; stream 1 still rejects them.
	tracker.Resolve(2, gw, listeners)
	require.Equal(t, []TypeRejection{
		{TypeURL: clusters, Message: "cluster-error"},
		{TypeURL: listeners, Message: "b-error"},
	}, tracker.Rejections().List()[0].Rejections)

	// Resolving something never rejected is a no-op.
	tracker.Resolve(3, gw, listeners)
	tracker.Resolve(1, types.NamespacedName{Namespace: "ns", Name: "other"}, listeners)
	require.Len(t, tracker.Rejections().List(), 1)

	tracker.Resolve(1, gw, listeners)
	tracker.Resolve(2, gw, clusters)
	require.Empty(t, tracker.Rejections().List(), "the row goes away once every rejection resolves")
}

func TestRejectionTrackerTruncatesMessages(t *testing.T) {
	tracker := NewRejectionTracker()
	gw := types.NamespacedName{Namespace: "ns", Name: "gw"}
	// A multi-byte rune straddling the limit must not be split.
	tracker.Reject(1, gw, "t", strings.Repeat("a", maxRejectionMessageLen-1)+"é"+strings.Repeat("b", 10))
	msg := tracker.Rejections().List()[0].Rejections[0].Message
	require.Equal(t, strings.Repeat("a", maxRejectionMessageLen-1)+"...", msg)
}

func TestNilRejectionTrackerIsANoop(t *testing.T) {
	var tracker *RejectionTracker
	tracker.Reject(1, types.NamespacedName{}, "t", "m")
	tracker.Resolve(1, types.NamespacedName{}, "t")
	require.Nil(t, tracker.Rejections())
}

func TestGatewayXdsRejectionEquals(t *testing.T) {
	base := GatewayXdsRejection{
		Gateway:    types.NamespacedName{Namespace: "ns", Name: "gw"},
		Rejections: []TypeRejection{{TypeURL: "t", Message: "m", Others: 1}},
	}
	require.True(t, base.Equals(base))
	for name, mutate := range map[string]func(*GatewayXdsRejection){
		"gateway": func(r *GatewayXdsRejection) { r.Gateway.Name = "other" },
		"type":    func(r *GatewayXdsRejection) { r.Rejections[0].TypeURL = "u" },
		"message": func(r *GatewayXdsRejection) { r.Rejections[0].Message = "n" },
		"others":  func(r *GatewayXdsRejection) { r.Rejections[0].Others = 2 },
		"length":  func(r *GatewayXdsRejection) { r.Rejections = append(r.Rejections, TypeRejection{}) },
	} {
		t.Run(name, func(t *testing.T) {
			other := base
			other.Rejections = append([]TypeRejection(nil), base.Rejections...)
			mutate(&other)
			require.False(t, base.Equals(other))
		})
	}
}
