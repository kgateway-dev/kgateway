package metrics

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"istio.io/istio/pkg/kube/controllers"
	"istio.io/istio/pkg/kube/krt"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
	gwv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gwxv1a1 "sigs.k8s.io/gateway-api/apisx/v1alpha1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/ir"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
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

	resourcesHistogramBuckets = []float64{0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600, 1200, 1800}
	resourcesManaged          = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: resourcesSubsystem,
			Name:      "managed",
			Help:      "Current number of resources managed",
		},
		[]string{namespaceLabel, parentLabel, resourceLabel},
	)
	resourcesStatusSyncsStartedTotal = metrics.NewCounter(metrics.CounterOpts{
		Subsystem: resourcesSubsystem,
		Name:      "status_syncs_started_total",
		Help:      "Total number of status syncs started",
	},
		[]string{gatewayLabel, namespaceLabel, resourceLabel})
	resourcesStatusSyncsCompletedTotal = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: resourcesSubsystem,
			Name:      "status_syncs_completed_total",
			Help:      "Total number of status syncs completed for resources",
		},
		[]string{gatewayLabel, namespaceLabel, resourceLabel})
	resourcesStatusSyncDuration = metrics.NewHistogram(
		metrics.HistogramOpts{
			Subsystem:                       resourcesSubsystem,
			Name:                            "status_sync_duration_seconds",
			Help:                            "Duration of time for a resource update to receive a status report",
			Buckets:                         resourcesHistogramBuckets,
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  100,
			NativeHistogramMinResetDuration: time.Hour,
		},
		[]string{gatewayLabel, namespaceLabel, resourceLabel},
	)
	resourcesUpdatesDroppedTotal = metrics.NewCounter(metrics.CounterOpts{
		Subsystem: resourcesSubsystem,
		Name:      "updates_dropped_total",
		Help:      "Total number of resources metrics updates dropped. If this metric is ever greater than 0, all resources subsystem metrics should be considered invalid until process restart",
	}, nil)
)

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

// ResourceSyncStartTime represents the start time of a resource sync.
type ResourceSyncStartTime struct {
	Time         time.Time
	ResourceType string
	ResourceName string
	Namespace    string
	Gateway      string
}

// resourceSyncStartTimes tracks the start times of resource syncs.
type resourceSyncStartTimes struct {
	sync.RWMutex
	times map[string]map[string]map[string]map[string]ResourceSyncStartTime
}

var startTimes = &resourceSyncStartTimes{}

// ResourceSyncDetails holds the details of a resource sync operation.
type ResourceSyncDetails struct {
	Namespace    string
	Gateway      string
	ResourceType string
	ResourceName string
}

type syncStartInfo struct {
	endTime     time.Time
	details     ResourceSyncDetails
	xdsSnapshot bool
}

// Buffered channel to handle resource sync metrics updates.
// The buffer size is assumed to be sufficient for any reasonable load.
// But, this may need to be configurable in the future, if needed for very high load.
var syncCh = make(chan *syncStartInfo, 1024)
var syncChLock sync.RWMutex

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

// StartResourceStatusSync records the start time of a status sync for a given resource and
// increments the resource syncs started counter.
func StartResourceStatusSync(details ResourceSyncDetails) {
	if !metrics.Active() {
		return
	}

	startTimes.Lock()
	defer startTimes.Unlock()

	if startTimes.times == nil {
		startTimes.times = make(map[string]map[string]map[string]map[string]ResourceSyncStartTime)
	}

	if startTimes.times[details.Gateway] == nil {
		startTimes.times[details.Gateway] = make(map[string]map[string]map[string]ResourceSyncStartTime)
	}

	st := ResourceSyncStartTime{
		Time:         time.Now(),
		ResourceType: details.ResourceType,
		ResourceName: details.ResourceName,
		Namespace:    details.Namespace,
		Gateway:      details.Gateway,
	}

	if startTimes.times[details.Gateway][details.ResourceType] == nil {
		startTimes.times[details.Gateway][details.ResourceType] = make(map[string]map[string]ResourceSyncStartTime)
	}

	if startTimes.times[details.Gateway][details.ResourceType][details.Namespace] == nil {
		startTimes.times[details.Gateway][details.ResourceType][details.Namespace] = make(map[string]ResourceSyncStartTime)
	}

	if _, exists := startTimes.times[details.Gateway][details.ResourceType][details.Namespace][details.ResourceName]; !exists {
		startTimes.times[details.Gateway][details.ResourceType][details.Namespace][details.ResourceName] = st

		resourcesStatusSyncsStartedTotal.Inc(
			metrics.Label{Name: gatewayLabel, Value: details.Gateway},
			metrics.Label{Name: namespaceLabel, Value: details.Namespace},
			metrics.Label{Name: resourceLabel, Value: details.ResourceType},
		)
	}
}

// StartResourceXDSSync records the start time of a XDS snapshot sync.
func StartResourceXDSSync(details ResourceSyncDetails) {
	if !metrics.Active() {
		return
	}

	startTimes.Lock()
	defer startTimes.Unlock()

	if startTimes.times == nil {
		startTimes.times = make(map[string]map[string]map[string]map[string]ResourceSyncStartTime)
	}

	if startTimes.times[details.Gateway] == nil {
		startTimes.times[details.Gateway] = make(map[string]map[string]map[string]ResourceSyncStartTime)
	}

	st := ResourceSyncStartTime{
		Time:         time.Now(),
		ResourceType: details.ResourceType,
		ResourceName: details.ResourceName,
		Namespace:    details.Namespace,
		Gateway:      details.Gateway,
	}

	if startTimes.times[details.Gateway][snapshotResourceType] == nil {
		startTimes.times[details.Gateway][snapshotResourceType] = make(map[string]map[string]ResourceSyncStartTime)
	}

	if startTimes.times[details.Gateway][snapshotResourceType][details.Namespace] == nil {
		startTimes.times[details.Gateway][snapshotResourceType][details.Namespace] = make(map[string]ResourceSyncStartTime)
	}

	startTimes.times[details.Gateway][snapshotResourceType][details.Namespace][details.ResourceName] = st
}

// EndResourceStatusSync records the end time of a status sync for a given resource and
// updates the resource sync metrics accordingly.
// Returns true if the sync was added to the channel, false if the channel is full.
// If the channel is full, an error is logged to call attention to the issue.
// The caller is not expected to handle this case.
func EndResourceStatusSync(details ResourceSyncDetails) bool {
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
		endTime:     time.Now(),
		details:     details,
		xdsSnapshot: false,
	}:
		syncChLock.RUnlock()

		return true
	default:
		syncChLock.RUnlock()

		logger.Log(context.Background(), slog.LevelError,
			"resource metrics sync channel is full, dropping end sync metrics update",
			"gateway", details.Gateway,
			"namespace", details.Namespace,
			"resourceType", details.ResourceType,
			"resourceName", details.ResourceName,
			"xdsSnapshot", false,
		)
		resourcesUpdatesDroppedTotal.Inc()
		return false
	}
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
		endTime:     time.Now(),
		details:     details,
		xdsSnapshot: true,
	}:
		syncChLock.RUnlock()

		return true
	default:
		syncChLock.RUnlock()

		logger.Log(context.Background(), slog.LevelError,
			"resource metrics sync channel is full, dropping end sync metrics update",
			"gateway", details.Gateway,
			"namespace", details.Namespace,
			"resourceType", details.ResourceType,
			"resourceName", details.ResourceName,
			"xdsSnapshot", true,
		)
		resourcesUpdatesDroppedTotal.Inc()
		return false
	}
}

func endResourceSync(syncInfo *syncStartInfo) {
	startTimes.Lock()
	defer startTimes.Unlock()

	if startTimes.times == nil {
		return
	}

	if startTimes.times[syncInfo.details.Gateway] == nil {
		return
	}

	rn := syncInfo.details.ResourceName
	rt := syncInfo.details.ResourceType

	if syncInfo.xdsSnapshot {
		rt = snapshotResourceType
		resourceTypeStartTimes, exists := startTimes.times[syncInfo.details.Gateway][rt]
		if !exists {
			return
		}

		deleteResources := map[string]map[string]struct{}{}

		for _, namespaceStartTimes := range resourceTypeStartTimes {
			for resourceName, st := range namespaceStartTimes {
				xdsSnapshotSyncsTotal.Inc(
					metrics.Label{Name: gatewayLabel, Value: st.Gateway},
					metrics.Label{Name: namespaceLabel, Value: st.Namespace},
				)

				xdsSnapshotSyncDuration.Observe(syncInfo.endTime.Sub(st.Time).Seconds(),
					metrics.Label{Name: gatewayLabel, Value: st.Gateway},
					metrics.Label{Name: namespaceLabel, Value: st.Namespace},
				)

				if deleteResources[st.Namespace] == nil {
					deleteResources[st.Namespace] = map[string]struct{}{}
				}

				deleteResources[st.Namespace][resourceName] = struct{}{}
			}
		}

		for namespace, resources := range deleteResources {
			for resourceName := range resources {
				delete(startTimes.times[syncInfo.details.Gateway][rt][namespace], resourceName)

				if len(startTimes.times[syncInfo.details.Gateway][rt][namespace]) == 0 {
					delete(startTimes.times[syncInfo.details.Gateway][rt], namespace)
				}

				if len(startTimes.times[syncInfo.details.Gateway][rt]) == 0 {
					delete(startTimes.times[syncInfo.details.Gateway], rt)
				}

				if len(startTimes.times[syncInfo.details.Gateway]) == 0 {
					delete(startTimes.times, syncInfo.details.Gateway)
				}
			}
		}

		return
	}

	if startTimes.times[syncInfo.details.Gateway][rt] == nil {
		return
	}

	if startTimes.times[syncInfo.details.Gateway][rt][syncInfo.details.Namespace] == nil {
		return
	}

	st, exists := startTimes.times[syncInfo.details.Gateway][rt][syncInfo.details.Namespace][rn]
	if !exists {
		return
	}

	resourcesStatusSyncsCompletedTotal.Inc(
		metrics.Label{Name: gatewayLabel, Value: st.Gateway},
		metrics.Label{Name: namespaceLabel, Value: st.Namespace},
		metrics.Label{Name: resourceLabel, Value: st.ResourceType},
	)

	resourcesStatusSyncDuration.Observe(syncInfo.endTime.Sub(st.Time).Seconds(),
		metrics.Label{Name: gatewayLabel, Value: st.Gateway},
		metrics.Label{Name: namespaceLabel, Value: st.Namespace},
		metrics.Label{Name: resourceLabel, Value: st.ResourceType},
	)

	delete(startTimes.times[syncInfo.details.Gateway][rt][syncInfo.details.Namespace], rn)

	if len(startTimes.times[syncInfo.details.Gateway][rt][syncInfo.details.Namespace]) == 0 {
		delete(startTimes.times[syncInfo.details.Gateway][rt], syncInfo.details.Namespace)
	}

	if len(startTimes.times[syncInfo.details.Gateway][rt]) == 0 {
		delete(startTimes.times[syncInfo.details.Gateway], rt)
	}

	if len(startTimes.times[syncInfo.details.Gateway]) == 0 {
		delete(startTimes.times, syncInfo.details.Gateway)
	}
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
		case *gwxv1a1.XListenerSet:
			resourceType = "XListenerSet"
			resourceName = obj.Name
			namespace = obj.Namespace
			names = []string{string(obj.Spec.ParentRef.Name)}

			if clientObjectOld != nil {
				namespaceOld = clientObjectOld.(*gwxv1a1.XListenerSet).Namespace
				namesOld = []string{string(clientObjectOld.(*gwxv1a1.XListenerSet).Spec.ParentRef.Name)}
			}
		}

		startResourceSync := func(details ResourceSyncDetails) {
			StartResourceStatusSync(details)

			if resourceType == wellknown.GatewayKind {
				StartResourceXDSSync(details)
			}
		}

		switch eventType {
		case controllers.EventAdd:
			for _, name := range names {
				resourcesManaged.Add(1, resourceMetricLabels{
					Parent:    name,
					Namespace: namespace,
					Resource:  resourceType,
				}.toMetricsLabels()...)

				startResourceSync(ResourceSyncDetails{
					Gateway:      name,
					Namespace:    namespace,
					ResourceType: resourceType,
					ResourceName: resourceName,
				})
			}
		case controllers.EventUpdate:
			for _, name := range namesOld {
				resourcesManaged.Sub(1, resourceMetricLabels{
					Parent:    name,
					Namespace: namespaceOld,
					Resource:  resourceType,
				}.toMetricsLabels()...)
			}

			for _, name := range names {
				resourcesManaged.Add(1, resourceMetricLabels{
					Parent:    name,
					Namespace: namespace,
					Resource:  resourceType,
				}.toMetricsLabels()...)

				startResourceSync(ResourceSyncDetails{
					Gateway:      name,
					Namespace:    namespace,
					ResourceType: resourceType,
					ResourceName: resourceName,
				})
			}
		case controllers.EventDelete:
			for _, name := range names {
				resourcesManaged.Sub(1, resourceMetricLabels{
					Parent:    name,
					Namespace: namespace,
					Resource:  resourceType,
				}.toMetricsLabels()...)

				startResourceSync(ResourceSyncDetails{
					Gateway:      name,
					Namespace:    namespace,
					ResourceType: resourceType,
					ResourceName: resourceName,
				})

				// There will not be a status sync for a deleted resource,
				// so end the resource status sync immediately.
				EndResourceStatusSync(ResourceSyncDetails{
					Gateway:      name,
					Namespace:    namespace,
					ResourceType: resourceType,
					ResourceName: resourceName,
				})
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
	resourcesStatusSyncsStartedTotal.Reset()
	resourcesStatusSyncsCompletedTotal.Reset()
	resourcesStatusSyncDuration.Reset()
	resourcesUpdatesDroppedTotal.Reset()

	startTimes.Lock()
	defer startTimes.Unlock()
	startTimes.times = make(map[string]map[string]map[string]map[string]ResourceSyncStartTime)

	syncChLock.Lock()
	syncCh = make(chan *syncStartInfo, 1024)
	syncChLock.Unlock()
}
