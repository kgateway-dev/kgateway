package setup

import (
	"context"
	"fmt"
	"strings"
	"sync"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	xdsserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/genproto/googleapis/rpc/status"
	"istio.io/istio/pkg/security"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/xds"
	"github.com/kgateway-dev/kgateway/v2/pkg/logging"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
)

const (
	gwNameLabel      = "gateway_name"
	gwNamespaceLabel = "gateway_namespace"
	typeURLLabel     = "type_url"
)

var (
	envoyLogger       = logging.New("xds/envoy")
	envoyXdsSubsystem = "envoy_xds"
	xdsRejectsTotal   = metrics.NewCounter(
		metrics.CounterOpts{
			Subsystem: envoyXdsSubsystem,
			Name:      "rejects_total",
			Help:      "Total xDS rejection episodes by stream and resource type; repeated NACKs count once until a request without an error",
		}, []string{gwNamespaceLabel, gwNameLabel, typeURLLabel})
	xdsRejectsCurrent = metrics.NewGauge(
		metrics.GaugeOpts{
			Subsystem: envoyXdsSubsystem,
			Name:      "rejects_active",
			Help:      "Number of connected stream and resource type pairs with unresolved xDS rejections",
		}, []string{gwNamespaceLabel, gwNameLabel, typeURLLabel})
)

type resourceKey struct {
	Namespace       string
	Name            string
	ResourceTypeUrl string
}

type resourceState struct {
	errors map[resourceKey]struct{}
}

func newResourceState() resourceState {
	return resourceState{
		errors: make(map[resourceKey]struct{}),
	}
}

type logNackCallback struct {
	xdsserver.CallbackFuncs
	streamState    map[int64]resourceState
	streamIdentity map[int64]resourceKey
	xdsAuth        bool

	lock sync.Mutex
}

var _ xdsserver.Callbacks = (*logNackCallback)(nil)

func newLogNackCallback(xdsAuth bool) *logNackCallback {
	return &logNackCallback{
		streamState:    make(map[int64]resourceState),
		streamIdentity: make(map[int64]resourceKey),
		xdsAuth:        xdsAuth,
	}
}

// OnStreamOpen pins metric identity to the authenticated peer. Unauthenticated
// clients share unknown labels rather than creating series from node metadata.
func (l *logNackCallback) OnStreamOpen(ctx context.Context, streamID int64, _ string) error {
	key := resourceKey{Namespace: "unknown", Name: "unknown"}
	if l.xdsAuth {
		caller, ok := ctx.Value(xds.PeerCtxKey).(*security.Caller)
		if !ok || caller == nil {
			return fmt.Errorf("missing authenticated xDS peer for stream %d", streamID)
		}
		key.Namespace = caller.KubernetesInfo.PodNamespace
		key.Name = caller.KubernetesInfo.PodServiceAccount
	}
	l.lock.Lock()
	l.streamIdentity[streamID] = key
	l.lock.Unlock()
	return nil
}

// OnStreamClosed implements server.Callbacks.
func (l *logNackCallback) OnStreamClosed(streamID int64, node *envoycorev3.Node) {
	l.lock.Lock()
	streamState := l.streamState[streamID]
	delete(l.streamState, streamID)
	delete(l.streamIdentity, streamID)
	l.lock.Unlock()

	for k := range streamState.errors {
		l.onErrorGone(k)
	}
}

// OnStreamRequest implements server.Callbacks.
func (l *logNackCallback) OnStreamRequest(streamID int64, req *discoveryv3.DiscoveryRequest) error {
	l.lock.Lock()
	key, ok := l.streamIdentity[streamID]
	l.lock.Unlock()
	if !ok {
		return nil
	}
	key.ResourceTypeUrl = rejectionTypeURL(req.GetTypeUrl())

	if req.ErrorDetail != nil {
		if !l.handleError(streamID, key) {
			// Log NACK only once per resource
			return nil
		}
		l.onNewError(key, req.ErrorDetail)
	} else {
		errorGone := l.handleNoError(streamID, key)
		if errorGone {
			l.onErrorGone(key)
		}
	}
	return nil
}

func (l *logNackCallback) onNewError(key resourceKey, err *status.Status) {
	labels := toLabels(key)
	xdsRejectsTotal.Inc(labels...)
	xdsRejectsCurrent.Add(1, labels...)
	envoyLogger.Warn("xds error", "gateway_name", key.Name, "gateway_ns", key.Namespace, "resource", key.ResourceTypeUrl, "error", err.Message)
}

func (l *logNackCallback) onErrorGone(key resourceKey) {
	xdsRejectsCurrent.Add(-1, toLabels(key)...)
}

func (l *logNackCallback) handleNoError(streamID int64, key resourceKey) bool {
	l.lock.Lock()
	defer l.lock.Unlock()
	streamState := l.streamState[streamID]
	_, hadKey := streamState.errors[key]
	delete(streamState.errors, key)
	return hadKey
}

func (l *logNackCallback) handleError(streamID int64, key resourceKey) bool {
	l.lock.Lock()
	defer l.lock.Unlock()
	streamState := l.streamState[streamID]
	if streamState.errors == nil {
		streamState = newResourceState()
		l.streamState[streamID] = streamState
	}
	if _, exists := streamState.errors[key]; exists {
		return false
	}
	streamState.errors[key] = struct{}{}
	return true
}

func toLabels(key resourceKey) []metrics.Label {
	return []metrics.Label{
		{
			Name:  gwNamespaceLabel,
			Value: key.Namespace,
		},
		{
			Name:  gwNameLabel,
			Value: key.Name,
		},
		{
			Name:  typeURLLabel,
			Value: key.ResourceTypeUrl,
		},
	}
}

// rejectionTypeURL preserves existing labels for known types and bounds all
// other client-supplied type URLs to a single series.
func rejectionTypeURL(typeURL string) string {
	switch typeURL {
	case resourcev3.ClusterType, resourcev3.EndpointType, resourcev3.RouteType,
		resourcev3.ScopedRouteType, resourcev3.VirtualHostType, resourcev3.ListenerType,
		resourcev3.SecretType, resourcev3.ExtensionConfigType, resourcev3.RuntimeType:
		return strings.TrimPrefix(typeURL, "type.googleapis.com/")
	default:
		return "other"
	}
}
