package waypoint

import (
	"context"

	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/common"
	extensionsplug "github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugin"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/extensions2/plugins/waypoint/waypointquery"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/query"
	"github.com/kgateway-dev/kgateway/v2/internal/kgateway/wellknown"
)

func NewPlugin(
	ctx context.Context,
	commonCols *common.CommonCollections,
) extensionsplug.Plugin {
	queries := query.NewData(
		commonCols,
	)
	waypointQueries := waypointquery.NewQueries(
		commonCols,
		queries,
	)
	return extensionsplug.Plugin{
		ContributesGwTranslator: func(gw *gwv1.Gateway) extensionsplug.KGwTranslator {
			if gw.Spec.GatewayClassName != wellknown.WaypointClassName {
				return nil
			}

			return NewTranslator(queries, waypointQueries)
		},
		ExtraHasSynced: func() bool {
			return waypointQueries.HasSynced()
		},
	}
}
