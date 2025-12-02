package gateway_test

import (
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	apisettings "github.com/kgateway-dev/kgateway/v2/api/settings"
	"github.com/kgateway-dev/kgateway/v2/pkg/utils/fsutils"
	translatortest "github.com/kgateway-dev/kgateway/v2/test/translator"
)

func TestStatuses(t *testing.T) {
	t.Run("Basic", func(t *testing.T) {
		dir := fsutils.MustGetThisDir()
		settingOpt := func(s *apisettings.Settings) {
			s.EnableExperimentalGatewayAPIFeatures = true
		}
		translatortest.TestTranslation(
			t,
			t.Context(),
			[]string{
				filepath.Join(dir, "testutils/inputs/status/basic.yaml"),
			},
			filepath.Join(dir, "testutils/outputs/status/basic.yaml"),
			types.NamespacedName{
				Namespace: "default",
				Name:      "example-gateway",
			},
			settingOpt,
		)
	})
}
