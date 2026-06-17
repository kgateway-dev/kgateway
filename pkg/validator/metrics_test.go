package validator

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics"
	"github.com/kgateway-dev/kgateway/v2/pkg/metrics/metricstest"
)

func TestRecordValidationMode(t *testing.T) {
	wasActive := metrics.Active()
	metrics.SetActive(true)
	validationModeInfo.Reset()
	t.Cleanup(func() {
		validationModeInfo.Reset()
		metrics.SetActive(wasActive)
	})

	RecordValidationMode(apisettings.ValidationStrict, apisettings.ValidatorCache)

	err := metricstest.CollectAndCompare(
		validationModeInfo,
		strings.NewReader(`
# HELP kgateway_validation_mode Configured validation mode. The active mode series has value 1.
# TYPE kgateway_validation_mode gauge
kgateway_validation_mode{mode="STRICT",validator_mode="CACHE"} 1
`),
		"kgateway_validation_mode",
	)
	require.NoError(t, err)
}
