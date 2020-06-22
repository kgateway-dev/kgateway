package helpers

import (
	"time"

	"github.com/gogo/protobuf/types"
	. "github.com/onsi/gomega"
	clienthelpers "github.com/solo-io/gloo/projects/gloo/cli/pkg/helpers"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
)

func UpdateSettings(installNamespace string, f func(settings *v1.Settings)) {
	settingsClient := clienthelpers.MustSettingsClient()
	settings, err := settingsClient.Read(installNamespace, "default", clients.ReadOpts{})
	Expect(err).NotTo(HaveOccurred())

	f(settings)

	_, err = settingsClient.Write(settings, clients.WriteOpts{OverwriteExisting: true})
	Expect(err).NotTo(HaveOccurred())

	// when validation config changes, the validation server restarts -- give time for it to come up again.
	// without the wait, the validation webhook may temporarily fallback to it's failurePolicy, which is not
	// what we want to test.
	time.Sleep(3 * time.Second)
}

// enable/disable strict validation
func UpdateAlwaysAcceptSetting(installNamespace string, alwaysAccept bool) {
	UpdateSettings(installNamespace, func(settings *v1.Settings) {
		Expect(settings.Gateway).NotTo(BeNil())
		Expect(settings.Gateway.Validation).NotTo(BeNil())
		settings.Gateway.Validation.AlwaysAccept = &types.BoolValue{Value: alwaysAccept}
	})
}
