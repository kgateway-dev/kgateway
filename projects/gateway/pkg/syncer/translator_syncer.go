package syncer

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	gloo_translator "github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"go.uber.org/zap/zapcore"

	"github.com/hashicorp/go-multierror"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	"github.com/solo-io/gloo/pkg/utils/syncutil"
	"github.com/solo-io/gloo/projects/gateway/pkg/reconciler"
	"github.com/solo-io/go-utils/hashutils"
	"go.uber.org/zap"

	v1 "github.com/solo-io/gloo/projects/gateway/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/gloo/projects/gateway/pkg/utils"
	"github.com/solo-io/gloo/projects/gloo/pkg/api/compress"
	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
	"github.com/solo-io/solo-kit/pkg/errors"
)

type translatorSyncer struct {
	writeNamespace     string
	reporter           reporter.Reporter
	proxyWatcher       gloov1.ProxyWatcher
	proxyReconciler    reconciler.ProxyReconciler
	translator         translator.Translator
	statusSyncer       statusSyncer
	managedProxyLabels map[string]string
}

func NewTranslatorSyncer(ctx context.Context, writeNamespace string, proxyWatcher gloov1.ProxyWatcher, proxyReconciler reconciler.ProxyReconciler, reporter reporter.StatusReporter, translator translator.Translator) v1.ApiSyncer {
	t := &translatorSyncer{
		writeNamespace:  writeNamespace,
		reporter:        reporter,
		proxyWatcher:    proxyWatcher,
		proxyReconciler: proxyReconciler,
		translator:      translator,
		statusSyncer:    newStatusSyncer(writeNamespace, proxyWatcher, reporter),
		managedProxyLabels: map[string]string{
			"created_by": "gateway",
		},
	}

	go t.statusSyncer.watchProxies(ctx)
	go t.statusSyncer.syncStatusOnEmit(ctx)
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

	desiredProxies := s.generatedDesiredProxies(ctx, snap)

	return s.reconcile(ctx, desiredProxies)
}

func (s *translatorSyncer) generatedDesiredProxies(ctx context.Context, snap *v1.ApiSnapshot) reconciler.GeneratedProxies {
	logger := contextutils.LoggerFrom(ctx)
	gatewaysByProxy := utils.GatewaysByProxyName(snap.Gateways)

	desiredProxies := make(reconciler.GeneratedProxies)

	for proxyName, gatewayList := range gatewaysByProxy {
		proxy, reports := s.translator.Translate(ctx, proxyName, s.writeNamespace, snap, gatewayList)
		if proxy != nil {

			if s.shouldCompresss(ctx) {
				compress.SetShouldCompressed(proxy)
			}

			logger.Infof("desired proxy %v", proxy.GetMetadata().Ref())
			proxy.GetMetadata().Labels = s.managedProxyLabels
			desiredProxies[proxy] = reports
		}
	}
	return desiredProxies
}

func (s *translatorSyncer) shouldCompresss(ctx context.Context) bool {
	return settingsutil.MaybeFromContext(ctx).GetGateway().GetCompressedProxySpec()
}

func (s *translatorSyncer) reconcile(ctx context.Context, desiredProxies reconciler.GeneratedProxies) error {
	if err := s.proxyReconciler.ReconcileProxies(ctx, desiredProxies, s.writeNamespace, s.managedProxyLabels); err != nil {
		return err
	}

	// repeat for all resources
	s.statusSyncer.setCurrentProxies(desiredProxies)
	s.statusSyncer.forceSync()
	return nil
}

type reportsAndStatus struct {
	Status  *core.Status
	Reports reporter.ResourceReports
}
type statusSyncer struct {
	proxyToLastStatus       map[string]reportsAndStatus
	inputResourceLastStatus map[resources.InputResource]*core.Status
	currentGeneratedProxies []*core.ResourceRef
	mapLock                 sync.RWMutex
	reporter                reporter.StatusReporter

	proxyWatcher   gloov1.ProxyWatcher
	writeNamespace string
	syncNeeded     chan struct{}
}

func newStatusSyncer(writeNamespace string, proxyWatcher gloov1.ProxyWatcher, reporter reporter.StatusReporter) statusSyncer {
	return statusSyncer{
		proxyToLastStatus:       map[string]reportsAndStatus{},
		currentGeneratedProxies: nil,
		reporter:                reporter,
		proxyWatcher:            proxyWatcher,
		writeNamespace:          writeNamespace,
		syncNeeded:              make(chan struct{}, 1),
	}
}

func (s *statusSyncer) setCurrentProxies(desiredProxies reconciler.GeneratedProxies) {
	s.mapLock.Lock()
	defer s.mapLock.Unlock()
	// clear out the status map
	s.inputResourceLastStatus = make(map[resources.InputResource]*core.Status)
	s.currentGeneratedProxies = nil
	for proxy, reports := range desiredProxies {
		// start propagating for new set of resources
		ref := proxy.GetMetadata().Ref()
		refKey := gloo_translator.UpstreamToClusterName(ref)
		if _, ok := s.proxyToLastStatus[refKey]; !ok {
			s.proxyToLastStatus[refKey] = reportsAndStatus{}
		}
		current := s.proxyToLastStatus[refKey]
		// These reports are for gateway resources: VirtualServices, RouteTables and Gateways
		current.Reports = reports
		s.proxyToLastStatus[refKey] = current
		s.currentGeneratedProxies = append(s.currentGeneratedProxies, ref)
	}
	sort.SliceStable(s.currentGeneratedProxies, func(i, j int) bool {
		refi := s.currentGeneratedProxies[i]
		refj := s.currentGeneratedProxies[j]
		if refi.GetNamespace() != refj.GetNamespace() {
			return refi.GetNamespace() < refj.GetNamespace()
		}
		return refi.GetName() < refj.GetName()
	})
}

// run this in the background
func (s *statusSyncer) watchProxies(ctx context.Context) error {
	ctx = contextutils.WithLogger(ctx, "proxy-err-watcher")
	logger := contextutils.LoggerFrom(ctx)
	defer logger.Debugw("done watching proxies")
	proxies, errs, err := s.proxyWatcher.Watch(s.writeNamespace, clients.WatchOpts{
		Ctx: ctx,
	})
	if err != nil {
		return errors.Wrapf(err, "creating watch for proxies in %v", s.writeNamespace)
	}
	return s.watchProxiesFromChannel(ctx, proxies, errs)
}

func (s *statusSyncer) watchProxiesFromChannel(ctx context.Context, proxies <-chan gloov1.ProxyList, errs <-chan error) error {

	logger := contextutils.LoggerFrom(ctx)
	var previousHash uint64
	for {
		select {
		case <-ctx.Done():
			return nil
		case err, ok := <-errs:
			if !ok {
				return nil
			}
			logger.Error(err)
		case proxyList, ok := <-proxies:
			if !ok {
				return nil
			}

			currentHash, err := hashStatuses(proxyList)
			if err != nil {
				logger.DPanicw("error while hashing, this should never happen", zap.Error(err))
			}
			// We use hashing here to be compatible with the memory client used in
			// the local e2e; it fires a watch update too all watch object, on any change,
			// this means that setting by the status of a virtual service we will get another
			// proxyList from the channel. This results in excessive CPU usage in CI.
			if currentHash != previousHash {
				logger.Debugw("proxy list updated", "len(proxyList)", len(proxyList), "currentHash", currentHash, "previousHash", previousHash)
				previousHash = currentHash
				s.setStatuses(proxyList)
				s.forceSync()
			}
		}
	}
}

func hashStatuses(proxyList gloov1.ProxyList) (uint64, error) {
	statuses := make([]interface{}, 0, len(proxyList))
	for _, proxy := range proxyList {
		statuses = append(statuses, proxy.GetStatus())
	}
	return hashutils.HashAllSafe(nil, statuses...)
}

func (s *statusSyncer) setStatuses(list gloov1.ProxyList) {
	s.mapLock.Lock()
	defer s.mapLock.Unlock()
	for _, proxy := range list {
		ref := proxy.GetMetadata().Ref()
		refKey := gloo_translator.UpstreamToClusterName(ref)
		status := proxy.Status
		if current, ok := s.proxyToLastStatus[refKey]; ok {
			current.Status = status
			s.proxyToLastStatus[refKey] = current
		} else {
			s.proxyToLastStatus[refKey] = reportsAndStatus{
				Status: status,
			}
		}
	}
}

func (s *statusSyncer) forceSync() {
	select {
	case s.syncNeeded <- struct{}{}:
	default:
	}
}

func (s *statusSyncer) syncStatusOnEmit(ctx context.Context) error {
	var retryChan <-chan time.Time

	sync := func() {
		err := s.syncStatus(ctx)
		if err != nil {
			contextutils.LoggerFrom(ctx).Debugw("failed to sync status; will try again shortly.", "error", err)
			retryChan = time.After(time.Second)
		} else {
			retryChan = nil
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-retryChan:
			sync()
		case <-s.syncNeeded:
			sync()
		}
	}
}

func (s *statusSyncer) syncStatus(ctx context.Context) error {
	var nilProxy *gloov1.Proxy
	allReports := reporter.ResourceReports{}
	inputResourceBySubresourceStatuses := map[resources.InputResource]map[string]*core.Status{}
	var localInputResourceLastStatus map[resources.InputResource]*core.Status

	func() {
		s.mapLock.RLock()
		defer s.mapLock.RUnlock()
		// grab a local copy of the map. it only updated here,
		// and the variable is cleared under the lock; so is safe.
		localInputResourceLastStatus = s.inputResourceLastStatus

		// iterate s.currentGeneratedProxies to guarantee order
		for _, ref := range s.currentGeneratedProxies {
			refKey := gloo_translator.UpstreamToClusterName(ref)
			reportsAndStatus, ok := s.proxyToLastStatus[refKey]
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
					allReports[inputResource] = report
				} else {
					allReports[inputResource] = subresourceStatuses
				}
			}
		}
	}()

	var errs error
	for inputResource, subresourceStatuses := range allReports {
		// write reports may update the status, so clone the object
		currentStatuses := inputResourceBySubresourceStatuses[inputResource]
		clonedInputResource := resources.Clone(inputResource).(resources.InputResource)
		reports := reporter.ResourceReports{clonedInputResource: subresourceStatuses}
		// set the last known status on the input resource.
		// this may be different than the status on the snapshot, as the snapshot doesn't get updated
		// on status changes.
		if status, ok := localInputResourceLastStatus[inputResource]; ok {
			clonedInputResource.SetStatus(status)
		}
		if err := s.reporter.WriteReports(ctx, reports, currentStatuses); err != nil {
			errs = multierror.Append(errs, err)
		} else {
			// cache the status written by the reporter
			status := s.reporter.StatusFromReport(subresourceStatuses, currentStatuses)
			localInputResourceLastStatus[inputResource] = status
		}
	}
	return errs
}
