package setuputils

import (
	"context"

	"github.com/solo-io/gloo/pkg/bootstrap/leaderelector"

	"github.com/solo-io/gloo/pkg/utils"
	"github.com/solo-io/gloo/pkg/utils/settingsutil"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/errors"
	"go.uber.org/zap"
)

// DO_NOT_SUBMIT:
// no-op change to test go-mod cache

var (
	mSetupsRun = utils.MakeSumCounter("gloo.solo.io/setups_run", "The number of times the main setup loop has run")
)

// tell us how to setup
type SetupFunc func(ctx context.Context,
	kubeCache kube.SharedCache,
	inMemoryCache memory.InMemoryResourceCache,
	settings *v1.Settings,
	identity leaderelector.Identity) error

type SetupSyncer struct {
	settingsRef   *core.ResourceRef
	setupFunc     SetupFunc
	inMemoryCache memory.InMemoryResourceCache
	identity      leaderelector.Identity
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

	utils.MeasureOne(
		ctx,
		mSetupsRun,
	)

	return s.setupFunc(ctx, kube.NewKubeCache(ctx), s.inMemoryCache, settings, s.identity)
}
