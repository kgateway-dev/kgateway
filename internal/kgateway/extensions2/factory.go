package extensions2

import (
	"context"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
)

type K8sGatewayExtensionsFactory func(ctx context.Context, commoncol *common.CommonCollections) sdk.Plugin
