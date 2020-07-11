package syncer

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"go.uber.org/zap/zapcore"

	"github.com/hashicorp/go-multierror"
	"github.com/solo-io/gloo/pkg/utils/syncutil"
	"github.com/solo-io/gloo/projects/gateway/pkg/reconciler"
	"github.com/solo-io/go-utils/hashutils"
	"go.uber.org/zap"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/pkg/errors"
)

type translatorSyncer struct {
	writeNamespace  string
	reporter        reporter.Reporter
	proxyClient     gloov1.ProxyClient
	gwClient        v1.GatewayClient
	vsClient        v1.VirtualServiceClient
	proxyReconciler reconciler.ProxyReconciler
	translator      translator.Translator
	statusSyncer    statusSyncer
	labels          map[string]string
}

func NewTranslatorSyncer(ctx context.Context, writeNamespace string, proxyClient gloov1.ProxyClient, proxyReconciler reconciler.ProxyReconciler, gwClient v1.GatewayClient, vsClient v1.VirtualServiceClient, reporter reporter.Reporter, translator translator.Translator) v1.ApiSyncer {
	t := &translatorSyncer{
		writeNamespace:  writeNamespace,
		reporter:        reporter,
		proxyClient:     proxyClient,
		gwClient:        gwClient,
		vsClient:        vsClient,
		proxyReconciler: proxyReconciler,
		translator:      translator,
		statusSyncer:    newStatusSyncer(writeNamespace, proxyClient, reporter),
		labels: map[string]string{
			"created_by": "gateway",
		},
	}

	go t.statusSyncer.watchProxies(ctx)
	go t.statusSyncer.syncStatusOnInterval(ctx)
	return t
}

// TODO (ilackarms): make sure that sync happens if proxies get updated as well; may need to resync
func (s *translatorSyncer) Sync(ctx context.Context, snap *v1.ApiSnapshot) error {
	ctx = contextutils.WithLogger(ctx, "translatorSyncer")

	logger := contextutils.LoggerFrom(ctx)
	logger.Debugw("begin sync", zap.Any("snapshot", snap.Stringer()))
	snapHash := hashutils.MustHash(snap)
	logger.Infof("begin sync %v (%v virtual services, %v gateways, %v route tables)", snapHash,
		len(snap.VirtualServices), len(snap.Gateways), len(snap.RouteTables))
	defer logger.Infof("end sync %v", snapHash)

	// stringify-ing the snapshot may be an expensive operation, so we'd like to avoid building the large
	// string if we're not even going to log it anyway
	if contextutils.GetLogLevel() == zapcore.DebugLevel {
		logger.Debug(syncutil.StringifySnapshot(snap))
	}

	gatewaysByProxy := utils.GatewaysByProxyName(snap.Gateways)

	desiredProxies := make(reconciler.GeneratedProxies)

	for proxyName, gatewayList := range gatewaysByProxy {
		proxy, reports := s.translator.Translate(ctx, proxyName, s.writeNamespace, snap, gatewayList)
		if proxy != nil {
			logger.Infof("desired proxy %v", proxy.Metadata.Ref())
			proxy.Metadata.Labels = s.labels
			desiredProxies[proxy] = reports
		}
	}

	return s.reconcile(ctx, desiredProxies)
}

func (s *translatorSyncer) reconcile(ctx context.Context, desiredProxies reconciler.GeneratedProxies) error {
	if err := s.proxyReconciler.ReconcileProxies(ctx, desiredProxies, s.writeNamespace, s.labels); err != nil {
		return err
	}

	// repeat for all resources
	s.statusSyncer.setCurrentProxies(desiredProxies)
	s.statusSyncer.scheduleSync()
	return nil
}

type reportsAndStatus struct {
	Status  *core.Status
	Reports reporter.ResourceReports
}
type statusSyncer struct {
	proxyToLastStatus       map[core.ResourceRef]reportsAndStatus
	currentGeneratedProxies []core.ResourceRef
	mapLock                 sync.RWMutex
	reporter                reporter.Reporter

	proxyClient    gloov1.ProxyWatcher
	writeNamespace string
	syncNeeded     chan struct{}
}

func newStatusSyncer(writeNamespace string, proxyClient gloov1.ProxyWatcher, reporter reporter.Reporter) statusSyncer {
	return statusSyncer{
		proxyToLastStatus:       map[core.ResourceRef]reportsAndStatus{},
		currentGeneratedProxies: nil,
		reporter:                reporter,
		proxyClient:             proxyClient,
		writeNamespace:          writeNamespace,
		syncNeeded:              make(chan struct{}, 1),
	}
}

func (s *statusSyncer) setCurrentProxies(desiredProxies reconciler.GeneratedProxies) {
	s.mapLock.Lock()
	defer s.mapLock.Unlock()
	s.currentGeneratedProxies = nil
	for proxy, reports := range desiredProxies {
		// start propagating for new set of resources
		ref := proxy.GetMetadata().Ref()
		if _, ok := s.proxyToLastStatus[ref]; !ok {
			s.proxyToLastStatus[ref] = reportsAndStatus{}
		}
		current := s.proxyToLastStatus[ref]
		// These reports are for gateway resources: VirtualServices and Gateways
		current.Reports = reports
		s.proxyToLastStatus[ref] = current
		s.currentGeneratedProxies = append(s.currentGeneratedProxies, ref)
	}
	sort.SliceStable(s.currentGeneratedProxies, func(i, j int) bool {
		refi := s.currentGeneratedProxies[i]
		refj := s.currentGeneratedProxies[j]
		if refi.Namespace != refj.Namespace {
			return refi.Namespace < refj.Namespace
		}
		return refi.Name < refj.Name
	})
}

// run this in the background
func (s *statusSyncer) watchProxies(ctx context.Context) error {
	ctx = contextutils.WithLogger(ctx, "proxy-err-watcher")
	proxies, errs, err := s.proxyClient.Watch(s.writeNamespace, clients.WatchOpts{
		Ctx: ctx,
	})
	if err != nil {
		return errors.Wrapf(err, "creating watch for proxies in %v", s.writeNamespace)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-errs:
			if !ok {
				return nil
			}
			contextutils.LoggerFrom(ctx).Error(err)
		case proxyList, ok := <-proxies:
			if !ok {
				return nil
			}
			s.setStatuses(proxyList)
			s.scheduleSync()
		}
	}
}

func (s *statusSyncer) setStatuses(list gloov1.ProxyList) {
	s.mapLock.Lock()
	defer s.mapLock.Unlock()
	for _, proxy := range list {
		ref := proxy.Metadata.Ref()
		status := proxy.Status
		if current, ok := s.proxyToLastStatus[ref]; ok {
			current.Status = &status
			s.proxyToLastStatus[ref] = current
		} else {
			s.proxyToLastStatus[ref] = reportsAndStatus{
				Status: &status,
			}
		}
	}
}

// run this on a timer
func (s *statusSyncer) scheduleSync() {
	select {
	case s.syncNeeded <- struct{}{}:
	default:
	}
}

func (s *statusSyncer) syncStatusOnInterval(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.syncNeeded:
			err := s.syncStatus(ctx)
			if err != nil {
				contextutils.LoggerFrom(ctx).Debugw("failed to sync status; will try again shortly.", "error", err)
			}
		}
	}
}

// TODO: it is possible that sync status is a heavy function?
// why does adding ginkgo write helps somewhat?
// do i need better mechanism? to sync only when sync happens and/or new proxy list?
func (s *statusSyncer) syncStatus(ctx context.Context) error {
	var nilProxy *gloov1.Proxy
	allReports := reporter.ResourceReports{}
	inputResourceBySubresourceStatuses := map[resources.InputResource]map[string]*core.Status{}
	func() {
		s.mapLock.RLock()
		defer s.mapLock.RUnlock()
		// iterate s.currentGeneratedProxies to guarantee order
		for _, ref := range s.currentGeneratedProxies {
			reportsAndStatus, ok := s.proxyToLastStatus[ref]
			if !ok {
				continue
			}
			// merge all the reports for the gateway resources from all the proxies.
			for inputResource, subresourceStatuses := range reportsAndStatus.Reports {
				if reportsAndStatus.Status != nil {
					// add the proxy status as well if we have it
					status := *reportsAndStatus.Status
					if _, ok := inputResourceBySubresourceStatuses[inputResource]; !ok {
						inputResourceBySubresourceStatuses[inputResource] = map[string]*core.Status{}
					}
					inputResourceBySubresourceStatuses[inputResource][fmt.Sprintf("%T.%s", nilProxy, ref.Key())] = &status
				}
				if report, ok := allReports[inputResource]; ok {
					if subresourceStatuses.Errors != nil {
						report.Errors = multierror.Append(report.Errors, subresourceStatuses.Errors)
					}
					if subresourceStatuses.Warnings != nil {
						report.Warnings = append(report.Warnings, subresourceStatuses.Warnings...)
					}
				} else {
					allReports[inputResource] = subresourceStatuses
				}
			}
		}
	}()

	var errs error
	for inputResource, subresourceStatuses := range allReports {
		reports := reporter.ResourceReports{inputResource: subresourceStatuses}
		currentStatuses := inputResourceBySubresourceStatuses[inputResource]
		if err := s.reporter.WriteReports(ctx, reports, currentStatuses); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	return errs
}
