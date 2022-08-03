package bootstrap

import (
	"context"

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

var (
	mSetupsRun = utils.MakeSumCounter("gloo.solo.io/setups_run", "The number of times the main setup loop has run")
)

type SetupSyncer struct {
	settingsRef   *core.ResourceRef
	runner        Runner
	inMemoryCache memory.InMemoryResourceCache
}

func NewSetupSyncer(settingsRef *core.ResourceRef, runner Runner) *SetupSyncer {
	return &SetupSyncer{
		settingsRef:   settingsRef,
		runner:        runner,
		inMemoryCache: memory.NewInMemoryResourceCache(),
	}
}

func (s *SetupSyncer) Sync(ctx context.Context, snap *v1.SetupSnapshot) error {
	settings, err := snap.Settings.Find(s.settingsRef.Strings())
	if err != nil {
		return errors.Wrapf(err, "finding bootstrap configuration")
	}

	ctx = settingsutil.WithSettings(ctx, settings)
	contextutils.LoggerFrom(ctx).Debugw("received settings snapshot", zap.Any("settings", settings))

	utils.MeasureOne(ctx, mSetupsRun)

	return s.runner.Run(ctx, kube.NewKubeCache(ctx), s.inMemoryCache, settings)
}
