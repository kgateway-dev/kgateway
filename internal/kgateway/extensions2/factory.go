package extensions2

import (
	"context"

	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
)

type K8sGatewayExtensionsFactory func(ctx context.Context, commoncol *common.CommonCollections) extensionsplug.Plugin
