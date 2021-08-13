package discovery

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"sync"

	"go.uber.org/zap"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
)

//go:generate mockgen -destination mocks/mock_discovery.go -package mocks github.com/solo-io/gloo/projects/gloo/pkg/discovery DiscoveryPlugin
type DiscoveryPlugin interface {
	plugins.Plugin

	// UDS API
	// send us an updated list of upstreams on every change
	// namespace is for writing to, not necessarily reading from
	DiscoverUpstreams(watchNamespaces []string, writeNamespace string, opts clients.WatchOpts, discOpts Opts) (chan v1.UpstreamList, chan error, error)
	// finalize any changes to the desired upstream before it gets written
	// for example, copying the functions from the old upstream to the new.
	// a value of false indicates that the resource does not need to be updated
	UpdateUpstream(original, desired *v1.Upstream) (bool, error)

	// EDS API
	// start the EDS watch which sends a new list of endpoints on any change
	// will send only endpoints for upstreams configured with TrackUpstreams
	WatchEndpoints(writeNamespace string, upstreamsToTrack v1.UpstreamList, opts clients.WatchOpts) (<-chan v1.EndpointList, <-chan error, error)
}

type UpstreamDiscovery struct {
	watchNamespaces        []string
	writeNamespace         string
	upstreamReconciler     v1.UpstreamReconciler
	discoveryPlugins       []DiscoveryPlugin
	lock                   sync.Mutex
	latestDesiredUpstreams map[DiscoveryPlugin]v1.UpstreamList
	extraSelectorLabels    map[string]string
}

type EndpointDiscovery struct {
	watchNamespaces    []string
	writeNamespace     string
	endpointReconciler v1.EndpointReconciler
	discoveryPlugins   []DiscoveryPlugin

	readyClosed    bool
	ready          chan struct{}
	endpointsByEds map[DiscoveryPlugin]v1.EndpointList
	lock           sync.Mutex
}

func NewEndpointDiscovery(watchNamespaces []string,
	writeNamespace string,
	endpointsClient v1.EndpointClient,
	discoveryPlugins []DiscoveryPlugin) *EndpointDiscovery {

	return &EndpointDiscovery{
		watchNamespaces:    watchNamespaces,
		writeNamespace:     writeNamespace,
		endpointReconciler: v1.NewEndpointReconciler(endpointsClient),
		discoveryPlugins:   discoveryPlugins,
		ready:              make(chan struct{}),
		endpointsByEds:     make(map[DiscoveryPlugin]v1.EndpointList),
		lock:               sync.Mutex{},
	}
}

func NewUpstreamDiscovery(watchNamespaces []string, writeNamespace string,
	upstreamClient v1.UpstreamClient,
	discoveryPlugins []DiscoveryPlugin) *UpstreamDiscovery {
	return &UpstreamDiscovery{
		watchNamespaces:        watchNamespaces,
		writeNamespace:         writeNamespace,
		upstreamReconciler:     v1.NewUpstreamReconciler(upstreamClient),
		discoveryPlugins:       discoveryPlugins,
		latestDesiredUpstreams: make(map[DiscoveryPlugin]v1.UpstreamList),
	}
}

// launch a goroutine for all the UDS plugins
func (d *UpstreamDiscovery) StartUds(opts clients.WatchOpts, discOpts Opts) (chan error, error) {
	aggregatedErrs := make(chan error)
	d.extraSelectorLabels = opts.Selector
	for _, uds := range d.discoveryPlugins {
		upstreams, errs, err := uds.DiscoverUpstreams(d.watchNamespaces, d.writeNamespace, opts, discOpts)
		if err != nil {
			contextutils.LoggerFrom(opts.Ctx).Warnw("initializing UDS plugin failed", "plugin", reflect.TypeOf(uds).String(), "error", err)
			continue
		}

		go func(uds DiscoveryPlugin) {
			// TODO (ilackarms): when we have less problems, solve this
			udsName := strings.Replace(reflect.TypeOf(uds).String(), "*", "", -1)
			udsName = strings.Replace(udsName, ".", "", -1)
			for {
				select {
				case upstreamList := <-upstreams:
					d.lock.Lock()
					upstreamList = setLabels(udsName, upstreamList)
					d.latestDesiredUpstreams[uds] = upstreamList
					d.lock.Unlock()
					if err := d.Resync(opts.Ctx); err != nil {
						aggregatedErrs <- errors.Wrapf(err, "error in uds plugin %v", reflect.TypeOf(uds).Name())
					}
				case err := <-errs:
					aggregatedErrs <- errors.Wrapf(err, "error in uds plugin %v", reflect.TypeOf(uds).Name())
				case <-opts.Ctx.Done():
					return
				}
			}
		}(uds)
	}
	return aggregatedErrs, nil
}

// ensures that the latest desired upstreams are in sync
func (d *UpstreamDiscovery) Resync(ctx context.Context) error {
	d.lock.Lock()
	defer d.lock.Unlock()
	logger := contextutils.LoggerFrom(ctx)
	for uds, desiredUpstreams := range d.latestDesiredUpstreams {
		udsName := strings.Replace(reflect.TypeOf(uds).String(), "*", "", -1)
		udsName = strings.Replace(udsName, ".", "", -1)
		selector := map[string]string{
			"discovered_by": udsName,
		}
		for k, v := range d.extraSelectorLabels {
			selector[k] = v
		}
		logger.Debugw("reconciling upstream details", zap.Any("upstreams", desiredUpstreams))
		if err := d.upstreamReconciler.Reconcile(d.writeNamespace, desiredUpstreams, uds.UpdateUpstream, clients.ListOpts{
			Ctx:      ctx,
			Selector: selector,
		}); err != nil {
			logger.Errorw("failed reconciling upstreams",
				zap.Any("discovered_by", udsName), zap.Int("upstreams", len(desiredUpstreams)), zap.Error(err))
			return err
		}
		logger.Infow("reconciled upstreams", zap.String("discovered_by", udsName), zap.Int("upstreams", len(desiredUpstreams)))
	}
	return nil
}

func setLabels(udsName string, upstreamList v1.UpstreamList) v1.UpstreamList {
	clone := upstreamList.Clone()
	for _, us := range clone {
		resources.UpdateMetadata(us, func(meta *core.Metadata) {
			if meta.GetLabels() == nil {
				meta.Labels = make(map[string]string)
			}
			meta.GetLabels()["discovered_by"] = udsName
		})
	}
	return clone
}

// launch a goroutine for all the UDS plugins with a single cancel to close them all
func (d *EndpointDiscovery) StartEds(upstreamsToTrack v1.UpstreamList, opts clients.WatchOpts) (chan error, error) {
	aggregatedErrs := make(chan error)
	logger := contextutils.LoggerFrom(opts.Ctx)
	for _, eds := range d.discoveryPlugins {
		endpoints, errs, err := eds.WatchEndpoints(d.writeNamespace, upstreamsToTrack, opts)
		if err != nil {
			logger.Warnw("initializing EDS plugin failed", "plugin", reflect.TypeOf(eds).String(), "error", err)
			continue
		}

		go func(eds DiscoveryPlugin) {
			edsName := reflect.TypeOf(eds).Name()
			for {
				select {
				case endpointList, ok := <-endpoints:
					if !ok {
						return
					}
					d.lock.Lock()
					if _, ok := d.endpointsByEds[eds]; !ok {
						logger.Infof("Received first EDS update from plugin: %T", eds)
					}
					d.endpointsByEds[eds] = endpointList
					desiredEndpoints := aggregateEndpoints(d.endpointsByEds)
					if err := d.endpointReconciler.Reconcile(d.writeNamespace, desiredEndpoints, txnEndpoint, clients.ListOpts{
						Ctx:      opts.Ctx,
						Selector: opts.Selector,
					}); err != nil {
						aggregatedErrs <- err
					}
					if len(d.endpointsByEds) == len(d.discoveryPlugins) {
						d.signalReady()
					}
					d.lock.Unlock()
				case err, ok := <-errs:
					if !ok {
						return
					}
					select {
					case aggregatedErrs <- errors.Wrapf(err, "error in eds plugin %v", edsName):
					default:
						logger.Desugar().Warn("received error and cannot aggregate it.", zap.Error(err))
					}
				case <-opts.Ctx.Done():
					return
				}
			}
		}(eds)
	}
	return aggregatedErrs, nil
}

func (d *EndpointDiscovery) signalReady() {
	if !d.readyClosed {
		d.readyClosed = true
		close(d.ready)
	}
}

func (d *EndpointDiscovery) Ready() <-chan struct{} {
	return d.ready
}

func aggregateEndpoints(endpointsByUds map[DiscoveryPlugin]v1.EndpointList) v1.EndpointList {
	var endpoints v1.EndpointList
	for _, endpointList := range endpointsByUds {
		endpoints = append(endpoints, endpointList...)
	}
	sort.SliceStable(endpoints, func(i, j int) bool {
		return endpoints[i].GetMetadata().Less(endpoints[j].GetMetadata())
	})
	return endpoints
}

func txnEndpoint(original, desired *v1.Endpoint) (bool, error) {
	equal := refsEqual(original.GetUpstreams(), desired.GetUpstreams()) &&
		original.GetAddress() == desired.GetAddress() &&
		original.GetPort() == desired.GetPort()
	return !equal, nil
}

func refsEqual(a, b []*core.ResourceRef) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
