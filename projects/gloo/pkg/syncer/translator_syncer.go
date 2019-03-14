package syncer

import (
	"context"

	"go.opencensus.io/tag"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	"github.com/solo-io/gloo/projects/gloo/pkg/xds"
	envoycache "github.com/solo-io/solo-kit/pkg/api/v1/control-plane/cache"
	"github.com/solo-io/solo-kit/pkg/api/v1/reporter"
)

var (
	proxyNameKey, _ = tag.NewKey("proxyname")
)

type translatorSyncer struct {
	translator translator.Translator
	xdsCache   envoycache.SnapshotCache
	xdsHasher  *xds.ProxyKeyHasher
	reporter   reporter.Reporter
	// used for debugging purposes only
	latestSnap *v1.ApiSnapshot
	extensions []TranslatorSyncerExtension
}

type TranslatorSyncerExtensionParams struct {
	SettingExtensions *v1.Extensions
}

type TranslatorSyncerExtensionFactory func(context.Context, TranslatorSyncerExtensionParams) (TranslatorSyncerExtension, error)

type TranslatorSyncerExtension interface {
	Sync(ctx context.Context, snap *v1.ApiSnapshot, xdsCache envoycache.SnapshotCache) error
}

func NewTranslatorSyncer(translator translator.Translator, xdsCache envoycache.SnapshotCache, xdsHasher *xds.ProxyKeyHasher, reporter reporter.Reporter, devMode bool, extensions []TranslatorSyncerExtension) v1.ApiSyncer {
	s := &translatorSyncer{
		translator: translator,
		xdsCache:   xdsCache,
		xdsHasher:  xdsHasher,
		reporter:   reporter,
		extensions: extensions,
	}
	if devMode {
		// TODO(ilackarms): move this somewhere else?
		go s.ServeXdsSnapshots()
	}
	return s
}

func (s *translatorSyncer) Sync(ctx context.Context, snap *v1.ApiSnapshot) error {
	err := s.syncEnvoy(ctx, snap)
	if err != nil {
		return err
	}
	for _, extension := range s.extensions {
		err := extension.Sync(ctx, snap, s.xdsCache)
		if err != nil {
			return err
		}
	}
	return nil
}
