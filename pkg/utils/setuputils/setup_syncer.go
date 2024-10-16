package setuputils

import (
	"context"
	"slices"
	"strings"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"
	"github.com/solo-io/gloo/pkg/utils/statsutils"
	"go.opencensus.io/tag"

	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"go.uber.org/zap"
)

var (
	mSetupsRun = statsutils.MakeSumCounter("gloo.solo.io/setups_run", "The number of times the main setup loop has run")

	mNamespacesWatched      = statsutils.MakeGauge("gloo.solo.io/namespaces_watched", "The number of namespaces watched by the gloo controller")
	namespacesWatchedKey, _ = tag.NewKey("namespaces_watched")
)

// tell us how to setup
type SetupFunc func(ctx context.Context,
	kubeCache kube.SharedCache,
	inMemoryCache memory.InMemoryResourceCache,
	settings *v1.Settings,
	identity leaderelector.Identity) error

type SetupSyncer struct {
	settingsRef         *core.ResourceRef
	setupFunc           SetupFunc
	inMemoryCache       memory.InMemoryResourceCache
	identity            leaderelector.Identity
	currentSettingsHash uint64
}

func NewSetupSyncer(settingsRef *core.ResourceRef, setupFunc SetupFunc, identity leaderelector.Identity) *SetupSyncer {
	return &SetupSyncer{
		settingsRef:   settingsRef,
		setupFunc:     setupFunc,
		inMemoryCache: memory.NewInMemoryResourceCache(),
		identity:      identity,
	}
}

func (s *SetupSyncer) Sync(ctx context.Context, snap *v1.SetupSnapshot) error {
	settings, err := snap.Settings.Find(s.settingsRef.Strings())
	if err != nil {
		return errors.Wrapf(err, "finding bootstrap configuration")
	}
	ctx = settingsutil.WithSettings(ctx, settings)

	contextutils.LoggerFrom(ctx).Debugw("received settings snapshot", zap.Any("settings", settings))

	_, err = settingsutil.UpdateNamespacesToWatch(settings, snap.Kubenamespaces)
	if err != nil {
		return err
	}

	watchedNamespaces := settingsutil.GetNamespacesToWatch(settings)
	contextutils.LoggerFrom(ctx).Debugw("received updated list of namespaces to watch", zap.Any("namespaces", watchedNamespaces))

	watchedNamespacesStr := strings.Join(watchedNamespaces, ",")
	statsutils.Measure(
		ctx,
		mNamespacesWatched,
		int64(len(watchedNamespaces)),
		tag.Insert(namespacesWatchedKey, watchedNamespacesStr),
	)

	statsutils.MeasureOne(
		ctx,
		mSetupsRun,
	)

	err = s.setupFunc(ctx, kube.NewKubeCache(ctx), s.inMemoryCache, settings, s.identity)
	if err != nil {
		return err
	}

	s.currentSettingsHash = settings.MustHash()
	return nil
}

// ShouldSync compares two snapshots and determines whether a sync is needed based on the following conditions
// 1. The settings CR has changed
// 2. A namespace is added / deleted / modified that changes the namespaces to watch
func (s *SetupSyncer) ShouldSync(ctx context.Context, oldSnapshot, newSnapshot *v1.SetupSnapshot) bool {
	// Basic sanity checks. Return a true if there is an error to ensure a sync to get into a good state
	if oldSnapshot == nil || newSnapshot == nil {
		return true
	}

	newSettings, err := newSnapshot.Settings.Find(s.settingsRef.Strings())
	if err != nil {
		return true
	}

	oldSettings, err := oldSnapshot.Settings.Find(s.settingsRef.Strings())
	if err != nil {
		return true
	}

	// A sync is triggered if either :
	// 1. The settings CR is changed
	// 2. A namespace is added / deleted / modified

	// 1. Check whether the settings CR is changed
	if s.currentSettingsHash != newSettings.MustHash() {
		contextutils.LoggerFrom(ctx).Debugw("syncing since settings have been updated")
		return true
	}

	// 2. Check if a namespace is added / deleted / modified
	// If a namespace was modified, check if it changes the namespaces to watch
	contextutils.LoggerFrom(ctx).Debugw("received updated list of namespaces", zap.Any("namespaces", newSnapshot.Kubenamespaces))
	newNamespacesToWatch, err := settingsutil.GenerateNamespacesToWatch(newSettings, newSnapshot.Kubenamespaces)
	if err != nil {
		return true
	}
	oldNamespacesToWatch := settingsutil.GetNamespacesToWatch(oldSettings)
	namespacesChanged := !slices.Equal(newNamespacesToWatch, oldNamespacesToWatch)

	contextutils.LoggerFrom(ctx).Debugw("list of namespaces to watch", zap.Any("oldNamespacesToWatch", oldNamespacesToWatch), zap.Any("newNamespacesToWatch", newNamespacesToWatch), zap.Any("namespacesChanged", namespacesChanged))

	return namespacesChanged
}
