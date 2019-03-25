package discovery

import (
	"reflect"
	"sort"
	"strings"
	"sync"

	"go.uber.org/zap"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"github.com/solo-io/solo-kit/pkg/utils/contextutils"
)

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
	watchNamespaces    []string
	writeNamespace     string
	upstreamReconciler v1.UpstreamReconciler
	discoveryPlugins   []DiscoveryPlugin
}

type EndpointDiscovery struct {
	watchNamespaces    []string
	writeNamespace     string
	endpointReconciler v1.EndpointReconciler
	discoveryPlugins   []DiscoveryPlugin
}

func NewEndpointDiscovery(watchNamespaces []string,
	writeNamespace string,
	endpointsClient v1.EndpointClient,
	discoveryPlugins []DiscoveryPlugin) *EndpointDiscovery {
	return &EndpointDiscovery{
		watchNamespaces:    watchNamespaces,
		writeNamespace:     writeNamespace,
		endpointReconciler: v1.NewEndpointReconciler(endpointsClient),
		discoveryPlugins:   discoveryPlugins}
}

func NewUpstreamDiscovery(watchNamespaces []string, writeNamespace string,
	upstreamClient v1.UpstreamClient,
	discoveryPlugins []DiscoveryPlugin) *UpstreamDiscovery {
	return &UpstreamDiscovery{
		watchNamespaces:    watchNamespaces,
		writeNamespace:     writeNamespace,
		upstreamReconciler: v1.NewUpstreamReconciler(upstreamClient),
		discoveryPlugins:   discoveryPlugins,
	}
}

// launch a goroutine for all the UDS plugins
func (d *UpstreamDiscovery) StartUds(opts clients.WatchOpts, discOpts Opts) (chan error, error) {
	aggregatedErrs := make(chan error)
	upstreamsByUds := make(map[DiscoveryPlugin]v1.UpstreamList)
	lock := sync.Mutex{}
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
			selector := map[string]string{
				"discovered_by": udsName,
			}
			for k, v := range opts.Selector {
				selector[k] = v
			}
			for {
				select {
				case upstreamList := <-upstreams:
					lock.Lock()
					upstreamList = setLabels(udsName, upstreamList)
					upstreamsByUds[uds] = upstreamList
					desiredUpstreams := aggregateUpstreams(upstreamsByUds)
					if err := d.upstreamReconciler.Reconcile(d.writeNamespace, desiredUpstreams, uds.UpdateUpstream, clients.ListOpts{
						Ctx:      opts.Ctx,
						Selector: selector,
					}); err != nil {
						aggregatedErrs <- errors.Wrapf(err, "reconciling %v desired upstreams", len(desiredUpstreams))
					}
					lock.Unlock()
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

func setLabels(udsName string, upstreamList v1.UpstreamList) v1.UpstreamList {
	clone := upstreamList.Clone()
	for _, us := range clone {
		resources.UpdateMetadata(us, func(meta *core.Metadata) {
			if meta.Labels == nil {
				meta.Labels = make(map[string]string)
			}
			meta.Labels["discovered_by"] = udsName
		})
	}
	return clone
}

func aggregateUpstreams(endpointsByUds map[DiscoveryPlugin]v1.UpstreamList) v1.UpstreamList {
	var upstreams v1.UpstreamList
	for _, upstreamList := range endpointsByUds {
		upstreams = append(upstreams, upstreamList...)
	}
	sort.SliceStable(upstreams, func(i, j int) bool {
		return upstreams[i].Metadata.Less(upstreams[j].Metadata)
	})
	return upstreams
}

// launch a goroutine for all the UDS plugins with a single cancel to close them all
func (d *EndpointDiscovery) StartEds(upstreamsToTrack v1.UpstreamList, opts clients.WatchOpts) (chan error, error) {
	aggregatedErrs := make(chan error)
	endpointsByEds := make(map[DiscoveryPlugin]v1.EndpointList)
	lock := sync.Mutex{}
	for _, eds := range d.discoveryPlugins {
		endpoints, errs, err := eds.WatchEndpoints(d.writeNamespace, upstreamsToTrack, opts)
		if err != nil {
			contextutils.LoggerFrom(opts.Ctx).Warnw("initializing EDS plugin failed", "plugin", reflect.TypeOf(eds).String(), "error", err)
			continue
		}

		go func(eds DiscoveryPlugin) {
			for {
				select {
				case endpointList, ok := <-endpoints:
					if !ok {
						return
					}
					lock.Lock()
					endpointsByEds[eds] = endpointList
					desiredEndpoints := aggregateEndpoints(endpointsByEds)
					if err := d.endpointReconciler.Reconcile(d.writeNamespace, desiredEndpoints, txnEndpoint, clients.ListOpts{
						Ctx:      opts.Ctx,
						Selector: opts.Selector,
					}); err != nil {
						aggregatedErrs <- err
					}
					lock.Unlock()
				case err, ok := <-errs:
					if !ok {
						return
					}
					select {
					case aggregatedErrs <- errors.Wrapf(err, "error in eds plugin %v", reflect.TypeOf(eds).Name()):
					default:
						contextutils.LoggerFrom(opts.Ctx).Desugar().Warn("received error and cannot aggregate it.", zap.Error(err))
					}
				case <-opts.Ctx.Done():
					return
				}
			}
		}(eds)
	}
	return aggregatedErrs, nil
}

func aggregateEndpoints(endpointsByUds map[DiscoveryPlugin]v1.EndpointList) v1.EndpointList {
	var endpoints v1.EndpointList
	for _, endpointList := range endpointsByUds {
		endpoints = append(endpoints, endpointList...)
	}
	sort.SliceStable(endpoints, func(i, j int) bool {
		return endpoints[i].Metadata.Less(endpoints[j].Metadata)
	})
	return endpoints
}

func txnEndpoint(original, desired *v1.Endpoint) (bool, error) {
	equal := refsEqual(original.Upstreams, desired.Upstreams) &&
		original.Address == desired.Address &&
		original.Port == desired.Port
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
