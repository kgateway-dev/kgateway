package proxy_syncer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/reports"
)

func TestWithCustomStatusSync(t *testing.T) {
	customStatusSync := func(ctx context.Context, rm reports.ReportMap) {}
	statusSyncer := NewStatusSyncer(StatusSyncerConfig{
		Plugins:        pluginsdk.Plugin{},
		ControllerName: "controller-name",
	}, WithCustomStatusSync(customStatusSync))

	assert.NotNil(t, statusSyncer.customStatusSync)
}
