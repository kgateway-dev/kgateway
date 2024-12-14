package extensions2

import (
	"context"

	extensionsplug "github.com/solo-io/gloo/projects/gateway2/extensions2/plugin"

	"github.com/solo-io/gloo/projects/gateway2/extensions2/common"
)

type K8sGatewayExtensionsFactory func(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin
