package translator_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	apifake "github.com/kgateway-dev/kgateway/v2/pkg/apiclient/fake"
	kgtranslator "github.com/kgateway-dev/kgateway/v2/pkg/kgateway/translator"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	sdk "github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
)

// TestInitInstallsValidationMemoInEveryValidatorMode pins that the backend
// translator's per-cluster verdict memo is installed regardless of
// ValidatorMode. The mode only selects how an unseen bootstrap is executed;
// the memo decides, by cluster content, whether a bootstrap is built at all.
// Dropping it for BINARY would fork envoy once per client per backend per walk
// under per-client strict validation.
func TestInitInstallsValidationMemoInEveryValidatorMode(t *testing.T) {
	for _, mode := range []apisettings.ValidatorMode{apisettings.ValidatorBinary, apisettings.ValidatorCache} {
		t.Run(string(mode), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)

			settings := apisettings.Settings{
				ValidationMode:     apisettings.ValidationStrict,
				ValidatorMode:      mode,
				ValidatorCacheSize: 7,
			}
			commoncol, err := collections.NewCommonCollections(
				ctx,
				krtutil.NewKrtOptions(ctx.Done(), nil),
				apifake.NewClient(t),
				wellknown.DefaultGatewayControllerName,
				settings,
			)
			require.NoError(t, err)

			translator := kgtranslator.NewCombinedTranslator(ctx, sdk.Plugin{}, commoncol, nil)
			translator.Init(ctx)

			require.NotNil(t, translator.GetBackendTranslator().ValidationMemo,
				"the per-cluster verdict memo must be installed in ValidatorMode %s", mode)
		})
	}
}
