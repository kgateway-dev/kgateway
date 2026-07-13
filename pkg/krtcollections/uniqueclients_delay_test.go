package krtcollections_test

import (
	"context"
	"testing"
	"time"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoy_service_discovery_v3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	. "github.com/onsi/gomega"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	. "github.com/kgateway-dev/kgateway/v2/pkg/krtcollections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// TestFirstConnectDelayGatesFirstRequestPerStream pins the contract of the
// first-connect grace period, replacing the previous reliance on the (slow,
// live-ADS) setup suite for this coverage:
//
//  1. the first DiscoveryRequest of a new stream does not return before the
//     configured delay has elapsed — go-control-plane creates the stream's
//     first watch only after OnStreamRequest returns, so this is what holds
//     the first response back;
//  2. the client is registered (kicking off per-client translation) BEFORE
//     the sleep, so translation runs during the grace period, not after it;
//  3. follow-up requests on the same stream do not sleep again;
//  4. a new stream sleeps again (the delay is per stream, not per client);
//  5. a zero delay disables the sleep entirely.
func TestFirstConnectDelayGatesFirstRequestPerStream(t *testing.T) {
	const delay = 200 * time.Millisecond
	t.Cleanup(SetXdsFirstConnectDelayForTest(delay))
	g := NewWithT(t)

	cb, uccBuilder := NewUniquelyConnectedClients(nil, false)
	ucc := uccBuilder(context.Background(), krtutil.KrtOptions{}, nil)
	ucc.WaitUntilSynced(context.Background().Done())

	req := &envoy_service_discovery_v3.DiscoveryRequest{
		Node: &envoycorev3.Node{
			Id: "delaypod.ns",
			Metadata: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					xds.RoleKey: structpb.NewStringValue(wellknown.GatewayApiProxyValue + "~delay-test-role"),
				},
			},
		},
	}
	clone := func() *envoy_service_discovery_v3.DiscoveryRequest {
		return proto.Clone(req).(*envoy_service_discovery_v3.DiscoveryRequest)
	}

	// Watch for the client to become visible to the UCC collection while the
	// first request is still blocked in the grace period.
	observedAt := make(chan time.Time, 1)
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if len(ucc.List()) > 0 {
				observedAt <- time.Now()
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	// (1) + (2): first request of a new stream absorbs the delay, and the
	// client registers before the sleep completes.
	start := time.Now()
	g.Expect(cb.OnStreamRequest(1, clone())).To(Succeed())
	returnedAt := time.Now()
	g.Expect(returnedAt.Sub(start)).To(BeNumerically(">=", delay),
		"the first request of a new stream must not return before the grace period elapses")

	var registeredAt time.Time
	g.Eventually(observedAt, "2s").Should(Receive(&registeredAt),
		"the client must become visible to the UCC collection")
	g.Expect(registeredAt.Before(returnedAt)).To(BeTrue(),
		"registration (which kicks off per-client translation) must precede the sleep, not follow it")

	// (3): a follow-up request on the same stream does not sleep. The bound
	// is deliberately loose (half the delay) to stay robust on slow CI.
	start = time.Now()
	g.Expect(cb.OnStreamRequest(1, clone())).To(Succeed())
	g.Expect(time.Since(start)).To(BeNumerically("<", delay/2),
		"follow-up requests on an established stream must not sleep")

	// (4): the same client reconnecting on a new stream sleeps again.
	start = time.Now()
	g.Expect(cb.OnStreamRequest(2, clone())).To(Succeed())
	g.Expect(time.Since(start)).To(BeNumerically(">=", delay),
		"a new stream must absorb the grace period even for an already-known client")

	// (5): zero disables the sleep.
	restore := SetXdsFirstConnectDelayForTest(0)
	defer restore()
	start = time.Now()
	g.Expect(cb.OnStreamRequest(3, clone())).To(Succeed())
	g.Expect(time.Since(start)).To(BeNumerically("<", delay/2),
		"a zero delay must disable the first-connect sleep")
}
