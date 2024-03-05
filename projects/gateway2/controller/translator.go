package controller

import (
	"context"
	"github.com/solo-io/gloo/projects/gloo/pkg/syncer/setup"

	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

func newGlooTranslator(ctx context.Context, opts bootstrap.Opts, extensions setup.Extensions) translator.Translator {

	return translator.NewDefaultTranslator(opts.Settings, extensions.PluginRegistryFactory(ctx))

}
