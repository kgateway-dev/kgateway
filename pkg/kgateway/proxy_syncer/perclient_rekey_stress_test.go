package proxy_syncer

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// stressUccSource mirrors the production uccCol (krtcollections.callbacksCollection):
// an in-memory map mutated from outside KRT (there: xDS stream callbacks) and surfaced
// via NewManyFromNothing + a RecomputeTrigger. This is the collection shape the
// per-client collections depend on, and where the #14184 stranding is hypothesized to
// originate. We reproduce it faithfully here so the stress test exercises the real
// propagation path, not a StaticCollection (which serializes events).
type stressUccSource struct {
	mu      sync.RWMutex
	clients map[string]ir.UniqlyConnectedClient
	trigger *krt.RecomputeTrigger
}

func newStressUccSource(krtopts krtutil.KrtOptions, initial []ir.UniqlyConnectedClient) (*stressUccSource, krt.Collection[ir.UniqlyConnectedClient]) {
	s := &stressUccSource{
		clients: make(map[string]ir.UniqlyConnectedClient),
		trigger: krt.NewRecomputeTrigger(true),
	}
	for _, c := range initial {
		s.clients[c.ResourceName()] = c
	}
	col := krt.NewManyFromNothing(func(ctx krt.HandlerContext) []ir.UniqlyConnectedClient {
		s.trigger.MarkDependant(ctx)
		s.mu.RLock()
		defer s.mu.RUnlock()
		out := make([]ir.UniqlyConnectedClient, 0, len(s.clients))
		for _, c := range s.clients {
			out = append(out, c)
		}
		return out
	}, krtopts.ToOptions("StressUniqueClients")...)
	return s, col
}

func (s *stressUccSource) add(c ir.UniqlyConnectedClient) {
	s.mu.Lock()
	s.clients[c.ResourceName()] = c
	s.mu.Unlock()
	s.trigger.TriggerRecomputation()
}

func (s *stressUccSource) del(rn string) {
	s.mu.Lock()
	delete(s.clients, rn)
	s.mu.Unlock()
	s.trigger.TriggerRecomputation()
}

// Faithful stranding repro: a stable client whose Envoy "blips" (del+re-add of the
// SAME client) concurrently with other-client churn and backend churn, all driven
// through the real trigger collection. After churn settles and the stable client is
// present, it must have a cluster for every backend. Permanent stranding (#14184)
// shows up as a stable client that is in uccCol yet has zero clusters that never
// recover. Run with -race.
func TestRekeyClusters_TriggerDrivenChurnNeverStrands(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	stable := rekeyClient("role-stable")
	src, uccs := newStressUccSource(krtopts, []ir.UniqlyConnectedClient{stable})

	backendNames := []string{"b1", "b2", "b3", "b4", "b5"}
	backends := make([]*ir.BackendObjectIR, 0, len(backendNames))
	for _, n := range backendNames {
		backends = append(backends, rekeyBackend(n))
	}
	finalBackends := krt.NewStaticCollection(nil, backends, krtopts.ToOptions("FinalBackends")...)

	clusters := NewPerClientEnvoyClusters(ctx, krtopts, rekeyStubTranslator(), finalBackends, uccs)
	eventuallyClusterCount(t, clusters, stable, len(backendNames))

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Blip the stable client: rapid del + re-add of the identical client.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			src.del(stable.ResourceName())
			src.add(stable)
		}
	}()

	// Churn other clients.
	for g := 0; g < 6; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			c := rekeyClient(fmt.Sprintf("role-churn-%d", g))
			for {
				select {
				case <-stop:
					return
				default:
				}
				src.add(c)
				src.del(c.ResourceName())
			}
		}(g)
	}

	// Churn a backend so per-client rows recompute during client blips.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			finalBackends.UpdateObject(rekeyBackend("b5"))
		}
	}()

	time.Sleep(2 * time.Second)
	close(stop)
	wg.Wait()

	// Ensure the stable client is present as the final state, then require recovery.
	src.add(stable)
	eventuallyClusterCount(t, clusters, stable, len(backendNames))
}
