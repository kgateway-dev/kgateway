package extensions2

import (
	"context"

	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
)

type K8sGatewayExtensionsFactory func(ctx context.Context, commoncol *collections.CommonCollections) sdk.Plugin
