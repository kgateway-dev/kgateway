package metrics

import (
	"context"
	"sync"
	"time"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

const (
	resourcesSubsystem   = "resources"
	snapshotSubsystem    = "xds_snapshot"
	snapshotResourceType = "XDSSnapshot"
	gatewayLabel         = "gateway"
	parentLabel          = "parent"
	namespaceLabel       = "namespace"
	resourceLabel        = "resource"
)

var logger = logging.New("translator.metrics")

var (
	xdsSnapshotHistogramBuckets = []float64{0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600, 1200, 1800}
	xdsSnapshotSyncsTotal       = metrics.NewCounter(metrics.CounterOpts{
		Subsystem: snapshotSubsystem,
		Name:      "syncs_total",
		Help:      "Total number of XDS snapshot syncs",
	},
		[]string{gatewayLabel, namespaceLabel})
	xdsSnapshotSyncDuration = metrics.NewHistogram(
		metrics.HistogramOpts{
			Subsystem:                       snapshotSubsystem,
			Name:                            "sync_duration_seconds",
			Help:                            "Duration of time for a gateway resource update to be synced in an XDS snapshot",
			Buckets:                         xdsSnapshotHistogramBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		},
		[]string{gatewayLabel, namespaceLabel},
	)

	resourcesManaged = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: resourcesSubsystem,
			Name:      "managed",
			Help:      "Current number of resources managed",
		},
		[]string{namespaceLabel, parentLabel, resourceLabel},
	)
	resourcesUpdatesDroppedTotal = metrics.NewCounter(metrics.CounterOpts{
		Subsystem: resourcesSubsystem,
		Name:      "updates_dropped_total",
		Help:      "Total number of xds_snapshot sync metrics updates dropped. If this metric is ever greater than 0, the xds_snapshot sync metrics should be considered invalid until process restart",
	}, nil)
)

func init() {
	// A Gateway's snapshot syncs carry the Gateway's own namespace.
	metrics.RegisterGatewayScoped(xdsSnapshotSyncsTotal, namespaceLabel, gatewayLabel)
	metrics.RegisterGatewayScoped(xdsSnapshotSyncDuration, namespaceLabel, gatewayLabel)

	// This carries the namespace of the route or policy and only the name of its parent, so
	// it identifies no Gateway and is removed with the namespace.
	metrics.RegisterNamespaceScoped(resourcesManaged, namespaceLabel)
}

type resourceMetricLabels struct {
	Parent    string
	Namespace string
	Resource  string
}

func (r resourceMetricLabels) toMetricsLabels() []metrics.Label {
	return []metrics.Label{
		{Name: parentLabel, Value: r.Parent},
		{Name: namespaceLabel, Value: r.Namespace},
		{Name: resourceLabel, Value: r.Resource},
	}
}

// managedCounts is the source of truth for resourcesManaged, which is always written with
// Set: the metrics sweeper may delete a namespace's series while delete events for its
// resources are still queued, and a relative update would then corrupt the series of a
// namespace recreated with that name. A series whose count reaches zero is deleted, so a
// deleted parent in a namespace that stays does not leave one behind.
var managedCounts = struct {
	sync.Mutex
	counts map[resourceMetricLabels]int
}{counts: map[resourceMetricLabels]int{}}

// addResourcesManaged adjusts the count of managed resources for labels and publishes it.
func addResourcesManaged(labels resourceMetricLabels, delta int) {
	metricLabels := labels.toMetricsLabels()

	managedCounts.Lock()
	defer managedCounts.Unlock()

	count := managedCounts.counts[labels] + delta
	if count == 0 {
		delete(managedCounts.counts, labels)
		resourcesManaged.Delete(metricLabels...)
		return
	}
	managedCounts.counts[labels] = count
	resourcesManaged.Set(float64(count), metricLabels...)
}

// startTimes tracks the start times of pending XDS snapshot syncs, keyed by xdsSyncKey.
var startTimes = struct {
	sync.Mutex
	times map[ResourceSyncDetails]time.Time
}{times: map[ResourceSyncDetails]time.Time{}}

// ResourceSyncDetails holds the details of a resource sync operation.
type ResourceSyncDetails struct {
	Namespace    string
	Gateway      string
	ResourceType string
	ResourceName string
}

// xdsSyncKey is the startTimes key of the XDS snapshot sync of the Gateway in d. A Gateway
// has at most one pending XDS sync, so the key leaves out the resource.
func (d ResourceSyncDetails) xdsSyncKey() ResourceSyncDetails {
	return ResourceSyncDetails{Namespace: d.Namespace, Gateway: d.Gateway}
}

type syncStartInfo struct {
	endTime time.Time
	details ResourceSyncDetails
}

// Buffered channel to handle resource sync metrics updates.
// The buffer size is assumed to be sufficient for any reasonable load.
// But, this may need to be configurable in the future, if needed for very high load.
var (
	syncCh     = make(chan *syncStartInfo, 1024)
	syncChLock sync.RWMutex
)

// StartResourceSyncMetricsProcessing starts a goroutine that processes resource sync metrics.
func StartResourceSyncMetricsProcessing(ctx context.Context) {
	resourcesUpdatesDroppedTotal.Add(0) // Initialize the counter to 0.

	go func() {
		for {
			syncChLock.RLock()
			select {
			case <-ctx.Done():
				syncChLock.RUnlock()

				return
			case syncInfo, ok := <-syncCh:
				if !ok || syncInfo == nil {
					syncChLock.RUnlock()

					return
				}

				syncChLock.RUnlock()
				endResourceSync(syncInfo)
			}
		}
	}()
}

// StartResourceXDSSync records the start time of a XDS snapshot sync.
func StartResourceXDSSync(details ResourceSyncDetails) {
	if !metrics.Active() {
		return
	}

	startTimes.Lock()
	defer startTimes.Unlock()

	startTimes.times[details.xdsSyncKey()] = time.Now()
}

// dropResourceXDSSync forgets the pending XDS sync of a deleted Gateway. A Gateway whose
// translation produced no snapshot has a sync that nothing ends, so it would otherwise be
// held forever, and the first sync of a Gateway recreated with the same name would record
// its age.
func dropResourceXDSSync(details ResourceSyncDetails) {
	startTimes.Lock()
	defer startTimes.Unlock()

	delete(startTimes.times, details.xdsSyncKey())
}

// EndResourceXDSSync records the end time of an XDS snapshot sync.
// Returns true if the sync was added to the channel, false if the channel is full.
// If the channel is full, an error is logged to call attention to the issue.
// The caller is not expected to handle this case.
func EndResourceXDSSync(details ResourceSyncDetails) bool {
	if !metrics.Active() {
		return true
	}

	// Add syncStartInfo to the channel for metrics processing.
	// If the channel is full, something is probably wrong, but translations shouldn't stop because of a metrics processing issue.
	// In that case, updating the metrics will be dropped, and translations will continue processing.
	// This will cause the metrics to become invalid, so an error is logged to call attention to the issue.
	syncChLock.RLock()
	select {
	case syncCh <- &syncStartInfo{
		endTime: time.Now(),
		details: details,
	}:
		syncChLock.RUnlock()

		return true
	default:
		syncChLock.RUnlock()

		logger.Error(
			"resource metrics sync channel is full, dropping end sync metrics update",
			"gateway", details.Gateway,
			"namespace", details.Namespace,
			"resourceType", details.ResourceType,
			"resourceName", details.ResourceName,
		)
		resourcesUpdatesDroppedTotal.Inc()
		return false
	}
}

func endResourceSync(syncInfo *syncStartInfo) {
	startTimes.Lock()
	defer startTimes.Unlock()

	key := syncInfo.details.xdsSyncKey()
	start, exists := startTimes.times[key]
	if !exists {
		return
	}
	delete(startTimes.times, key)

	labels := []metrics.Label{
		{Name: gatewayLabel, Value: key.Gateway},
		{Name: namespaceLabel, Value: key.Namespace},
	}
	xdsSnapshotSyncsTotal.Inc(labels...)
	xdsSnapshotSyncDuration.Observe(syncInfo.endTime.Sub(start).Seconds(), labels...)
}

// GetResourceMetricEventHandler returns a function that handles krt events for various Gateway API resources.
func GetResourceMetricEventHandler[T any]() func(krt.Event[T]) {
	var (
		eventType       controllers.EventType
		resourceType    string
		resourceName    string
		names           []string
		namesOld        []string
		namespace       string
		namespaceOld    string
		clientObject    any
		clientObjectOld any
	)

	return func(o krt.Event[T]) {
		clientObject = o.Latest()
		eventType = o.Event

		// If the event is an update, we must decrement resource metrics using the old label
		// values before incrementing the resource count with the new label values.
		if eventType == controllers.EventUpdate && o.Old != nil {
			clientObjectOld = *o.Old
		}

		switch obj := clientObject.(type) {
		case ir.PolicyWrapper:
			resourceType = obj.Kind
			resourceName = obj.Name
			namespace = obj.Namespace
			names = []string{""}

			if clientObjectOld != nil {
				namespaceOld = clientObjectOld.(ir.PolicyWrapper).Namespace
				namesOld = []string{""}
			}
		case *gwv1.HTTPRoute:
			resourceType = "HTTPRoute"
			resourceName = obj.Name
			namespace = obj.Namespace
			names = make([]string, 0, len(obj.Spec.ParentRefs))
			for _, pr := range obj.Spec.ParentRefs {
				names = append(names, string(pr.Name))
			}

			if clientObjectOld != nil {
				oldObj := clientObjectOld.(*gwv1.HTTPRoute)
				namespaceOld = oldObj.Namespace
				namesOld = make([]string, 0, len(oldObj.Spec.ParentRefs))
				for _, pr := range oldObj.Spec.ParentRefs {
					namesOld = append(namesOld, string(pr.Name))
				}
			}
		case *gwv1a2.TCPRoute:
			resourceType = "TCPRoute"
			resourceName = obj.Name
			namespace = obj.Namespace
			names = make([]string, 0, len(obj.Spec.ParentRefs))
			for _, pr := range obj.Spec.ParentRefs {
				names = append(names, string(pr.Name))
			}

			if clientObjectOld != nil {
				oldObj := clientObjectOld.(*gwv1a2.TCPRoute)
				namespaceOld = oldObj.Namespace
				namesOld = make([]string, 0, len(oldObj.Spec.ParentRefs))
				for _, pr := range oldObj.Spec.ParentRefs {
					namesOld = append(namesOld, string(pr.Name))
				}
			}
		case *gwv1a2.TLSRoute:
			resourceType = "TLSRoute"
			resourceName = obj.Name
			namespace = obj.Namespace
			names = make([]string, 0, len(obj.Spec.ParentRefs))
			for _, pr := range obj.Spec.ParentRefs {
				names = append(names, string(pr.Name))
			}

			if clientObjectOld != nil {
				oldObj := clientObjectOld.(*gwv1a2.TLSRoute)
				namespaceOld = oldObj.Namespace
				namesOld = make([]string, 0, len(oldObj.Spec.ParentRefs))
				for _, pr := range oldObj.Spec.ParentRefs {
					namesOld = append(namesOld, string(pr.Name))
				}
			}
		case *gwv1.GRPCRoute:
			resourceType = "GRPCRoute"
			resourceName = obj.Name
			namespace = obj.Namespace
			names = make([]string, 0, len(obj.Spec.ParentRefs))
			for _, pr := range obj.Spec.ParentRefs {
				names = append(names, string(pr.Name))
			}

			if clientObjectOld != nil {
				oldObj := clientObjectOld.(*gwv1.GRPCRoute)
				namespaceOld = oldObj.Namespace
				namesOld = make([]string, 0, len(oldObj.Spec.ParentRefs))
				for _, pr := range oldObj.Spec.ParentRefs {
					namesOld = append(namesOld, string(pr.Name))
				}
			}
		case *gwv1.Gateway:
			resourceType = "Gateway"
			resourceName = obj.Name
			namespace = obj.Namespace
			names = []string{obj.Name}

			if clientObjectOld != nil {
				namespaceOld = clientObjectOld.(*gwv1.Gateway).Namespace
				namesOld = []string{clientObjectOld.(*gwv1.Gateway).Name}
			}
		case *gwv1.ListenerSet:
			// Promoted ListenerSets and legacy XListenerSets both normalize to
			// *gwv1.ListenerSet, so this arm reports the two flavors as ONE metric series,
			// under the legacy label value "XListenerSet" — including resources that were
			// never XListenerSets.
			//
			// TODO: Rename the label value to "ListenerSet" (or split the two flavors into
			// separate series keyed by their real GVK, if per-flavor visibility is wanted) in
			// a follow-up that updates the metrics tests that assert resource="XListenerSet",
			// and release notes for anyone with dashboards or alerts on the old value.
			resourceType = "XListenerSet"
			resourceName = obj.Name
			namespace = obj.Namespace
			names = []string{string(obj.Spec.ParentRef.Name)}

			if clientObjectOld != nil {
				namespaceOld = clientObjectOld.(*gwv1.ListenerSet).Namespace
				namesOld = []string{string(clientObjectOld.(*gwv1.ListenerSet).Spec.ParentRef.Name)}
			}
		}

		startResourceSync := func(details ResourceSyncDetails) {
			if resourceType == wellknown.GatewayKind {
				StartResourceXDSSync(details)
			}
		}

		switch eventType {
		case controllers.EventAdd:
			for _, name := range names {
				addResourcesManaged(resourceMetricLabels{
					Parent:    name,
					Namespace: namespace,
					Resource:  resourceType,
				}, 1)

				startResourceSync(ResourceSyncDetails{
					Gateway:      name,
					Namespace:    namespace,
					ResourceType: resourceType,
					ResourceName: resourceName,
				})
			}
		case controllers.EventUpdate:
			// Apply the net change, so an update that keeps a parent does not delete and
			// re-create its series as the count passes through zero.
			deltas := make(map[resourceMetricLabels]int, len(namesOld)+len(names))
			for _, name := range namesOld {
				deltas[resourceMetricLabels{Parent: name, Namespace: namespaceOld, Resource: resourceType}]--
			}
			for _, name := range names {
				deltas[resourceMetricLabels{Parent: name, Namespace: namespace, Resource: resourceType}]++
			}
			for labels, delta := range deltas {
				if delta != 0 {
					addResourcesManaged(labels, delta)
				}
			}

			for _, name := range names {
				startResourceSync(ResourceSyncDetails{
					Gateway:      name,
					Namespace:    namespace,
					ResourceType: resourceType,
					ResourceName: resourceName,
				})
			}
		case controllers.EventDelete:
			for _, name := range names {
				addResourcesManaged(resourceMetricLabels{
					Parent:    name,
					Namespace: namespace,
					Resource:  resourceType,
				}, -1)

				details := ResourceSyncDetails{
					Gateway:      name,
					Namespace:    namespace,
					ResourceType: resourceType,
					ResourceName: resourceName,
				}
				if resourceType == wellknown.GatewayKind {
					// A deleted Gateway gets no further XDS sync.
					dropResourceXDSSync(details)
				}
			}
		}
	}
}

// ResetMetrics resets the metrics from this package.
// This is provided for testing purposes only.
func ResetMetrics() {
	xdsSnapshotSyncsTotal.Reset()
	xdsSnapshotSyncDuration.Reset()
	resourcesManaged.Reset()
	resourcesUpdatesDroppedTotal.Reset()

	managedCounts.Lock()
	managedCounts.counts = map[resourceMetricLabels]int{}
	managedCounts.Unlock()

	startTimes.Lock()
	defer startTimes.Unlock()
	startTimes.times = map[ResourceSyncDetails]time.Time{}

	syncChLock.Lock()
	syncCh = make(chan *syncStartInfo, 1024)
	syncChLock.Unlock()
}
