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

// stressUccSource mirrors the production connected-client collection
// (krtcollections.callbacksCollection): an in-memory map mutated from outside KRT
// (there: xDS stream callbacks) and surfaced via NewManyFromNothing + a
// RecomputeTrigger. The per-client collections depend on this collection shape, so
// reproducing it faithfully here exercises the real propagation path rather than a
// StaticCollection (which serializes events). This is the path along which #14184
// (a connected client stranded with empty per-client config) would arise.
type stressUccSource struct {
	mu      sync.RWMutex
	clients map[string]ir.UniquelyConnectedClient
	trigger *krt.RecomputeTrigger
}

func newStressUccSource(krtopts krtutil.KrtOptions, initial []ir.UniquelyConnectedClient) (*stressUccSource, krt.Collection[ir.UniquelyConnectedClient]) {
	s := &stressUccSource{
		clients: make(map[string]ir.UniquelyConnectedClient),
		trigger: krt.NewRecomputeTrigger(true),
	}
	for _, c := range initial {
		s.clients[c.ResourceName()] = c
	}
	col := krt.NewManyFromNothing(func(ctx krt.HandlerContext) []ir.UniquelyConnectedClient {
		s.trigger.MarkDependant(ctx)
		s.mu.RLock()
		defer s.mu.RUnlock()
		out := make([]ir.UniquelyConnectedClient, 0, len(s.clients))
		for _, c := range s.clients {
			out = append(out, c)
		}
		return out
	}, krtopts.ToOptions("StressUniqueClients")...)
	return s, col
}

func (s *stressUccSource) add(c ir.UniquelyConnectedClient) {
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

// A stable client whose Envoy "blips" (delete + re-add of the SAME client)
// concurrently with other-client churn and backend churn, all driven through the
// real trigger collection. After churn settles and the stable client is present, it
// must have a cluster for every backend. Run with -race.
func TestPerClientClusters_TriggerDrivenChurnNeverStrands(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	krtopts := krtutil.NewKrtOptions(ctx.Done(), nil)

	stable := clustersTestClient("role-stable")
	src, uccs := newStressUccSource(krtopts, []ir.UniquelyConnectedClient{stable})

	backendNames := []string{"b1", "b2", "b3", "b4", "b5"}
	backends := make([]*ir.BackendObjectIR, 0, len(backendNames))
	for _, n := range backendNames {
		backends = append(backends, clustersTestBackend(n))
	}
	finalBackends := krt.NewStaticCollection(nil, backends, krtopts.ToOptions("FinalBackends")...)

	clusters := NewPerClientEnvoyClusters(ctx, krtopts, clustersTestTranslator(), finalBackends, uccs)
	eventuallyClusterCount(t, clusters, stable, len(backendNames))

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Blip the stable client: rapid delete + re-add of the identical client.
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			src.del(stable.ResourceName())
			src.add(stable)
		}
	})

	// Churn other clients.
	for g := range 6 {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			c := clustersTestClient(fmt.Sprintf("role-churn-%d", g))
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
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			finalBackends.UpdateObject(clustersTestBackend("b5"))
		}
	})

	time.Sleep(2 * time.Second)
	close(stop)
	wg.Wait()

	// Ensure the stable client is present as the final state, then require recovery.
	src.add(stable)
	eventuallyClusterCount(t, clusters, stable, len(backendNames))
}
