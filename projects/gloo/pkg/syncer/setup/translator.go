package setup

import (
	"context"

	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/gloo/projects/gloo/pkg/translator"
	sslutils "github.com/solo-io/gloo/projects/gloo/pkg/utils"
)

type TranslatorFactory struct {
	PluginRegistry plugins.PluginRegistryFactory
}

func (tf TranslatorFactory) NewTranslator(ctx context.Context, settings *v1.Settings) translator.Translator {
	return translator.NewTranslatorWithHasher(
		sslutils.NewSslConfigTranslator(),
		settings,
		tf.PluginRegistry(ctx),
		translator.EnvoyCacheResourcesListToFnvHash,
	)
}

func (tf TranslatorFactory) NewClusterTranslator(ctx context.Context, settings *v1.Settings) translator.ClusterTranslator {
	return translator.NewTranslatorWithHasher(
		sslutils.NewSslConfigTranslator(),
		settings,
		tf.PluginRegistry(ctx),
		translator.EnvoyCacheResourcesListToFnvHash,
	)
}
