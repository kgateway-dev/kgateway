package controller

import (
	"context"

	gloov1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/bootstrap"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins/registry"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
)

func newGlooTranslator(ctx context.Context) translator.Translator {
	settings := &gloov1.Settings{}
	opts := bootstrap.Opts{}
	return translator.NewDefaultTranslator(settings, registry.GetPluginRegistryFactory(opts)(ctx))

}
