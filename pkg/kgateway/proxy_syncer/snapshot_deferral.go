package proxy_syncer

import (
	"sync"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"

	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

// snapshotDeferralTracker counts connected clients without a current snapshot
// row, including clients that have never received a snapshot. A deferred client
// may still have its previous snapshot in the cache.
//
// Client and snapshot events maintain the sets and per-gateway counts in O(1)
// per event. Gateway and namespace labels identify affected gateways without
// adding a metric series per client.
type snapshotDeferralTracker struct {
	mu sync.Mutex
	// clients is the set of connected client keys.
	clients map[string]struct{}
	// snapshots is the set of client keys that currently have a snapshot row.
	snapshots map[string]struct{}
	// deferred counts, per gateway label pair, the clients in clients but not
	// in snapshots. Kept incrementally so events are O(1).
	deferred map[resourceNameDetails]int
}

func newSnapshotDeferralTracker() *snapshotDeferralTracker {
	return &snapshotDeferralTracker{
		clients:   make(map[string]struct{}),
		snapshots: make(map[string]struct{}),
		deferred:  make(map[resourceNameDetails]int),
	}
}

// register subscribes to client and snapshot events, replaying existing state.
func (d *snapshotDeferralTracker) register(clients krt.Collection[ir.UniquelyConnectedClient], snapshots krt.Collection[XdsSnapWrapper]) {
	clients.RegisterBatch(d.clientEvents, true)
	snapshots.RegisterBatch(d.snapshotEvents, true)
}

func (d *snapshotDeferralTracker) clientEvents(events []krt.Event[ir.UniquelyConnectedClient]) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range events {
		key := e.Latest().ResourceName()
		if e.Event == controllers.EventDelete {
			if _, known := d.clients[key]; known {
				delete(d.clients, key)
				if _, has := d.snapshots[key]; !has {
					d.adjust(key, -1)
				}
			}
			continue
		}
		if _, known := d.clients[key]; !known {
			d.clients[key] = struct{}{}
			if _, has := d.snapshots[key]; !has {
				d.adjust(key, +1)
			}
		}
	}
}

func (d *snapshotDeferralTracker) snapshotEvents(events []krt.Event[XdsSnapWrapper]) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, e := range events {
		key := e.Latest().ResourceName()
		if e.Event == controllers.EventDelete {
			// A snapshot row disappears when the client disconnects, or when
			// its transform returned nil: the deferral this gauge exists for.
			if _, had := d.snapshots[key]; had {
				delete(d.snapshots, key)
				if _, known := d.clients[key]; known {
					d.adjust(key, +1)
				}
			}
			continue
		}
		if _, had := d.snapshots[key]; !had {
			d.snapshots[key] = struct{}{}
			if _, known := d.clients[key]; known {
				d.adjust(key, -1)
			}
		}
	}
}

// adjust moves one gateway's deferred count and publishes it. Called with mu held.
func (d *snapshotDeferralTracker) adjust(clientKey string, delta int) {
	cd := getDetailsFromXDSClientResourceName(clientKey)
	labels := resourceNameDetails{Namespace: cd.Namespace, Gateway: cd.Gateway}
	n := d.deferred[labels] + delta
	if n <= 0 {
		delete(d.deferred, labels)
		n = 0
	} else {
		d.deferred[labels] = n
	}
	snapshotDeferredClients.Set(float64(n),
		metrics.Label{Name: gatewayLabel, Value: labels.Gateway},
		metrics.Label{Name: namespaceLabel, Value: labels.Namespace},
	)
}

// deferredClients returns the keys of connected clients that currently have no
// snapshot row, for tests and diagnostics.
func (d *snapshotDeferralTracker) deferredClients() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []string
	for key := range d.clients {
		if _, has := d.snapshots[key]; !has {
			out = append(out, key)
		}
	}
	return out
}
